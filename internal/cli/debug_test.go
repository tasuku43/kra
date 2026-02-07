package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_GlobalDebugFlag_BeforeHelp(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer

	c := New(&out, &err)
	code := c.Run([]string{"--debug", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("stdout missing usage: %q", out.String())
	}
	if strings.Contains(err.String(), "unknown") {
		t.Fatalf("stderr should not include unknown command: %q", err.String())
	}
}

func TestCLI_WSList_Debug_WritesLogUnderGIONXRoot(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"--debug", "ws", "list"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	logDir := filepath.Join(root, ".gionx", "logs")
	entries, readErr := os.ReadDir(logDir)
	if readErr != nil {
		t.Fatalf("read debug log dir: %v", readErr)
	}
	if len(entries) == 0 {
		t.Fatalf("debug log file not found in %s", logDir)
	}

	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		b, fileErr := os.ReadFile(filepath.Join(logDir, entry.Name()))
		if fileErr != nil {
			t.Fatalf("read debug log file: %v", fileErr)
		}
		if strings.Contains(string(b), "run ws list") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("debug log should contain command trace, got files=%d", len(entries))
	}
}
