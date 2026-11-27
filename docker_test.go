package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var dockerTestID = strings.Repeat("a", 64)

func dockerFixture() string {
	return "Login banner\nOXEE_DOCKER_V1\nREADY\n{\"version\":\"27.5.1\",\"name\":\"node-one\"}\nCONTAINERS\n" +
		fmt.Sprintf(`{"id":%q,"name":"api","image":"oxee/api:latest","state":"running","status":"Up 2 hours (healthy)","ports":"127.0.0.1:8080->80/tcp","project":"platform","service":"api"}`, dockerTestID) +
		"\nSTATS\n" + fmt.Sprintf(`{"ID":%q,"CPUPerc":"2.50%%","MemUsage":"32MiB / 1GiB","MemPerc":"3.13%%","NetIO":"1MB / 2MB","BlockIO":"0B / 1MB"}`, dockerTestID) + "\n"
}

func TestDockerOverview(t *testing.T) {
	data, err := parseDockerOverview(dockerFixture())
	if err != nil || data.Status != "ready" || data.Name != "node-one" || data.Version != "27.5.1" || len(data.Containers) != 1 {
		t.Fatalf("overview: %+v, %v", data, err)
	}
	container := data.Containers[0]
	if container.ID != dockerTestID || container.CPU != "2.50%" || container.Memory != "32MiB / 1GiB" || container.MemoryPercent != "3.13%" || container.Project != "platform" || container.NetIO != "1MB / 2MB" {
		t.Fatalf("container: %+v", container)
	}
	for _, status := range []string{"MISSING", "UNAVAILABLE"} {
		data, err := parseDockerOverview("OXEE_DOCKER_V1\n" + status + "\npermission denied\n")
		if err != nil || data.Status != strings.ToLower(status) || data.Message == "" || data.Containers == nil {
			t.Fatalf("availability %s: %+v, %v", status, data, err)
		}
	}
	empty, err := parseDockerOverview("OXEE_DOCKER_V1\nREADY\n{}\nCONTAINERS\nSTATS\n")
	if err != nil || len(empty.Containers) != 0 {
		t.Fatalf("empty engine: %+v, %v", empty, err)
	}
	partial, err := parseDockerOverview(strings.Split(dockerFixture(), "STATS\n")[0] + "STATS\nSTATS_UNAVAILABLE\n")
	if err != nil || partial.Warning == "" || len(partial.Containers) != 1 {
		t.Fatalf("unavailable stats: %+v, %v", partial, err)
	}
	for _, output := range []string{"docker: not found", "OXEE_DOCKER_V1\n???\n", "OXEE_DOCKER_V1\nREADY\nno json\n", "OXEE_DOCKER_V1\nREADY\n{}\nCONTAINERS\n{}\n", strings.Split(dockerFixture(), "STATS\n")[0] + "STATS\ninvalid json"} {
		if _, err := parseDockerOverview(output); err == nil {
			t.Fatalf("accepted malformed response: %s", output)
		}
	}
}

func TestDockerCommandValidation(t *testing.T) {
	for _, id := range []string{"", "api", "-a", dockerTestID + "; reboot", strings.Repeat("a", 65), "$(whoami)", "abc\n123456789"} {
		if _, err := dockerActionScript(id, "stop"); err == nil {
			t.Fatalf("accepted invalid ID %q", id)
		}
	}
	for _, action := range []string{"exec", "rm -f", "stop; reboot", "prune"} {
		if _, err := dockerActionScript(dockerTestID, action); err == nil {
			t.Fatalf("accepted invalid action %q", action)
		}
	}
	script, err := dockerActionScript(dockerTestID, "rm")
	if err != nil || script != "docker container rm "+dockerTestID {
		t.Fatalf("removal must not force or remove volumes: %q, %v", script, err)
	}
	app := &App{}
	if _, err := app.GetDockerOverview("closed", false); err == nil {
		t.Fatal("missing SSH session accepted")
	}
	if _, err := app.GetDockerLogs("closed", "unsafe;id"); err == nil {
		t.Fatal("invalid log ID accepted")
	}
	if _, err := app.GetDockerDetails("closed", "unsafe;id"); err == nil {
		t.Fatal("invalid inspect ID accepted")
	}
}

func TestDockerBuffers(t *testing.T) {
	buffer := &dockerBuffer{limit: 8, tail: true}
	_, _ = buffer.Write([]byte("123456"))
	_, _ = buffer.Write([]byte("789"))
	if buffer.String() != "23456789" || !buffer.truncated {
		t.Fatalf("tail: %q", buffer.String())
	}
	_, _ = buffer.Write([]byte("abcdefghijkl"))
	if buffer.String() != "efghijkl" {
		t.Fatalf("large tail: %q", buffer.String())
	}
	bounded := &dockerBuffer{limit: 8}
	if _, err := bounded.Write([]byte("123456789")); err == nil {
		t.Fatal("oversized structured output accepted")
	}
	// Docker's stdout and stderr may be copied concurrently by SSH.
	concurrent := &dockerBuffer{limit: 64, tail: true}
	var writers sync.WaitGroup
	for range 2 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for range 1000 {
				_, _ = concurrent.Write([]byte("log\n"))
			}
		}()
	}
	writers.Wait()
	if len(concurrent.String()) != 64 || !concurrent.truncated {
		t.Fatal("concurrent tail was not bounded")
	}
}

func TestDockerOverviewShell(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		shell = filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "sh.exe")
		if _, err := os.Stat(shell); err != nil {
			t.Skip("POSIX shell not available")
		}
	}
	probe := exec.Command(shell, "-c", "exit 0")
	if output, err := probe.CombinedOutput(); err != nil {
		t.Skipf("POSIX shell cannot run in this environment: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	// Exercise the actual script without requiring Docker or touching a daemon.
	for _, mode := range []string{"missing", "denied", "ready", "no-stats", "ls-error"} {
		t.Run(mode, func(t *testing.T) {
			stub := `docker() {
case "$1 $2" in
  "info --format")
    if [ "$mode" = denied ]; then printf 'permission denied\n' >&2; return 1; fi
    printf '{"version":"27.5.1","name":"test"}\n';;
  "container ls")
    if [ "$mode" = ls-error ]; then return 1; fi;;
  "container stats")
    if [ "$mode" = no-stats ]; then return 1; fi;;
  *) printf 'unexpected Docker arguments\n' >&2; return 1;;
esac
}
`
			if mode == "missing" {
				stub = "command() { return 1; }\n"
			}
			cmd := exec.Command(shell, "-s")
			cmd.Stdin = strings.NewReader("mode=" + mode + "\n" + stub + dockerOverviewScript + dockerStatsScript)
			output, err := cmd.CombinedOutput()
			if mode == "ls-error" {
				if err == nil {
					t.Fatal("list failure was swallowed")
				}
				return
			}
			if err != nil {
				t.Fatalf("script: %s, %v", output, err)
			}
			data, err := parseDockerOverview(string(output))
			if err != nil {
				t.Fatal(err)
			}
			want := "ready"
			if mode == "missing" {
				want = "missing"
			} else if mode == "denied" {
				want = "unavailable"
			}
			if data.Status != want || (mode == "no-stats" && data.Warning == "") {
				t.Fatalf("availability: %+v", data)
			}
		})
	}
}

func dockerSSHApp(t *testing.T, execute func(string, ssh.Channel) uint32, interactive ...bool) (*App, *bufio.Reader) {
	t.Helper()
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
					case "pty-req", "window-change":
						_ = request.Reply(len(interactive) > 0 && interactive[0], nil)
					case "shell":
						_ = request.Reply(true, nil)
						_, _ = io.Copy(channel, channel)
						return
					case "exec":
						var payload struct{ Command string }
						_ = ssh.Unmarshal(request.Payload, &payload)
						isExec := len(interactive) > 0 && interactive[0] && strings.HasPrefix(payload.Command, "env LC_ALL=C docker container exec ")
						if payload.Command != "env LC_ALL=C sh -s" && !isExec {
							_ = request.Reply(false, nil)
							return
						}
						_ = request.Reply(true, nil)
						command := payload.Command
						if !isExec {
							stdin, _ := io.ReadAll(channel)
							command = string(stdin)
						}
						status := execute(command, channel)
						_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
						return
					default:
						_ = request.Reply(false, nil) // Docker must never allocate a PTY.
					}
				}
			}()
		}
	}()
	conn, channels, requests, err := ssh.NewClientConn(clientWire, "docker.test:22", &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey())})
	if err != nil {
		t.Fatal(err)
	}
	client := ssh.NewClient(conn, channels, requests)
	t.Cleanup(func() { client.Close() })
	terminal, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := terminal.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := terminal.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := terminal.Shell(); err != nil {
		t.Fatal(err)
	}
	app := &App{sessions: map[string]*activeSession{"ssh": {client: client, ssh: terminal, stdin: stdin}}}
	return app, bufio.NewReader(stdout)
}

func TestDockerMetricsAreOptIn(t *testing.T) {
	commands := make(chan string, 2)
	app, _ := dockerSSHApp(t, func(script string, channel ssh.Channel) uint32 {
		commands <- script
		_, _ = io.WriteString(channel, dockerFixture())
		return 0
	})
	for _, metrics := range []bool{false, true} {
		if _, err := app.GetDockerOverview("ssh", metrics); err != nil {
			t.Fatal(err)
		}
		script := <-commands
		if strings.Contains(script, "docker container stats") != metrics {
			t.Fatalf("includeMetrics=%t ran the wrong script: %s", metrics, script)
		}
	}
}

func TestDockerSSHCommandsKeepTerminalUsable(t *testing.T) {
	var scripts []string
	app, reader := dockerSSHApp(t, func(script string, channel ssh.Channel) uint32 {
		scripts = append(scripts, script)
		switch {
		case script == dockerOverviewScript:
			_, _ = io.WriteString(channel, dockerFixture())
		case strings.HasPrefix(script, "docker container logs"):
			_, _ = io.WriteString(channel, "stdout log\n")
			_, _ = io.WriteString(channel.Stderr(), "stderr log\n")
		case strings.HasPrefix(script, "docker container inspect"):
			_, _ = fmt.Fprintf(channel, "Banner\n{\"id\":%q,\"exitCode\":0,\"mounts\":null,\"networks\":null}\n", dockerTestID)
		case script == "docker container stop "+dockerTestID:
			_, _ = io.WriteString(channel, dockerTestID)
		default:
			_, _ = io.WriteString(channel.Stderr(), "permission denied")
			return 1
		}
		return 0
	})
	data, err := app.GetDockerOverview("ssh", false)
	if err != nil || len(data.Containers) != 1 {
		t.Fatalf("overview: %+v, %v", data, err)
	}
	logs, err := app.GetDockerLogs("ssh", dockerTestID)
	if err != nil || !strings.Contains(logs.Text, "stdout log") || !strings.Contains(logs.Text, "stderr log") {
		t.Fatalf("logs: %+v, %v", logs, err)
	}
	details, err := app.GetDockerDetails("ssh", dockerTestID)
	if err != nil || details.ID != dockerTestID || details.Mounts == nil || details.Networks == nil {
		t.Fatalf("details: %+v, %v", details, err)
	}
	if err := app.DockerContainerAction("ssh", dockerTestID, "stop"); err != nil {
		t.Fatal(err)
	}
	if err := app.DockerContainerAction("ssh", dockerTestID, "restart"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("action failure: %v", err)
	}
	if len(scripts) != 5 || strings.Contains(scripts[2], ".Config.Env") {
		t.Fatalf("unexpected commands: %v", scripts)
	}
	if err := app.WriteSession("ssh", "terminal-still-works\n"); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "terminal-still-works\n" {
		t.Fatalf("Docker interrupted or polluted terminal: %q, %v", line, err)
	}
}

func TestDockerTimeoutPreservesSSHTransport(t *testing.T) {
	started := make(chan struct{})
	closed := make(chan struct{})
	app, reader := dockerSSHApp(t, func(_ string, channel ssh.Channel) uint32 {
		close(started)
		_, _ = io.Copy(io.Discard, channel)
		// The client already sent EOF, so wait for the exec channel's actual close.
		for {
			if _, err := channel.Write([]byte(".")); err != nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		close(closed)
		return 0
	})
	failed := make(chan error, 1)
	go func() {
		_, err := app.runDocker("ssh", "slow Docker request", 100*time.Millisecond, false)
		failed <- err
	}()
	<-started
	if _, err := app.GetDockerOverview("ssh", false); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("overlapping requests accepted: %v", err)
	}
	if err := <-failed; err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("timed out channel remained open")
	}
	if err := app.WriteSession("ssh", "still-live\n"); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "still-live\n" {
		t.Fatalf("timeout closed the SSH transport: %q, %v", line, err)
	}
}
