package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/gionx/internal/paths"
)

type Env struct {
	Root      string
	GionxHome string
}

func NewEnv(t *testing.T) Env {
	t.Helper()

	root := t.TempDir()
	gionxHome := filepath.Join(t.TempDir(), ".gionx")

	t.Setenv("GIONX_HOME", gionxHome)
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext(%q): %v", root, err)
	}

	return Env{
		Root:      root,
		GionxHome: gionxHome,
	}
}

func (e Env) RepoPoolPath() string {
	return filepath.Join(e.GionxHome, "repo-pool")
}

func (e Env) EnsureRootLayout(t *testing.T) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(e.Root, "workspaces"))
	mustMkdirAll(t, filepath.Join(e.Root, "archive"))
	mustMkdirAll(t, filepath.Join(e.Root, "templates", "default", "notes"))
	mustMkdirAll(t, filepath.Join(e.Root, "templates", "default", "artifacts"))
	agentsPath := filepath.Join(e.Root, "templates", "default", "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("# test template\n"), 0o644); err != nil {
		t.Fatalf("write %q: %v", agentsPath, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}
