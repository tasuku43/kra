package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/statestore"
	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_AddRepo_CorruptedRepoPool_FailsWithoutStateMutation(t *testing.T) {
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
	spec, err := repospec.Normalize(repoSpec)
	if err != nil {
		t.Fatalf("Normalize(repoSpec): %v", err)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	if err := os.MkdirAll(filepath.Dir(barePath), 0o755); err != nil {
		t.Fatalf("mkdir bare path parent: %v", err)
	}
	if err := os.WriteFile(barePath, []byte("not-a-bare-repo\n"), 0o644); err != nil {
		t.Fatalf("write corrupted bare path: %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("\nWS1/test\n")
		code := c.Run([]string{"ws", "add-repo", "WS1", repoSpec})
		if code != exitError {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "ensure bare repo") {
			t.Fatalf("stderr missing ensure bare repo error: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1", "repos", "r")); err == nil {
		t.Fatalf("worktree should not be created on failure")
	}

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var cnt int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_repos WHERE workspace_id = ?", "WS1").Scan(&cnt); err != nil {
		t.Fatalf("query workspace_repos count: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("workspace_repos count = %d, want 0", cnt)
	}
}

func TestCLI_WS_Close_RepoMetadataDrift_FailsWithoutArchiving(t *testing.T) {
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
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("\nWS1/test\n")
		code := c.Run([]string{"ws", "add-repo", "WS1", repoSpec})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM repos WHERE repo_uid = ?", "github.com/o/r"); err != nil {
		t.Fatalf("delete repos row: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "WS1"})
		if code != exitError {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "repo not found in repos table") {
			t.Fatalf("stderr missing repo metadata drift error: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspaces/WS1 should remain after failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err == nil {
		t.Fatalf("archive/WS1 should not exist on failure")
	}

	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM workspaces WHERE id = ?", "WS1").Scan(&status); err != nil {
		t.Fatalf("query workspaces status: %v", err)
	}
	if status != "active" {
		t.Fatalf("workspace status = %q, want %q", status, "active")
	}
}
