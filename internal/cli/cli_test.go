package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_Root_NoArgs_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run(nil)
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "Usage:") {
		t.Fatalf("stderr missing usage: %q", err.String())
	}
}

func TestCLI_Root_Help_ExitOK(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("stdout missing usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_UnknownCommand_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"nope"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "unknown command") || !strings.Contains(err.String(), "Usage:") {
		t.Fatalf("stderr missing error+usage: %q", err.String())
	}
}

func TestCLI_WS_NoArgs_ShowsWSUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))
	t.Setenv("GIONX_ROOT", "")

	code := c.Run([]string{"ws"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "resolve GIONX_ROOT:") {
		t.Fatalf("stderr missing root resolution error: %q", err.String())
	}
}

func TestCLI_WS_ListAlias_LS_DelegatesToList(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "ls"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Workspaces(active):") {
		t.Fatalf("stdout missing workspace heading: %q", out.String())
	}
}

func TestCLI_WS_SelectFlagRejected(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "go", args: []string{"ws", "--act", "go", "--select", "WS-1"}},
		{name: "close", args: []string{"ws", "--act", "close", "--select", "WS-1"}},
		{name: "add-repo", args: []string{"ws", "--act", "add-repo", "--select", "WS-1"}},
		{name: "reopen", args: []string{"ws", "--act", "reopen", "--select", "WS-1"}},
		{name: "purge", args: []string{"ws", "--act", "purge", "--select", "WS-1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var err bytes.Buffer
			c := New(&out, &err)
			code := c.Run(tc.args)
			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
			}
			if !strings.Contains(err.String(), "unknown flag") {
				t.Fatalf("stderr missing flag error: %q", err.String())
			}
		})
	}
}

func TestCLI_WS_Select_UnsupportedAct(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "select", "--act", "unknown"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
	}
	if !strings.Contains(err.String(), "unsupported --act") {
		t.Fatalf("stderr missing unsupported act error: %q", err.String())
	}
}

func TestCLI_WS_Select_IDFlagRejected(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "select", "--id", "WS-1"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
	}
	if !strings.Contains(err.String(), "ws select does not support --id") {
		t.Fatalf("stderr missing --id rejection: %q", err.String())
	}
}

func TestCLI_WS_Select_ActScopeMismatch(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "select", "--archived", "--act", "go"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
	}
	if !strings.Contains(err.String(), "cannot be used with --archived") {
		t.Fatalf("stderr missing scope mismatch: %q", err.String())
	}
}

func TestCLI_WS_Create_NotImplemented(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "create"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "gionx ws create") {
		t.Fatalf("stderr missing ws create usage: %q", err.String())
	}
}

func TestCLI_Init_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"init", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx init") {
		t.Fatalf("stdout missing init usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_Init_CreatesLayoutGitignoreGitRepoAndSettings(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	setGitIdentity(t)

	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"init"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
	if got := out.String(); !strings.Contains(got, "Result:") || !strings.Contains(got, "Initialized: "+root) {
		t.Fatalf("stdout missing result section: %q", got)
	}

	if _, statErr := os.Stat(filepath.Join(root, "workspaces")); statErr != nil {
		t.Fatalf("workspaces/ not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "archive")); statErr != nil {
		t.Fatalf("archive/ not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "AGENTS.md")); statErr != nil {
		t.Fatalf("AGENTS.md not created: %v", statErr)
	}
	b, statErr := os.ReadFile(filepath.Join(root, ".gitignore"))
	if statErr != nil {
		t.Fatalf(".gitignore not created: %v", statErr)
	}
	if !strings.Contains(string(b), "workspaces/**/repos/**") {
		t.Fatalf(".gitignore missing pattern: %q", string(b))
	}
	if _, statErr := os.Stat(filepath.Join(root, ".git")); statErr != nil {
		t.Fatalf(".git not created: %v", statErr)
	}
	commitCount := runGit(t, root, "rev-list", "--count", "HEAD")
	if strings.TrimSpace(commitCount) != "1" {
		t.Fatalf("init commit count = %q, want %q", strings.TrimSpace(commitCount), "1")
	}
	tracked := runGit(t, root, "ls-files")
	if strings.TrimSpace(tracked) != ".gitignore\nAGENTS.md" && strings.TrimSpace(tracked) != "AGENTS.md\n.gitignore" {
		t.Fatalf("tracked files = %q, want only .gitignore and AGENTS.md", strings.TrimSpace(tracked))
	}

}

func TestCLI_Init_CreatesMissingGIONXRootDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	setGitIdentity(t)

	parent := t.TempDir()
	root := filepath.Join(parent, "new-gionx-root")
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"init"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
	if got := out.String(); !strings.Contains(got, "Result:") || !strings.Contains(got, "Initialized: "+root) {
		t.Fatalf("stdout missing result section: %q", got)
	}

	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("root dir not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "workspaces")); statErr != nil {
		t.Fatalf("workspaces/ not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "archive")); statErr != nil {
		t.Fatalf("archive/ not created: %v", statErr)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}

func setGitIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "gionx-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "gionx-test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "gionx-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "gionx-test@example.com")
}

func TestCLI_WS_Create_CreatesScaffoldAndStateStoreRows(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("hello world\n")

	code := c.Run([]string{"ws", "create", "MVP-020"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if got := out.String(); !strings.Contains(got, "Result:") || !strings.Contains(got, "Created 1 / 1") || !strings.Contains(got, "✔ MVP-020") {
		t.Fatalf("stdout missing ws create result section: %q", got)
	}

	wsDir := filepath.Join(root, "workspaces", "MVP-020")
	if _, statErr := os.Stat(wsDir); statErr != nil {
		t.Fatalf("workspace dir not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(wsDir, "notes")); statErr != nil {
		t.Fatalf("notes/ not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(wsDir, "artifacts")); statErr != nil {
		t.Fatalf("artifacts/ not created: %v", statErr)
	}
	agentsBytes, statErr := os.ReadFile(filepath.Join(wsDir, "AGENTS.md"))
	if statErr != nil {
		t.Fatalf("AGENTS.md not created: %v", statErr)
	}
	if !strings.Contains(string(agentsBytes), "ID: MVP-020") {
		t.Fatalf("AGENTS.md missing id: %q", string(agentsBytes))
	}
	if !strings.Contains(string(agentsBytes), "Title: hello world") {
		t.Fatalf("AGENTS.md missing title: %q", string(agentsBytes))
	}
	metaBytes, statErr := os.ReadFile(filepath.Join(wsDir, workspaceMetaFilename))
	if statErr != nil {
		t.Fatalf("%s not created: %v", workspaceMetaFilename, statErr)
	}
	var meta workspaceMetaFile
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal %s: %v", workspaceMetaFilename, err)
	}
	if meta.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want %d", meta.SchemaVersion, 1)
	}
	if meta.Workspace.ID != "MVP-020" {
		t.Fatalf("workspace.id = %q, want %q", meta.Workspace.ID, "MVP-020")
	}
	if meta.Workspace.Title != "hello world" {
		t.Fatalf("workspace.title = %q, want %q", meta.Workspace.Title, "hello world")
	}
	if meta.Workspace.Status != "active" {
		t.Fatalf("workspace.status = %q, want %q", meta.Workspace.Status, "active")
	}
	if len(meta.ReposRestore) != 0 {
		t.Fatalf("repos_restore length = %d, want %d", len(meta.ReposRestore), 0)
	}
	if meta.Workspace.CreatedAt <= 0 || meta.Workspace.UpdatedAt <= 0 {
		t.Fatalf("workspace timestamps should be positive: created_at=%d updated_at=%d", meta.Workspace.CreatedAt, meta.Workspace.UpdatedAt)
	}
	if meta.Workspace.CreatedAt != meta.Workspace.UpdatedAt {
		t.Fatalf("workspace created/updated should match on create: created_at=%d updated_at=%d", meta.Workspace.CreatedAt, meta.Workspace.UpdatedAt)
	}

}

func TestCLI_WS_Create_ArchivedCollision_GuidesReopen(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	if err := os.MkdirAll(filepath.Join(root, "archive", "MVP-020"), 0o755); err != nil {
		t.Fatalf("create archived workspace dir: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("should-not-be-read\n")

	code := c.Run([]string{"ws", "create", "MVP-020"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "ws --act reopen") {
		t.Fatalf("stderr missing reopen guidance: %q", err.String())
	}
}

func TestCLI_WS_Create_ActiveCollision_Errors(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	if err := os.MkdirAll(filepath.Join(root, "workspaces", "MVP-020"), 0o755); err != nil {
		t.Fatalf("create active workspace dir: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("should-not-be-read\n")

	code := c.Run([]string{"ws", "create", "MVP-020"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "already exists") {
		t.Fatalf("stderr missing already-exists: %q", err.String())
	}
}

func TestCLI_WS_AddRepo_CreatesWorktreeAndRecordsState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
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

	remoteBare := filepath.Join(t.TempDir(), "github.com", "tasuku43", "sample.git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remoteBare dir: %v", err)
	}
	runGit("", "clone", "--bare", src, remoteBare)
	repoSpec := "file://" + remoteBare

	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	env := testutil.Env{Root: root, DataHome: dataHome, CacheHome: cacheHome}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)

		code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		_, _, _ = seedRepoPoolAndState(t, env, repoSpec)
		c.In = strings.NewReader(addRepoSelectionInput("", "MVP-020/test"))

		code := c.Run([]string{"ws", "--act", "add-repo", "MVP-020"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}

		worktreePath := filepath.Join(root, "workspaces", "MVP-020", "repos", "sample")
		if _, statErr := os.Stat(filepath.Join(worktreePath, ".git")); statErr != nil {
			t.Fatalf("worktree .git missing: %v", statErr)
		}
	}

	metaBytes, readErr := os.ReadFile(filepath.Join(root, "workspaces", "MVP-020", workspaceMetaFilename))
	if readErr != nil {
		t.Fatalf("read %s: %v", workspaceMetaFilename, readErr)
	}
	var meta workspaceMetaFile
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal %s: %v", workspaceMetaFilename, err)
	}
	if len(meta.ReposRestore) != 1 {
		t.Fatalf("repos_restore length = %d, want %d", len(meta.ReposRestore), 1)
	}
	got := meta.ReposRestore[0]
	if got.RepoUID != "github.com/tasuku43/sample" {
		t.Fatalf("repos_restore.repo_uid = %q, want %q", got.RepoUID, "github.com/tasuku43/sample")
	}
	if got.RepoKey != "tasuku43/sample" {
		t.Fatalf("repos_restore.repo_key = %q, want %q", got.RepoKey, "tasuku43/sample")
	}
	if got.RemoteURL != repoSpec {
		t.Fatalf("repos_restore.remote_url = %q, want %q", got.RemoteURL, repoSpec)
	}
	if got.Alias != "sample" {
		t.Fatalf("repos_restore.alias = %q, want %q", got.Alias, "sample")
	}
	if got.Branch != "MVP-020/test" {
		t.Fatalf("repos_restore.branch = %q, want %q", got.Branch, "MVP-020/test")
	}
	if got.BaseRef != "origin/main" {
		t.Fatalf("repos_restore.base_ref = %q, want %q", got.BaseRef, "origin/main")
	}
}

func TestCLI_WS_AddRepo_DBUnavailable_FallsBackToFilesystem(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}

	// Prepare a local remote bare repo.
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

	remoteBare := filepath.Join(t.TempDir(), "github.com", "tasuku43", "sample.git")
	if err := os.MkdirAll(filepath.Dir(remoteBare), 0o755); err != nil {
		t.Fatalf("mkdir remoteBare dir: %v", err)
	}
	runGit("", "clone", "--bare", src, remoteBare)
	repoSpec := "file://" + remoteBare

	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	env := testutil.Env{Root: root, DataHome: dataHome, CacheHome: cacheHome}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-021"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	// Seed repo-pool, then break SQLite schema to force DB fallback path.
	{
		_, _, _ = seedRepoPoolAndState(t, env, repoSpec)
		if err := os.Remove(env.StateDBPath()); err != nil {
			t.Fatalf("remove state db: %v", err)
		}
		if err := os.MkdirAll(env.StateDBPath(), 0o755); err != nil {
			t.Fatalf("break state db path: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "workspaces", "MVP-021", "repos"), 0o755); err != nil {
			t.Fatalf("prepare workspace dir: %v", err)
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader(addRepoSelectionInput("", "MVP-021/test"))

		code := c.Run([]string{"ws", "--act", "add-repo", "MVP-021"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}

		worktreePath := filepath.Join(root, "workspaces", "MVP-021", "repos", "sample")
		if _, statErr := os.Stat(filepath.Join(worktreePath, ".git")); statErr != nil {
			t.Fatalf("worktree .git missing: %v", statErr)
		}
	}

	metaBytes, readErr := os.ReadFile(filepath.Join(root, "workspaces", "MVP-021", workspaceMetaFilename))
	if readErr != nil {
		t.Fatalf("read %s: %v", workspaceMetaFilename, readErr)
	}
	var meta workspaceMetaFile
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal %s: %v", workspaceMetaFilename, err)
	}
	if len(meta.ReposRestore) != 1 {
		t.Fatalf("repos_restore length = %d, want %d", len(meta.ReposRestore), 1)
	}
	got := meta.ReposRestore[0]
	if got.RepoUID != "github.com/tasuku43/sample" {
		t.Fatalf("repos_restore.repo_uid = %q, want %q", got.RepoUID, "github.com/tasuku43/sample")
	}
	if got.RepoKey != "tasuku43/sample" {
		t.Fatalf("repos_restore.repo_key = %q, want %q", got.RepoKey, "tasuku43/sample")
	}
	if got.RemoteURL != repoSpec {
		t.Fatalf("repos_restore.remote_url = %q, want %q", got.RemoteURL, repoSpec)
	}
	if got.Alias != "sample" {
		t.Fatalf("repos_restore.alias = %q, want %q", got.Alias, "sample")
	}
	if got.Branch != "MVP-021/test" {
		t.Fatalf("repos_restore.branch = %q, want %q", got.Branch, "MVP-021/test")
	}
	if got.BaseRef != "origin/main" {
		t.Fatalf("repos_restore.base_ref = %q, want %q", got.BaseRef, "origin/main")
	}
}
