package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_AddRepo_JSON_Preset_AddAndSkip(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := createTestRemoteRepoSpec(t)
	_, repoKey, _ := seedRepoPoolAndState(t, env, repoSpec)
	writeRootConfigYAML(t, env.Root, fmt.Sprintf("workspace:\n  repo_presets:\n    backend:\n      repos:\n        - %s\n", repoKey))

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
		code := c.Run([]string{
			"ws", "add-repo",
			"--format", "json",
			"--id", "WS1",
			"--preset", "backend",
			"--yes",
		})
		if code != exitOK {
			t.Fatalf("ws add-repo json --preset exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		resp := decodeJSONResponse(t, out.String())
		if !resp.OK || resp.Action != "add-repo" {
			t.Fatalf("unexpected json response: %+v", resp)
		}
		if got := int(resp.Result["added"].(float64)); got != 1 {
			t.Fatalf("result.added = %d, want 1", got)
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{
			"ws", "add-repo",
			"--format", "json",
			"--id", "WS1",
			"--preset", "backend",
			"--yes",
		})
		if code != exitOK {
			t.Fatalf("ws add-repo json --preset second run exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		resp := decodeJSONResponse(t, out.String())
		if !resp.OK || resp.Action != "add-repo" {
			t.Fatalf("unexpected json response: %+v", resp)
		}
		if got := int(resp.Result["added"].(float64)); got != 0 {
			t.Fatalf("result.added = %d, want 0", got)
		}
		skippedAny, ok := resp.Result["skipped"]
		if !ok {
			t.Fatalf("result.skipped missing: %+v", resp.Result)
		}
		skipped, ok := skippedAny.([]any)
		if !ok || len(skipped) != 1 || skipped[0].(string) != repoKey {
			t.Fatalf("result.skipped = %#v, want [%s]", skippedAny, repoKey)
		}
	}
}

func TestCLI_WS_AddRepo_JSON_Preset_StrictMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	writeRootConfigYAML(t, env.Root, "workspace:\n  repo_presets:\n    backend:\n      repos:\n        - org/missing\n")

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
	code := c.Run([]string{
		"ws", "add-repo",
		"--format", "json",
		"--id", "WS1",
		"--preset", "backend",
		"--yes",
	})
	if code != exitUsage {
		t.Fatalf("ws add-repo json --preset strict missing exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "add-repo" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if !strings.Contains(resp.Error.Message, "org/missing") {
		t.Fatalf("error message should include missing repo: %+v", resp.Error)
	}
}
