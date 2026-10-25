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
  node    domain, application, infrastructure, and node:http`,
		Example: `  envcraft generate
  envcraft generate go --name ledger --module github.com/acme/ledger
  envcraft generate node --name web --out ./web`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"go", "node"},
		RunE:      runGenerate,
	}
	cmd.Flags().String("name", "", "Project name")
	cmd.Flags().String("module", "", "Go module path")
	cmd.Flags().String("out", "", "Output directory (default: the project name)")
	cmd.Flags().Bool("force", false, "Write into a non-empty directory")
	return cmd
}

func runGenerate(cmd *cobra.Command, args []string) error {
	opts := scaffolder.Options{}
	if len(args) == 1 {
		stack, err := parseStack(args[0])
		if err != nil {
			return err
		}
		opts.Stack = stack
	}
	var err error
	opts.Name, err = cmd.Flags().GetString("name")
	if err != nil {
		return err
	}
	opts.Module, err = cmd.Flags().GetString("module")
	if err != nil {
		return err
	}
	opts.OutDir, err = cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	opts.Force, err = cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	if err := completeOptions(&opts); err != nil {
		return abort(err)
	}

	start := time.Now()
	files, err := scaffolder.Generate(opts)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	writeln(out, banner(version))
	writeln(out)
	writeln(out, heading("Scaffold"))
	writeln(out)
	for _, file := range files {
		writeln(out, success(file.RelPath))
	}
	writeln(out)
	writeln(out, rule())
	writeln(out)
	writeln(out, doneLine(fmt.Sprintf("scaffolded %s project %s · %d files · %s", opts.Stack, opts.Name, len(files), formatLatency(time.Since(start)))))
	return nil
}

func completeOptions(opts *scaffolder.Options) error {
	if opts.Stack != "" && opts.Stack != scaffolder.StackGo && opts.Stack != scaffolder.StackNode {
		return fmt.Errorf("unknown stack %q", opts.Stack)
	}
	if opts.Name != "" {
		if err := scaffolder.ValidateName(opts.Name); err != nil {
			return err
		}
	}
	if opts.Module != "" {
		if err := scaffolder.ValidateModule(opts.Module); err != nil {
			return err
		}
	}
	if opts.Stack == "" || opts.Name == "" {
		if !interactive() {
			return errors.New("pass a stack and --name when stdin is not a terminal")
		}
		if err := promptIdentity(opts, opts.Stack == "", opts.Name == ""); err != nil {
			return err
		}
	}
	if opts.Stack == scaffolder.StackGo && opts.Module == "" {
		if !interactive() {
			return errors.New("go scaffolds require --module when stdin is not a terminal")
		}
		if err := promptModule(opts); err != nil {
			return err
		}
	}
	if opts.OutDir == "" {
		opts.OutDir = opts.Name
	}
	return nil
}

func promptIdentity(opts *scaffolder.Options, needStack, needName bool) error {
	stack := string(opts.Stack)
	name := opts.Name
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
	opts.Stack = scaffolder.Stack(stack)
	opts.Name = name
	return nil
}

func promptModule(opts *scaffolder.Options) error {
	module := "example.com/" + strings.ToLower(opts.Name)
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Go module path").
			Value(&module).
			Validate(scaffolder.ValidateModule),
	)).WithTheme(formTheme()).Run()
	if err != nil {
		return err
	}
	opts.Module = module
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
