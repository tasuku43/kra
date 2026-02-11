package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Create_InvalidRoot_Errors(t *testing.T) {
	_ = testutil.NewEnv(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "does not look like a gionx root") {
		t.Fatalf("stderr missing root error: %q", errBuf.String())
	}
}

func TestCLI_WS_Create_FilesystemCollision_Fails(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "MVP-020")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("create existing ws dir: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "workspace directory already exists") {
		t.Fatalf("stderr missing collision error: %q", errBuf.String())
	}

}

func TestCLI_WS_Create_Purged_AllowsNewGeneration(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "MVP-020", workspaceMetaFilename)); err != nil {
		t.Fatalf("workspace meta should exist after create: %v", err)
	}
}

func TestCLI_WS_Create_StateDBCorrupted_StillCreatesWorkspaceFromFS(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "MVP-020", workspaceMetaFilename)); err != nil {
		t.Fatalf("workspace meta not created: %v", err)
	}
}

func TestCLI_Init_UsesDifferentRootsWithoutStateDBDependency(t *testing.T) {
	testutil.RequireCommand(t, "git")
	setGitIdentity(t)

	env := testutil.NewEnv(t)

	var out1 bytes.Buffer
	var err1 bytes.Buffer
	c1 := New(&out1, &err1)

	code := c1.Run([]string{"init", "--root", env.Root, "--context", "root-a"})
	if code != exitOK {
		t.Fatalf("first init exit code = %d, want %d (stderr=%q)", code, exitOK, err1.String())
	}

	otherRoot := t.TempDir()

	var out2 bytes.Buffer
	var err2 bytes.Buffer
	c2 := New(&out2, &err2)

	code = c2.Run([]string{"init", "--root", otherRoot, "--context", "root-b"})
	if code != exitOK {
		t.Fatalf("second init exit code = %d, want %d (stderr=%q)", code, exitOK, err2.String())
	}
	if err2.Len() != 0 {
		t.Fatalf("second init stderr not empty: %q", err2.String())
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces")); err != nil {
		t.Fatalf("first root workspaces missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(otherRoot, "workspaces")); err != nil {
		t.Fatalf("second root workspaces missing: %v", err)
	}
}

func TestCLI_Init_IgnoresLegacyRepoPoolDrift(t *testing.T) {
	testutil.RequireCommand(t, "git")

	root := t.TempDir()
	setGionxHomeForTest(t)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "xdg-cache"))

	var out1 bytes.Buffer
	var err1 bytes.Buffer
	c1 := New(&out1, &err1)

	code := c1.Run([]string{"init", "--root", root, "--context", "drift"})
	if code != exitOK {
		t.Fatalf("first init exit code = %d, want %d (stderr=%q)", code, exitOK, err1.String())
	}

	newCacheHome := filepath.Join(t.TempDir(), "other-xdg-cache")
	t.Setenv("XDG_CACHE_HOME", newCacheHome)

	var out2 bytes.Buffer
	var err2 bytes.Buffer
	c2 := New(&out2, &err2)

	code = c2.Run([]string{"init", "--root", root, "--context", "drift"})
	if code != exitOK {
		t.Fatalf("second init exit code = %d, want %d (stderr=%q)", code, exitOK, err2.String())
	}
	if err2.Len() != 0 {
		t.Fatalf("second init stderr not empty: %q", err2.String())
	}
}
