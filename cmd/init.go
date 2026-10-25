package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Ahmadhaff/envcraft/internal/contract"
)

func init() {
	rootCmd.AddCommand(newInitCmd())
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a project contract for this directory",
		Long: `Inspect the current directory and write envcraft.yaml.

The contract is built from a Compose file, go.mod, package.json, and
.env.example. doctor then checks those services, toolchains, and env keys
instead of a fixed PostgreSQL and Redis list.`,
		Example: `  envcraft init
  envcraft init --dir ./ledger --force`,
		Args: cobra.NoArgs,
		RunE: runInit,
	}
	cmd.Flags().String("dir", ".", "Directory to inspect")
	cmd.Flags().Bool("force", false, "Overwrite an existing envcraft.yaml")
	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	dir, err := cmd.Flags().GetString("dir")
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	start := time.Now()
	loaded, err := contract.Discover(dir)
	if err != nil {
		return err
	}
	probes, err := loaded.Probes(dir, loaded.Doctor.Host)
	if err != nil {
		return err
	}
	path, err := contract.Write(dir, loaded, force)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	writeln(out, banner(version))
	writeln(out)
	writeln(out, heading("Contract"))
	writeln(out)
	for _, probe := range probes {
		writeln(out, success(probe.Name))
	}
	writeln(out)
	writeln(out, rule())
	writeln(out)
	writeln(out, doneLine(fmt.Sprintf("wrote %s · %d checks · %s", path, len(probes), formatLatency(time.Since(start)))))
	return nil
}
