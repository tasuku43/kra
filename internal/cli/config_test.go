package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/config"
)

func TestCLI_LoadMergedConfig_GlobalAndRootPrecedence(t *testing.T) {
	gionxHome := setGionxHomeForTest(t)
	root := t.TempDir()

	globalPath := filepath.Join(gionxHome, "config.yaml")
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

	rootPath := filepath.Join(root, ".gionx", "config.yaml")
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

func TestCLI_LoadMergedConfig_ConflictingScopeFails(t *testing.T) {
	gionxHome := setGionxHomeForTest(t)
	root := t.TempDir()

	globalPath := filepath.Join(gionxHome, "config.yaml")
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

	rootPath := filepath.Join(root, ".gionx", "config.yaml")
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
