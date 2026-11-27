package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

type DockerContainer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Image         string `json:"image"`
	State         string `json:"state"`
	Status        string `json:"status"`
	Ports         string `json:"ports"`
	Project       string `json:"project"`
	Service       string `json:"service"`
	CPU           string `json:"cpu"`
	Memory        string `json:"memory"`
	MemoryPercent string `json:"memoryPercent"`
	NetIO         string `json:"netIO"`
	BlockIO       string `json:"blockIO"`
}

type DockerOverview struct {
	Status     string            `json:"status"`
	Message    string            `json:"message"`
	Version    string            `json:"version"`
	Name       string            `json:"name"`
	Containers []DockerContainer `json:"containers"`
	Warning    string            `json:"warning"`
}

type DockerMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

type DockerNetwork struct {
	IPAddress string `json:"IPAddress"`
	Gateway   string `json:"Gateway"`
}

type DockerDetails struct {
	ID            string                   `json:"id"`
	Created       string                   `json:"created"`
	Started       string                   `json:"started"`
	Finished      string                   `json:"finished"`
	ExitCode      int                      `json:"exitCode"`
	RestartCount  int                      `json:"restartCount"`
	RestartPolicy string                   `json:"restartPolicy"`
	Mounts        []DockerMount            `json:"mounts"`
	Networks      map[string]DockerNetwork `json:"networks"`
}

type DockerLogs struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// Only fixed commands and validated hexadecimal container IDs reach the shell.
// Docker runs as the connected user, without sudo or changes to the server.
const dockerOverviewScript = `
printf 'OXEE_DOCKER_V1\n'
if ! command -v docker >/dev/null 2>&1; then
  printf 'MISSING\n'
  exit 0
fi
if ! info=$(docker info --format '{"version":{{json .ServerVersion}},"name":{{json .Name}}}' 2>&1); then
  printf 'UNAVAILABLE\n%s\n' "$info"
  exit 0
fi
printf 'READY\n%s\nCONTAINERS\n' "$info"
docker container ls --all --no-trunc --format '{"id":{{json .ID}},"name":{{json .Names}},"image":{{json .Image}},"state":{{json .State}},"status":{{json .Status}},"ports":{{json .Ports}},"project":{{json (.Label "com.docker.compose.project")}},"service":{{json (.Label "com.docker.compose.service")}}}' || exit 1
printf 'STATS\n'
`

const dockerStatsScript = `
docker container stats --no-stream --no-trunc --format '{{json .}}' || printf 'STATS_UNAVAILABLE\n'
`

var dockerContainerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

// stdout is bounded; log buffers retain the tail instead of allocating without limit.
// A mutex also permits stdout and stderr to share the same log buffer safely.
type dockerBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	tail      bool
	truncated bool
}

func (b *dockerBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(data)
	if len(b.data)+n <= b.limit {
		b.data = append(b.data, data...)
	} else if !b.tail {
		return 0, errors.New("Docker response is too large")
	} else {
		b.truncated = true
		if n >= b.limit {
			b.data = append(b.data[:0], data[n-b.limit:]...)
		} else {
			b.data = append(b.data[len(b.data)+n-b.limit:], data...)
		}
	}
	return n, nil
}

func (b *dockerBuffer) String() string { return strings.ToValidUTF8(string(b.data), "�") }

// Separate SSH channels leave terminal input and output untouched. Hold dockerMu
// until channel creation and execution end, even if channel creation stalls.
func (a *App) runDocker(sessionID, script string, timeout time.Duration, logs bool) (DockerLogs, error) {
	a.mu.RLock()
	session := a.sessions[sessionID]
	a.mu.RUnlock()
	if session == nil || session.client == nil {
		return DockerLogs{}, errors.New("SSH session is closed")
	}
	if !session.dockerMu.TryLock() {
		return DockerLogs{}, errors.New("A Docker request is already running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	type response struct {
		output DockerLogs
		err    error
	}
	result := make(chan response, 1)
	go func() {
		var reply response
		defer func() {
			// Finish closing the channel and release the lock before returning data,
			// so a caller can immediately request details after the overview.
			session.dockerMu.Unlock()
			result <- reply
		}()
		channel, err := session.client.NewSession()
		if err != nil {
			reply.err = err
			return
		}
		defer channel.Close()
		if ctx.Err() != nil {
			reply.err = ctx.Err()
			return
		}
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				_ = channel.Close()
			case <-done:
			}
		}()
		stdout := &dockerBuffer{limit: 2 * 1024 * 1024}
		stderr := &dockerBuffer{limit: 16 * 1024, tail: true}
		var errout io.Writer = stderr
		if logs {
			stdout.limit, stdout.tail = 256*1024, true
			errout = stdout // Docker emits container stderr on its own stderr stream.
		}
		channel.Stdin = strings.NewReader(script)
		channel.Stdout, channel.Stderr = stdout, errout
		err = channel.Run("env LC_ALL=C sh -s")
		if err != nil {
			message := strings.TrimSpace(stderr.String())
			if logs {
				message = strings.TrimSpace(stdout.String())
			}
			if len(message) > 600 {
				message = message[len(message)-600:]
			}
			if message != "" {
				err = fmt.Errorf("Docker: %s", message)
			}
		}
		reply = response{DockerLogs{Text: stdout.String(), Truncated: stdout.truncated}, err}
	}()
	select {
	case reply := <-result:
		if ctx.Err() != nil {
			return DockerLogs{}, errors.New("Docker request timed out; refresh to check the current state")
		}
		return reply.output, reply.err
	case <-ctx.Done():
		return DockerLogs{}, errors.New("Docker request timed out; refresh to check the current state")
	}
}

func (a *App) GetDockerOverview(sessionID string, includeMetrics bool) (DockerOverview, error) {
	script := dockerOverviewScript
	if includeMetrics {
		script += dockerStatsScript
	}
	output, err := a.runDocker(sessionID, script, 15*time.Second, false)
	if err != nil {
		return DockerOverview{}, err
	}
	return parseDockerOverview(output.Text)
}

func parseDockerOverview(output string) (DockerOverview, error) {
	result := DockerOverview{Containers: []DockerContainer{}}
	_, body, found := strings.Cut(output, "OXEE_DOCKER_V1\n")
	if !found {
		return result, errors.New("Unrecognized Docker response from the server")
	}
	status, body, _ := strings.Cut(body, "\n")
	switch status {
	case "MISSING":
		result.Status, result.Message = "missing", "Docker is not installed or is not available in this SSH user's PATH"
		return result, nil
	case "UNAVAILABLE":
		result.Status = "unavailable"
		result.Message = "Docker is installed, but the engine is unavailable to this SSH user"
		result.Warning = strings.TrimSpace(body)
		if len(result.Warning) > 1200 {
			result.Warning = result.Warning[:1200]
		}
		return result, nil
	case "READY":
		result.Status = "ready"
	default:
		return result, errors.New("Unrecognized Docker availability status")
	}
	info, body, _ := strings.Cut(body, "\n")
	if err := json.Unmarshal([]byte(info), &result); err != nil {
		return result, fmt.Errorf("Read Docker engine information: %w", err)
	}
	section := ""
	stats := map[string]struct{ CPUPerc, MemUsage, MemPerc, NetIO, BlockIO string }{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch line {
		case "CONTAINERS", "STATS":
			section = line
			continue
		case "STATS_UNAVAILABLE":
			result.Warning = "Resource usage is unavailable; container controls are still available"
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if section == "CONTAINERS" {
			var container DockerContainer
			if err := json.Unmarshal([]byte(line), &container); err != nil || !dockerContainerID.MatchString(container.ID) {
				return result, errors.New("Read Docker containers: invalid response")
			}
			result.Containers = append(result.Containers, container)
		} else if section == "STATS" {
			var item struct{ ID, CPUPerc, MemUsage, MemPerc, NetIO, BlockIO string }
			if err := json.Unmarshal([]byte(line), &item); err != nil {
				return result, fmt.Errorf("Read Docker resource usage: %w", err)
			}
			stats[item.ID] = struct{ CPUPerc, MemUsage, MemPerc, NetIO, BlockIO string }{item.CPUPerc, item.MemUsage, item.MemPerc, item.NetIO, item.BlockIO}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	for i := range result.Containers {
		container := &result.Containers[i]
		item := stats[container.ID]
		container.CPU, container.Memory, container.NetIO, container.BlockIO = item.CPUPerc, item.MemUsage, item.NetIO, item.BlockIO
		container.MemoryPercent = item.MemPerc
	}
	return result, nil
}

func dockerActionScript(id, action string) (string, error) {
	if !dockerContainerID.MatchString(id) {
		return "", errors.New("Invalid Docker container ID")
	}
	// No force removal, volume removal, arbitrary commands or shell arguments.
	switch action {
	case "start", "stop", "restart", "pause", "unpause", "rm":
		return "docker container " + action + " " + id, nil
	default:
		return "", errors.New("Unsupported Docker action")
	}
}

func (a *App) DockerContainerAction(sessionID, id, action string) error {
	script, err := dockerActionScript(id, action)
	if err != nil {
		return err
	}
	_, err = a.runDocker(sessionID, script, 40*time.Second, false)
	return err
}

func (a *App) GetDockerLogs(sessionID, id string) (DockerLogs, error) {
	if !dockerContainerID.MatchString(id) {
		return DockerLogs{}, errors.New("Invalid Docker container ID")
	}
	return a.runDocker(sessionID, "docker container logs --tail 200 --timestamps "+id, 12*time.Second, true)
}

func (a *App) GetDockerDetails(sessionID, id string) (DockerDetails, error) {
	if !dockerContainerID.MatchString(id) {
		return DockerDetails{}, errors.New("Invalid Docker container ID")
	}
	// Select fields instead of exposing the inspect payload, which includes secrets.
	script := `docker container inspect --format '{"id":{{json .Id}},"created":{{json .Created}},"started":{{json .State.StartedAt}},"finished":{{json .State.FinishedAt}},"exitCode":{{json .State.ExitCode}},"restartCount":{{json .RestartCount}},"restartPolicy":{{json .HostConfig.RestartPolicy.Name}},"mounts":{{json .Mounts}},"networks":{{json .NetworkSettings.Networks}}}' ` + id
	output, err := a.runDocker(sessionID, script, 12*time.Second, false)
	if err != nil {
		return DockerDetails{}, err
	}
	var details DockerDetails
	// Allow login banners before the JSON without trusting them as data.
	for _, line := range strings.Split(output.Text, "\n") {
		if json.Unmarshal([]byte(line), &details) == nil && details.ID == id {
			if details.Mounts == nil {
				details.Mounts = []DockerMount{}
			}
			if details.Networks == nil {
				details.Networks = map[string]DockerNetwork{}
			}
			return details, nil
		}
	}
	return DockerDetails{}, errors.New("Read Docker container details: invalid response")
}
