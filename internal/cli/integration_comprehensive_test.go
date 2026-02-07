package cli

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/statestore"
	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_AddRepo_BaseRefNotFound_FailsWithoutMutatingState(t *testing.T) {
	testutil.RequireCommand(t, "git")

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	repoSpec := prepareRemoteRepoSpec(t, runGit)
	_, _, _ = seedRepoPoolAndState(t, env, repoSpec)
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("origin/does-not-exist", "WS1/test"))
		code := c.Run([]string{"ws", "add-repo", "WS1"})
		if code != exitError {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "base_ref not found") {
			t.Fatalf("stderr missing base_ref not found: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1", "repos", "r")); err == nil {
		t.Fatalf("worktree should not be created")
	}

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var repoCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM repos").Scan(&repoCount); err != nil {
		t.Fatalf("query repos count: %v", err)
	}
	if repoCount != 1 {
		t.Fatalf("repos count = %d, want 1", repoCount)
	}

	var wsRepoCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_repos WHERE workspace_id = ?", "WS1").Scan(&wsRepoCount); err != nil {
		t.Fatalf("query workspace_repos count: %v", err)
	}
	if wsRepoCount != 0 {
		t.Fatalf("workspace_repos count = %d, want 0", wsRepoCount)
	}
}

func TestCLI_WS_List_ArchivedWorkspace_DoesNotMarkRepoMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := statestore.EnsureSettings(ctx, db, env.Root, env.RepoPoolPath()); err != nil {
		t.Fatalf("EnsureSettings error: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO workspaces (
  id, generation, status, description, source_url,
  created_at, updated_at,
  archived_commit_sha, reopened_commit_sha
)
VALUES ('WS1', 1, 'archived', '', '', 1, 1, 'deadbeef', NULL)
`); err != nil {
		t.Fatalf("insert workspace: %v", err)
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
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}

	var missingAt sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT missing_at
FROM workspace_repos
WHERE workspace_id = 'WS1' AND repo_uid = 'github.com/o/r'
`).Scan(&missingAt); err != nil {
		t.Fatalf("query missing_at: %v", err)
	}
	if missingAt.Valid {
		t.Fatalf("missing_at should remain NULL for archived workspace, got=%v", missingAt.Int64)
	}
}

func TestCLI_WS_Close_WithStagedChanges_FailsBeforeMutatingWorkspace(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	p := filepath.Join(env.Root, "README.tmp")
	if err := os.WriteFile(p, []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	cmd := exec.Command("git", "add", "README.tmp")
	cmd.Dir = env.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "WS1"})
		if code != exitError {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "git index has staged changes") {
			t.Fatalf("stderr missing staged changes guard: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspaces/WS1 should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err == nil {
		t.Fatalf("archive/WS1 should not exist")
	}
}

func TestCLI_WS_Reopen_WithStagedChanges_FailsBeforeMutatingWorkspace(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	p := filepath.Join(env.Root, "README.tmp")
	if err := os.WriteFile(p, []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	cmd := exec.Command("git", "add", "README.tmp")
	cmd.Dir = env.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "reopen", "WS1"})
		if code != exitError {
			t.Fatalf("ws reopen exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "git index has staged changes") {
			t.Fatalf("stderr missing staged changes guard: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err == nil {
		t.Fatalf("workspaces/WS1 should not exist")
	}
}

func TestCLI_WS_Purge_WithStagedChanges_FailsBeforeDeletingWorkspace(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	p := filepath.Join(env.Root, "README.tmp")
	if err := os.WriteFile(p, []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	cmd := exec.Command("git", "add", "README.tmp")
	cmd.Dir = env.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "purge", "--no-prompt", "--force", "WS1"})
		if code != exitError {
			t.Fatalf("ws purge exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "git index has staged changes") {
			t.Fatalf("stderr missing staged changes guard: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspaces/WS1 should remain: %v", err)
	}
}
