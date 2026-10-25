package contract

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type fileDocument struct {
	Doctor doctorDocument `yaml:"doctor"`
	Env    envDocument    `yaml:"env"`
}

type doctorDocument struct {
	Host     string            `yaml:"host"`
	Timeout  string            `yaml:"timeout"`
	Docker   *bool             `yaml:"docker"`
	Services []serviceDocument `yaml:"services"`
	Tools    []toolDocument    `yaml:"tools"`
}

type serviceDocument struct {
	Name string `yaml:"name"`
	Host string `yaml:"host,omitempty"`
	Port int    `yaml:"port"`
}

type toolDocument struct {
	Name    string       `yaml:"name"`
	Version versionValue `yaml:"version,omitempty"`
}

type envDocument struct {
	File     string   `yaml:"file"`
	Vault    string   `yaml:"vault"`
	Example  string   `yaml:"example,omitempty"`
	Required []string `yaml:"required,omitempty"`
}

type versionValue string

func (v *versionValue) UnmarshalYAML(node *yaml.Node) error {
	var text string
	if err := node.Decode(&text); err == nil {
		*v = versionValue(text)
		return nil
	}
	var number float64
	if err := node.Decode(&number); err != nil {
		return err
	}
	*v = versionValue(fmt.Sprint(number))
	return nil
}

func (raw fileDocument) contract() Contract {
	c := Contract{
		project: raw.Doctor.Docker != nil || len(raw.Doctor.Services) > 0 || len(raw.Doctor.Tools) > 0 || len(raw.Env.Required) > 0,
		Doctor: Doctor{
			Host:    raw.Doctor.Host,
			Timeout: parseTimeout(raw.Doctor.Timeout),
		},
		Env: Env{
			File:     raw.Env.File,
			Vault:    raw.Env.Vault,
			Example:  raw.Env.Example,
			Required: raw.Env.Required,
		},
	}
	if raw.Doctor.Docker != nil {
		c.Doctor.Docker = *raw.Doctor.Docker
	}
	if c.Doctor.Host == "" {
		c.Doctor.Host = "127.0.0.1"
	}
	if c.Env.File == "" {
		c.Env.File = ".env"
	}
	if c.Env.Vault == "" {
		c.Env.Vault = ".env.vault"
	}
	for _, service := range raw.Doctor.Services {
		c.Doctor.Services = append(c.Doctor.Services, Service(service))
	}
	for _, tool := range raw.Doctor.Tools {
		c.Doctor.Tools = append(c.Doctor.Tools, Tool{
			Name:    tool.Name,
			Version: string(tool.Version),
		})
	}
	return c
}

func parseTimeout(text string) time.Duration {
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return 2 * time.Second
	}
	return parsed
}

// YAML renders the contract in a stable order.
func (c Contract) YAML() []byte {
	var b bytes.Buffer
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&b, format, args...)
	}
	write("# Project contract written by envcraft init.\n")
	write("# Passphrases are never read from this file.\n\n")
	write("doctor:\n")
	host := c.Doctor.Host
	if host == "" {
		host = "127.0.0.1"
	}
	timeout := c.Doctor.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	write("  host: %s\n", host)
	write("  timeout: %s\n", timeout)
	write("  docker: %t\n", c.Doctor.Docker)
	if len(c.Doctor.Services) > 0 {
		write("  services:\n")
		for _, service := range c.Doctor.Services {
			write("    - name: %s\n", service.Name)
			if service.Host != "" {
				write("      host: %s\n", service.Host)
			}
			write("      port: %d\n", service.Port)
		}
	}
	if len(c.Doctor.Tools) > 0 {
		write("  tools:\n")
		for _, tool := range c.Doctor.Tools {
			write("    - name: %s\n", tool.Name)
			if tool.Version != "" {
				write("      version: %q\n", tool.Version)
			}
		}
	}
	write("\nenv:\n")
	file := c.Env.File
	if file == "" {
		file = ".env"
	}
	vault := c.Env.Vault
	if vault == "" {
		vault = ".env.vault"
	}
	write("  file: %s\n", file)
	write("  vault: %s\n", vault)
	if c.Env.Example != "" {
		write("  example: %s\n", c.Env.Example)
	}
	if len(c.Env.Required) > 0 {
		write("  required:\n")
		for _, key := range c.Env.Required {
			write("    - %s\n", key)
		}
	}
	return b.Bytes()
}

// Write stores the contract as envcraft.yaml inside dir.
func Write(dir string, c Contract, force bool) (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileName)
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("%w: %s", ErrExists, path)
		}
		return "", err
	}
	if _, err := file.Write(c.YAML()); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return path, nil
}
