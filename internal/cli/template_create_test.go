package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_TemplateCreate_NameFlag_CreatesScaffold(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "create", "--name", "custom"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}

	for _, rel := range []string{
		filepath.Join("templates", "custom", "notes"),
		filepath.Join("templates", "custom", "artifacts"),
		filepath.Join("templates", "custom", ".cmux", "dock.json"),
		filepath.Join("templates", "custom", "AGENTS.md"),
		filepath.Join("templates", "custom", workspaceDocumentFilename),
	} {
		if _, statErr := os.Stat(filepath.Join(env.Root, rel)); statErr != nil {
			t.Fatalf("missing scaffold path %q: %v", rel, statErr)
		}
	}
	dockBytes, readErr := os.ReadFile(filepath.Join(env.Root, "templates", "custom", ".cmux", "dock.json"))
	if readErr != nil {
		t.Fatalf("read scaffold dock.json: %v", readErr)
	}
	assertDefaultDockJSON(t, dockBytes)
	if !strings.Contains(out.String(), "✔ custom") {
		t.Fatalf("stdout missing created template name: %q", out.String())
	}
	if !strings.Contains(out.String(), "commit: ") {
		t.Fatalf("stdout missing commit line: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr not empty: %q", errBuf.String())
	}
	msg, err := gitLogHeadSubject(env.Root)
	if err != nil {
		t.Fatalf("git log head subject: %v", err)
	}
	if msg != "template-create: custom" {
		t.Fatalf("commit subject=%q, want %q", msg, "template-create: custom")
	}
}

func TestCLI_TemplateCreate_PromptWhenNameOmitted(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	c.In = strings.NewReader("prompted\n")

	code := c.Run([]string{"template", "create"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "templates", "prompted", "AGENTS.md")); statErr != nil {
		t.Fatalf("missing prompted template scaffold: %v", statErr)
	}
	if !strings.Contains(errBuf.String(), "Inputs:") || !strings.Contains(errBuf.String(), "name: ") {
		t.Fatalf("stderr missing prompt: %q", errBuf.String())
	}
}

func TestCLI_TemplateCreate_InvalidName_ReturnsUsage(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "create", "--name", "bad/name"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errBuf.String(), "invalid template name") {
		t.Fatalf("stderr missing invalid-name error: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "kra template create") {
		t.Fatalf("stderr missing usage: %q", errBuf.String())
	}
}

func TestCLI_TemplateCreate_AlreadyExists_ReturnsError(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	if err := os.MkdirAll(filepath.Join(env.Root, "templates", "custom"), 0o755); err != nil {
		t.Fatalf("mkdir custom template: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "create", "--name", "custom"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(errBuf.String(), "Error:") || !strings.Contains(errBuf.String(), "reason:") || !strings.Contains(errBuf.String(), "already exists") {
		t.Fatalf("stderr missing already-exists error: %q", errBuf.String())
	}
}

func TestCLI_TemplateCreate_From_CopiesSourceTemplate(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	src := filepath.Join(env.Root, "templates", "source")
	if err := os.MkdirAll(filepath.Join(src, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir source notes/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "notes", "README.md"), []byte("from source\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "create", "--name", "copied", "--from", "source"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	got, err := os.ReadFile(filepath.Join(env.Root, "templates", "copied", "notes", "README.md"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if strings.TrimSpace(string(got)) != "from source" {
		t.Fatalf("copied file content=%q, want %q", string(got), "from source\n")
	}
	if !strings.Contains(out.String(), "from: source") {
		t.Fatalf("stdout missing from summary: %q", out.String())
	}
}

func TestCLI_TemplateCreate_From_MissingSource_ReturnsError(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"template", "create", "--name", "copied", "--from", "missing"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(errBuf.String(), "Error:") || !strings.Contains(errBuf.String(), "from: missing") || !strings.Contains(errBuf.String(), `template "missing" not found`) {
		t.Fatalf("stderr missing missing-source error: %q", errBuf.String())
	}
}

func TestCLI_TemplateCreate_From_WithPromptedName(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	src := filepath.Join(env.Root, "templates", "source")
	if err := os.MkdirAll(filepath.Join(src, "artifacts"), 0o755); err != nil {
		t.Fatalf("mkdir source artifacts/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "artifacts", "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write source seed: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	c.In = strings.NewReader("derived\n")

	code := c.Run([]string{"template", "create", "--from", "source"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(env.Root, "templates", "derived", "artifacts", "seed.txt")); err != nil {
		t.Fatalf("copied scaffold missing file: %v", err)
	}
	if !strings.Contains(errBuf.String(), "Inputs:") || !strings.Contains(errBuf.String(), "name: ") {
		t.Fatalf("stderr missing template-name prompt: %q", errBuf.String())
	}
}

func TestCLI_TemplateCreate_AbortWhenStagedOutsideAllowlist(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	outside := filepath.Join(env.Root, "README.md")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	cmd := exec.Command("git", "add", "--", "README.md")
	cmd.Dir = env.Root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add README.md: %v (out=%s)", err, string(b))
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"template", "create", "--name", "scoped"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(errBuf.String(), "outside allowlist") {
		t.Fatalf("stderr missing allowlist error: %q", errBuf.String())
	}
}

func gitLogHeadSubject(root string) (string, error) {
	cmd := exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
