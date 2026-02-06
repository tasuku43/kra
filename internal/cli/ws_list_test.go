package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

func TestCLI_WS_List_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws list") {
		t.Fatalf("stdout missing ws list usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_List_ImportsWorkspaceDirAndPrintsIt(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "IMPORTED"), 0o755); err != nil {
		t.Fatalf("create imported ws dir: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "IMPORTED") {
		t.Fatalf("stdout missing imported workspace: %q", out.String())
	}

	ctx := context.Background()
	dbPath, pathErr := paths.StateDBPathForRoot(root)
	if pathErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", pathErr)
	}
	db, openErr := statestore.Open(ctx, dbPath)
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	t.Cleanup(func() { _ = db.Close() })

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces WHERE id = 'IMPORTED'").Scan(&count); err != nil {
		t.Fatalf("query imported workspace: %v", err)
	}
	if count != 1 {
		t.Fatalf("imported workspace row count = %d, want 1", count)
	}
}

func TestCLI_WS_List_MarksMissingRepoWorktree(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WS1"), 0o755); err != nil {
		t.Fatalf("create ws dir: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	ctx := context.Background()
	dbPath, pathErr := paths.StateDBPathForRoot(root)
	if pathErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", pathErr)
	}
	db, openErr := statestore.Open(ctx, dbPath)
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	t.Cleanup(func() { _ = db.Close() })

	repoPoolPath := filepath.Join(cacheHome, "gionx", "repo-pool")
	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		t.Fatalf("EnsureSettings error: %v", err)
	}
	if _, err := statestore.CreateWorkspace(ctx, db, statestore.CreateWorkspaceInput{
		ID:          "WS1",
		Description: "",
		SourceURL:   "",
		Now:         123,
	}); err != nil {
		t.Fatalf("CreateWorkspace error: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO repos (repo_uid, repo_key, remote_url, created_at, updated_at)
VALUES ('github.com/o/r', 'o/r', 'https://example.com/o/r.git', 1, 1)
`); err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspace_repos (
  workspace_id, repo_uid, repo_key, alias, branch, base_ref, repo_spec_input, missing_at, created_at, updated_at
) VALUES (
  'WS1', 'github.com/o/r', 'o/r', 'r', 'main', '', 'github.com/o/r', NULL, 1, 1
)
`); err != nil {
		t.Fatalf("insert workspace_repo: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	var missingAt *int64
	if qErr := db.QueryRowContext(ctx, `
SELECT missing_at
FROM workspace_repos
WHERE workspace_id = 'WS1' AND repo_uid = 'github.com/o/r'
`).Scan(&missingAt); qErr != nil {
		t.Fatalf("query missing_at: %v", qErr)
	}
	if missingAt == nil || *missingAt == 0 {
		t.Fatalf("missing_at not set: %v", missingAt)
	}
}
