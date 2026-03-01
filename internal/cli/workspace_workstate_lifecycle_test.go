package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestWorkspaceBaselineLifecycle_CreateCloseReopenPurge(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	run := func(args ...string) {
		t.Helper()
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run(args)
		if code != exitOK {
			t.Fatalf("%v exit code=%d want=%d stderr=%q", args, code, exitOK, err.String())
		}
	}

	baselinePath := filepath.Join(env.Root, ".kra", "state", workspaceBaselineDirName, "WS1.json")

	run("ws", "create", "--no-prompt", "WS1")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline should exist after create: %v", err)
	}

	run("ws", "close", "--id", "WS1")
	if _, err := os.Stat(baselinePath); !os.IsNotExist(err) {
		t.Fatalf("baseline should be removed after close: %v", err)
	}

	run("ws", "reopen", "--id", "WS1")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline should exist after reopen: %v", err)
	}

	run("ws", "close", "--id", "WS1")
	run("ws", "unlock", "--id", "WS1")
	run("ws", "purge", "--no-prompt", "--force", "--id", "WS1")
	if _, err := os.Stat(baselinePath); !os.IsNotExist(err) {
		t.Fatalf("baseline should be removed after purge: %v", err)
	}
}
