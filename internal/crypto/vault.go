// Package crypto locks .env files with AES-256-GCM.
//
// Vault file format version 1:
//
//	envcraft-vault 1\n
//	base64(salt || nonce || ciphertext)
//
// salt is 16 bytes and nonce is 12 bytes. ciphertext includes the GCM tag.
// The key is scrypt(passphrase, salt, N=32768, r=8, p=1) and the GCM
// additional data is "envcraft-vault-v1". A new salt and nonce are used
// every time a file is locked.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	headerPrefix = "envcraft-vault "
	version1     = 1
	aad          = "envcraft-vault-v1"

	scryptN  = 1 << 15
	scryptR  = 8
	scryptP  = 1
	keyLen   = 32
	saltLen  = 16
	nonceLen = 12

	maxPlain = 2 << 20
	maxVault = 3 << 20
	fileMode = 0o600
)

// Sentinel errors for vault operations.
var (
	ErrFormat   = errors.New("not an envcraft vault")
	ErrAuth     = errors.New("vault authentication failed")
	ErrTooLarge = errors.New("file exceeds 2MiB limit")
	ErrExists   = errors.New("destination exists")
	ErrSamePath = errors.New("source and destination must differ")
)

// Info describes a vault without decrypting it.
type Info struct {
	Version int
	Size    int64
}

// ValidatePassphrase rejects empty and oversized passphrases.
// enforceMin applies the 8 character floor used when creating a vault.
func ValidatePassphrase(passphrase string, enforceMin bool) error {
	if passphrase == "" {
		return errors.New("passphrase is required")
	}
	if len(passphrase) > 1024 {
		return errors.New("passphrase is too long")
	}
	if enforceMin && len([]rune(passphrase)) < 8 {
		return errors.New("passphrase must be at least 8 characters")
	}
	return nil
}

// Encrypt seals plaintext as a version 1 vault.
func Encrypt(plaintext []byte, passphrase string) ([]byte, error) {
	if err := ValidatePassphrase(passphrase, true); err != nil {
		return nil, err
	}
	if len(plaintext) > maxPlain {
		return nil, ErrTooLarge
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("salt: %w", err)
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	defer clear(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	if gcm.NonceSize() != nonceLen {
		return nil, errors.New("unexpected gcm nonce size")
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, []byte(aad))
	payload := make([]byte, 0, saltLen+nonceLen+len(sealed))
	payload = append(payload, salt...)
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)
	return armor(version1, payload), nil
}

// Decrypt opens a version 1 vault. Authentication failures collapse to ErrAuth.
func Decrypt(vault []byte, passphrase string) ([]byte, error) {
	if err := ValidatePassphrase(passphrase, false); err != nil {
		return nil, err
	}
	if len(vault) > maxVault {
		return nil, ErrTooLarge
	}
	version, payload, err := unarmor(vault)
	if err != nil {
		return nil, err
	}
	if version != version1 {
		return nil, fmt.Errorf("%w: version %d", ErrFormat, version)
	}
	if len(payload) < saltLen+nonceLen+gcmTagSize() {
		return nil, ErrFormat
	}
	salt := payload[:saltLen]
	nonce := payload[saltLen : saltLen+nonceLen]
	sealed := payload[saltLen+nonceLen:]
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	defer clear(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, sealed, []byte(aad))
	if err != nil {
		return nil, ErrAuth
	}
	return plain, nil
}

// PrepareLock checks paths before a passphrase prompt.
func PrepareLock(src, dst string, force bool) error {
	return prepare(src, dst, force, false)
}

// PrepareUnlock checks paths and the vault header before a passphrase prompt.
func PrepareUnlock(src, dst string, force bool) error {
	return prepare(src, dst, force, true)
}

// LockFile encrypts src into dst. The destination mode is 0600.
func LockFile(src, dst, passphrase string, force bool) error {
	if err := PrepareLock(src, dst, force); err != nil {
		return err
	}
	plain, err := readLimited(src, maxPlain)
	if err != nil {
		return err
	}
	if vault, peekErr := IsVault(src); peekErr != nil {
		return peekErr
	} else if vault {
		return fmt.Errorf("source already looks like an envcraft vault: %s", src)
	}
	blob, err := Encrypt(plain, passphrase)
	if err != nil {
		return err
	}
	return writeAtomic(dst, blob, fileMode)
}

// UnlockFile decrypts src into dst. The destination mode is 0600.
func UnlockFile(src, dst, passphrase string, force bool) error {
	if err := PrepareUnlock(src, dst, force); err != nil {
		return err
	}
	blob, err := readLimited(src, maxVault)
	if err != nil {
		return err
	}
	plain, err := Decrypt(blob, passphrase)
	if err != nil {
		return err
	}
	return writeAtomic(dst, plain, fileMode)
}

// IsVault reports whether path starts with an envcraft vault header.
func IsVault(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, 64)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return strings.HasPrefix(string(buf[:n]), headerPrefix), nil
}

// Inspect reads the vault version and file size without decrypting.
func Inspect(path string) (Info, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Info{}, err
	}
	if info.Size() > maxVault {
		return Info{}, ErrTooLarge
	}
	data, err := readLimited(path, maxVault)
	if err != nil {
		return Info{}, err
	}
	version, _, err := unarmor(data)
	if err != nil {
		return Info{}, err
	}
	return Info{Version: version, Size: info.Size()}, nil
}

func prepare(src, dst string, force bool, wantVault bool) error {
	if samePath(src, dst) {
		return ErrSamePath
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if wantVault {
		ok, err := IsVault(src)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s", ErrFormat, src)
		}
	}
	if force {
		return nil
	}
	_, err := os.Stat(dst)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrExists, dst)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("destination: %w", err)
	}
	return nil
}

func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}

func armor(version int, payload []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(payload)
	var b strings.Builder
	b.WriteString(headerPrefix)
	b.WriteString(strconv.Itoa(version))
	b.WriteByte('\n')
	for len(encoded) > 64 {
		b.WriteString(encoded[:64])
		b.WriteByte('\n')
		encoded = encoded[64:]
	}
	if encoded != "" {
		b.WriteString(encoded)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func unarmor(vault []byte) (int, []byte, error) {
	text := string(vault)
	if !strings.HasPrefix(text, headerPrefix) {
		return 0, nil, ErrFormat
	}
	rest := text[len(headerPrefix):]
	line, body, ok := strings.Cut(rest, "\n")
	if !ok {
		return 0, nil, ErrFormat
	}
	version, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || version < 1 {
		return 0, nil, ErrFormat
	}
	compact := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', ' ', '\t':
			return -1
		default:
			return r
		}
	}, body)
	if compact == "" {
		return 0, nil, ErrFormat
	}
	payload, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return 0, nil, ErrFormat
	}
	return version, payload, nil
}

func gcmTagSize() int {
	return 16
}

func readLimited(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, ErrTooLarge
	}
	return data, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".envcraft-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmpName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	success = true
	return nil
}

func replaceFile(tmp, dst string) error {
	err := os.Rename(tmp, dst)
	if err == nil {
		return nil
	}
	if _, statErr := os.Stat(dst); statErr != nil {
		return err
	}
	if rmErr := os.Remove(dst); rmErr != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return aa == bb
}
