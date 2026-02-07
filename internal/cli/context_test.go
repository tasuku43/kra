package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/stateregistry"
)

func TestCLI_Context_Help(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"context", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx context") {
		t.Fatalf("stdout missing context usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_Context_UseAndCurrent(t *testing.T) {
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("GIONX_ROOT", "")

	root := t.TempDir()
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext() error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaces): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("MkdirAll(archive): %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"context", "use", root})
		if code != exitOK {
			t.Fatalf("context use exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if got := out.String(); !strings.Contains(got, "Result:") || !strings.Contains(got, "Context set: "+root) {
			t.Fatalf("context use stdout missing result section: %q", got)
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"context", "current"})
	if code != exitOK {
		t.Fatalf("context current exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if strings.TrimSpace(out.String()) != root {
		t.Fatalf("context current stdout = %q, want %q", strings.TrimSpace(out.String()), root)
	}
}

func TestCLI_Context_ListShowsRegistryEntries(t *testing.T) {
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)

	rootA := t.TempDir()
	rootB := t.TempDir()
	stateA, err := paths.StateDBPathForRoot(rootA)
	if err != nil {
		t.Fatalf("StateDBPathForRoot(rootA): %v", err)
	}
	stateB, err := paths.StateDBPathForRoot(rootB)
	if err != nil {
		t.Fatalf("StateDBPathForRoot(rootB): %v", err)
	}
	if err := stateregistry.Touch(rootA, stateA, time.Unix(100, 0)); err != nil {
		t.Fatalf("Touch(rootA): %v", err)
	}
	if err := stateregistry.Touch(rootB, stateB, time.Unix(200, 0)); err != nil {
		t.Fatalf("Touch(rootB): %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"context", "list"})
	if code != exitOK {
		t.Fatalf("context list exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	text := out.String()
	if !strings.Contains(text, rootA) || !strings.Contains(text, rootB) {
		t.Fatalf("context list missing roots: %q", text)
	}
	if strings.Index(text, rootB) > strings.Index(text, rootA) {
		t.Fatalf("context list order should prefer newer last_used_at: %q", text)
	}
}

func TestCLI_Context_CurrentFailsWhenContextMissing(t *testing.T) {
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("GIONX_ROOT", "")

	missing := filepath.Join(t.TempDir(), "missing-root")
	if err := paths.WriteCurrentContext(missing); err != nil {
		t.Fatalf("WriteCurrentContext() error: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"context", "current"})
	if code != exitError {
		t.Fatalf("context current exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "current-context points to missing directory") {
		t.Fatalf("stderr missing context error: %q", err.String())
	}
}
