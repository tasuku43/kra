package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_Launcher_RequiresIDOrWorkspaceContext(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws"})
	if code != exitUsage {
		t.Fatalf("ws exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "ws requires one of --id <id>, --current, or --select") {
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
	code := c.Run([]string{"ws", "close", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws close --id exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); statErr != nil {
		t.Fatalf("archive/WS1 should exist after close: %v", statErr)
	}
}

func TestCLI_WS_Launcher_ReopenAcceptsIDWithNoCommitFlag(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "close", "--id", "WS1", "--no-commit", "--force"}); code != exitOK {
			t.Fatalf("ws close --id --no-commit exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "reopen", "--id", "WS1", "--no-commit"})
	if code != exitOK {
		t.Fatalf("ws reopen --id --no-commit exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); statErr != nil {
		t.Fatalf("workspaces/WS1 should exist after reopen: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); !os.IsNotExist(statErr) {
		t.Fatalf("archive/WS1 should not exist after reopen: %v", statErr)
	}
}

func TestCLI_WS_Launcher_PurgeAcceptsIDWithNoPromptForce(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "close", "--id", "WS1", "--no-commit", "--force"}); code != exitOK {
			t.Fatalf("ws close --id --no-commit exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "unlock", "WS1"}); code != exitOK {
			t.Fatalf("ws unlock exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "purge", "--id", "WS1", "--no-prompt", "--force", "--no-commit"})
	if code != exitOK {
		t.Fatalf("ws purge --id --no-prompt --force --no-commit exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); !os.IsNotExist(statErr) {
		t.Fatalf("workspaces/WS1 should not exist after purge: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); !os.IsNotExist(statErr) {
		t.Fatalf("archive/WS1 should not exist after purge: %v", statErr)
	}
}

func TestCLI_WS_Launcher_UnlockActionAcceptsIDWithFormatJSON(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "--id", "WS1", "unlock", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws --id unlock --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), `"action":"ws.unlock"`) {
		t.Fatalf("stdout missing ws.unlock action: %q", out.String())
	}
}
