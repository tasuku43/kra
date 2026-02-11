package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRepoPoolPath_UsesGionxHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIONX_HOME", "")

	got, err := DefaultRepoPoolPath()
	if err != nil {
		t.Fatalf("DefaultRepoPoolPath() err = %v", err)
	}
	want := filepath.Join(home, ".gionx", "repo-pool")
	if got != want {
		t.Fatalf("DefaultRepoPoolPath() = %q, want %q", got, want)
	}
}

func TestRegistryPath_UsesGionxHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIONX_HOME", "")

	got, err := RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath() err = %v", err)
	}
	want := filepath.Join(home, ".gionx", "state", "root-registry.json")
	if got != want {
		t.Fatalf("RegistryPath() = %q, want %q", got, want)
	}
}

func TestCurrentContextPath_UsesGionxHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIONX_HOME", "")

	got, err := CurrentContextPath()
	if err != nil {
		t.Fatalf("CurrentContextPath() err = %v", err)
	}
	want := filepath.Join(home, ".gionx", "state", "current-context")
	if got != want {
		t.Fatalf("CurrentContextPath() = %q, want %q", got, want)
	}
}

func TestPaths_UsesGionxHomeOverride(t *testing.T) {
	gionxHome := filepath.Join(t.TempDir(), ".gionx")
	t.Setenv("GIONX_HOME", gionxHome)

	gotPool, err := DefaultRepoPoolPath()
	if err != nil {
		t.Fatalf("DefaultRepoPoolPath() err = %v", err)
	}
	if gotPool != filepath.Join(gionxHome, "repo-pool") {
		t.Fatalf("repo pool path = %q", gotPool)
	}

	gotRegistry, err := RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath() err = %v", err)
	}
	if gotRegistry != filepath.Join(gionxHome, "state", "root-registry.json") {
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

func TestResolveExistingRoot_UsesCurrentContext(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "workspaces"))
	mustMkdirAll(t, filepath.Join(root, "archive"))

	if err := WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext() err = %v", err)
	}

	got, err := ResolveExistingRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveExistingRoot() err = %v", err)
	}
	if got != root {
		t.Fatalf("ResolveExistingRoot() = %q, want %q", got, root)
	}
}

func TestResolveExistingRoot_CurrentContextMissingPathErrors(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	missingRoot := filepath.Join(t.TempDir(), "missing-root")
	if err := WriteCurrentContext(missingRoot); err != nil {
		t.Fatalf("WriteCurrentContext() err = %v", err)
	}

	_, err := ResolveExistingRoot(t.TempDir())
	if err == nil {
		t.Fatalf("ResolveExistingRoot() err = nil, want error")
	}
	if !strings.Contains(err.Error(), "current-context points to missing directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteAndReadCurrentContext(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	root := t.TempDir()
	if err := WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext() err = %v", err)
	}

	got, ok, err := ReadCurrentContext()
	if err != nil {
		t.Fatalf("ReadCurrentContext() err = %v", err)
	}
	if !ok {
		t.Fatalf("ReadCurrentContext() ok = false, want true")
	}
	if got != root {
		t.Fatalf("ReadCurrentContext() root = %q, want %q", got, root)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}
