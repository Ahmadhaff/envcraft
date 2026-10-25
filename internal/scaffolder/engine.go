package scaffolder

import (
	"embed"
	"fmt"
)

// templateFS is the compiled Clean Architecture catalog.
// templates/* embeds the go and node directory trees.
//
//go:embed templates/*
var templateFS embed.FS

// Generate renders the selected stack into cfg.OutDir.
// Go templates receive ProjectName and ModulePath. Node templates receive ProjectName.
func Generate(cfg ScaffoldConfig) (Result, error) {
	switch cfg.Stack {
	case StackGo, StackNode:
	default:
		return Result{}, fmt.Errorf("unknown stack %q", cfg.Stack)
	}
	if err := ValidateName(cfg.ProjectName); err != nil {
		return Result{}, err
	}
	if cfg.Stack == StackGo {
		if err := ValidateModule(cfg.ModulePath); err != nil {
			return Result{}, err
		}
	}
	if cfg.OutDir == "" {
		return Result{}, fmt.Errorf("output directory is required")
	}
	if err := ensureOut(cfg.OutDir, cfg.Force); err != nil {
		return Result{}, err
	}

	rendered, err := renderTree("templates/"+string(cfg.Stack), templateData{
		ProjectName: cfg.ProjectName,
		ModulePath:  cfg.ModulePath,
	})
	if err != nil {
		return Result{}, err
	}
	files, err := writeAll(cfg.OutDir, rendered)
	if err != nil {
		return Result{}, err
	}
	return Result{Files: files}, nil
}
