package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

type testJSONResponse struct {
	OK          bool           `json:"ok"`
	Action      string         `json:"action"`
	WorkspaceID string         `json:"workspace_id"`
	Result      map[string]any `json:"result"`
	Error       struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeJSONResponse(t *testing.T, out string) testJSONResponse {
	t.Helper()
	var resp testJSONResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", err, out)
	}
	return resp
}

func TestCLI_WS_Create_JSON_Success(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "create", "--format", "json", "--no-prompt", "WS-CREATE-JSON-1"})
	if code != exitOK {
		t.Fatalf("ws create --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.create" || resp.WorkspaceID != "WS-CREATE-JSON-1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := resp.Result["created"]; got != float64(1) {
		t.Fatalf("created = %v, want 1", got)
	}
	if got := resp.Result["path"]; got != filepath.Join(env.Root, "workspaces", "WS-CREATE-JSON-1") {
		t.Fatalf("path = %v, want %q", got, filepath.Join(env.Root, "workspaces", "WS-CREATE-JSON-1"))
	}
}

func TestCLI_WS_Create_JSON_WithIDAndTitleFlags(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "create", "--format", "json", "--id", "WS-CREATE-JSON-2", "--title", "Automation Title"})
	if code != exitOK {
		t.Fatalf("ws create --format json --id/--title exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.create" || resp.WorkspaceID != "WS-CREATE-JSON-2" {
		t.Fatalf("unexpected json response: %+v", resp)
	}

	metaPath := filepath.Join(env.Root, "workspaces", "WS-CREATE-JSON-2", workspaceMetaFilename)
	metaBytes, readErr := os.ReadFile(metaPath)
	if readErr != nil {
		t.Fatalf("read workspace meta: %v", readErr)
	}
	if !bytes.Contains(metaBytes, []byte(`"title": "Automation Title"`)) {
		t.Fatalf("workspace meta missing explicit title: %q", string(metaBytes))
	}
}

func TestCLI_WS_Create_JSON_MissingID_ReturnsInvalidArgument(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "create", "--format", "json", "--no-prompt"})
	if code != exitUsage {
		t.Fatalf("ws create --format json missing id exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "ws.create" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_WS_ActGo_JSON_Success(t *testing.T) {
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
	code := c.Run([]string{"ws", "--act", "go", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws --act go --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "go" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := resp.Result["target_path"]; got != filepath.Join(env.Root, "workspaces", "WS1") {
		t.Fatalf("target_path = %v, want %q", got, filepath.Join(env.Root, "workspaces", "WS1"))
	}
}

func TestCLI_WS_ActClose_JSON_Success(t *testing.T) {
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
	code := c.Run([]string{"ws", "--act", "close", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws --act close --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "close" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := resp.Result["archived_path"]; got != filepath.Join(env.Root, "archive", "WS1") {
		t.Fatalf("archived_path = %v, want %q", got, filepath.Join(env.Root, "archive", "WS1"))
	}
}

func TestCLI_WS_ActClose_DryRun_JSON_Success(t *testing.T) {
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
	code := c.Run([]string{"ws", "--act", "close", "--dry-run", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws --act close --dry-run --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.close.dry-run" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := resp.Result["executable"]; got != true {
		t.Fatalf("executable = %v, want true", got)
	}
}

func TestCLI_WS_ActReopen_DryRun_JSON_Success(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if code := c.Run([]string{"ws", "--act", "close", "--format", "json", "--id", "WS1"}); code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "--act", "reopen", "--dry-run", "--format", "json", "WS1"})
	if code != exitOK {
		t.Fatalf("ws --act reopen --dry-run --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.reopen.dry-run" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_WS_ActPurge_DryRun_JSON_ArchivedOnly(t *testing.T) {
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
	code := c.Run([]string{"ws", "--act", "purge", "--dry-run", "--format", "json", "WS1"})
	if code != exitError {
		t.Fatalf("ws --act purge --dry-run --format json exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "ws.purge.dry-run" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := resp.Result["executable"]; got != false {
		t.Fatalf("executable = %v, want false", got)
	}
}

func TestCLI_WS_ActAddRepo_JSON_RequiresRepo(t *testing.T) {
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
	code := c.Run([]string{"ws", "--act", "add-repo", "--format", "json", "--id", "WS1", "--yes"})
	if code != exitUsage {
		t.Fatalf("ws --act add-repo --format json exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "add-repo" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_WS_ActRemoveRepo_JSON_RequiresRepo(t *testing.T) {
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
	code := c.Run([]string{"ws", "--act", "remove-repo", "--format", "json", "--id", "WS1", "--yes"})
	if code != exitUsage {
		t.Fatalf("ws --act remove-repo --format json exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "remove-repo" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}
