package cmd

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/ahmadhaffoudhi/envcraft/internal/crypto"
)

func init() {
	rootCmd.AddCommand(newEnvCmd())
}

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Lock and unlock .env vaults",
		Long: `Lock and unlock .env files. Vaults use AES-256-GCM with a scrypt key.

The passphrase comes from --passphrase, ENVCRAFT_PASSPHRASE, or a prompt.
It is never read from the config file. Locked and unlocked files are mode 0600.
The source file is left in place.`,
		Example: `  envcraft env lock
  envcraft env unlock --force
  envcraft env status`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newLockCmd(), newUnlockCmd(), newStatusCmd())
	return cmd
}

func newLockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "Encrypt a .env file into a vault",
		Args:  cobra.NoArgs,
		RunE:  runLock,
	}
	cmd.Flags().String("file", ".env", "Plaintext env file")
	cmd.Flags().String("out", ".env.vault", "Vault destination")
	cmd.Flags().Bool("force", false, "Overwrite the destination")
	cmd.Flags().String("passphrase", "", "Passphrase (prefer ENVCRAFT_PASSPHRASE)")
	return cmd
}

func newUnlockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Decrypt a vault back into a .env file",
		Args:  cobra.NoArgs,
		RunE:  runUnlock,
	}
	cmd.Flags().String("file", ".env.vault", "Vault file")
	cmd.Flags().String("out", ".env", "Plaintext destination")
	cmd.Flags().Bool("force", false, "Overwrite the destination")
	cmd.Flags().String("passphrase", "", "Passphrase (prefer ENVCRAFT_PASSPHRASE)")
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show plaintext and vault files",
		Args:  cobra.NoArgs,
		RunE:  runStatus,
	}
	cmd.Flags().String("file", ".env", "Plaintext env file")
	cmd.Flags().String("vault", ".env.vault", "Vault file")
	return cmd
}

func runLock(cmd *cobra.Command, _ []string) error {
	src, dst := lockPaths(cmd)
	force, _ := cmd.Flags().GetBool("force")
	if err := crypto.PrepareLock(src, dst, force); err != nil {
		return err
	}
	pass, err := passphraseFrom(cmd, true)
	if err != nil {
		return abort(err)
	}
	start := time.Now()
	if err := crypto.LockFile(src, dst, pass, force); err != nil {
		return err
	}
	return printVaultResult(cmd, fmt.Sprintf("locked %s %s %s", src, arrow(), dst), dst, time.Since(start))
}

func runUnlock(cmd *cobra.Command, _ []string) error {
	src, dst := unlockPaths(cmd)
	force, _ := cmd.Flags().GetBool("force")
	if err := crypto.PrepareUnlock(src, dst, force); err != nil {
		return err
	}
	pass, err := passphraseFrom(cmd, false)
	if err != nil {
		return abort(err)
	}
	start := time.Now()
	if err := crypto.UnlockFile(src, dst, pass, force); err != nil {
		return err
	}
	return printVaultResult(cmd, fmt.Sprintf("unlocked %s %s %s", src, arrow(), dst), dst, time.Since(start))
}

func printVaultResult(cmd *cobra.Command, message, dst string, elapsed time.Duration) error {
	info, err := os.Stat(dst)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	writeln(out, banner(version))
	writeln(out)
	writeln(out, heading("Vault"))
	writeln(out)
	writeln(out, success(message))
	writeln(out, sub(fmt.Sprintf("%s  ·  0600  ·  %s", formatBytes(info.Size()), formatLatency(elapsed))))
	writeln(out, sub("source was left in place"))
	return nil
}

func runStatus(cmd *cobra.Command, _ []string) error {
	plain := stringSetting(cmd, "file", "ENVCRAFT_ENV_FILE", "env.file", ".env")
	vault := stringSetting(cmd, "vault", "ENVCRAFT_ENV_VAULT", "env.vault", ".env.vault")
	out := cmd.OutOrStdout()
	writeln(out, banner(version))
	writeln(out)
	writeln(out, heading("Vault"))
	writeln(out)
	if err := printPlainStatus(out, plain); err != nil {
		return err
	}
	bad, err := printVaultStatus(out, vault)
	if err != nil {
		return err
	}
	if bad {
		return &ExitError{Code: 1}
	}
	return nil
}

func printPlainStatus(out io.Writer, path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		writeln(out, idleRow(path, "missing", ""))
		return nil
	}
	if err != nil {
		return err
	}
	writeln(out, passRow(path, "plaintext", formatBytes(info.Size())))
	return nil
}

func printVaultStatus(out io.Writer, path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		writeln(out, idleRow(path, "missing", ""))
		return false, nil
	}
	if err != nil {
		return false, err
	}
	inspected, err := crypto.Inspect(path)
	if err != nil {
		writeln(out, failRow(path, "unrecognized", formatBytes(info.Size())))
		return true, nil
	}
	writeln(out, passRow(path, fmt.Sprintf("vault v%d", inspected.Version), formatBytes(inspected.Size)))
	return false, nil
}

func lockPaths(cmd *cobra.Command) (string, string) {
	src := stringSetting(cmd, "file", "ENVCRAFT_ENV_FILE", "env.file", ".env")
	dst := stringSetting(cmd, "out", "ENVCRAFT_ENV_VAULT", "env.vault", ".env.vault")
	return src, dst
}

func unlockPaths(cmd *cobra.Command) (string, string) {
	src := stringSetting(cmd, "file", "ENVCRAFT_ENV_VAULT", "env.vault", ".env.vault")
	dst := stringSetting(cmd, "out", "ENVCRAFT_ENV_FILE", "env.file", ".env")
	return src, dst
}

func passphraseFrom(cmd *cobra.Command, create bool) (string, error) {
	if cmd.Flags().Changed("passphrase") {
		value, err := cmd.Flags().GetString("passphrase")
		if err != nil {
			return "", err
		}
		if err := crypto.ValidatePassphrase(value, create); err != nil {
			return "", err
		}
		return value, nil
	}
	if value, ok := os.LookupEnv("ENVCRAFT_PASSPHRASE"); ok && value != "" {
		if err := crypto.ValidatePassphrase(value, create); err != nil {
			return "", err
		}
		return value, nil
	}
	if !interactive() {
		return "", errors.New("passphrase required via --passphrase or ENVCRAFT_PASSPHRASE")
	}
	if create {
		return promptNewPassphrase()
	}
	return promptPassphrase()
}

func promptNewPassphrase() (string, error) {
	var pass, again string
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Vault passphrase").
			Description("At least 8 characters. It is not stored.").
			EchoMode(huh.EchoModePassword).
			Value(&pass).
			Validate(func(s string) error { return crypto.ValidatePassphrase(s, true) }),
		huh.NewInput().
			Title("Confirm passphrase").
			EchoMode(huh.EchoModePassword).
			Value(&again).
			Validate(func(s string) error {
				if s == "" {
					return errors.New("confirmation is required")
				}
				return nil
			}),
	)).WithTheme(formTheme()).Run()
	if err != nil {
		return "", err
	}
	if subtle.ConstantTimeCompare([]byte(pass), []byte(again)) != 1 {
		return "", errors.New("passphrases do not match")
	}
	return pass, nil
}

func promptPassphrase() (string, error) {
	var pass string
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Vault passphrase").
			EchoMode(huh.EchoModePassword).
			Value(&pass).
			Validate(func(s string) error { return crypto.ValidatePassphrase(s, false) }),
	)).WithTheme(formTheme()).Run()
	if err != nil {
		return "", err
	}
	return pass, nil
}

func abort(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return &ExitError{Code: 130}
	}
	return err
}
