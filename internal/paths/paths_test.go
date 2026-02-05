package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultStateDBPath_UsesXDGDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	got, err := DefaultStateDBPath()
	if err != nil {
		t.Fatalf("DefaultStateDBPath() err = %v", err)
	}
	want := filepath.Join(home, ".local", "share", "gionx", "state.db")
	if got != want {
		t.Fatalf("DefaultStateDBPath() = %q, want %q", got, want)
	}
}

func TestDefaultRepoPoolPath_UsesXDGDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")

	got, err := DefaultRepoPoolPath()
	if err != nil {
		t.Fatalf("DefaultRepoPoolPath() err = %v", err)
	}
	want := filepath.Join(home, ".cache", "gionx", "repo-pool")
	if got != want {
		t.Fatalf("DefaultRepoPoolPath() = %q, want %q", got, want)
	}
}

func TestDefaultPaths_UseXDGOverrides(t *testing.T) {
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	gotDB, err := DefaultStateDBPath()
	if err != nil {
		t.Fatalf("DefaultStateDBPath() err = %v", err)
	}
	if gotDB != filepath.Join(dataHome, "gionx", "state.db") {
		t.Fatalf("state db path = %q", gotDB)
	}

	gotPool, err := DefaultRepoPoolPath()
	if err != nil {
		t.Fatalf("DefaultRepoPoolPath() err = %v", err)
	}
	if gotPool != filepath.Join(cacheHome, "gionx", "repo-pool") {
		t.Fatalf("repo pool path = %q", gotPool)
	}
}

func TestFindRoot_FindsNearestRoot(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "workspaces"))
	mustMkdirAll(t, filepath.Join(root, "archive"))

	start := filepath.Join(root, "workspaces", "W1", "notes")
	mustMkdirAll(t, start)

	got, err := FindRoot(start)
	if err != nil {
		t.Fatalf("FindRoot() err = %v", err)
	}
	if got != root {
		t.Fatalf("FindRoot() = %q, want %q", got, root)
	}
}

func TestFindRoot_NotFound(t *testing.T) {
	start := t.TempDir()
	_, err := FindRoot(start)
	if err == nil {
		t.Fatalf("FindRoot() err = nil, want error")
	}
}

func TestResolveExistingRoot_UsesEnv(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "workspaces"))
	mustMkdirAll(t, filepath.Join(root, "archive"))
	t.Setenv("GIONX_ROOT", root)

	other := t.TempDir()
	got, err := ResolveExistingRoot(other)
	if err != nil {
		t.Fatalf("ResolveExistingRoot() err = %v", err)
	}
	if got != root {
		t.Fatalf("ResolveExistingRoot() = %q, want %q", got, root)
	}
}

func TestResolveExistingRoot_EnvMustLookLikeRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIONX_ROOT", root)

	_, err := ResolveExistingRoot(t.TempDir())
	if err == nil {
		t.Fatalf("ResolveExistingRoot() err = nil, want error")
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}
