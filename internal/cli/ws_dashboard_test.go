package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WSDashboard_JSON_ActiveDefault(t *testing.T) {
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
	code := c.Run([]string{"ws", "dashboard", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["scope"]; got != "active" {
		t.Fatalf("result.scope = %v, want %q", got, "active")
	}
	workspaces, ok := resp.Result["workspaces"].([]any)
	if !ok || len(workspaces) == 0 {
		t.Fatalf("result.workspaces missing: %+v", resp.Result)
	}
}

func TestCLI_WSDashboard_JSON_ArchivedScope(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if code := c.Run([]string{"ws", "--act", "close", "WS1"}); code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--archived", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --archived --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["scope"]; got != "archived" {
		t.Fatalf("result.scope = %v, want %q", got, "archived")
	}
}

func TestCLI_WSDashboard_JSON_WarningsOnBrokenAgentsFile(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	if err := os.MkdirAll(filepath.Join(env.Root, ".kra", "state"), 0o755); err != nil {
		t.Fatalf("mkdir .kra/state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(env.Root, ".kra", "state", "agents.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write agents.json: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK {
		t.Fatalf("expected ok=true even in degraded mode: %+v", resp)
	}
	warnings, ok := resp.Result["warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("expected non-empty warnings, got: %+v", resp.Result["warnings"])
	}
}
