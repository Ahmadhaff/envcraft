package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var composeNames = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

// Discover inspects dir and builds a project contract from the files it finds.
func Discover(dir string) (Contract, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Contract{}, err
	}
	if !info.IsDir() {
		return Contract{}, fmt.Errorf("%s is not a directory", dir)
	}

	c := Contract{
		project: true,
		Doctor: Doctor{
			Host:    "127.0.0.1",
			Timeout: 2 * time.Second,
		},
		Env: Env{
			File:  ".env",
			Vault: ".env.vault",
		},
	}

	composePath, err := findCompose(dir)
	if err != nil {
		return Contract{}, err
	}
	if composePath != "" {
		services, err := readCompose(composePath)
		if err != nil {
			return Contract{}, err
		}
		c.Doctor.Docker = true
		c.Doctor.Services = services
	}

	goVersion, goProject, err := readGoMod(filepath.Join(dir, "go.mod"))
	if err != nil {
		return Contract{}, err
	}
	if goProject {
		c.Doctor.Tools = append(c.Doctor.Tools, Tool{Name: "go", Version: goVersion})
	}

	nodeVersion, nodeProject, err := readPackageJSON(filepath.Join(dir, "package.json"))
	if err != nil {
		return Contract{}, err
	}
	if nodeProject {
		c.Doctor.Tools = append(c.Doctor.Tools, Tool{Name: "node", Version: nodeVersion})
	}

	keys, example, err := readExample(filepath.Join(dir, ".env.example"))
	if err != nil {
		return Contract{}, err
	}
	if example {
		c.Env.Example = ".env.example"
		c.Env.Required = keys
	}

	if !c.Doctor.Docker && len(c.Doctor.Tools) == 0 && len(c.Env.Required) == 0 {
		return Contract{}, ErrEmpty
	}
	return c, nil
}

func findCompose(dir string) (string, error) {
	for _, name := range composeNames {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			continue
		}
		return path, nil
	}
	return "", nil
}

func readCompose(path string) ([]Service, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Services map[string]struct {
			Ports []yaml.Node `yaml:"ports"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	var services []Service
	for _, name := range names {
		for _, node := range doc.Services[name].Ports {
			ports, err := publishedPorts(&node)
			if err != nil {
				return nil, fmt.Errorf("%s: service %s: %w", filepath.Base(path), name, err)
			}
			for _, published := range ports {
				services = append(services, Service{Name: name, Host: published.host, Port: published.port})
			}
		}
	}
	return services, nil
}

type published struct {
	host string
	port int
}

func publishedPorts(node *yaml.Node) ([]published, error) {
	if node == nil {
		return nil, nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		item, ok, err := parsePortSpec(node.Value)
		if err != nil || !ok {
			return nil, err
		}
		return []published{item}, nil
	case yaml.MappingNode:
		publishedValue := ""
		host := ""
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := node.Content[i+1].Value
			switch key {
			case "published":
				publishedValue = value
			case "host_ip":
				host = value
			}
		}
		if publishedValue == "" {
			return nil, nil
		}
		port, err := strconv.Atoi(publishedValue)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid published port %q", publishedValue)
		}
		return []published{{host: usableHost(host), port: port}}, nil
	default:
		return nil, nil
	}
}

func parsePortSpec(spec string) (published, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return published{}, false, nil
	}
	if cut, _, ok := strings.Cut(spec, "/"); ok {
		spec = cut
	}
	parts := splitPortSpec(spec)
	switch len(parts) {
	case 1:
		return published{}, false, nil
	case 2:
		port, err := strconv.Atoi(parts[0])
		if err != nil || port < 1 || port > 65535 {
			return published{}, false, nil
		}
		return published{port: port}, true, nil
	default:
		port, err := strconv.Atoi(parts[len(parts)-2])
		if err != nil || port < 1 || port > 65535 {
			return published{}, false, nil
		}
		return published{host: usableHost(parts[0]), port: port}, true, nil
	}
}

func splitPortSpec(spec string) []string {
	if strings.HasPrefix(spec, "[") {
		end := strings.Index(spec, "]")
		if end > 0 {
			host := spec[1:end]
			rest := strings.TrimPrefix(spec[end+1:], ":")
			if rest == "" {
				return []string{host}
			}
			return append([]string{host}, strings.Split(rest, ":")...)
		}
	}
	return strings.Split(spec, ":")
}

func usableHost(host string) string {
	host = strings.Trim(host, "[]")
	switch host {
	case "", "0.0.0.0", "::", "::0":
		return ""
	default:
		return host
	}
}

func readGoMod(path string) (string, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	version := ""
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == "go" && versionLooks(fields[1]) {
			version = fields[1]
			break
		}
	}
	return version, true, nil
}

func readPackageJSON(path string) (string, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var pkg struct {
		Engines struct {
			Node any `json:"node"`
		} `json:"engines"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", false, fmt.Errorf("parse package.json: %w", err)
	}
	text, _ := pkg.Engines.Node.(string)
	return firstVersion(text), true, nil
}

func readExample(path string) ([]string, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	values, err := parseEnv(string(body))
	if err != nil {
		return nil, false, err
	}
	keys := make([]string, 0, len(values))
	for _, line := range strings.Split(string(body), "\n") {
		key, _, ok := envLine(line)
		if ok {
			keys = append(keys, key)
		}
	}
	return keys, true, nil
}

func readEnvFile(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseEnv(string(body))
}

func parseEnv(text string) (map[string]string, error) {
	values := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := envLine(line)
		if !ok {
			continue
		}
		values[key] = value
	}
	return values, nil
}

func envLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if !validEnvKey(key) {
		return "", "", false
	}
	return key, unquote(strings.TrimSpace(value)), true
}

func unquote(value string) string {
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1]
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func firstVersion(text string) string {
	start := -1
	for i, r := range text {
		if r >= '0' && r <= '9' {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := start
	for end < len(text) {
		r := text[end]
		if (r < '0' || r > '9') && r != '.' {
			break
		}
		end++
	}
	return strings.Trim(text[start:end], ".")
}

func versionLooks(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}
