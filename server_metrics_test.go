package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const metricsFixture = `Shell banner from a noninteractive login
OXEE_METRICS_V1
HOST	node-one
OS	Ubuntu 24.04 LTS
KERNEL	6.8.0
UPTIME	86401.25
LOAD	0.10	0.20	0.30
CPU	4	10000	7000	100
MEM	MemTotal	1048576
MEM	MemAvailable	524288
MEM	MemFree	200000
MEM	Buffers	10000
MEM	Cached	100000
MEM	SReclaimable	5000
MEM	Shmem	1000
MEM	SwapTotal	262144
MEM	SwapFree	131072
FILESYSTEMS
Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/vda1 1048576 524288 500000 52% /
/dev/mapper/data disk 2097152 1048576 1048576 50% /mnt/data with spaces
tmpfs 200000 500 199500 1% /run
/dev/loop0 1000 1000 0 100% /snap/tool
END_FILESYSTEMS
NET	lo	100	200	0	0
NET	eth0	1000000	2000000	1	2
PRIMARY	eth0
IO	vda	100	2000	50	4000	6000
`

func TestParseServerMetrics(t *testing.T) {
	data, err := parseServerMetrics(metricsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if !data.Supported || data.Hostname != "node-one" || data.OS != "Ubuntu 24.04 LTS" || data.UptimeSeconds != 86401.25 {
		t.Fatalf("incorrect identity: %+v", data)
	}
	if data.CPU == nil || data.CPU.Cores != 4 || data.CPU.Total != 10000 || data.CPU.Idle != 7000 {
		t.Fatalf("CPU counters: %+v", data.CPU)
	}
	if data.Memory == nil || data.Memory.Total != 1<<30 || data.Memory.Available != 1<<29 || data.Memory.Cached != 104000*1024 {
		t.Fatalf("memory counters: %+v", data.Memory)
	}
	if len(data.Storage) != 2 || data.Storage[1].Device != "/dev/mapper/data disk" || data.Storage[1].Mount != "/mnt/data with spaces" {
		t.Fatalf("filesystems or pseudo-device filtering: %+v", data.Storage)
	}
	if len(data.Network) != 1 || data.Network[0].Name != "eth0" || data.PrimaryInterface != "eth0" {
		t.Fatalf("network: %+v", data.Network)
	}
	if len(data.Disks) != 1 || data.Disks[0].ReadBytes != 2000*512 || data.Disks[0].WriteBytes != 4000*512 || data.Disks[0].BusyMilliseconds != 6000 {
		t.Fatalf("disk I/O: %+v", data.Disks)
	}
	if len(data.LoadAverage) != 3 || data.LoadAverage[1] != 0.2 {
		t.Fatalf("load: %+v", data.LoadAverage)
	}
}

func TestServerMetricsFallbackAndUnsupported(t *testing.T) {
	withoutAvailable := strings.Replace(metricsFixture, "MEM\tMemAvailable\t524288\n", "", 1)
	data, err := parseServerMetrics(withoutAvailable)
	if err != nil || data.Memory == nil || data.Memory.Available != 314000*1024 {
		t.Fatalf("memory fallback: %+v, %v", data.Memory, err)
	}
	data, err = parseServerMetrics("OXEE_METRICS_V1\nUNSUPPORTED\tDarwin\n")
	if err != nil || data.Supported || data.OS != "Darwin" || data.Message == "" {
		t.Fatalf("unsupported system: %+v, %v", data, err)
	}
	if _, err := parseServerMetrics("command unavailable"); err == nil {
		t.Fatal("unrecognized output must fail")
	}
}

func TestServerMetricsRejectsMalformedCounters(t *testing.T) {
	data, err := parseServerMetrics("OXEE_METRICS_V1\nCPU\t4\t100\t200\t0\nMEM\tMemTotal\t18446744073709551615\nLOAD\tNaN\t+Inf\t-1\nNET\teth0\t-1\t2\t0\t0\nIO\tvda\t1\t18446744073709551615\t1\t2\t0\n")
	if err != nil {
		t.Fatal(err)
	}
	if data.CPU != nil || data.Memory != nil || len(data.LoadAverage) != 0 || len(data.Network) != 0 || len(data.Disks) != 0 {
		t.Fatalf("malformed values were accepted: %+v", data)
	}
}

type fakeMetricsCommand struct {
	output string
	stderr string
	err    error
	block  bool
	closed chan struct{}
	once   sync.Once
	ran    atomic.Bool
}

func (c *fakeMetricsCommand) Run(stdout, stderr io.Writer) error {
	c.ran.Store(true)
	if c.block {
		<-c.closed
		return errors.New("channel closed")
	}
	if _, err := io.Copy(stdout, strings.NewReader(c.output)); err != nil {
		return err
	}
	if _, err := io.WriteString(stderr, c.stderr); err != nil {
		return err
	}
	return c.err
}
func (c *fakeMetricsCommand) Close() error { c.once.Do(func() { close(c.closed) }); return nil }

func TestCollectServerMetricsChannel(t *testing.T) {
	command := &fakeMetricsCommand{output: metricsFixture, closed: make(chan struct{})}
	data, err := collectServerMetrics(context.Background(), func() (metricsCommand, error) { return command, nil })
	if err != nil || data.Hostname != "node-one" {
		t.Fatalf("collection: %+v, %v", data, err)
	}
	select {
	case <-command.closed:
	default:
		t.Fatal("the metrics channel was not closed")
	}
}

func TestCollectServerMetricsTimeoutClosesOnlyCommand(t *testing.T) {
	command := &fakeMetricsCommand{block: true, closed: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := collectServerMetrics(ctx, func() (metricsCommand, error) { return command, nil })
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
	select {
	case <-command.closed:
	default:
		t.Fatal("timed-out channel remained open")
	}
}

func TestCollectServerMetricsRejectsExcessOutput(t *testing.T) {
	command := &fakeMetricsCommand{output: strings.Repeat("x", 512*1024+1), closed: make(chan struct{})}
	_, err := collectServerMetrics(context.Background(), func() (metricsCommand, error) { return command, nil })
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected bounded output error, got %v", err)
	}
}

func TestGetServerMetricsRequiresSSHSession(t *testing.T) {
	app := &App{}
	if _, err := app.GetServerMetrics("closed"); err == nil {
		t.Fatal("closed sessions must not report local statistics")
	}
	session := &activeSession{}
	session.metricsMu.Lock()
	defer session.metricsMu.Unlock()
	app.sessions = map[string]*activeSession{"busy": session}
	if _, err := app.GetServerMetrics("busy"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("overlapping request: %v", err)
	}
}

// Read ahead so both SSH peers can exchange their version strings over an
// in-memory pipe, without depending on sandbox permissions for loopback sockets.
type metricsBufferedConn struct {
	net.Conn
	reader *io.PipeReader
}

func bufferedMetricsConn(conn net.Conn) net.Conn {
	reader, writer := io.Pipe()
	go func() { _, err := io.Copy(writer, conn); _ = writer.CloseWithError(err) }()
	return &metricsBufferedConn{Conn: conn, reader: reader}
}
func (c *metricsBufferedConn) Read(data []byte) (int, error) { return c.reader.Read(data) }
func (c *metricsBufferedConn) Close() error                  { _ = c.reader.Close(); return c.Conn.Close() }

func TestServerMetricsUsesSeparateSSHChannel(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	left, right := net.Pipe()
	_ = left.SetDeadline(time.Now().Add(8 * time.Second))
	_ = right.SetDeadline(time.Now().Add(8 * time.Second))
	clientWire, serverWire := bufferedMetricsConn(left), bufferedMetricsConn(right)
	t.Cleanup(func() { clientWire.Close(); serverWire.Close() })
	var ptys atomic.Int32
	commands := make(chan string, 1)
	scripts := make(chan string, 1)
	go func() {
		conn, channels, requests, err := ssh.NewServerConn(serverWire, config)
		if err != nil {
			return
		}
		defer conn.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			channel, requests, err := incoming.Accept()
			if err != nil {
				return
			}
			go func() {
				defer channel.Close()
				for request := range requests {
					switch request.Type {
					case "pty-req":
						ptys.Add(1)
						_ = request.Reply(true, nil)
					case "shell":
						_ = request.Reply(true, nil)
						_, _ = io.WriteString(channel, "terminal-ready\n")
						_, _ = io.Copy(channel, channel)
						return
					case "exec":
						var payload struct{ Command string }
						_ = ssh.Unmarshal(request.Payload, &payload)
						commands <- payload.Command
						_ = request.Reply(true, nil)
						stdin, _ := io.ReadAll(channel)
						scripts <- string(stdin)
						_, _ = io.WriteString(channel, metricsFixture)
						_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
						return
					default:
						_ = request.Reply(false, nil)
					}
				}
			}()
		}
	}()
	conn, channels, requests, err := ssh.NewClientConn(clientWire, "server.test:22", &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey())})
	if err != nil {
		t.Fatal(err)
	}
	client := ssh.NewClient(conn, channels, requests)
	t.Cleanup(func() { client.Close() })
	terminal, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	stdout, err := terminal.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := terminal.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := terminal.RequestPty("xterm", 24, 80, nil); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Shell(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || line != "terminal-ready\n" {
		t.Fatalf("interactive shell: %q, %v", line, err)
	}
	app := &App{sessions: map[string]*activeSession{"ssh": {client: client, ssh: terminal, stdin: stdin}}}
	data, err := app.GetServerMetrics("ssh")
	if err != nil || data.Hostname != "node-one" {
		t.Fatalf("SSH collection: %+v, %v", data, err)
	}
	if command := <-commands; command != "env LC_ALL=C sh -s" {
		t.Fatalf("unexpected command: %q", command)
	}
	if script := <-scripts; script != serverMetricsScript {
		t.Fatal("collector script was not isolated on the exec channel")
	}
	if ptys.Load() != 1 {
		t.Fatal("monitoring requested an extra PTY")
	}
	if err := app.WriteSession("ssh", "terminal-still-works\n"); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "terminal-still-works\n" {
		t.Fatalf("terminal interrupted or polluted: %q, %v", line, err)
	}
}
