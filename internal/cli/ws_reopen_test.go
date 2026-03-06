package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_Reopen_Help_ShowsUsage(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "reopen", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws reopen") {
		t.Fatalf("stdout missing ws reopen usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Reopen_RestoresWorkspaceRecreatesWorktreesAndCanCommitAndUpdatesDB(t *testing.T) {
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

	repoSpec := prepareRemoteRepoSpec(t, func(dir string, args ...string) {
		runGit(t, dir, args...)
	})
	_, _, _ = seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))

		code := c.Run([]string{"ws", "add-repo", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "reopen", "--commit", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws reopen exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err == nil {
		t.Fatalf("archive/WS1 should not exist after reopen")
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspaces/WS1 should exist after reopen: %v", err)
	}
	worktree := filepath.Join(env.Root, "workspaces", "WS1", "repos", "r")
	if _, err := os.Stat(filepath.Join(worktree, ".git")); err != nil {
		t.Fatalf("reopened worktree missing .git: %v", err)
	}

	subj := strings.TrimSpace(mustGitOutput(t, env.Root, "log", "-1", "--pretty=%s"))
	if subj != "reopen: WS1" {
		t.Fatalf("commit subject = %q, want %q", subj, "reopen: WS1")
	}
	archiveStatus := strings.TrimSpace(mustGitOutput(t, env.Root, "status", "--short", "--", filepath.ToSlash(filepath.Join("archive", "WS1"))))
	if archiveStatus != "" {
		t.Fatalf("archive/WS1 should not remain as unstaged deletion after reopen commit: %q", archiveStatus)
	}

}

func TestCLI_WS_Reopen_RecreatesWorktreesWithoutWorkspaceRepoBindings(t *testing.T) {
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

	repoSpec := prepareRemoteRepoSpec(t, func(dir string, args ...string) {
		runGit(t, dir, args...)
	})
	_, _, _ = seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))

		code := c.Run([]string{"ws", "add-repo", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "reopen", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws reopen exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	worktree := filepath.Join(env.Root, "workspaces", "WS1", "repos", "r")
	if _, err := os.Stat(filepath.Join(worktree, ".git")); err != nil {
		t.Fatalf("reopened worktree missing .git: %v", err)
	}
}

func TestCLI_WS_Reopen_ErrorsWhenBranchCheckedOutElsewhere(t *testing.T) {
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

	repoSpec := prepareRemoteRepoSpec(t, func(dir string, args ...string) {
		runGit(t, dir, args...)
	})
	_, _, _ = seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))

		code := c.Run([]string{"ws", "add-repo", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	spec, err := repospec.Normalize(repoSpec)
	if err != nil {
		t.Fatalf("Normalize(repoSpec): %v", err)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	otherWorktree := filepath.Join(t.TempDir(), "other-worktree")
	runGit(t, "", "--git-dir", barePath, "branch", "-f", "WS1/test", "origin/main")
	runGit(t, "", "--git-dir", barePath, "worktree", "add", otherWorktree, "WS1/test")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "reopen", "--id", "WS1"})
		if code != exitError {
			t.Fatalf("ws reopen exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "already checked out") {
			t.Fatalf("stderr missing branch conflict: %q", err.String())
		}
	}

}

func TestCLI_WS_Reopen_SelectorModeWithoutTTY_Errors(t *testing.T) {
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
		code := c.Run([]string{"ws", "close", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")

		code := c.Run([]string{"ws", "reopen"})
		if code != exitUsage {
			t.Fatalf("ws reopen exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
		}
		if !strings.Contains(err.String(), "ws reopen requires <id>") && !strings.Contains(err.String(), "Usage:") {
			t.Fatalf("stderr missing id requirement: %q", err.String())
		}
	}
}
