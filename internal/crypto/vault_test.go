package crypto

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEncryptRoundTrip(t *testing.T) {
	plain := []byte("# comment\nDB_PASSWORD=p@ss word\nUNICODE=héllo\nEMPTY=\n")
	vault, err := Encrypt(plain, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(vault, []byte("envcraft-vault 1\n")) {
		t.Fatalf("header %q", vault[:24])
	}
	got, err := Decrypt(vault, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plaintext mismatch\n got %q\nwant %q", got, plain)
	}

	if _, err := Decrypt(vault, "correct horse battery!"); !errors.Is(err, ErrAuth) {
		t.Fatalf("wrong passphrase error %v", err)
	}

	tampered := bytes.Clone(vault)
	tampered[len(tampered)-2] ^= 0x01
	if _, err := Decrypt(tampered, "correct horse battery"); !errors.Is(err, ErrAuth) && !errors.Is(err, ErrFormat) {
		t.Fatalf("tamper error %v", err)
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	cases := [][]byte{
		[]byte("KEY=value\n"),
		[]byte("envcraft-vault\n"),
		[]byte("envcraft-vault 1\n@@@\n"),
		[]byte("envcraft-vault 0\nAAAA\n"),
	}
	for _, vault := range cases {
		if _, err := Decrypt(vault, "long-enough"); !errors.Is(err, ErrFormat) {
			t.Fatalf("vault %q error %v", vault, err)
		}
	}
}

func TestEncryptRejectsShortPassphrase(t *testing.T) {
	if _, err := Encrypt([]byte("A=B\n"), "short"); err == nil {
		t.Fatal("expected short passphrase error")
	}
}

func TestLockUnlockFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, ".env")
	dst := filepath.Join(dir, ".env.vault")
	plain := []byte("TOKEN=secret\n")
	if err := os.WriteFile(src, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LockFile(src, dst, "long-enough", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("plaintext should remain")
	}
	if err := LockFile(src, dst, "long-enough", false); !errors.Is(err, ErrExists) {
		t.Fatalf("overwrite error %v", err)
	}
	info, err := Inspect(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != 1 {
		t.Fatalf("version %d", info.Version)
	}
	if runtime.GOOS != "windows" {
		stat, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if stat.Mode().Perm() != 0o600 {
			t.Fatalf("mode %o", stat.Mode().Perm())
		}
	}
	out := filepath.Join(dir, ".env.out")
	if err := UnlockFile(dst, out, "long-enough", false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("unlocked %q", got)
	}
	if err := LockFile(dst, filepath.Join(dir, "again.vault"), "long-enough", false); err == nil || !strings.Contains(err.Error(), "already looks like") {
		t.Fatalf("double lock error %v", err)
	}
}

func TestEmptyPlaintext(t *testing.T) {
	vault, err := Encrypt(nil, "long-enough")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(vault, "long-enough")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len %d", len(got))
	}
}
