package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tasuku43/kra/internal/paths"
)

type Env struct {
	Root    string
	KraHome string
}

func NewEnv(t *testing.T) Env {
	t.Helper()

	root := t.TempDir()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", home, err)
	}
	kraHome := filepath.Join(base, ".kra")

	t.Setenv("HOME", home)
	t.Setenv("KRA_HOME", kraHome)
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext(%q): %v", root, err)
	}

	return Env{
		Root:    root,
		KraHome: kraHome,
	}
}

func (e Env) RepoPoolPath() string {
	return filepath.Join(e.KraHome, "repo-pool")
}

func (e Env) EnsureRootLayout(t *testing.T) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(e.Root, "workspaces"))
	mustMkdirAll(t, filepath.Join(e.Root, "archive"))
	mustMkdirAll(t, filepath.Join(e.Root, "templates", "default", "notes"))
	mustMkdirAll(t, filepath.Join(e.Root, "templates", "default", "artifacts"))
	guidePath := filepath.Join(e.Root, "templates", "default", "AGENTS.md")
	if err := os.WriteFile(guidePath, []byte("# test template\n"), 0o644); err != nil {
		t.Fatalf("write %q: %v", guidePath, err)
	}
	if _, err := os.Stat(filepath.Join(e.Root, ".git")); os.IsNotExist(err) {
		runGit(t, e.Root, "init", "-b", "main")
		runGit(t, e.Root, "config", "user.email", "test@example.com")
		runGit(t, e.Root, "config", "user.name", "test")
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v (output=%s)", args, err, string(out))
	}
}
