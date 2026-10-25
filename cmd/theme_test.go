package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestFormatLatency(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{12 * time.Millisecond, "12ms"},
		{1500 * time.Microsecond, "1.5ms"},
		{800 * time.Microsecond, "800µs"},
		{2 * time.Second, "2.0s"},
		{10 * time.Second, "10s"},
	}
	for _, tc := range cases {
		if got := formatLatency(tc.d); got != tc.want {
			t.Fatalf("formatLatency(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestPad(t *testing.T) {
	if got := pad("PostgreSQL :5432", 18); len([]rune(got)) != 18 {
		t.Fatalf("pad width %d %q", len([]rune(got)), got)
	}
	if got := pad("connection refused!!", 8); got != "connect…" {
		t.Fatalf("truncate %q", got)
	}
}

func TestStringSettingPrecedence(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := &cobra.Command{Use: "doctor"}
	cmd.Flags().String("host", "127.0.0.1", "")

	if got := stringSetting(cmd, "host", "ENVCRAFT_DOCTOR_HOST", "doctor.host", "127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("default %s", got)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "envcraft.yaml")
	if err := os.WriteFile(path, []byte("doctor:\n  host: 10.0.0.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if got := stringSetting(cmd, "host", "ENVCRAFT_DOCTOR_HOST", "doctor.host", "127.0.0.1"); got != "10.0.0.5" {
		t.Fatalf("config %s", got)
	}

	t.Setenv("ENVCRAFT_DOCTOR_HOST", "10.0.0.9")
	if got := stringSetting(cmd, "host", "ENVCRAFT_DOCTOR_HOST", "doctor.host", "127.0.0.1"); got != "10.0.0.9" {
		t.Fatalf("env %s", got)
	}

	if err := cmd.Flags().Set("host", "10.1.1.1"); err != nil {
		t.Fatal(err)
	}
	if got := stringSetting(cmd, "host", "ENVCRAFT_DOCTOR_HOST", "doctor.host", "127.0.0.1"); got != "10.1.1.1" {
		t.Fatalf("flag %s", got)
	}
}

func TestFindDefaultConfigIgnoresBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "envcraft"), []byte{0xff, 0xfe, 0x00}, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findDefaultConfig([]string{dir}); got != "" {
		t.Fatalf("extensionless file was used as config: %s", got)
	}

	yamlPath := filepath.Join(dir, "envcraft.yaml")
	if err := os.WriteFile(yamlPath, []byte("doctor:\n  host: 10.0.0.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findDefaultConfig([]string{dir}); got != yamlPath {
		t.Fatalf("config path %s", got)
	}
}

func TestValidatePortAndTimeout(t *testing.T) {
	if err := validatePort("redis", 0); err == nil {
		t.Fatal("expected port error")
	}
	if err := validatePort("redis", 6379); err != nil {
		t.Fatal(err)
	}
	if err := validateTimeout(10 * time.Millisecond); err == nil {
		t.Fatal("expected timeout error")
	}
	if err := validateTimeout(2 * time.Second); err != nil {
		t.Fatal(err)
	}
}
