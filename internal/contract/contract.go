// Package contract reads a project directory and turns it into the checks
// envcraft doctor runs.
package contract

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ahmadhaff/envcraft/internal/checker"
	"gopkg.in/yaml.v3"
)

const fileName = "envcraft.yaml"

// ErrExists is returned when a contract file is already present.
var ErrExists = errors.New("envcraft.yaml already exists")

// ErrEmpty is returned when a directory has nothing to describe.
var ErrEmpty = errors.New("no compose file, go.mod, package.json, or .env.example")

// ErrNoChecks is returned when a project contract declares no probes.
var ErrNoChecks = errors.New("contract has no checks")

// Contract is the project file doctor understands.
type Contract struct {
	project bool
	Doctor  Doctor
	Env     Env
}

// Doctor is the stack half of a contract.
type Doctor struct {
	Host     string
	Timeout  time.Duration
	Docker   bool
	Services []Service
	Tools    []Tool
}

// Service is one published port.
type Service struct {
	Name string
	Host string
	Port int
}

// Tool is a toolchain the project asks for.
type Tool struct {
	Name    string
	Version string
}

// Env is the file half of a contract.
type Env struct {
	File     string
	Vault    string
	Example  string
	Required []string
}

// Project reports whether this file replaces the built-in doctor list.
func (c Contract) Project() bool {
	return c.project || len(c.Doctor.Services) > 0 || len(c.Doctor.Tools) > 0 || len(c.Env.Required) > 0
}

// Load reads a contract file. Files that only set the legacy doctor ports
// load successfully and report Project false.
func Load(path string) (Contract, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, err
	}
	var raw fileDocument
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return Contract{}, fmt.Errorf("parse %s: %w", path, err)
	}
	c := raw.contract()
	if err := c.validate(); err != nil {
		return Contract{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Probes builds the checks for a project contract. root is the directory
// that contains the env files. host is the dial host for services that do
// not publish on a specific address.
func (c Contract) Probes(root, host string) ([]checker.Probe, error) {
	if !c.Project() {
		return nil, errors.New("not a project contract")
	}
	if host == "" {
		host = c.Doctor.Host
	}
	if host == "" {
		host = "127.0.0.1"
	}
	var probes []checker.Probe
	if c.Doctor.Docker {
		probes = append(probes, checker.Docker())
	}
	for _, service := range c.Doctor.Services {
		dialHost := service.Host
		if dialHost == "" {
			dialHost = host
		}
		name := serviceLabel(service.Name, service.Port)
		id := "svc-" + slug(service.Name) + "-" + fmt.Sprint(service.Port)
		probes = append(probes, checker.Port(id, name, dialHost, service.Port))
	}
	for _, tool := range c.Doctor.Tools {
		switch tool.Name {
		case "go":
			probes = append(probes, checker.GoAtLeast(tool.Version))
		case "node":
			probes = append(probes, checker.NodeAtLeast(tool.Version))
		default:
			return nil, fmt.Errorf("unknown tool %q", tool.Name)
		}
	}
	if len(c.Env.Required) > 0 {
		probes = append(probes, envProbe(root, c, host))
	}
	if len(probes) == 0 {
		return nil, ErrNoChecks
	}
	return probes, nil
}

func (c Contract) validate() error {
	for _, service := range c.Doctor.Services {
		if service.Name == "" {
			return errors.New("service name is required")
		}
		if service.Port < 1 || service.Port > 65535 {
			return fmt.Errorf("%s port must be between 1 and 65535", service.Name)
		}
	}
	for _, tool := range c.Doctor.Tools {
		switch tool.Name {
		case "go", "node":
		default:
			return fmt.Errorf("unknown tool %q", tool.Name)
		}
	}
	for _, key := range c.Env.Required {
		if !validEnvKey(key) {
			return fmt.Errorf("invalid env key %q", key)
		}
	}
	return nil
}

func serviceLabel(name string, port int) string {
	label := name
	switch strings.ToLower(name) {
	case "postgres", "postgresql":
		label = "PostgreSQL"
	case "redis":
		label = "Redis"
	case "mysql":
		label = "MySQL"
	case "mongo", "mongodb":
		label = "MongoDB"
	}
	return fmt.Sprintf("%s :%d", label, port)
}

func slug(name string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, name)
	out = strings.Trim(out, "-")
	if out == "" {
		return "service"
	}
	return out
}

func envProbe(root string, c Contract, host string) checker.Probe {
	file := c.Env.File
	if file == "" {
		file = ".env"
	}
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, file)
	}
	return checker.Probe{
		ID:     "env",
		Name:   "Environment",
		Target: file,
		Run: func(ctx context.Context) checker.Result {
			start := time.Now()
			res := checker.Result{ID: "env", Name: "Environment", Target: file}
			values, err := readEnvFile(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					res.Detail = "missing " + file
				} else {
					res.Detail = err.Error()
				}
				res.Duration = time.Since(start)
				return res
			}
			var missing []string
			var present []string
			for _, key := range c.Env.Required {
				value, ok := values[key]
				if !ok || strings.TrimSpace(value) == "" {
					missing = append(missing, key)
					continue
				}
				present = append(present, key+"="+value)
			}
			var refused string
			for _, item := range present {
				key, value, _ := strings.Cut(item, "=")
				dialHost, port, local := localEndpoint(value, host)
				if !local {
					continue
				}
				if _, err := checker.Dial(ctx, dialHost, port); err != nil {
					refused = key + " " + dialDetail(err)
					break
				}
			}
			res.Duration = time.Since(start)
			switch {
			case len(missing) == 1 && refused != "":
				res.Detail = "missing " + missing[0] + ", " + refused
			case len(missing) == 1:
				res.Detail = "missing " + missing[0]
			case len(missing) > 1:
				res.Detail = fmt.Sprintf("missing %d keys", len(missing))
			case refused != "":
				res.Detail = refused
			default:
				res.OK = true
				res.Detail = "required keys set"
			}
			return res
		},
	}
}

func dialDetail(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timed out"
	}
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "refused"):
		return "connection refused"
	case strings.Contains(low, "timeout"), strings.Contains(low, "timed out"):
		return "timed out"
	default:
		return "unreachable"
	}
}

func localEndpoint(value, fallbackHost string) (string, int, bool) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", 0, false
	}
	portText := parsed.Port()
	if portText == "" {
		return "", 0, false
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	host := parsed.Hostname()
	if !isLocalHost(host, fallbackHost) {
		return "", 0, false
	}
	return host, port, true
}

func isLocalHost(host, fallback string) bool {
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return fallback != "" && strings.EqualFold(host, fallback)
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
