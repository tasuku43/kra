package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/statestore"
	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Purge_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "purge", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws purge") {
		t.Fatalf("stdout missing ws purge usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Purge_ArchivedWorkspace_DeletesPathsCommitsAndUpdatesDB(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should exist before purge: %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("y\n")
		code := c.Run([]string{"ws", "purge", "WS1"})
		if code != exitOK {
			t.Fatalf("ws purge exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if !strings.Contains(err.String(), "purge workspace WS1?") {
			t.Fatalf("stderr missing purge confirmation prompt: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err == nil {
		t.Fatalf("workspaces/WS1 should not exist after purge")
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err == nil {
		t.Fatalf("archive/WS1 should not exist after purge")
	}

	subj := strings.TrimSpace(mustGitOutput(t, env.Root, "log", "-1", "--pretty=%s"))
	if subj != "purge: WS1" {
		t.Fatalf("commit subject = %q, want %q", subj, "purge: WS1")
	}

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var cnt int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces WHERE id = ?", "WS1").Scan(&cnt); err != nil {
		t.Fatalf("query workspace count: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("workspace row count = %d, want 0", cnt)
	}

	var eventType string
	if err := db.QueryRowContext(ctx, `
SELECT event_type
FROM workspace_events
WHERE workspace_id = ?
ORDER BY id DESC
LIMIT 1
`, "WS1").Scan(&eventType); err != nil {
		t.Fatalf("query last event: %v", err)
	}
	if eventType != "purged" {
		t.Fatalf("last event_type = %q, want %q", eventType, "purged")
	}
}

func TestCLI_WS_Purge_NoPromptWithoutForce_Refuses(t *testing.T) {
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

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "purge", "--no-prompt", "WS1"})
	if code != exitUsage {
		t.Fatalf("ws purge exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--no-prompt requires --force") {
		t.Fatalf("stderr missing refusal reason: %q", err.String())
	}
}

func TestCLI_WS_Purge_ActiveDirtyRepo_AsksSecondConfirmationAndCanAbort(t *testing.T) {
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
		code := c.Run([]string{"ws", "add-repo", "WS1"})
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
		c.In = strings.NewReader("y\nn\n")
		code := c.Run([]string{"ws", "purge", "WS1"})
		if code != exitError {
			t.Fatalf("ws purge exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "purge workspace WS1?") {
			t.Fatalf("stderr missing first confirmation prompt: %q", err.String())
		}
		if !strings.Contains(err.String(), "workspace has risk; continue purging?") {
			t.Fatalf("stderr missing second confirmation prompt: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspaces/WS1 should still exist after abort: %v", err)
	}
}

func TestCLI_WS_Purge_NoPromptForce_ActiveWorkspace_Succeeds(t *testing.T) {
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

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "purge", "--no-prompt", "--force", "WS1"})
	if code != exitOK {
		t.Fatalf("ws purge exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); statErr == nil {
		t.Fatalf("workspaces/WS1 should not exist after purge")
	}
}

func TestCLI_WS_Purge_SelectorModeWithoutTTY_Errors(t *testing.T) {
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

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")

		code := c.Run([]string{"ws", "purge"})
		if code != exitError {
			t.Fatalf("ws purge exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "interactive workspace selection requires a TTY") {
			t.Fatalf("stderr missing non-tty error: %q", err.String())
		}
	}
}

func TestCLI_WS_Purge_NoPromptForce_WithoutID_Refuses(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "purge", "--no-prompt", "--force"})
	if code != exitUsage {
		t.Fatalf("ws purge exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--no-prompt selector mode is not supported") {
		t.Fatalf("stderr missing refusal reason: %q", err.String())
	}
}

func TestPrintPurgeRiskSection_UsesSharedIndent(t *testing.T) {
	var out bytes.Buffer
	selectedIDs := []string{"WS1"}
	riskMeta := map[string]purgeWorkspaceMeta{
		"WS1": {
			status: "active",
			risk:   workspacerisk.WorkspaceRiskDirty,
			perRepo: []repoRiskItem{
				{alias: "repo1", state: workspacerisk.RepoStateDirty},
			},
		},
	}

	printPurgeRiskSection(&out, selectedIDs, riskMeta, false)
	got := out.String()
	if !strings.Contains(got, "\nRisk:\n  purge is permanent and cannot be undone.\n  selected: 1\n") {
		t.Fatalf("risk section header/body indentation mismatch: %q", got)
	}
	if !strings.Contains(got, "\n  active workspace risk detected:\n  - WS1 [dirty]\n    - repo1\tdirty\n") {
		t.Fatalf("risk detail indentation mismatch: %q", got)
	}
}

func initAndConfigureRootRepo(t *testing.T, root string) {
	t.Helper()
	testutil.RequireCommand(t, "git")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"init"})
		if code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	cmd := exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}

	cmd = exec.Command("git", "config", "user.name", "test")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}
}

func prepareRemoteRepoSpec(t *testing.T, runGit func(dir string, args ...string)) string {
	t.Helper()

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
	return "file://" + remoteBare
}
