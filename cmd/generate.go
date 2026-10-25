package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/ahmadhaffoudhi/envcraft/internal/scaffolder"
)

func init() {
	rootCmd.AddCommand(newGenerateCmd())
}

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate [go|node]",
		Aliases: []string{"gen"},
		Short:   "Scaffold a Clean Architecture project",
		Long: `Scaffold a Clean Architecture project.

  go      command, domain, use case, memory adapter, and HTTP API
  node    TypeScript modules, application wiring, and an HTTP server`,
		Example: `  envcraft generate
  envcraft generate go --name ledger --module github.com/acme/ledger
  envcraft generate --stack node --name web --out ./web`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"go", "node"},
		RunE:      runGenerate,
	}
	cmd.Flags().String("stack", "", "Project stack: go or node")
	cmd.Flags().String("name", "", "Project name")
	cmd.Flags().String("module", "", "Go module path")
	cmd.Flags().String("out", "", "Output directory (default: the project name)")
	cmd.Flags().Bool("force", false, "Write into a non-empty directory")
	return cmd
}

func runGenerate(cmd *cobra.Command, args []string) error {
	cfg := scaffolder.ScaffoldConfig{}
	if len(args) == 1 {
		stack, err := parseStack(args[0])
		if err != nil {
			return err
		}
		cfg.Stack = stack
	}
	var err error
	flagStack, err := cmd.Flags().GetString("stack")
	if err != nil {
		return err
	}
	if flagStack != "" {
		stack, err := parseStack(flagStack)
		if err != nil {
			return err
		}
		if cfg.Stack != "" && cfg.Stack != stack {
			return fmt.Errorf("argument %s does not match --stack %s", cfg.Stack, flagStack)
		}
		cfg.Stack = stack
	}
	cfg.ProjectName, err = cmd.Flags().GetString("name")
	if err != nil {
		return err
	}
	cfg.ModulePath, err = cmd.Flags().GetString("module")
	if err != nil {
		return err
	}
	cfg.OutDir, err = cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	cfg.Force, err = cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	if err := completeOptions(&cfg); err != nil {
		return abort(err)
	}

	start := time.Now()
	result, err := scaffolder.Generate(cfg)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	writeln(out, banner(version))
	writeln(out)
	writeln(out, heading("Scaffold"))
	writeln(out)
	for _, file := range result.Files {
		writeln(out, success(file.RelPath))
	}
	writeln(out)
	writeln(out, rule())
	writeln(out)
	writeln(out, doneLine(fmt.Sprintf("scaffolded %s project %s · %d files · %s", cfg.Stack, cfg.ProjectName, len(result.Files), formatLatency(time.Since(start)))))
	return nil
}

func completeOptions(cfg *scaffolder.ScaffoldConfig) error {
	if cfg.Stack != "" && cfg.Stack != scaffolder.StackGo && cfg.Stack != scaffolder.StackNode {
		return fmt.Errorf("unknown stack %q", cfg.Stack)
	}
	if cfg.ProjectName != "" {
		if err := scaffolder.ValidateName(cfg.ProjectName); err != nil {
			return err
		}
	}
	if cfg.ModulePath != "" {
		if err := scaffolder.ValidateModule(cfg.ModulePath); err != nil {
			return err
		}
	}
	if cfg.Stack == "" || cfg.ProjectName == "" {
		if !interactive() {
			return errors.New("pass a stack and --name when stdin is not a terminal")
		}
		if err := promptIdentity(cfg, cfg.Stack == "", cfg.ProjectName == ""); err != nil {
			return err
		}
	}
	if cfg.Stack == scaffolder.StackGo && cfg.ModulePath == "" {
		if !interactive() {
			return errors.New("go scaffolds require --module when stdin is not a terminal")
		}
		if err := promptModule(cfg); err != nil {
			return err
		}
	}
	if cfg.OutDir == "" {
		cfg.OutDir = cfg.ProjectName
	}
	return nil
}

func promptIdentity(cfg *scaffolder.ScaffoldConfig, needStack, needName bool) error {
	stack := string(cfg.Stack)
	name := cfg.ProjectName
	var fields []huh.Field
	if needStack {
		fields = append(fields, huh.NewSelect[string]().
			Title("Architecture").
			Options(
				huh.NewOption("Go — Clean Architecture", "go"),
				huh.NewOption("Node — TypeScript layers", "node"),
			).
			Value(&stack))
	}
	if needName {
		fields = append(fields, huh.NewInput().
			Title("Project name").
			Placeholder("ledger").
			Value(&name).
			Validate(scaffolder.ValidateName))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(formTheme()).Run(); err != nil {
		return err
	}
	cfg.Stack = scaffolder.Stack(stack)
	cfg.ProjectName = name
	return nil
}

func promptModule(cfg *scaffolder.ScaffoldConfig) error {
	module := "example.com/" + strings.ToLower(cfg.ProjectName)
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Go module path").
			Value(&module).
			Validate(scaffolder.ValidateModule),
	)).WithTheme(formTheme()).Run()
	if err != nil {
		return err
	}
	cfg.ModulePath = module
	return nil
}

func parseStack(arg string) (scaffolder.Stack, error) {
	switch strings.ToLower(arg) {
	case "go":
		return scaffolder.StackGo, nil
	case "node", "ts", "typescript":
		return scaffolder.StackNode, nil
	default:
		return "", fmt.Errorf("unknown stack %q (use go or node)", arg)
	}
}
