package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/gionx/internal/paths"
)

type Env struct {
	Root      string
	DataHome  string
	CacheHome string
}

func NewEnv(t *testing.T) Env {
	t.Helper()

	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext(%q): %v", root, err)
	}

	return Env{
		Root:      root,
		DataHome:  dataHome,
		CacheHome: cacheHome,
	}
}

func (e Env) RepoPoolPath() string {
	return filepath.Join(e.CacheHome, "gionx", "repo-pool")
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
