package cli

import (
	"bytes"
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
		if code := c.Run([]string{"ws", "close", "--id", "WS1"}); code != exitOK {
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

func TestCLI_WSDashboard_JSON_WorkspaceFilterKeepsGlobalSummaryCounts(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		for _, id := range []string{"WS1", "WS2"} {
			if code := c.Run([]string{"ws", "create", "--no-prompt", id}); code != exitOK {
				t.Fatalf("ws create %s exit code = %d, want %d (stderr=%q)", id, code, exitOK, err.String())
			}
			out.Reset()
			err.Reset()
		}
		if code := c.Run([]string{"ws", "close", "--id", "WS2"}); code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--workspace", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --workspace WS1 --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	summary, ok := resp.Result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("result.summary missing: %+v", resp.Result)
	}
	if got := summary["active"]; got != float64(1) {
		t.Fatalf("summary.active = %v, want %v", got, float64(1))
	}
	if got := summary["archived"]; got != float64(1) {
		t.Fatalf("summary.archived = %v, want %v", got, float64(1))
	}
	workspaces, ok := resp.Result["workspaces"].([]any)
	if !ok || len(workspaces) != 1 {
		t.Fatalf("result.workspaces = %+v, want single workspace", resp.Result["workspaces"])
	}
}
