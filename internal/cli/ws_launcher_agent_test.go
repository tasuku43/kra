package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_WS_Launcher_RunAgent_UsesWorkspaceContext(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsDir := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	workDir := filepath.Join(wsDir, "notes")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace subdir: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir workspace subdir: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	c := New(&out, &stderr)
	code := c.Run([]string{"ws", "--act", "run-agent", "--kind", "true"})
	if code != exitOK {
		t.Fatalf("ws --act run-agent exit code = %d, want %d (stderr=%q)", code, exitOK, stderr.String())
	}
	if !strings.Contains(out.String(), "agent started: workspace=WS1") {
		t.Fatalf("stdout should include resolved workspace id: %q", out.String())
	}
}
