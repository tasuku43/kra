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
	if cfg != (Config{}) {
		t.Fatalf("LoadFile() = %+v, want zero", cfg)
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
integration:
  jira:
    base_url: " https://jira.example.com "
    defaults:
      space: " abc "
      type: " JQL "
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
	if cfg.Integration.Jira.BaseURL != "https://jira.example.com" {
		t.Fatalf("integration.jira.base_url = %q, want %q", cfg.Integration.Jira.BaseURL, "https://jira.example.com")
	}
	if cfg.Integration.Jira.Defaults.Space != "ABC" {
		t.Fatalf("integration.jira.defaults.space = %q, want %q", cfg.Integration.Jira.Defaults.Space, "ABC")
	}
	if cfg.Integration.Jira.Defaults.Type != JiraTypeJQL {
		t.Fatalf("integration.jira.defaults.type = %q, want %q", cfg.Integration.Jira.Defaults.Type, JiraTypeJQL)
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
      space: SRE
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

func TestMerge_RootOverridesGlobal(t *testing.T) {
	global := Config{
		Workspace: WorkspaceConfig{
			Defaults: WorkspaceDefaults{Template: "default"},
			Branch:   WorkspaceBranch{Template: "feature/{{workspace_id}}"},
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
		},
		Integration: IntegrationConfig{
			Jira: JiraConfig{
				BaseURL: "https://jira.root.example.com",
				Defaults: JiraDefaults{
					Project: "APP",
					Type:    JiraTypeJQL,
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
	if got.Integration.Jira.BaseURL != "https://jira.root.example.com" {
		t.Fatalf("integration.jira.base_url = %q, want %q", got.Integration.Jira.BaseURL, "https://jira.root.example.com")
	}
	if got.Integration.Jira.Defaults.Project != "APP" {
		t.Fatalf("integration.jira.defaults.project = %q, want %q", got.Integration.Jira.Defaults.Project, "APP")
	}
	if got.Integration.Jira.Defaults.Type != JiraTypeJQL {
		t.Fatalf("integration.jira.defaults.type = %q, want %q", got.Integration.Jira.Defaults.Type, JiraTypeJQL)
	}
}
