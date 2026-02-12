package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
		code := c.Run([]string{"ws", "--act", "add-repo", "WS1"})
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

}

func TestCLI_WS_List_ArchivedWorkspace_DoesNotMarkRepoMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	archived := filepath.Join(env.Root, "archive", "WS1")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatalf("create archived workspace: %v", err)
	}
	meta := newWorkspaceMetaFileForCreate("WS1", "", "", 1)
	meta.Workspace.Status = "archived"
	meta.ReposRestore = []workspaceMetaRepoRestore{{
		RepoUID:   "github.com/o/r",
		RepoKey:   "o/r",
		RemoteURL: "https://example.com/o/r.git",
		Alias:     "r",
		Branch:    "main",
		BaseRef:   "origin/main",
	}}
	if err := writeWorkspaceMetaFile(archived, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}

	if strings.Contains(out.String(), "WS1") {
		t.Fatalf("default ws list should not include archived workspace: %q", out.String())
	}
}

func TestCLI_WS_Close_WithStagedChanges_CommitsOnlyWorkspaceScope(t *testing.T) {
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
		code := c.Run([]string{"ws", "--act", "close", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err == nil {
		t.Fatalf("workspaces/WS1 should not exist")
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should exist: %v", err)
	}

	staged := strings.TrimSpace(mustGitOutput(t, env.Root, "diff", "--cached", "--name-only"))
	if !strings.Contains(staged, "README.tmp") {
		t.Fatalf("pre-existing staged file should remain staged: %q", staged)
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
		code := c.Run([]string{"ws", "--act", "close", "WS1"})
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
		code := c.Run([]string{"ws", "--act", "reopen", "WS1"})
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
		code := c.Run([]string{"ws", "--act", "purge", "--no-prompt", "--force", "WS1"})
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
