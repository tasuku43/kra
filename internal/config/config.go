package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	JiraTypeSprint = "sprint"
	JiraTypeJQL    = "jql"
)

type Config struct {
	Workspace   WorkspaceConfig   `yaml:"workspace"`
	Integration IntegrationConfig `yaml:"integration"`
}

type WorkspaceConfig struct {
	Defaults WorkspaceDefaults `yaml:"defaults"`
}

type WorkspaceDefaults struct {
	Template string `yaml:"template"`
}

type IntegrationConfig struct {
	Jira JiraConfig `yaml:"jira"`
}

type JiraConfig struct {
	Defaults JiraDefaults `yaml:"defaults"`
}

type JiraDefaults struct {
	Space   string `yaml:"space"`
	Project string `yaml:"project"`
	Type    string `yaml:"type"`
}

func LoadFile(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config file: %w", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return Config{}, nil
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) Normalize() {
	c.Workspace.Defaults.Template = strings.TrimSpace(c.Workspace.Defaults.Template)
	c.Integration.Jira.Defaults.Space = strings.ToUpper(strings.TrimSpace(c.Integration.Jira.Defaults.Space))
	c.Integration.Jira.Defaults.Project = strings.ToUpper(strings.TrimSpace(c.Integration.Jira.Defaults.Project))
	c.Integration.Jira.Defaults.Type = strings.ToLower(strings.TrimSpace(c.Integration.Jira.Defaults.Type))
}

func (c Config) Validate() error {
	issues := make([]string, 0, 2)
	if c.Integration.Jira.Defaults.Type != "" &&
		c.Integration.Jira.Defaults.Type != JiraTypeSprint &&
		c.Integration.Jira.Defaults.Type != JiraTypeJQL {
		issues = append(issues, "integration.jira.defaults.type must be one of: sprint, jql")
	}
	if c.Integration.Jira.Defaults.Space != "" && c.Integration.Jira.Defaults.Project != "" {
		issues = append(issues, "integration.jira.defaults.space and integration.jira.defaults.project cannot be combined")
	}
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("invalid config: %s", strings.Join(issues, "; "))
}

func Merge(global Config, root Config) Config {
	global.Normalize()
	root.Normalize()

	out := global
	if root.Workspace.Defaults.Template != "" {
		out.Workspace.Defaults.Template = root.Workspace.Defaults.Template
	}
	if root.Integration.Jira.Defaults.Space != "" {
		out.Integration.Jira.Defaults.Space = root.Integration.Jira.Defaults.Space
	}
	if root.Integration.Jira.Defaults.Project != "" {
		out.Integration.Jira.Defaults.Project = root.Integration.Jira.Defaults.Project
	}
	if root.Integration.Jira.Defaults.Type != "" {
		out.Integration.Jira.Defaults.Type = root.Integration.Jira.Defaults.Type
	}
	out.Normalize()
	return out
}
