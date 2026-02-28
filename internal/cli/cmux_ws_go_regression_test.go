package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_CMUX_Regression_WSGo_RemainsIndependent(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	actionFile := filepath.Join(t.TempDir(), "action.sh")
	t.Setenv(shellActionFileEnv, actionFile)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "--act", "go", "WS1"})
	if code != exitOK {
		t.Fatalf("ws go exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if out.Len() != 0 {
		t.Fatalf("ws go stdout should be empty in default mode: %q", out.String())
	}
	action, readErr := os.ReadFile(actionFile)
	if readErr != nil {
		t.Fatalf("read action file: %v", readErr)
	}
	wantPath := filepath.Join(env.Root, "workspaces", "WS1")
	if !strings.Contains(string(action), "cd ") || !strings.Contains(string(action), wantPath) {
		t.Fatalf("action file should include workspace cd: %q", string(action))
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, ".kra", "state", "cmux-workspaces.json")); !os.IsNotExist(statErr) {
		t.Fatalf("ws go should not create cmux mapping file, stat err=%v", statErr)
	}
}
