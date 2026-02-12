package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
