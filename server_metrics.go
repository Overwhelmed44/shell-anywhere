package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type ServerCPU struct {
	Cores  int    `json:"cores"`
	Total  uint64 `json:"total"`
	Idle   uint64 `json:"idle"`
	IOWait uint64 `json:"ioWait"`
}

type ServerMemory struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
	Cached    uint64 `json:"cached"`
	SwapTotal uint64 `json:"swapTotal"`
	SwapFree  uint64 `json:"swapFree"`
}

type ServerFilesystem struct {
	Device      string  `json:"device"`
	Mount       string  `json:"mount"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"usedPercent"`
}

type ServerNetwork struct {
	Name          string `json:"name"`
	Received      uint64 `json:"received"`
	Sent          uint64 `json:"sent"`
	ReceiveErrors uint64 `json:"receiveErrors"`
	SendErrors    uint64 `json:"sendErrors"`
}

type ServerDiskIO struct {
	Name             string `json:"name"`
	ReadOperations   uint64 `json:"readOperations"`
	ReadBytes        uint64 `json:"readBytes"`
	WriteOperations  uint64 `json:"writeOperations"`
	WriteBytes       uint64 `json:"writeBytes"`
	BusyMilliseconds uint64 `json:"busyMilliseconds"`
}

type ServerMetrics struct {
	Supported        bool               `json:"supported"`
	Message          string             `json:"message"`
	CollectedAt      int64              `json:"collectedAt"`
	Hostname         string             `json:"hostname"`
	OS               string             `json:"os"`
	Kernel           string             `json:"kernel"`
	UptimeSeconds    float64            `json:"uptimeSeconds"`
	LoadAverage      []float64          `json:"loadAverage"`
	CPU              *ServerCPU         `json:"cpu"`
	Memory           *ServerMemory      `json:"memory"`
	Storage          []ServerFilesystem `json:"storage"`
	Network          []ServerNetwork    `json:"network"`
	PrimaryInterface string             `json:"primaryInterface"`
	Disks            []ServerDiskIO     `json:"disks"`
}

// The command is fixed and only reads system counters. It runs in a separate
// channel on the authenticated transport, with no PTY or interactive-shell input.
const serverMetricsScript = `
printf 'OXEE_METRICS_V1\n'
system=$(uname -s 2>/dev/null)
if [ "$system" != Linux ] || [ ! -r /proc/stat ] || [ ! -r /proc/meminfo ]; then
  printf 'UNSUPPORTED\t%s\n' "$system"
  exit 0
fi
printf 'HOST\t%s\n' "$(hostname 2>/dev/null)"
printf 'OS\t%s\n' "$(awk -F= '/^PRETTY_NAME=/{sub(/^[^=]*=/, ""); gsub(/^"|"$/, ""); print; exit}' /etc/os-release 2>/dev/null)"
printf 'KERNEL\t%s\n' "$(uname -r 2>/dev/null)"
awk '{print "UPTIME\t" $1}' /proc/uptime
awk '{print "LOAD\t" $1 "\t" $2 "\t" $3}' /proc/loadavg
awk '/^cpu / {for(i=2;i<=9;i++) total+=$i; idle=$5+$6; wait=$6} /^cpu[0-9]+ / {cores++} END {printf "CPU\t%d\t%.0f\t%.0f\t%.0f\n", cores, total, idle, wait}' /proc/stat
awk '/^(MemTotal|MemAvailable|MemFree|Buffers|Cached|SReclaimable|Shmem|SwapTotal|SwapFree):/ {gsub(/:/, "", $1); print "MEM\t" $1 "\t" $2}' /proc/meminfo
printf 'FILESYSTEMS\n'
df -Pk 2>/dev/null
printf 'END_FILESYSTEMS\n'
awk 'NR>2 {gsub(/:/, " "); print "NET\t" $1 "\t" $2 "\t" $10 "\t" $4 "\t" $12}' /proc/net/dev
awk '$2=="00000000" {print "PRIMARY\t" $1; exit}' /proc/net/route
for disk in /sys/block/*; do
  name=${disk##*/}
  case "$name" in loop*|ram*|fd*|sr*) continue;; esac
  [ -r "$disk/stat" ] || continue
  printf 'IO\t%s\t' "$name"
  awk '{print $1 "\t" $3 "\t" $5 "\t" $7 "\t" $10}' "$disk/stat"
done
exit 0
`

type metricsCommand interface {
	Run(io.Writer, io.Writer) error
	Close() error
}

type sshMetricsCommand struct{ session *ssh.Session }

func (c sshMetricsCommand) Run(stdout, stderr io.Writer) error {
	c.session.Stdin = strings.NewReader(serverMetricsScript)
	c.session.Stdout, c.session.Stderr = stdout, stderr
	return c.session.Run("env LC_ALL=C sh -s")
}
func (c sshMetricsCommand) Close() error { return c.session.Close() }

type metricsResult struct {
	metrics ServerMetrics
	err     error
}

func (a *App) GetServerMetrics(sessionID string) (ServerMetrics, error) {
	a.mu.RLock()
	session := a.sessions[sessionID]
	a.mu.RUnlock()
	if session == nil {
		return ServerMetrics{}, errors.New("SSH session is closed")
	}
	if !session.metricsMu.TryLock() {
		return ServerMetrics{}, errors.New("A server information request is already running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	result := make(chan metricsResult, 1)
	go func() {
		// Keep the lock if opening a channel stalls, even after the caller times
		// out. Disconnecting the SSH transport releases any pending channel open.
		defer session.metricsMu.Unlock()
		metrics, err := collectServerMetrics(ctx, func() (metricsCommand, error) {
			channel, err := session.client.NewSession()
			if err != nil {
				return nil, err
			}
			return sshMetricsCommand{channel}, nil
		})
		result <- metricsResult{metrics, err}
	}()
	select {
	case response := <-result:
		return response.metrics, response.err
	case <-ctx.Done():
		return ServerMetrics{}, errors.New("Server information request timed out")
	}
}

type metricsBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *metricsBuffer) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.buffer.Len() {
		return 0, errors.New("server information response is too large")
	}
	return b.buffer.Write(data)
}

func (b *metricsBuffer) String() string { return b.buffer.String() }

func collectServerMetrics(ctx context.Context, open func() (metricsCommand, error)) (ServerMetrics, error) {
	command, err := open()
	if err != nil {
		return ServerMetrics{}, err
	}
	defer command.Close()
	if err := ctx.Err(); err != nil {
		return ServerMetrics{}, err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = command.Close()
		case <-done:
		}
	}()
	stdout, stderr := &metricsBuffer{limit: 512 * 1024}, &metricsBuffer{limit: 8 * 1024}
	err = command.Run(stdout, stderr)
	if ctx.Err() != nil {
		return ServerMetrics{}, errors.New("Server information request timed out")
	}
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 400 {
			message = message[:400]
		}
		if message != "" {
			return ServerMetrics{}, fmt.Errorf("read server information: %s", message)
		}
		return ServerMetrics{}, fmt.Errorf("read server information: %w", err)
	}
	return parseServerMetrics(stdout.String())
}

var filesystemLine = regexp.MustCompile(`^(.+?)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+(?:\.\d+)?)%\s+(.+)$`)

func metricUint(value string) (uint64, bool) {
	n, err := strconv.ParseUint(value, 10, 64)
	return n, err == nil
}
func metricFloat(value string) (float64, bool) {
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil && n >= 0 && !math.IsInf(n, 0) && !math.IsNaN(n)
}
func metricBytes(value string, unit uint64) (uint64, bool) {
	n, ok := metricUint(value)
	if !ok || n > math.MaxUint64/unit {
		return 0, false
	}
	return n * unit, true
}
func metricText(parts []string) string {
	text := strings.Join(parts[1:], "\t")
	if len(text) > 256 {
		text = text[:256]
	}
	return text
}

func parseServerMetrics(output string) (ServerMetrics, error) {
	result := ServerMetrics{
		CollectedAt: time.Now().UnixMilli(), LoadAverage: []float64{},
		Storage: []ServerFilesystem{}, Network: []ServerNetwork{}, Disks: []ServerDiskIO{},
	}
	memory := make(map[string]uint64)
	started, inFilesystems := false, false
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "OXEE_METRICS_V1" {
			started = true
			result.Supported = true
			continue
		}
		if !started {
			continue
		}
		if line == "FILESYSTEMS" {
			inFilesystems = true
			continue
		}
		if line == "END_FILESYSTEMS" {
			inFilesystems = false
			continue
		}
		if inFilesystems {
			match := filesystemLine.FindStringSubmatch(line)
			if len(match) != 7 || len(result.Storage) >= 128 {
				continue
			}
			device := strings.TrimSpace(match[1])
			if device == "tmpfs" || device == "devtmpfs" || device == "udev" || device == "shm" || strings.HasPrefix(device, "/dev/loop") {
				continue
			}
			total, t := metricBytes(match[2], 1024)
			used, u := metricBytes(match[3], 1024)
			available, a := metricBytes(match[4], 1024)
			percent, p := metricFloat(match[5])
			if t && u && a && p && total > 0 {
				result.Storage = append(result.Storage, ServerFilesystem{device, match[6], total, used, available, math.Min(100, percent)})
			}
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "UNSUPPORTED":
			result.Supported = false
			result.OS = metricText(parts)
			result.Message = "Live server information is available for Linux SSH hosts"
		case "HOST":
			result.Hostname = metricText(parts)
		case "OS":
			result.OS = metricText(parts)
		case "KERNEL":
			result.Kernel = metricText(parts)
		case "PRIMARY":
			result.PrimaryInterface = metricText(parts)
		case "UPTIME":
			result.UptimeSeconds, _ = metricFloat(parts[1])
		case "LOAD":
			if len(parts) == 4 {
				for _, value := range parts[1:] {
					if n, ok := metricFloat(value); ok {
						result.LoadAverage = append(result.LoadAverage, n)
					}
				}
			}
		case "CPU":
			if len(parts) == 5 {
				cores, c := metricUint(parts[1])
				total, t := metricUint(parts[2])
				idle, i := metricUint(parts[3])
				wait, w := metricUint(parts[4])
				if c && t && i && w && cores > 0 && cores <= 65536 && idle <= total {
					result.CPU = &ServerCPU{int(cores), total, idle, wait}
				}
			}
		case "MEM":
			if len(parts) == 3 {
				if value, ok := metricBytes(parts[2], 1024); ok {
					memory[parts[1]] = value
				}
			}
		case "NET":
			if len(parts) == 6 && len(result.Network) < 256 {
				rx, r := metricUint(parts[2])
				tx, t := metricUint(parts[3])
				re, a := metricUint(parts[4])
				te, b := metricUint(parts[5])
				if r && t && a && b && parts[1] != "lo" {
					result.Network = append(result.Network, ServerNetwork{parts[1], rx, tx, re, te})
				}
			}
		case "IO":
			if len(parts) == 7 && len(result.Disks) < 256 {
				reads, r := metricUint(parts[2])
				readBytes, rb := metricBytes(parts[3], 512)
				writes, w := metricUint(parts[4])
				writeBytes, wb := metricBytes(parts[5], 512)
				busy, b := metricUint(parts[6])
				if r && rb && w && wb && b {
					result.Disks = append(result.Disks, ServerDiskIO{parts[1], reads, readBytes, writes, writeBytes, busy})
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ServerMetrics{}, fmt.Errorf("read server information response: %w", err)
	}
	if !started {
		return ServerMetrics{}, errors.New("The server did not return system information")
	}
	if total := memory["MemTotal"]; total > 0 {
		available, reported := memory["MemAvailable"]
		cached := memory["Cached"] + memory["SReclaimable"]
		if cached >= memory["Shmem"] {
			cached -= memory["Shmem"]
		} else {
			cached = 0
		}
		if !reported {
			available = memory["MemFree"] + memory["Buffers"] + cached
		}
		result.Memory = &ServerMemory{total, min(total, available), cached, memory["SwapTotal"], min(memory["SwapTotal"], memory["SwapFree"])}
	}
	return result, nil
}
