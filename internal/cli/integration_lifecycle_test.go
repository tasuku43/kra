package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/testutil"
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
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

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
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))
		code := c.Run([]string{"ws", "add-repo", "--id", "WS1"})
		if code != exitError {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "no repos available in pool") {
			t.Fatalf("stderr missing pool candidate error: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1", "repos", "r")); err == nil {
		t.Fatalf("worktree should not be created on failure")
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

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err == nil {
		t.Fatalf("workspaces/WS1 should be archived after close")
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should exist: %v", err)
	}
}
