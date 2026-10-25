package checker

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDialOpenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	elapsed, err := Dial(ctx, "127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed <= 0 {
		t.Fatalf("elapsed %s", elapsed)
	}
}

func TestDialClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	probe := Port("closed", "closed", "127.0.0.1", port)
	res := probe.Run(ctx)
	if res.OK {
		t.Fatal("expected closed port")
	}
	if res.Detail != "connection refused" {
		t.Fatalf("detail %q", res.Detail)
	}
	if res.Duration < 0 {
		t.Fatalf("duration %s", res.Duration)
	}
}

func TestRunCompletesAll(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	probes := []Probe{
		Port("open", "open", "127.0.0.1", port),
		{ID: "instant", Name: "instant", Run: func(context.Context) Result {
			return Result{OK: true, Detail: "ready"}
		}},
	}
	var got []Result
	for res := range Run(ctx, probes) {
		got = append(got, res)
	}
	if len(got) != 2 {
		t.Fatalf("results %d", len(got))
	}
	byID := map[string]Result{}
	for _, res := range got {
		byID[res.ID] = res
	}
	if !byID["open"].OK || !byID["instant"].OK {
		t.Fatalf("results %+v", got)
	}
	if byID["instant"].Name != "instant" {
		t.Fatalf("name was not filled: %+v", byID["instant"])
	}
}

func TestRunCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Port("pg", "PostgreSQL", "127.0.0.1", 1).Run(ctx)
	if res.OK {
		t.Fatal("expected cancel")
	}
	if res.Detail != "canceled" && res.Detail != "connection refused" {
		t.Fatalf("detail %q", res.Detail)
	}
}

func TestGoProbe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := Go().Run(ctx)
	if !res.OK {
		t.Fatal(res.Detail)
	}
	if !strings.HasPrefix(res.Detail, "go1.") && !strings.HasPrefix(res.Detail, "go2.") {
		t.Fatalf("detail %q", res.Detail)
	}
}

func TestCommandMissing(t *testing.T) {
	res := commandProbe("missing", "Missing", "nope", "envcraft-binary-missing", nil, func(string) string { return "ready" }, runtimeFailure).Run(context.Background())
	if res.OK || res.Detail != "not installed" {
		t.Fatalf("%+v", res)
	}
}

func TestDetailParsers(t *testing.T) {
	if got := goDetail("go version go1.22.5 darwin/arm64\n"); got != "go1.22.5" {
		t.Fatalf("go %q", got)
	}
	if got := nodeDetail("v22.11.0\n"); got != "v22.11.0" {
		t.Fatalf("node %q", got)
	}
	if got := dockerDetail("27.3.1\n"); got != "daemon 27.3.1" {
		t.Fatalf("docker %q", got)
	}
	if got := dockerDetail("  \n"); got != "daemon reachable" {
		t.Fatalf("docker empty %q", got)
	}
	if got := dialDetail(context.DeadlineExceeded); got != "timed out" {
		t.Fatalf("timeout %q", got)
	}
	err := &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
	if got := dialDetail(err); got != "connection refused" {
		t.Fatalf("refused %q", got)
	}
	if got := dockerFailure(errors.New("exit"), "permission denied while trying to connect"); got != "permission denied" {
		t.Fatalf("docker fail %q", got)
	}
	if got := dockerFailure(context.DeadlineExceeded, ""); got != "timed out" {
		t.Fatalf("docker timeout %q", got)
	}
}

func TestRecoverProbePanic(t *testing.T) {
	ch := Run(context.Background(), []Probe{{
		ID:   "boom",
		Name: "Boom",
		Run: func(context.Context) Result {
			panic("bad probe")
		},
	}})
	res := <-ch
	if _, ok := <-ch; ok {
		t.Fatal("channel still open")
	}
	if res.OK || !strings.Contains(res.Detail, "bad probe") {
		t.Fatalf("%+v", res)
	}
}
