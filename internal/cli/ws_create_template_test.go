package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WSCreate_DefaultTemplateMissing_Fails(t *testing.T) {
	env := testutil.NewEnv(t)
	if err := os.MkdirAll(filepath.Join(env.Root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(env.Root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "WS-TPL-001"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "templates directory does not exist") {
		t.Fatalf("stderr missing templates-directory error: %q", errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS-TPL-001")); !os.IsNotExist(err) {
		t.Fatalf("workspace dir should not exist, stat err=%v", err)
	}
}

func TestCLI_WSCreate_TemplateReservedPath_FailsBeforeCreate(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	bad := filepath.Join(env.Root, "templates", "bad")
	if err := os.MkdirAll(filepath.Join(bad, "repos"), 0o755); err != nil {
		t.Fatalf("mkdir bad/repos: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "--template", "bad", "WS-TPL-002"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "template validation failed") || !strings.Contains(errBuf.String(), "path=repos") {
		t.Fatalf("stderr missing validation detail: %q", errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS-TPL-002")); !os.IsNotExist(err) {
		t.Fatalf("workspace dir should not exist, stat err=%v", err)
	}
}

func TestCLI_WSCreate_TemplateOption_CopiesSelectedTemplate(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	custom := filepath.Join(env.Root, "templates", "custom")
	if err := os.MkdirAll(filepath.Join(custom, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir custom/notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(custom, "README.md"), []byte("custom template\n"), 0o644); err != nil {
		t.Fatalf("write custom README: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "--template", "custom", "WS-TPL-003"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}

	wsPath := filepath.Join(env.Root, "workspaces", "WS-TPL-003")
	if _, err := os.Stat(filepath.Join(wsPath, "README.md")); err != nil {
		t.Fatalf("workspace missing template file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsPath, "notes")); err != nil {
		t.Fatalf("workspace missing template dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsPath, workspaceMetaFilename)); err != nil {
		t.Fatalf("workspace missing %s: %v", workspaceMetaFilename, err)
	}
}
