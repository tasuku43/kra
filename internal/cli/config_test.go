package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/config"
)

func TestCLI_LoadMergedConfig_GlobalAndRootPrecedence(t *testing.T) {
	kraHome := setKraHomeForTest(t)
	root := t.TempDir()

	globalPath := filepath.Join(kraHome, "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(globalPath, []byte(`
workspace:
  defaults:
    template: global
integration:
  jira:
    defaults:
      space: global-space
      type: sprint
`), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	rootPath := filepath.Join(root, ".kra", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(rootPath), 0o755); err != nil {
		t.Fatalf("mkdir root config dir: %v", err)
	}
	if err := os.WriteFile(rootPath, []byte(`
workspace:
  defaults:
    template: root
integration:
  jira:
    defaults:
      type: jql
`), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	c := New(&bytes.Buffer{}, &bytes.Buffer{})
	cfg, err := c.loadMergedConfig(root)
	if err != nil {
		t.Fatalf("loadMergedConfig() error = %v", err)
	}
	if cfg.Workspace.Defaults.Template != "root" {
		t.Fatalf("workspace.defaults.template = %q, want %q", cfg.Workspace.Defaults.Template, "root")
	}
	if cfg.Integration.Jira.Defaults.Space != "GLOBAL-SPACE" {
		t.Fatalf("integration.jira.defaults.space = %q, want %q", cfg.Integration.Jira.Defaults.Space, "GLOBAL-SPACE")
	}
	if cfg.Integration.Jira.Defaults.Type != config.JiraTypeJQL {
		t.Fatalf("integration.jira.defaults.type = %q, want %q", cfg.Integration.Jira.Defaults.Type, config.JiraTypeJQL)
	}
}

func TestCLI_LoadMergedConfig_WorkspaceCloseEmptyRecordPolicyPrecedence(t *testing.T) {
	kraHome := setKraHomeForTest(t)
	root := t.TempDir()

	globalPath := filepath.Join(kraHome, "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(globalPath, []byte(`
workspace:
  close:
    empty_record_policy: warn
`), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	rootPath := filepath.Join(root, ".kra", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(rootPath), 0o755); err != nil {
		t.Fatalf("mkdir root config dir: %v", err)
	}
	if err := os.WriteFile(rootPath, []byte(`
workspace:
  close:
    empty_record_policy: require-confirmation
`), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	c := New(&bytes.Buffer{}, &bytes.Buffer{})
	cfg, err := c.loadMergedConfig(root)
	if err != nil {
		t.Fatalf("loadMergedConfig() error = %v", err)
	}
	if cfg.Workspace.Close.EmptyRecordPolicy != config.WorkspaceCloseEmptyRecordPolicyRequireConfirmation {
		t.Fatalf("workspace.close.empty_record_policy = %q, want %q", cfg.Workspace.Close.EmptyRecordPolicy, config.WorkspaceCloseEmptyRecordPolicyRequireConfirmation)
	}
}

func TestCLI_LoadMergedConfig_ConflictingScopeFails(t *testing.T) {
	kraHome := setKraHomeForTest(t)
	root := t.TempDir()

	globalPath := filepath.Join(kraHome, "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(globalPath, []byte(`
integration:
  jira:
    defaults:
      space: team
`), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	rootPath := filepath.Join(root, ".kra", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(rootPath), 0o755); err != nil {
		t.Fatalf("mkdir root config dir: %v", err)
	}
	if err := os.WriteFile(rootPath, []byte(`
integration:
  jira:
    defaults:
      project: app
`), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	c := New(&bytes.Buffer{}, &bytes.Buffer{})
	_, err := c.loadMergedConfig(root)
	if err == nil {
		t.Fatalf("loadMergedConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("error = %q, want conflict hint", err)
	}
}

func TestCLI_LoadMergedConfig_InvalidEmptyRecordPolicyFails(t *testing.T) {
	kraHome := setKraHomeForTest(t)
	root := t.TempDir()

	globalPath := filepath.Join(kraHome, "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(globalPath, []byte(`
workspace:
  close:
    empty_record_policy: nope
`), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	c := New(&bytes.Buffer{}, &bytes.Buffer{})
	_, err := c.loadMergedConfig(root)
	if err == nil {
		t.Fatalf("loadMergedConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "workspace.close.empty_record_policy") {
		t.Fatalf("error = %q, want empty_record_policy validation hint", err)
	}
}
