package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoPoolPath_IsRootLocal(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "kra-root")
	got := RepoPoolPath(root)
	want := filepath.Join(root, ".kra", "repo-pool")
	if got != want {
		t.Fatalf("RepoPoolPath() = %q, want %q", got, want)
	}
}

func TestLegacyRepoPoolPath_UsesKraHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KRA_HOME", "")

	got, err := LegacyRepoPoolPath()
	if err != nil {
		t.Fatalf("LegacyRepoPoolPath() err = %v", err)
	}
	want := filepath.Join(home, ".kra", "repo-pool")
	if got != want {
		t.Fatalf("LegacyRepoPoolPath() = %q, want %q", got, want)
	}
}

func TestRegistryPath_UsesKraHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KRA_HOME", "")

	got, err := RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath() err = %v", err)
	}
	want := filepath.Join(home, ".kra", "state", "root-registry.json")
	if got != want {
		t.Fatalf("RegistryPath() = %q, want %q", got, want)
	}
}

func TestCurrentContextPath_UsesKraHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KRA_HOME", "")

	got, err := CurrentContextPath()
	if err != nil {
		t.Fatalf("CurrentContextPath() err = %v", err)
	}
	want := filepath.Join(home, ".kra", "state", "current-context")
	if got != want {
		t.Fatalf("CurrentContextPath() = %q, want %q", got, want)
	}
}

func TestConfigPath_UsesKraHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KRA_HOME", "")

	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() err = %v", err)
	}
	want := filepath.Join(home, ".kra", "config.yaml")
	if got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestPaths_UsesKraHomeOverride(t *testing.T) {
	kraHome := filepath.Join(t.TempDir(), ".kra")
	t.Setenv("KRA_HOME", kraHome)

	gotPool, err := LegacyRepoPoolPath()
	if err != nil {
		t.Fatalf("LegacyRepoPoolPath() err = %v", err)
	}
	if gotPool != filepath.Join(kraHome, "repo-pool") {
		t.Fatalf("repo pool path = %q", gotPool)
	}

	gotRegistry, err := RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath() err = %v", err)
	}
	if gotRegistry != filepath.Join(kraHome, "state", "root-registry.json") {
		t.Fatalf("registry path = %q", gotRegistry)
	}

	gotConfig, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() err = %v", err)
	}
	if gotConfig != filepath.Join(kraHome, "config.yaml") {
		t.Fatalf("config path = %q", gotConfig)
	}
}

func TestRootConfigPath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "kra-root")
	got := RootConfigPath(root)
	want := filepath.Join(root, ".kra", "config.yaml")
	if got != want {
		t.Fatalf("RootConfigPath() = %q, want %q", got, want)
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
	t.Setenv("KRA_HOME", filepath.Join(t.TempDir(), ".kra"))

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
	t.Setenv("KRA_HOME", filepath.Join(t.TempDir(), ".kra"))

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
	t.Setenv("KRA_HOME", filepath.Join(t.TempDir(), ".kra"))

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
