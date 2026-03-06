package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestWorkspaceBaselineLifecycle_CreateWritesBaselineToMeta(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	runWorkspaceLifecycleCommand(t, "ws", "create", "--no-prompt", "WS1")

	meta := loadWorkspaceMetaForLifecycleTest(t, filepath.Join(env.Root, "workspaces", "WS1"))
	if meta.Baseline == nil {
		t.Fatalf("baseline should exist in .kra.meta.json after create")
	}
	assertNoLegacyWorkspaceBaseline(t, env.Root, "WS1")
}

func TestWorkspaceBaselineLifecycle_ClosePreservesBaselineInArchive(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceWithBaseline(t, env.Root, "active", "WS1")

	runWorkspaceLifecycleCommand(t, "ws", "close", "--no-commit", "--id", "WS1")

	meta := loadWorkspaceMetaForLifecycleTest(t, filepath.Join(env.Root, "archive", "WS1"))
	if meta.Baseline == nil {
		t.Fatalf("baseline should remain in archived .kra.meta.json after close")
	}
	assertNoLegacyWorkspaceBaseline(t, env.Root, "WS1")
}

func TestWorkspaceBaselineLifecycle_ReopenRefreshesBaselineInWorkspaceMeta(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceWithBaseline(t, env.Root, "archived", "WS1")

	runWorkspaceLifecycleCommand(t, "ws", "reopen", "--no-commit", "--id", "WS1")

	meta := loadWorkspaceMetaForLifecycleTest(t, filepath.Join(env.Root, "workspaces", "WS1"))
	if meta.Baseline == nil {
		t.Fatalf("baseline should exist in .kra.meta.json after reopen")
	}
	assertNoLegacyWorkspaceBaseline(t, env.Root, "WS1")
}

func TestWorkspaceBaselineLifecycle_PurgeRemovesWorkspaceMetaAndLegacyBaseline(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceWithBaseline(t, env.Root, "archived", "WS1")

	runWorkspaceLifecycleCommand(t, "ws", "unlock", "--id", "WS1")
	runWorkspaceLifecycleCommand(t, "ws", "purge", "--no-prompt", "--force", "--no-commit", "--id", "WS1")

	assertNoLegacyWorkspaceBaseline(t, env.Root, "WS1")
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1", workspaceMetaFilename)); !os.IsNotExist(err) {
		t.Fatalf("workspace meta should be removed after purge: %v", err)
	}
}

func runWorkspaceLifecycleCommand(t *testing.T, args ...string) {
	t.Helper()
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run(args)
	if code != exitOK {
		t.Fatalf("%v exit code=%d want=%d stderr=%q", args, code, exitOK, err.String())
	}
}

func loadWorkspaceMetaForLifecycleTest(t *testing.T, path string) workspaceMetaFile {
	t.Helper()
	meta, err := loadWorkspaceMetaFile(path)
	if err != nil {
		t.Fatalf("load workspace meta: %v", err)
	}
	return meta
}

func assertNoLegacyWorkspaceBaseline(t *testing.T, root string, workspaceID string) {
	t.Helper()
	legacyBaselinePath := filepath.Join(root, ".kra", "state", workspaceBaselineDirName, workspaceID+".json")
	if _, err := os.Stat(legacyBaselinePath); !os.IsNotExist(err) {
		t.Fatalf("legacy baseline file should not exist: %v", err)
	}
}

func seedWorkspaceWithBaseline(t *testing.T, root string, scope string, workspaceID string) string {
	t.Helper()

	wsPath := seedWorkspaceMeta(t, root, scope, workspaceID)
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		t.Fatalf("load workspace meta: %v", err)
	}
	meta.Baseline = &workspaceBaseline{
		Version:   1,
		CreatedAt: time.Now().Unix(),
		Repos:     map[string]workspaceBaselineRepo{},
		FS:        map[string]string{},
	}
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	return wsPath
}
