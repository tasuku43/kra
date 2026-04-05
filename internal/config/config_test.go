package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile_MissingIsEmpty(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Workspace.Defaults.Template != "" ||
		cfg.Workspace.Branch.Template != "" ||
		len(cfg.Workspace.RepoPresets) != 0 ||
		cfg.Integration.Jira.BaseURL != "" ||
		cfg.Integration.Jira.Defaults.Space != "" ||
		cfg.Integration.Jira.Defaults.Project != "" ||
		cfg.Integration.Jira.Defaults.Type != "" ||
		cfg.Integration.GitHub.Defaults.Issue.Org != "" ||
		cfg.Integration.GitHub.Defaults.Issue.Repo != "" ||
		cfg.Integration.GitHub.Defaults.Issue.State != "" ||
		cfg.Integration.GitHub.Defaults.Review.Org != "" ||
		cfg.Integration.GitHub.Defaults.Review.Repo != "" {
		t.Fatalf("LoadFile() = %+v, want zero-value config", cfg)
	}
}

func TestLoadFile_NormalizeAndValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
workspace:
  defaults:
    template: "  custom "
  branch:
    template: " feature/{{workspace_id}}/{{repo_name}} "
  repo_presets:
    " backend ":
      repos:
        - " org/api "
        - "org/web"
        - "org/api"
integration:
  jira:
    base_url: " https://jira.example.com "
    defaults:
      space: " abc "
      type: " JQL "
  github:
    defaults:
      issue:
        org: " my-org "
        state: " CLOSED "
      review:
        repo: " my-org/api "
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Workspace.Defaults.Template != "custom" {
		t.Fatalf("workspace.defaults.template = %q, want %q", cfg.Workspace.Defaults.Template, "custom")
	}
	if cfg.Workspace.Branch.Template != "feature/{{workspace_id}}/{{repo_name}}" {
		t.Fatalf("workspace.branch.template = %q, want %q", cfg.Workspace.Branch.Template, "feature/{{workspace_id}}/{{repo_name}}")
	}
	preset, ok := cfg.Workspace.RepoPresets["backend"]
	if !ok {
		t.Fatalf("workspace.repo_presets.backend missing: %+v", cfg.Workspace.RepoPresets)
	}
	if len(preset.Repos) != 2 || preset.Repos[0] != "org/api" || preset.Repos[1] != "org/web" {
		t.Fatalf("workspace.repo_presets.backend.repos = %v, want [org/api org/web]", preset.Repos)
	}
	if cfg.Integration.Jira.BaseURL != "https://jira.example.com" {
		t.Fatalf("integration.jira.base_url = %q, want %q", cfg.Integration.Jira.BaseURL, "https://jira.example.com")
	}
	if cfg.Integration.Jira.Defaults.Space != "ABC" {
		t.Fatalf("integration.jira.defaults.space = %q, want %q", cfg.Integration.Jira.Defaults.Space, "ABC")
	}
	if cfg.Integration.Jira.Defaults.Type != JiraTypeJQL {
		t.Fatalf("integration.jira.defaults.type = %q, want %q", cfg.Integration.Jira.Defaults.Type, JiraTypeJQL)
	}
	if cfg.Integration.GitHub.Defaults.Issue.Org != "my-org" {
		t.Fatalf("integration.github.defaults.issue.org = %q, want %q", cfg.Integration.GitHub.Defaults.Issue.Org, "my-org")
	}
	if cfg.Integration.GitHub.Defaults.Issue.State != GitHubStateClosed {
		t.Fatalf("integration.github.defaults.issue.state = %q, want %q", cfg.Integration.GitHub.Defaults.Issue.State, GitHubStateClosed)
	}
	if cfg.Integration.GitHub.Defaults.Review.Repo != "my-org/api" {
		t.Fatalf("integration.github.defaults.review.repo = %q, want %q", cfg.Integration.GitHub.Defaults.Review.Repo, "my-org/api")
	}
}

func TestLoadFile_InvalidTypeFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
integration:
  jira:
    defaults:
      type: board
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("LoadFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "integration.jira.defaults.type") {
		t.Fatalf("error = %q, want defaults.type hint", err)
	}
}

func TestLoadFile_SpaceProjectConflictFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
integration:
  jira:
    defaults:
      space: DEMO
      project: APP
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("LoadFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("error = %q, want conflict hint", err)
	}
}

func TestLoadFile_InvalidJiraBaseURLFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
integration:
  jira:
    base_url: jira.example.com
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("LoadFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "integration.jira.base_url") {
		t.Fatalf("error = %q, want base_url hint", err)
	}
}

func TestLoadFile_GitHubConflictFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
integration:
  github:
    defaults:
      issue:
        org: my-org
        repo: my-org/api
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("LoadFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "integration.github.defaults.issue.org and integration.github.defaults.issue.repo") {
		t.Fatalf("error = %q, want github issue conflict hint", err)
	}
}

func TestLoadFile_RepoPresetEmptyReposFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
workspace:
  repo_presets:
    backend:
      repos: []
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("LoadFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "workspace.repo_presets.backend.repos") {
		t.Fatalf("error = %q, want repo preset hint", err)
	}
}

func TestLoadFile_RepoPresetNameWithPathSeparatorFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
workspace:
  repo_presets:
    bad/name:
      repos:
        - org/api
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatalf("LoadFile() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "workspace.repo_presets.bad/name") {
		t.Fatalf("error = %q, want preset name hint", err)
	}
}

func TestMerge_RootOverridesGlobal(t *testing.T) {
	global := Config{
		Workspace: WorkspaceConfig{
			Defaults: WorkspaceDefaults{Template: "default"},
			Branch:   WorkspaceBranch{Template: "feature/{{workspace_id}}"},
			RepoPresets: map[string]WorkspaceRepoPreset{
				"base": {Repos: []string{"org/api"}},
			},
		},
		Integration: IntegrationConfig{
			Jira: JiraConfig{
				BaseURL: "https://jira.global.example.com",
				Defaults: JiraDefaults{
					Space: "TEAM",
					Type:  JiraTypeSprint,
				},
			},
		},
	}
	root := Config{
		Workspace: WorkspaceConfig{
			Defaults: WorkspaceDefaults{Template: "custom"},
			Branch:   WorkspaceBranch{Template: "bugfix/{{workspace_id}}/{{repo_name}}"},
			RepoPresets: map[string]WorkspaceRepoPreset{
				"base":   {Repos: []string{"org/api", "org/web"}},
				"mobile": {Repos: []string{"org/mobile"}},
			},
		},
		Integration: IntegrationConfig{
			Jira: JiraConfig{
				BaseURL: "https://jira.root.example.com",
				Defaults: JiraDefaults{
					Project: "APP",
					Type:    JiraTypeJQL,
				},
			},
			GitHub: GitHubConfig{
				Defaults: GitHubDefaults{
					Issue: GitHubIssueDefaults{
						Repo:  "root/api",
						State: GitHubStateClosed,
					},
					Review: GitHubReviewDefaults{
						Org: "root-org",
					},
				},
			},
		},
	}

	got := Merge(global, root)
	if got.Workspace.Defaults.Template != "custom" {
		t.Fatalf("workspace.defaults.template = %q, want %q", got.Workspace.Defaults.Template, "custom")
	}
	if got.Workspace.Branch.Template != "bugfix/{{workspace_id}}/{{repo_name}}" {
		t.Fatalf("workspace.branch.template = %q, want %q", got.Workspace.Branch.Template, "bugfix/{{workspace_id}}/{{repo_name}}")
	}
	if len(got.Workspace.RepoPresets) != 2 {
		t.Fatalf("workspace.repo_presets len=%d, want 2", len(got.Workspace.RepoPresets))
	}
	if repos := got.Workspace.RepoPresets["base"].Repos; len(repos) != 2 || repos[1] != "org/web" {
		t.Fatalf("workspace.repo_presets.base.repos = %v, want [org/api org/web]", repos)
	}
	if repos := got.Workspace.RepoPresets["mobile"].Repos; len(repos) != 1 || repos[0] != "org/mobile" {
		t.Fatalf("workspace.repo_presets.mobile.repos = %v, want [org/mobile]", repos)
	}
	if got.Integration.Jira.BaseURL != "https://jira.root.example.com" {
		t.Fatalf("integration.jira.base_url = %q, want %q", got.Integration.Jira.BaseURL, "https://jira.root.example.com")
	}
	if got.Integration.Jira.Defaults.Project != "APP" {
		t.Fatalf("integration.jira.defaults.project = %q, want %q", got.Integration.Jira.Defaults.Project, "APP")
	}
	if got.Integration.Jira.Defaults.Type != JiraTypeJQL {
		t.Fatalf("integration.jira.defaults.type = %q, want %q", got.Integration.Jira.Defaults.Type, JiraTypeJQL)
	}
	if got.Integration.GitHub.Defaults.Issue.Repo != "root/api" {
		t.Fatalf("integration.github.defaults.issue.repo = %q, want %q", got.Integration.GitHub.Defaults.Issue.Repo, "root/api")
	}
	if got.Integration.GitHub.Defaults.Issue.State != GitHubStateClosed {
		t.Fatalf("integration.github.defaults.issue.state = %q, want %q", got.Integration.GitHub.Defaults.Issue.State, GitHubStateClosed)
	}
	if got.Integration.GitHub.Defaults.Review.Org != "root-org" {
		t.Fatalf("integration.github.defaults.review.org = %q, want %q", got.Integration.GitHub.Defaults.Review.Org, "root-org")
	}
}
