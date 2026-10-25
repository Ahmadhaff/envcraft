package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func stringSetting(cmd *cobra.Command, flag, envKey, key, fallback string) string {
	if cmd.Flags().Changed(flag) {
		value, err := cmd.Flags().GetString(flag)
		if err == nil {
			return value
		}
	}
	if value, ok := os.LookupEnv(envKey); ok && value != "" {
		return value
	}
	if viper.InConfig(key) {
		return viper.GetString(key)
	}
	return fallback
}

func intSetting(cmd *cobra.Command, flag, envKey, key string, fallback int) (int, error) {
	if cmd.Flags().Changed(flag) {
		return cmd.Flags().GetInt(flag)
	}
	if value, ok := os.LookupEnv(envKey); ok && value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", envKey, err)
		}
		return parsed, nil
	}
	if viper.InConfig(key) {
		return viper.GetInt(key), nil
	}
	return fallback, nil
}

func durationSetting(cmd *cobra.Command, flag, envKey, key string, fallback time.Duration) (time.Duration, error) {
	if cmd.Flags().Changed(flag) {
		return cmd.Flags().GetDuration(flag)
	}
	if value, ok := os.LookupEnv(envKey); ok && value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", envKey, err)
		}
		return parsed, nil
	}
	if viper.InConfig(key) {
		if parsed := viper.GetDuration(key); parsed != 0 {
			return parsed, nil
		}
		parsed, err := time.ParseDuration(viper.GetString(key))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}
	return fallback, nil
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", name)
	}
	return nil
}

func validateTimeout(d time.Duration) error {
	if d < 50*time.Millisecond || d > 30*time.Second {
		return fmt.Errorf("timeout must be between 50ms and 30s")
	}
	return nil
}
