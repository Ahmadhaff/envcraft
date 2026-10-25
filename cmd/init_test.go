package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ahmadhaff/envcraft/internal/contract"
)

func TestInitCommandWritesContract(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("ADDR=:8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "cli.yaml")
	if err := os.WriteFile(cfg, []byte("doctor:\n  host: 127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--config", cfg, "init", "--dir", dir})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "envcraft.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `version: "1.26.0"`) || !strings.Contains(text, "ADDR") {
		t.Fatalf("contract\n%s", text)
	}

	rootCmd.SetArgs([]string{"--config", cfg, "init", "--dir", dir})
	err = rootCmd.Execute()
	if !errors.Is(err, contract.ErrExists) {
		t.Fatalf("second init %v", err)
	}
}
