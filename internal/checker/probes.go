package checker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Dial measures a TCP connect to host:port.
func Dial(ctx context.Context, host string, port int) (time.Duration, error) {
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %d", port)
	}
	if host == "" {
		return 0, errors.New("host is required")
	}
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, err
	}
	_ = conn.Close()
	return elapsed, nil
}

// Port probes TCP reachability. An open port is not proof of the named service.
func Port(id, name, host string, port int) Probe {
	target := "tcp://" + net.JoinHostPort(host, strconv.Itoa(port))
	return Probe{
		ID:     id,
		Name:   name,
		Target: target,
		Run: func(ctx context.Context) Result {
			elapsed, err := Dial(ctx, host, port)
			if err != nil {
				return Result{
					ID:       id,
					Name:     name,
					Target:   target,
					Detail:   dialDetail(err),
					Duration: elapsed,
				}
			}
			return Result{
				ID:       id,
				Name:     name,
				Target:   target,
				OK:       true,
				Detail:   "accepting",
				Duration: elapsed,
			}
		},
	}
}

// Docker reports whether the docker CLI can reach a daemon.
func Docker() Probe {
	return commandProbe("docker", "Docker", "docker info", "docker", []string{"info", "--format", "{{.ServerVersion}}"}, dockerDetail, dockerFailure)
}

// Go reports the local Go toolchain.
func Go() Probe {
	return commandProbe("go", "Go", "go version", "go", []string{"version"}, goDetail, runtimeFailure)
}

// Node reports the local Node.js runtime.
func Node() Probe {
	return commandProbe("node", "Node.js", "node --version", "node", []string{"--version"}, nodeDetail, runtimeFailure)
}

func commandProbe(id, name, target, bin string, args []string, okDetail func(string) string, failDetail func(error, string) string) Probe {
	return Probe{
		ID:     id,
		Name:   name,
		Target: target,
		Run: func(ctx context.Context) Result {
			start := time.Now()
			path, err := exec.LookPath(bin)
			if err != nil {
				return Result{ID: id, Name: name, Target: target, Detail: "not installed", Duration: time.Since(start)}
			}
			stdout, stderr, err := runCommand(ctx, path, args)
			elapsed := time.Since(start)
			if err != nil {
				return Result{ID: id, Name: name, Target: target, Detail: failDetail(err, stderr), Duration: elapsed}
			}
			return Result{ID: id, Name: name, Target: target, OK: true, Detail: okDetail(stdout), Duration: elapsed}
		},
	}
}

func runCommand(ctx context.Context, path string, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func dockerDetail(out string) string {
	version := firstToken(out)
	if version == "" {
		return "daemon reachable"
	}
	return "daemon " + version
}

func goDetail(out string) string {
	for _, field := range strings.Fields(out) {
		if strings.HasPrefix(field, "go1.") || strings.HasPrefix(field, "go2.") {
			return field
		}
	}
	if line := firstToken(out); line != "" {
		return line
	}
	return "ready"
}

func nodeDetail(out string) string {
	if line := firstToken(out); line != "" {
		return line
	}
	return "ready"
}

func dockerFailure(err error, stderr string) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return contextDetail(err)
	}
	low := strings.ToLower(stderr)
	switch {
	case strings.Contains(low, "permission denied"):
		return "permission denied"
	default:
		return "daemon unreachable"
	}
}

func runtimeFailure(err error, _ string) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return contextDetail(err)
	}
	return "unavailable"
}

func dialDetail(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return contextDetail(err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timed out"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection refused"
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if msg == "" {
		return "unreachable"
	}
	return msg
}

func contextDetail(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "timed out"
}

func firstToken(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
