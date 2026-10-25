package scaffolder

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateGoBuilds(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "demo")
	result, err := Generate(ScaffoldConfig{
		Stack:       StackGo,
		ProjectName: "demo",
		ModulePath:  "example.com/demo",
		OutDir:      out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) < 6 {
		t.Fatalf("files %d", len(result.Files))
	}
	body, err := os.ReadFile(filepath.Join(out, "cmd", "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "example.com/demo/internal/usecase") {
		t.Fatalf("module was not rendered:\n%s", body)
	}
	contractBody, err := os.ReadFile(filepath.Join(out, "envcraft.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contractBody), "name: postgres") || !strings.Contains(string(contractBody), "DATABASE_URL") {
		t.Fatalf("contract was not rendered:\n%s", contractBody)
	}
	if _, err := os.Stat(filepath.Join(out, "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, ".env.example")); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(ScaffoldConfig{Stack: StackGo, ProjectName: "demo", ModulePath: "example.com/demo", OutDir: out}); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("overwrite error %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Dir = out
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, output)
	}
}

func TestGenerateNodeFiles(t *testing.T) {
	out := filepath.Join(t.TempDir(), "web")
	result, err := Generate(ScaffoldConfig{Stack: StackNode, ProjectName: "web", OutDir: out})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) == 0 {
		t.Fatal("no files")
	}
	raw, err := os.ReadFile(filepath.Join(out, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg map[string]any
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg["name"] != "web" {
		t.Fatalf("name %v", pkg["name"])
	}
	engines, _ := pkg["engines"].(map[string]any)
	if engines["node"] != ">=22" {
		t.Fatalf("engines %v", pkg["engines"])
	}
	contractBody, err := os.ReadFile(filepath.Join(out, "envcraft.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contractBody), "name: node") || !strings.Contains(string(contractBody), "PORT") {
		t.Fatalf("contract was not rendered:\n%s", contractBody)
	}
	if _, err := os.Stat(filepath.Join(out, ".env.example")); err != nil {
		t.Fatal(err)
	}
	server, err := os.ReadFile(filepath.Join(out, "src", "server.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(server), "createApp") {
		t.Fatal("server template missing createApp")
	}
	health, err := os.ReadFile(filepath.Join(out, "src", "modules", "health", "health.controller.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(health), "healthController") {
		t.Fatal("health module was not rendered")
	}
	if _, err := os.Stat(filepath.Join(out, ".gitignore")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNameAndModule(t *testing.T) {
	if err := ValidateName("A"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateName("1app"); err == nil {
		t.Fatal("expected digit rejection")
	}
	if err := ValidateModule("example.com/demo"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateModule("../secret"); err == nil {
		t.Fatal("expected module rejection")
	}
}
