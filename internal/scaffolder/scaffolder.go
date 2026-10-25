// Package scaffolder writes Clean Architecture projects from embedded templates.
package scaffolder

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/ahmadhaffoudhi/envcraft/templates"
)

// Stack selects an embedded project template.
type Stack string

const (
	StackGo   Stack = "go"
	StackNode Stack = "node"
)

// ErrNotEmpty is returned when the output directory already has files.
var ErrNotEmpty = errors.New("output directory is not empty")

// Options controls one scaffold.
type Options struct {
	Stack  Stack
	Name   string
	Module string
	OutDir string
	Force  bool
}

// File is one rendered path, relative to OutDir.
type File struct {
	RelPath string
	Bytes   int
}

type templateData struct {
	Name   string
	Module string
}

// ValidateName accepts project directory names.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("project name is required")
	}
	if len(name) > 64 {
		return errors.New("project name is too long")
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'):
		default:
			return errors.New("project name must start with a letter and use letters, digits, '-', '_' or '.'")
		}
	}
	return nil
}

// ValidateModule accepts a Go module path used inside generated imports.
func ValidateModule(module string) error {
	if module == "" {
		return errors.New("module path is required")
	}
	if strings.Contains(module, "..") || strings.HasPrefix(module, "/") || strings.HasSuffix(module, "/") {
		return errors.New("invalid module path")
	}
	for _, r := range module {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '/' || r == '~':
		default:
			return errors.New("module path contains unsupported characters")
		}
	}
	return nil
}

// Generate renders the selected stack into OutDir.
func Generate(opts Options) ([]File, error) {
	if opts.Stack != StackGo && opts.Stack != StackNode {
		return nil, fmt.Errorf("unknown stack %q", opts.Stack)
	}
	if err := ValidateName(opts.Name); err != nil {
		return nil, err
	}
	if opts.Stack == StackGo {
		if err := ValidateModule(opts.Module); err != nil {
			return nil, err
		}
	}
	if opts.OutDir == "" {
		return nil, errors.New("output directory is required")
	}
	if err := ensureOut(opts.OutDir, opts.Force); err != nil {
		return nil, err
	}

	rendered, err := render(opts)
	if err != nil {
		return nil, err
	}
	var written []File
	for _, file := range rendered {
		full, err := safeJoin(opts.OutDir, file.rel)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, file.body, 0o644); err != nil {
			return nil, err
		}
		written = append(written, File{RelPath: file.rel, Bytes: len(file.body)})
	}
	sort.Slice(written, func(i, j int) bool { return written[i].RelPath < written[j].RelPath })
	return written, nil
}

type renderedFile struct {
	rel  string
	body []byte
}

func render(opts Options) ([]renderedFile, error) {
	root := string(opts.Stack)
	data := templateData{Name: opts.Name, Module: opts.Module}
	var rendered []renderedFile
	err := fs.WalkDir(templates.FS, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, root+"/")
		rel = strings.TrimSuffix(rel, ".tmpl")
		rel = dotify(rel)
		body, err := templates.FS.ReadFile(path)
		if err != nil {
			return err
		}
		tmpl, err := template.New(rel).Option("missingkey=error").Parse(string(body))
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return fmt.Errorf("render %s: %w", path, err)
		}
		rendered = append(rendered, renderedFile{rel: filepath.FromSlash(rel), body: buf.Bytes()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(rendered) == 0 {
		return nil, fmt.Errorf("no templates for %s", opts.Stack)
	}
	return rendered, nil
}

func dotify(rel string) string {
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	if base == "gitignore" {
		return strings.TrimSuffix(rel, "gitignore") + ".gitignore"
	}
	return rel
}

func ensureOut(dir string, force bool) error {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is a file: %s", dir)
	}
	if force {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %s", ErrNotEmpty, dir)
	}
	return nil
}

func safeJoin(root, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid template path %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid template path %q", rel)
	}
	full := filepath.Join(root, clean)
	relCheck, err := filepath.Rel(root, full)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes output dir: %s", rel)
	}
	return full, nil
}
