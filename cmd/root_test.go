package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"envcraft", "doctor", "generate", "vault"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help missing %q\n%s", want, got)
		}
	}
}
