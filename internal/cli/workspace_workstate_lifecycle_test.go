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

	legacyBaselinePath := filepath.Join(env.Root, ".kra", "state", workspaceBaselineDirName, "WS1.json")
	loadMeta := func(path string) workspaceMetaFile {
		t.Helper()
		meta, err := loadWorkspaceMetaFile(path)
		if err != nil {
			t.Fatalf("load workspace meta: %v", err)
		}
		return meta
	}

	run("ws", "create", "--no-prompt", "WS1")
	createdMeta := loadMeta(filepath.Join(env.Root, "workspaces", "WS1"))
	if createdMeta.Baseline == nil {
		t.Fatalf("baseline should exist in .kra.meta.json after create")
	}
	if _, err := os.Stat(legacyBaselinePath); !os.IsNotExist(err) {
		t.Fatalf("legacy baseline file should not exist after create: %v", err)
	}

	run("ws", "close", "--id", "WS1")
	archivedMeta := loadMeta(filepath.Join(env.Root, "archive", "WS1"))
	if archivedMeta.Baseline == nil {
		t.Fatalf("baseline should remain in archived .kra.meta.json after close")
	}
	if _, err := os.Stat(legacyBaselinePath); !os.IsNotExist(err) {
		t.Fatalf("legacy baseline file should be removed after close: %v", err)
	}

	run("ws", "reopen", "--id", "WS1")
	reopenedMeta := loadMeta(filepath.Join(env.Root, "workspaces", "WS1"))
	if reopenedMeta.Baseline == nil {
		t.Fatalf("baseline should exist in .kra.meta.json after reopen")
	}
	if _, err := os.Stat(legacyBaselinePath); !os.IsNotExist(err) {
		t.Fatalf("legacy baseline file should not exist after reopen: %v", err)
	}

	run("ws", "close", "--id", "WS1")
	run("ws", "unlock", "--id", "WS1")
	run("ws", "purge", "--no-prompt", "--force", "--id", "WS1")
	if _, err := os.Stat(legacyBaselinePath); !os.IsNotExist(err) {
		t.Fatalf("legacy baseline file should be removed after purge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1", workspaceMetaFilename)); !os.IsNotExist(err) {
		t.Fatalf("workspace meta should be removed after purge: %v", err)
	}
}
