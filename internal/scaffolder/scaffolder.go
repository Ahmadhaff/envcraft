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
)

// Stack selects an embedded project template.
type Stack string

const (
	StackGo   Stack = "go"
	StackNode Stack = "node"
)

// ErrNotEmpty is returned when the output directory already has files.
var ErrNotEmpty = errors.New("output directory is not empty")

// ScaffoldConfig is the input for one generated project.
type ScaffoldConfig struct {
	Stack       Stack
	ProjectName string
	ModulePath  string
	OutDir      string
	Force       bool
}

// File is one rendered path, relative to OutDir.
type File struct {
	RelPath string
	Bytes   int
}

// Result is the set of files written by Generate.
type Result struct {
	Files []File
}

type templateData struct {
	ProjectName string
	ModulePath  string
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

func dotify(rel string) string {
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	switch base {
	case "gitignore":
		return strings.TrimSuffix(rel, "gitignore") + ".gitignore"
	case "env.example":
		return strings.TrimSuffix(rel, "env.example") + ".env.example"
	}
	return rel
}

func writeSecure(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".scaffold-*")
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
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmpName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o644); err != nil {
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

func renderTree(root string, data templateData) ([]renderedFile, error) {
	var rendered []renderedFile
	err := fs.WalkDir(templateFS, root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, root+"/")
		rel = strings.TrimSuffix(rel, ".tmpl")
		rel = dotify(rel)
		body, err := templateFS.ReadFile(path)
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
		return nil, fmt.Errorf("no templates for %s", root)
	}
	return rendered, nil
}

type renderedFile struct {
	rel  string
	body []byte
}

func writeAll(outDir string, rendered []renderedFile) ([]File, error) {
	var written []File
	for _, file := range rendered {
		full, err := safeJoin(outDir, file.rel)
		if err != nil {
			return nil, err
		}
		if err := writeSecure(full, file.body); err != nil {
			return nil, err
		}
		written = append(written, File{RelPath: file.rel, Bytes: len(file.body)})
	}
	sort.Slice(written, func(i, j int) bool { return written[i].RelPath < written[j].RelPath })
	return written, nil
}
