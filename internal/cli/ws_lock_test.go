package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_Unlock_JSON_Success(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "unlock", "--id", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws unlock exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	var resp testJSONResponse
	if unmarshalErr := json.Unmarshal(out.Bytes(), &resp); unmarshalErr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", unmarshalErr, out.String())
	}
	if !resp.OK || resp.Action != "ws.unlock" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_WS_Purge_BlockedByDefaultGuard(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "archived", "WS1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "purge", "--id", "WS1"})
	if code != exitError {
		t.Fatalf("ws purge exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	if !strings.Contains(err.String(), "purge guard is enabled") {
		t.Fatalf("stderr missing guard message: %q", err.String())
	}

	{
		var out2 bytes.Buffer
		var err2 bytes.Buffer
		c2 := New(&out2, &err2)
		if code := c2.Run([]string{"ws", "unlock", "--id", "WS1"}); code != exitOK {
			t.Fatalf("ws unlock exit code = %d, want %d (stderr=%q)", code, exitOK, err2.String())
		}
		c2.In = strings.NewReader("y\n")
		if code := c2.Run([]string{"ws", "purge", "--id", "WS1"}); code != exitOK {
			t.Fatalf("ws purge after unlock exit code = %d, want %d (stderr=%q)", code, exitOK, err2.String())
		}
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); statErr == nil {
		t.Fatalf("archive/WS1 should be deleted after purge")
	}
}
