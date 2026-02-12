package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_RemoveRepo_Help(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "--act", "remove-repo", "--help"})
	if code != exitOK {
		t.Fatalf("ws remove-repo help exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws --act remove-repo") {
		t.Fatalf("stdout missing ws remove-repo usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_RemoveRepo_JSON_RemovesBindingAndWorktree(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := createTestRemoteRepoSpec(t)
	_, repoKey, alias := seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "WS1"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	worktreePath := filepath.Join(env.Root, "workspaces", "WS1", "repos", alias)
	if _, err := os.Stat(filepath.Join(worktreePath, ".git")); err != nil {
		t.Fatalf("worktree missing before remove: %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{
			"ws", "--act", "remove-repo",
			"--format", "json",
			"--id", "WS1",
			"--repo", repoKey,
			"--yes",
		})
		if code != exitOK {
			t.Fatalf("ws remove-repo json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		resp := decodeJSONResponse(t, out.String())
		if !resp.OK || resp.Action != "remove-repo" || resp.WorkspaceID != "WS1" {
			t.Fatalf("unexpected json response: %+v", resp)
		}
		if got := resp.Result["removed"]; got != float64(1) {
			t.Fatalf("result.removed = %v, want 1", got)
		}
	}

	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree should be removed: %v", err)
	}
	metaBytes, err := os.ReadFile(filepath.Join(env.Root, "workspaces", "WS1", workspaceMetaFilename))
	if err != nil {
		t.Fatalf("read %s: %v", workspaceMetaFilename, err)
	}
	var meta workspaceMetaFile
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal %s: %v", workspaceMetaFilename, err)
	}
	if len(meta.ReposRestore) != 0 {
		t.Fatalf("repos_restore length = %d, want 0", len(meta.ReposRestore))
	}
}

func TestCLI_WS_RemoveRepo_JSON_RiskyRequiresForce(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := createTestRemoteRepoSpec(t)
	_, repoKey, alias := seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "WS1"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	worktreePath := filepath.Join(env.Root, "workspaces", "WS1", "repos", alias)
	dirtyFile := filepath.Join(worktreePath, "DIRTY.txt")
	if err := os.WriteFile(dirtyFile, []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{
		"ws", "--act", "remove-repo",
		"--format", "json",
		"--id", "WS1",
		"--repo", repoKey,
		"--yes",
	})
	if code != exitError {
		t.Fatalf("ws remove-repo json exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "remove-repo" || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if _, statErr := os.Stat(worktreePath); statErr != nil {
		t.Fatalf("worktree should remain on preflight conflict: %v", statErr)
	}
}

func TestCLI_WS_RemoveRepo_JSON_ShiftsCWDWhenInsideTargetWorkspace(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := createTestRemoteRepoSpec(t)
	_, repoKey, alias := seedRepoPoolAndState(t, env, repoSpec)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "WS1/test"))
		if code := c.Run([]string{"ws", "--act", "add-repo", "WS1"}); code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	worktreePath := filepath.Join(env.Root, "workspaces", "WS1", "repos", alias)
	if err := os.Chdir(worktreePath); err != nil {
		t.Fatalf("Chdir(%s) error: %v", worktreePath, err)
	}

	actionFile := filepath.Join(t.TempDir(), "action.sh")
	t.Setenv(shellActionFileEnv, actionFile)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{
		"ws", "--act", "remove-repo",
		"--format", "json",
		"--id", "WS1",
		"--repo", repoKey,
		"--yes",
	})
	if code != exitOK {
		t.Fatalf("ws remove-repo json exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}

	afterWD, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("Getwd() after remove-repo error: %v", wdErr)
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

	actionBytes, actionErr := os.ReadFile(actionFile)
	if actionErr != nil {
		t.Fatalf("read action file: %v", actionErr)
	}
	action := string(actionBytes)
	if !strings.Contains(action, env.Root) {
		t.Fatalf("shell action should include root path: %q", action)
	}
}

func TestPrintRemoveRepoResult_UsesSameSuccessPatternAsAddRepo(t *testing.T) {
	var out bytes.Buffer
	printRemoveRepoResult(&out, []removeRepoCandidate{
		{RepoKey: "example-org/ai-secretary-tools"},
		{RepoKey: "example-org/astraea-tools"},
	}, false)

	got := out.String()
	if !strings.Contains(got, "Removed 2 / 2") {
		t.Fatalf("result summary missing: %q", got)
	}
	if !strings.Contains(got, "• ✔ example-org/ai-secretary-tools") {
		t.Fatalf("result line should use check marker: %q", got)
	}
	if !strings.Contains(got, "• ✔ example-org/astraea-tools") {
		t.Fatalf("result line should use check marker: %q", got)
	}
}

func TestPrintRemoveRepoPlan_ShowsFilesSectionForDirtyRepo(t *testing.T) {
	var out bytes.Buffer
	selected := []removeRepoCandidate{
		{RepoKey: "example-org/ai-secretary-tools", Alias: "ai-secretary-tools"},
	}
	details := []removeRepoPlanDetail{
		{
			candidate: selected[0],
			branch:    "DEMO-0000",
			state:     workspacerisk.RepoStateDirty,
			upstream:  "origin/main",
			ahead:     2,
			behind:    0,
			unstaged:  2,
			files: []string{
				" M docs/backlog/README.md",
				" M docs/backlog/UX-WS.md",
			},
		},
	}

	printRemoveRepoPlan(&out, "DEMO-0000", selected, details, false)
	got := out.String()
	if !strings.Contains(got, "files:") {
		t.Fatalf("plan should include files section: %q", got)
	}
	if !strings.Contains(got, "(branch: DEMO-0000)") {
		t.Fatalf("plan should include branch on repo line: %q", got)
	}
	if !strings.Contains(got, "M docs/backlog/README.md") {
		t.Fatalf("plan should include changed file line: %q", got)
	}
	if !strings.Contains(got, "M docs/backlog/UX-WS.md") {
		t.Fatalf("plan should include changed file line: %q", got)
	}
}

func createTestRemoteRepoSpec(t *testing.T) string {
	t.Helper()

	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	runGit(t, src, "init", "-b", "main")
	runGit(t, src, "config", "user.email", "test@example.com")
	runGit(t, src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "-m", "init")

	remoteBare := filepath.Join(t.TempDir(), "github.com", "tasuku43", "sample.git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remote bare dir: %v", err)
	}
	runGit(t, "", "clone", "--bare", src, remoteBare)
	return "file://" + remoteBare
}
