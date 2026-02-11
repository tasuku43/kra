package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_TemplateValidate_AllTemplates_CollectsViolations(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	bad := filepath.Join(env.Root, "templates", "bad")
	if err := os.MkdirAll(filepath.Join(bad, "repos"), 0o755); err != nil {
		t.Fatalf("mkdir repos/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bad, workspaceMetaFilename), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", workspaceMetaFilename, err)
	}
	if err := os.Symlink(filepath.Join(env.Root, "templates", "default", "AGENTS.md"), filepath.Join(bad, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "validate"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(out.String(), "Validated 1 / 2") {
		t.Fatalf("stdout missing validation summary: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "template=bad path=repos") {
		t.Fatalf("stderr missing repos violation: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "template=bad path=.gionx.meta.json") {
		t.Fatalf("stderr missing workspace meta violation: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "template=bad path=link") {
		t.Fatalf("stderr missing symlink violation: %q", errBuf.String())
	}
}

func TestCLI_TemplateValidate_Name_Success(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "validate", "--name", "default"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	if !strings.Contains(out.String(), "✔ default") {
		t.Fatalf("stdout missing template name: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr not empty: %q", errBuf.String())
	}
}

func TestCLI_TemplateValidate_Name_NotFound_ShowsAvailable(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	if err := os.MkdirAll(filepath.Join(env.Root, "templates", "custom"), 0o755); err != nil {
		t.Fatalf("mkdir custom template: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "validate", "--name", "missing"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), `template "missing" not found`) {
		t.Fatalf("stderr missing template-not-found: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "custom") || !strings.Contains(errBuf.String(), "default") {
		t.Fatalf("stderr missing available template names: %q", errBuf.String())
	}
}
