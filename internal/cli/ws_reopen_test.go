package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Reopen_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "--act", "reopen", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws --act reopen") {
		t.Fatalf("stdout missing ws reopen usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Reopen_RestoresWorkspaceRecreatesWorktreesCommitsAndUpdatesDB(t *testing.T) {
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

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"init"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	runGit(env.Root, "config", "user.email", "test@example.com")
	runGit(env.Root, "config", "user.name", "test")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	runGit(src, "init", "-b", "main")
	runGit(src, "config", "user.email", "test@example.com")
	runGit(src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(src, "add", ".")
	runGit(src, "commit", "-m", "init")

	remoteBare := filepath.Join(t.TempDir(), "github.com", "o", "r.git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remoteBare dir: %v", err)
	}
	runGit("", "clone", "--bare", src, remoteBare)
	repoSpec := "file://" + remoteBare
	_, _, _ = seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))

		code := c.Run([]string{"ws", "--act", "add-repo", "WS1"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
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

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "--act", "reopen", "WS1"})
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

}

func TestCLI_WS_Reopen_RecreatesWorktreesWithoutWorkspaceRepoBindings(t *testing.T) {
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

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"init"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	runGit(env.Root, "config", "user.email", "test@example.com")
	runGit(env.Root, "config", "user.name", "test")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	runGit(src, "init", "-b", "main")
	runGit(src, "config", "user.email", "test@example.com")
	runGit(src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(src, "add", ".")
	runGit(src, "commit", "-m", "init")

	remoteBare := filepath.Join(t.TempDir(), "github.com", "o", "r.git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remoteBare dir: %v", err)
	}
	runGit("", "clone", "--bare", src, remoteBare)
	repoSpec := "file://" + remoteBare
	_, _, _ = seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))

		code := c.Run([]string{"ws", "--act", "add-repo", "WS1"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
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

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "--act", "reopen", "WS1"})
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

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"init"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	runGit(env.Root, "config", "user.email", "test@example.com")
	runGit(env.Root, "config", "user.name", "test")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	runGit(src, "init", "-b", "main")
	runGit(src, "config", "user.email", "test@example.com")
	runGit(src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(src, "add", ".")
	runGit(src, "commit", "-m", "init")

	remoteBare := filepath.Join(t.TempDir(), "github.com", "o", "r.git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remoteBare dir: %v", err)
	}
	runGit("", "clone", "--bare", src, remoteBare)
	repoSpec := "file://" + remoteBare
	_, _, _ = seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))

		code := c.Run([]string{"ws", "--act", "add-repo", "WS1"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
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

	spec, err := repospec.Normalize(repoSpec)
	if err != nil {
		t.Fatalf("Normalize(repoSpec): %v", err)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	otherWorktree := filepath.Join(t.TempDir(), "other-worktree")
	runGit("", "--git-dir", barePath, "worktree", "add", otherWorktree, "WS1/test")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "--act", "reopen", "WS1"})
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
		code := c.Run([]string{"ws", "--act", "close", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")

		code := c.Run([]string{"ws", "--act", "reopen"})
		if code != exitUsage {
			t.Fatalf("ws reopen exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
		}
		if !strings.Contains(err.String(), "ws reopen requires <id>") && !strings.Contains(err.String(), "Usage:") {
			t.Fatalf("stderr missing id requirement: %q", err.String())
		}
	}
}
