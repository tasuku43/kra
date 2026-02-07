package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDBPathForRoot_UsesXDGDefaults(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "gionroot")
	mustMkdirAll(t, root)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	got, err := StateDBPathForRoot(root)
	if err != nil {
		t.Fatalf("StateDBPathForRoot() err = %v", err)
	}
	prefix := filepath.Join(home, ".local", "share", "gionx", "roots")
	if filepath.Dir(filepath.Dir(got)) != prefix {
		t.Fatalf("state db parent = %q, want under %q", filepath.Dir(filepath.Dir(got)), prefix)
	}
	if filepath.Base(got) != "state.db" {
		t.Fatalf("state db filename = %q, want %q", filepath.Base(got), "state.db")
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

func TestRegistryPath_UsesXDGDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	got, err := RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath() err = %v", err)
	}
	want := filepath.Join(home, ".local", "share", "gionx", "registry.json")
	if got != want {
		t.Fatalf("RegistryPath() = %q, want %q", got, want)
	}
}

func TestStateDBPathForRoot_UsesXDGOverridesAndDiffersByRoot(t *testing.T) {
	root1 := filepath.Join(t.TempDir(), "gionroot1")
	root2 := filepath.Join(t.TempDir(), "gionroot2")
	mustMkdirAll(t, root1)
	mustMkdirAll(t, root2)

	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	gotDB1, err := StateDBPathForRoot(root1)
	if err != nil {
		t.Fatalf("StateDBPathForRoot(root1) err = %v", err)
	}
	gotDB2, err := StateDBPathForRoot(root2)
	if err != nil {
		t.Fatalf("StateDBPathForRoot(root2) err = %v", err)
	}
	if gotDB1 == gotDB2 {
		t.Fatalf("state db path should differ by root, got %q", gotDB1)
	}
	if filepath.Dir(filepath.Dir(gotDB1)) != filepath.Join(dataHome, "gionx", "roots") {
		t.Fatalf("state db path = %q", gotDB1)
	}
	if filepath.Dir(filepath.Dir(gotDB2)) != filepath.Join(dataHome, "gionx", "roots") {
		t.Fatalf("state db path = %q", gotDB2)
	}

	gotPool, err := DefaultRepoPoolPath()
	if err != nil {
		t.Fatalf("DefaultRepoPoolPath() err = %v", err)
	}
	if gotPool != filepath.Join(cacheHome, "gionx", "repo-pool") {
		t.Fatalf("repo pool path = %q", gotPool)
	}

	gotRegistry, err := RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath() err = %v", err)
	}
	if gotRegistry != filepath.Join(dataHome, "gionx", "registry.json") {
		t.Fatalf("registry path = %q", gotRegistry)
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
