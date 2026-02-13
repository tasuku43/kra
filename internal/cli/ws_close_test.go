package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/core/workspacerisk"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_Close_Help_ShowsUsage(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "--act", "close", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws --act close") {
		t.Fatalf("stdout missing ws close usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Close_ArchivesWorkspaceRemovesWorktreesCommitsAndUpdatesDB(t *testing.T) {
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
		code := c.Run([]string{"init", "--root", env.Root, "--context", "ws-close"})
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

	// Prepare a local "remote" bare repo addressable via a file:// spec that ends with <host>/<owner>/<repo>.
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
		code := c.Run([]string{"ws", "--act", "close", "--commit", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if strings.Contains(err.String(), "type yes to apply close on non-clean workspaces:") {
			t.Fatalf("clean close should not require confirmation: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err == nil {
		t.Fatalf("workspaces/WS1 should not exist after close")
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should exist after close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1", "repos")); err == nil {
		t.Fatalf("archive/WS1/repos should not exist after close")
	}
	metaBytes, readErr := os.ReadFile(filepath.Join(env.Root, "archive", "WS1", workspaceMetaFilename))
	if readErr != nil {
		t.Fatalf("read %s: %v", workspaceMetaFilename, readErr)
	}
	var meta workspaceMetaFile
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal %s: %v", workspaceMetaFilename, err)
	}
	if meta.Workspace.Status != "archived" {
		t.Fatalf("workspace meta status = %q, want %q", meta.Workspace.Status, "archived")
	}
	if len(meta.ReposRestore) != 1 {
		t.Fatalf("repos_restore length = %d, want %d", len(meta.ReposRestore), 1)
	}
	if got := meta.ReposRestore[0]; got.Alias != "r" || got.Branch != "WS1/test" {
		t.Fatalf("repos_restore[0] = %+v, want alias=%q branch=%q", got, "r", "WS1/test")
	}

	subj := strings.TrimSpace(mustGitOutput(t, env.Root, "log", "-1", "--pretty=%s"))
	if subj != "archive: WS1" {
		t.Fatalf("commit subject = %q, want %q", subj, "archive: WS1")
	}

}

func TestCLI_WS_Close_DirtyRepo_PromptsAndCanAbort(t *testing.T) {
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
		code := c.Run([]string{"init", "--root", env.Root, "--context", "ws-close"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	runGit(env.Root, "config", "user.email", "test@example.com")
	runGit(env.Root, "config", "user.name", "test")

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

	worktreePath := filepath.Join(env.Root, "workspaces", "WS1", "repos", "r")
	if err := os.WriteFile(filepath.Join(worktreePath, "DIRTY.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("n\n")

		code := c.Run([]string{"ws", "--act", "close", "WS1"})
		if code != exitError {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "type yes to apply close on non-clean workspaces:") {
			t.Fatalf("stderr missing confirmation prompt: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspaces/WS1 should still exist after abort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err == nil {
		t.Fatalf("archive/WS1 should not exist after abort")
	}
}

func TestCLI_WS_Close_SelectorModeWithoutTTY_Errors(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"init", "--root", env.Root, "--context", "ws-close"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
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
		c.In = strings.NewReader("")

		code := c.Run([]string{"ws", "--act", "close"})
		if code != exitUsage {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
		}
		if !strings.Contains(err.String(), "ws close requires --id <id> or active workspace context") && !strings.Contains(err.String(), "Usage:") {
			t.Fatalf("stderr missing id requirement: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspace should remain: %v", err)
	}
}

func TestCLI_WS_Close_ShiftsProcessCWDWhenInsideTargetWorkspace(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"init", "--root", env.Root, "--context", "ws-close"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	wsDir := filepath.Join(env.Root, "workspaces", "WS1")
	if err := os.Chdir(wsDir); err != nil {
		t.Fatalf("Chdir(%s) error: %v", wsDir, err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"ws", "--act", "close", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	afterWD, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("Getwd() after close error: %v", wdErr)
	}
	afterResolved := afterWD
	if resolved, err := filepath.EvalSymlinks(afterWD); err == nil {
		afterResolved = resolved
	}
	rootResolved := env.Root
	if resolved, err := filepath.EvalSymlinks(env.Root); err == nil {
		rootResolved = resolved
	}
	if afterResolved != rootResolved {
		t.Fatalf("process cwd = %q (resolved=%q), want %q (resolved=%q)", afterWD, afterResolved, env.Root, rootResolved)
	}
}

func TestCLI_WS_Close_AllowsUnrelatedPreStagedChangesOutsideWorkspaceAllowlist(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"init", "--root", env.Root, "--context", "ws-close"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	runGit(t, env.Root, "config", "user.email", "test@example.com")
	runGit(t, env.Root, "config", "user.name", "test")
	unrelated := filepath.Join(env.Root, "UNRELATED.md")
	if err := os.WriteFile(unrelated, []byte("keep staged\n"), 0o644); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	runGit(t, env.Root, "add", "UNRELATED.md")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "--act", "close", "--commit", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should exist after close: %v", err)
	}

	staged := strings.TrimSpace(mustGitOutput(t, env.Root, "diff", "--cached", "--name-only"))
	if !strings.Contains(staged, "UNRELATED.md") {
		t.Fatalf("unrelated staged file should remain staged: %q", staged)
	}

	subj := strings.TrimSpace(mustGitOutput(t, env.Root, "log", "-1", "--pretty=%s"))
	if subj != "archive: WS1" {
		t.Fatalf("commit subject = %q, want %q", subj, "archive: WS1")
	}
}

func TestPrintCloseRiskSection_UsesSharedSpacingAndIndent(t *testing.T) {
	var out bytes.Buffer
	items := []workspaceRiskDetail{
		{
			id:   "WS1",
			risk: workspacerisk.WorkspaceRiskDirty,
			perRepo: []repoRiskItem{
				{alias: "repo-a", state: workspacerisk.RepoStateDirty},
			},
		},
		{
			id:      "WS2",
			risk:    workspacerisk.WorkspaceRiskClean,
			perRepo: []repoRiskItem{{alias: "repo-b", state: workspacerisk.RepoStateClean}},
		},
	}

	printRiskSection(&out, items, false)
	got := out.String()

	if !strings.HasPrefix(got, "Plan:\n") {
		t.Fatalf("close plan section should start with plan heading: %q", got)
	}
	if !strings.Contains(got, "\n  • close 2 workspaces\n") {
		t.Fatalf("plan heading row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n  • workspace WS1\n") {
		t.Fatalf("workspace label row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n    └─ repo-a\n") {
		t.Fatalf("repo tree row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n       risk: dirty\n") {
		t.Fatalf("repo risk row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n       sync: upstream=(none) ahead=0 behind=0\n") {
		t.Fatalf("sync row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n  • workspace WS2\n") {
		t.Fatalf("second workspace label row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n    └─ repo-b\n") {
		t.Fatalf("repo risk detail indentation mismatch: %q", got)
	}
}

func TestEnsureRootGitWorktree_AllowsNestedRootInSameGitWorktree(t *testing.T) {
	testutil.RequireCommand(t, "git")

	parent := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	run(parent, "init", "-b", "main")

	root := filepath.Join(parent, "nested", "work")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir nested root: %v", err)
	}
	if err := ensureRootGitWorktree(context.Background(), root); err != nil {
		t.Fatalf("ensureRootGitWorktree() error = %v, want nil", err)
	}
}

func TestEnsureRootGitWorktree_RejectsRootOutsideGitWorktree(t *testing.T) {
	testutil.RequireCommand(t, "git")

	repo := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	run(repo, "init", "-b", "main")

	outside := t.TempDir()
	err := ensureRootGitWorktree(context.Background(), outside)
	if err == nil {
		t.Fatalf("ensureRootGitWorktree() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "KRA_ROOT must be a git working tree") {
		t.Fatalf("error = %q, want git working tree guidance", err.Error())
	}
}

func TestCommitArchiveChange_AllowlistsNestedRootRelativePaths(t *testing.T) {
	testutil.RequireCommand(t, "git")

	parent := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	run(parent, "init", "-b", "main")
	run(parent, "config", "user.email", "test@example.com")
	run(parent, "config", "user.name", "test")

	root := filepath.Join(parent, "tasuku-yamashita", "work")
	wsID := "DEMO-0000"
	archiveDir := filepath.Join(root, "archive", wsID)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".claude/settings.local.json\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	metaPath := filepath.Join(archiveDir, workspaceMetaFilename)
	if err := os.WriteFile(metaPath, []byte(`{"workspace":{"id":"DEMO-0000","status":"archived"}}`), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	ignoredPath := filepath.Join(archiveDir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(ignoredPath), 0o755); err != nil {
		t.Fatalf("mkdir ignored dir: %v", err)
	}
	if err := os.WriteFile(ignoredPath, []byte("{\"local\":true}\n"), 0o644); err != nil {
		t.Fatalf("write ignored settings: %v", err)
	}

	sha, err := commitArchiveChange(context.Background(), root, wsID, []string{
		workspaceMetaFilename,
		".claude/settings.local.json",
	})
	if err != nil {
		t.Fatalf("commitArchiveChange() error = %v, want nil", err)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("commitArchiveChange() sha is empty")
	}
}

func mustGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}
