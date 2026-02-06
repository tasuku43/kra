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

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	return Env{
		Root:      root,
		DataHome:  dataHome,
		CacheHome: cacheHome,
	}
}

func (e Env) StateDBPath() string {
	p, err := paths.StateDBPathForRoot(e.Root)
	if err != nil {
		panic(err)
	}
	return p
}

func (e Env) RepoPoolPath() string {
	return filepath.Join(e.CacheHome, "gionx", "repo-pool")
}

func (e Env) EnsureRootLayout(t *testing.T) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(e.Root, "workspaces"))
	mustMkdirAll(t, filepath.Join(e.Root, "archive"))
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}
