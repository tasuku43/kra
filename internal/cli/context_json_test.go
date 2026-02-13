package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/stateregistry"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_Context_List_JSON_IncludesCurrent(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	other := t.TempDir()
	if err := stateregistry.SetContextName(env.Root, "alpha", time.Unix(100, 0)); err != nil {
		t.Fatalf("SetContextName(alpha): %v", err)
	}
	if err := stateregistry.SetContextName(other, "beta", time.Unix(200, 0)); err != nil {
		t.Fatalf("SetContextName(beta): %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"context", "list", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "context.list" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	contextsAny, ok := resp.Result["contexts"].([]any)
	if !ok || len(contextsAny) < 2 {
		t.Fatalf("result.contexts missing or too short: %#v", resp.Result["contexts"])
	}
	currentFound := false
	for _, raw := range contextsAny {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := row["name"].(string)
		isCurrent, _ := row["current"].(bool)
		if name == "alpha" && isCurrent {
			currentFound = true
			break
		}
	}
	if !currentFound {
		t.Fatalf("expected alpha to be current: %#v", contextsAny)
	}
}

func TestCLI_Context_Use_JSON_RequiresName(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"context", "use", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "context.use" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
}

func TestCLI_Context_CreateUseRenameRemove_JSON(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	targetRoot := filepath.Join(t.TempDir(), "dev-root")
	otherRoot := filepath.Join(t.TempDir(), "other-root")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatalf("mkdir targetRoot: %v", err)
	}
	if err := os.MkdirAll(otherRoot, 0o755); err != nil {
		t.Fatalf("mkdir otherRoot: %v", err)
	}
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"context", "create", "dev", "--path", targetRoot, "--format", "json"})
	if code != exitOK {
		t.Fatalf("context create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "context.create" {
		t.Fatalf("unexpected create response: %+v", resp)
	}
	out.Reset()
	err.Reset()

	code = c.Run([]string{"context", "use", "dev", "--format", "json"})
	if code != exitOK {
		t.Fatalf("context use exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp = decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "context.use" {
		t.Fatalf("unexpected use response: %+v", resp)
	}
	out.Reset()
	err.Reset()

	code = c.Run([]string{"context", "rename", "dev", "dev2", "--format", "json"})
	if code != exitOK {
		t.Fatalf("context rename exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp = decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "context.rename" {
		t.Fatalf("unexpected rename response: %+v", resp)
	}
	out.Reset()
	err.Reset()

	code = c.Run([]string{"context", "rm", "dev2", "--format", "json"})
	if code != exitError {
		t.Fatalf("context rm current exit code = %d, want %d", code, exitError)
	}
	resp = decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "context.remove" || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected remove conflict response: %+v", resp)
	}
	out.Reset()
	err.Reset()

	code = c.Run([]string{"context", "create", "other", "--path", otherRoot, "--format", "json"})
	if code != exitOK {
		t.Fatalf("context create(other) exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	out.Reset()
	err.Reset()

	code = c.Run([]string{"context", "rm", "other", "--format", "json"})
	if code != exitOK {
		t.Fatalf("context rm(other) exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp = decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "context.remove" {
		t.Fatalf("unexpected remove response: %+v", resp)
	}
}
