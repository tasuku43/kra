package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Launcher_RequiresIDOrWorkspaceContext(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws"})
	if code != exitError {
		t.Fatalf("ws exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "ws requires --id <id> or workspace context") {
		t.Fatalf("stderr missing unresolved launcher message: %q", err.String())
	}
}

func TestCLI_WS_Launcher_WithIDAndFixedAction(t *testing.T) {
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

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "--id", "WS1", "--act", "go"})
	if code != exitOK {
		t.Fatalf("ws --id --act go exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	want := filepath.Join(env.Root, "workspaces", "WS1")
	if !strings.Contains(out.String(), "Result:") || !strings.Contains(out.String(), "Destination: "+want) {
		t.Fatalf("stdout missing destination result: %q", out.String())
	}
}
