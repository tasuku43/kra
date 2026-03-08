package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	JiraTypeSprint = "sprint"
	JiraTypeJQL    = "jql"

	WorkspaceCloseEmptyRecordPolicyWarn                = "warn"
	WorkspaceCloseEmptyRecordPolicyRequireConfirmation = "require-confirmation"
)

type Config struct {
	Workspace   WorkspaceConfig   `yaml:"workspace"`
	Integration IntegrationConfig `yaml:"integration"`
}

type WorkspaceConfig struct {
	Defaults    WorkspaceDefaults              `yaml:"defaults"`
	Close       WorkspaceClose                 `yaml:"close"`
	Branch      WorkspaceBranch                `yaml:"branch"`
	RepoPresets map[string]WorkspaceRepoPreset `yaml:"repo_presets"`
}

type WorkspaceDefaults struct {
	Template string `yaml:"template"`
}

type WorkspaceClose struct {
	EmptyRecordPolicy string `yaml:"empty_record_policy"`
}

type WorkspaceBranch struct {
	Template string `yaml:"template"`
}

type WorkspaceRepoPreset struct {
	Repos []string `yaml:"repos"`
}

type IntegrationConfig struct {
	Jira JiraConfig `yaml:"jira"`
}

type JiraConfig struct {
	BaseURL  string       `yaml:"base_url"`
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
	c.Workspace.Close.EmptyRecordPolicy = strings.ToLower(strings.TrimSpace(c.Workspace.Close.EmptyRecordPolicy))
	c.Workspace.Branch.Template = strings.TrimSpace(c.Workspace.Branch.Template)
	c.Workspace.RepoPresets = normalizeWorkspaceRepoPresets(c.Workspace.RepoPresets)
	c.Integration.Jira.BaseURL = strings.TrimSpace(c.Integration.Jira.BaseURL)
	c.Integration.Jira.Defaults.Space = strings.ToUpper(strings.TrimSpace(c.Integration.Jira.Defaults.Space))
	c.Integration.Jira.Defaults.Project = strings.ToUpper(strings.TrimSpace(c.Integration.Jira.Defaults.Project))
	c.Integration.Jira.Defaults.Type = strings.ToLower(strings.TrimSpace(c.Integration.Jira.Defaults.Type))
}

func (c Config) Validate() error {
	issues := make([]string, 0, 3)
	if c.Integration.Jira.BaseURL != "" {
		u, err := url.Parse(c.Integration.Jira.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			issues = append(issues, "integration.jira.base_url must be an absolute URL")
		}
	}
	if c.Integration.Jira.Defaults.Type != "" &&
		c.Integration.Jira.Defaults.Type != JiraTypeSprint &&
		c.Integration.Jira.Defaults.Type != JiraTypeJQL {
		issues = append(issues, "integration.jira.defaults.type must be one of: sprint, jql")
	}
	if c.Integration.Jira.Defaults.Space != "" && c.Integration.Jira.Defaults.Project != "" {
		issues = append(issues, "integration.jira.defaults.space and integration.jira.defaults.project cannot be combined")
	}
	if c.Workspace.Close.EmptyRecordPolicy != "" &&
		c.Workspace.Close.EmptyRecordPolicy != WorkspaceCloseEmptyRecordPolicyWarn &&
		c.Workspace.Close.EmptyRecordPolicy != WorkspaceCloseEmptyRecordPolicyRequireConfirmation {
		issues = append(issues, "workspace.close.empty_record_policy must be one of: warn, require-confirmation")
	}
	for name, preset := range c.Workspace.RepoPresets {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			issues = append(issues, "workspace.repo_presets key must not be empty")
			continue
		}
		if strings.ContainsAny(trimmedName, "/\\") {
			issues = append(issues, fmt.Sprintf("workspace.repo_presets.%s must not contain path separators", trimmedName))
		}
		if len(preset.Repos) == 0 {
			issues = append(issues, fmt.Sprintf("workspace.repo_presets.%s.repos must not be empty", trimmedName))
		}
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
	if root.Workspace.Close.EmptyRecordPolicy != "" {
		out.Workspace.Close.EmptyRecordPolicy = root.Workspace.Close.EmptyRecordPolicy
	}
	if root.Workspace.Branch.Template != "" {
		out.Workspace.Branch.Template = root.Workspace.Branch.Template
	}
	if len(root.Workspace.RepoPresets) > 0 {
		if out.Workspace.RepoPresets == nil {
			out.Workspace.RepoPresets = map[string]WorkspaceRepoPreset{}
		}
		for name, preset := range root.Workspace.RepoPresets {
			out.Workspace.RepoPresets[name] = preset
		}
	}
	if root.Integration.Jira.BaseURL != "" {
		out.Integration.Jira.BaseURL = root.Integration.Jira.BaseURL
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

func normalizeWorkspaceRepoPresets(presets map[string]WorkspaceRepoPreset) map[string]WorkspaceRepoPreset {
	if len(presets) == 0 {
		return nil
	}
	out := make(map[string]WorkspaceRepoPreset, len(presets))
	for rawName, preset := range presets {
		name := strings.TrimSpace(rawName)
		repos := normalizeWorkspaceRepoPresetRepos(preset.Repos)
		out[name] = WorkspaceRepoPreset{Repos: repos}
	}
	return out
}

func normalizeWorkspaceRepoPresetRepos(repos []string) []string {
	if len(repos) == 0 {
		return nil
	}
	out := make([]string, 0, len(repos))
	seen := map[string]bool{}
	for _, raw := range repos {
		repoKey := strings.TrimSpace(raw)
		if repoKey == "" || seen[repoKey] {
			continue
		}
		seen[repoKey] = true
		out = append(out, repoKey)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
