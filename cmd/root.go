package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	cfgFile   string
	debug     bool
	noColor   bool
	configErr error
)

var rootCmd = &cobra.Command{
	Use:   "envcraft",
	Short: "Local development toolkit",
	Long: `Lock .env vaults with AES-256-GCM, check a local stack concurrently,
and scaffold Clean Architecture projects for Go and Node/TypeScript.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if configErr != nil {
			return configErr
		}
		if debug && viper.ConfigFileUsed() != "" {
			writef(cmd.ErrOrStderr(), "config %s\n", viper.ConfigFileUsed())
		}
		return nil
	},
}

// Execute runs the envcraft CLI and exits the process on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Err != nil {
				writeln(os.Stderr, errorText(exitErr.Err.Error()))
			}
			code := exitErr.Code
			if code == 0 {
				code = 1
			}
			os.Exit(code)
		}
		writeln(os.Stderr, errorText(err.Error()))
		os.Exit(1)
	}
}

func init() {
	cobra.EnableTraverseRunHooks = true
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ./envcraft.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "print probe targets before a command runs")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("envcraft {{.Version}} (" + commit + ", " + date + ")\n")
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.SetHelpFunc(writeHelp)

	prepareCommands(rootCmd)
}

func prepareCommands(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetHelpFunc(writeHelp)
	for _, child := range cmd.Commands() {
		prepareCommands(child)
	}
}

func initConfig() {
	configErr = nil
	applyColor(noColor)
	viper.Reset()
	viper.SetConfigType("yaml")

	path := cfgFile
	if path == "" {
		path = findDefaultConfig(defaultConfigDirs())
	}
	if path == "" {
		return
	}
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		configErr = fmt.Errorf("read config %s: %w", path, err)
	}
}

// defaultConfigDirs is the current directory, then the user config directory.
// An extensionless ./envcraft binary must not be treated as configuration.
func defaultConfigDirs() []string {
	dirs := []string{"."}
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(dir, "envcraft"))
	}
	return dirs
}

func findDefaultConfig(dirs []string) string {
	names := []string{"envcraft.yaml", "envcraft.yml"}
	for _, dir := range dirs {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func writeHelp(cmd *cobra.Command, _ []string) {
	out := cmd.OutOrStdout()
	writeln(out, banner(version))
	writeln(out)
	body := cmd.Long
	if body == "" {
		body = cmd.Short
	}
	if body != "" {
		writeln(out, body)
		writeln(out)
	}
	writeln(out, heading("Usage"))
	writef(out, "  %s\n", cmd.UseLine())
	if cmd.Example != "" {
		writef(out, "\n%s\n%s\n", heading("Examples"), cmd.Example)
	}
	if subs := visibleCommands(cmd); len(subs) > 0 {
		width := 0
		for _, c := range subs {
			if n := len(c.Name()); n > width {
				width = n
			}
		}
		writef(out, "\n%s\n", heading("Commands"))
		for _, c := range subs {
			padded := c.Name() + strings.Repeat(" ", width-len(c.Name()))
			writef(out, "  %s  %s\n", accentText(padded), c.Short)
		}
	}
	if cmd.HasAvailableLocalFlags() {
		writef(out, "\n%s\n%s", heading("Flags"), cmd.LocalFlags().FlagUsages())
	}
	// Persistent flags live in LocalFlags on the command that defined them.
	// Children surface that same set through InheritedFlags.
	if cmd.HasParent() && cmd.InheritedFlags().HasAvailableFlags() {
		writef(out, "\n%s\n%s", heading("Global Flags"), cmd.InheritedFlags().FlagUsages())
	}
	if !cmd.HasParent() && cmd.HasAvailableSubCommands() {
		writef(out, "\nUse \"%s [command] --help\" for more information about a command.\n", cmd.CommandPath())
	}
}

func visibleCommands(cmd *cobra.Command) []*cobra.Command {
	var cmds []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() && !c.Hidden {
			cmds = append(cmds, c)
		}
	}
	return cmds
}
