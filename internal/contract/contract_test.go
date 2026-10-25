package contract

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDiscoverWriteAndLoad(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "compose.yaml"), `
services:
  postgres:
    image: postgres:16
    ports:
      - "127.0.0.1:5432:5432"
  redis:
    ports:
      - target: 6379
        published: 6379
        host_ip: 0.0.0.0
`)
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/demo\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(dir, "package.json"), `{"name":"web","engines":{"node":">=22"}}`)
	writeFile(t, filepath.Join(dir, ".env.example"), "# local\nDATABASE_URL=postgres://127.0.0.1:5432/app\nADDR=:8080\n")

	found, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !found.Doctor.Docker {
		t.Fatal("expected docker")
	}
	if len(found.Doctor.Services) != 2 {
		t.Fatalf("services %#v", found.Doctor.Services)
	}
	if found.Doctor.Services[0].Name != "postgres" || found.Doctor.Services[0].Host != "127.0.0.1" || found.Doctor.Services[0].Port != 5432 {
		t.Fatalf("postgres %#v", found.Doctor.Services[0])
	}
	if found.Doctor.Services[1].Host != "" || found.Doctor.Services[1].Port != 6379 {
		t.Fatalf("redis %#v", found.Doctor.Services[1])
	}
	if len(found.Doctor.Tools) != 2 || found.Doctor.Tools[0].Version != "1.26.0" || found.Doctor.Tools[1].Version != "22" {
		t.Fatalf("tools %#v", found.Doctor.Tools)
	}
	if strings.Join(found.Env.Required, ",") != "DATABASE_URL,ADDR" {
		t.Fatalf("keys %v", found.Env.Required)
	}

	path, err := Write(dir, found, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, found, false); !errors.Is(err, ErrExists) {
		t.Fatalf("overwrite %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Project() || !loaded.Doctor.Docker || len(loaded.Doctor.Services) != 2 {
		t.Fatalf("loaded %#v", loaded)
	}
}

func TestLegacyConfigIsNotAProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "envcraft.yaml")
	writeFile(t, path, "doctor:\n  host: 127.0.0.1\n  postgres_port: 5432\n  redis_port: 6379\n  timeout: 2s\nenv:\n  file: .env\n  vault: .env.vault\n")
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project() {
		t.Fatal("legacy config became a project contract")
	}
}

func TestEmptyDirectory(t *testing.T) {
	if _, err := Discover(t.TempDir()); !errors.Is(err, ErrEmpty) {
		t.Fatalf("err %v", err)
	}
}

func TestEnvProbeDialsLocalURL(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "DATABASE_URL=postgres://postgres:postgres@127.0.0.1:"+strconv.Itoa(port)+"/app?sslmode=disable\n")
	c := Contract{
		project: true,
		Env:     Env{File: ".env", Required: []string{"DATABASE_URL"}},
	}
	probes, err := c.Probes(dir, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res := probes[0].Run(ctx)
	if !res.OK {
		t.Fatalf("result %+v", res)
	}

	writeFile(t, filepath.Join(dir, ".env"), "DATABASE_URL=postgres://127.0.0.1:1/app\n")
	res = probes[0].Run(ctx)
	if res.OK || !strings.Contains(res.Detail, "connection refused") {
		t.Fatalf("result %+v", res)
	}

	writeFile(t, filepath.Join(dir, ".env"), "DATABASE_URL=postgres://192.0.2.1:5432/app\n")
	res = probes[0].Run(ctx)
	if !res.OK {
		t.Fatalf("remote URL should not be dialed: %+v", res)
	}
}

func TestEnvProbeMissingKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "ADDR=:8080\n")
	c := Contract{project: true, Env: Env{File: ".env", Required: []string{"ADDR", "DATABASE_URL"}}}
	probes, err := c.Probes(dir, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	res := probes[0].Run(context.Background())
	if res.OK || res.Detail != "missing DATABASE_URL" {
		t.Fatalf("result %+v", res)
	}
}

func TestQuotedVersionLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "envcraft.yaml")
	writeFile(t, path, "doctor:\n  docker: false\n  tools:\n    - name: go\n      version: \"1.26\"\n")
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Project() || loaded.Doctor.Tools[0].Version != "1.26" {
		t.Fatalf("%#v", loaded.Doctor.Tools)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
