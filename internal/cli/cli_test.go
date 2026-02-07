package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
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

	code := c.Run([]string{"ws"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "gionx ws") || !strings.Contains(err.String(), "Subcommands:") {
		t.Fatalf("stderr missing ws usage: %q", err.String())
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

	ctx := context.Background()
	dbPath, pathErr := paths.StateDBPathForRoot(root)
	if pathErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", pathErr)
	}
	db, openErr := statestore.Open(ctx, dbPath)
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	t.Cleanup(func() { _ = db.Close() })

	var gotRoot string
	var gotPool string
	qErr := db.QueryRowContext(ctx, "SELECT root_path, repo_pool_path FROM settings WHERE id = 1").Scan(&gotRoot, &gotPool)
	if qErr != nil {
		t.Fatalf("query settings: %v", qErr)
	}
	if gotRoot != root {
		t.Fatalf("settings.root_path = %q, want %q", gotRoot, root)
	}
	wantPool := filepath.Join(cacheHome, "gionx", "repo-pool")
	if gotPool != wantPool {
		t.Fatalf("settings.repo_pool_path = %q, want %q", gotPool, wantPool)
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
	if !strings.Contains(string(agentsBytes), "Description: hello world") {
		t.Fatalf("AGENTS.md missing description: %q", string(agentsBytes))
	}

	ctx := context.Background()
	dbPath, pathErr := paths.StateDBPathForRoot(root)
	if pathErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", pathErr)
	}
	db, openErr := statestore.Open(ctx, dbPath)
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	t.Cleanup(func() { _ = db.Close() })

	var status string
	var desc string
	var gen int
	qErr := db.QueryRowContext(ctx, "SELECT status, description, generation FROM workspaces WHERE id = ?", "MVP-020").Scan(&status, &desc, &gen)
	if qErr != nil {
		t.Fatalf("query workspaces: %v", qErr)
	}
	if status != "active" {
		t.Fatalf("workspaces.status = %q, want %q", status, "active")
	}
	if desc != "hello world" {
		t.Fatalf("workspaces.description = %q, want %q", desc, "hello world")
	}
	if gen != 1 {
		t.Fatalf("workspaces.generation = %d, want %d", gen, 1)
	}

	var eventType string
	var eventGen int
	evErr := db.QueryRowContext(ctx, `
SELECT event_type, workspace_generation
FROM workspace_events
WHERE workspace_id = ?
ORDER BY id DESC
LIMIT 1
`, "MVP-020").Scan(&eventType, &eventGen)
	if evErr != nil {
		t.Fatalf("query workspace_events: %v", evErr)
	}
	if eventType != "created" {
		t.Fatalf("workspace_events.event_type = %q, want %q", eventType, "created")
	}
	if eventGen != 1 {
		t.Fatalf("workspace_events.workspace_generation = %d, want %d", eventGen, 1)
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

	ctx := context.Background()
	dbPath, pathErr := paths.StateDBPathForRoot(root)
	if pathErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", pathErr)
	}
	db, openErr := statestore.Open(ctx, dbPath)
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	t.Cleanup(func() { _ = db.Close() })

	repoPoolPath := filepath.Join(cacheHome, "gionx", "repo-pool")
	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		t.Fatalf("EnsureSettings error: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspaces (
  id, generation, status, description, source_url,
  created_at, updated_at,
  archived_commit_sha, reopened_commit_sha
)
VALUES ('MVP-020', 1, 'archived', '', '', 1, 1, NULL, NULL)
`); err != nil {
		t.Fatalf("insert archived workspace: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("should-not-be-read\n")

	code := c.Run([]string{"ws", "create", "MVP-020"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "ws reopen") {
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

	ctx := context.Background()
	dbPath, pathErr := paths.StateDBPathForRoot(root)
	if pathErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", pathErr)
	}
	db, openErr := statestore.Open(ctx, dbPath)
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	t.Cleanup(func() { _ = db.Close() })

	repoPoolPath := filepath.Join(cacheHome, "gionx", "repo-pool")
	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		t.Fatalf("EnsureSettings error: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspaces (
  id, generation, status, description, source_url,
  created_at, updated_at,
  archived_commit_sha, reopened_commit_sha
)
VALUES ('MVP-020', 1, 'active', '', '', 1, 1, NULL, NULL)
`); err != nil {
		t.Fatalf("insert active workspace: %v", err)
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

		code := c.Run([]string{"ws", "add-repo", "MVP-020"})
		if code != exitOK {
			t.Fatalf("ws add-repo exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}

		worktreePath := filepath.Join(root, "workspaces", "MVP-020", "repos", "sample")
		if _, statErr := os.Stat(filepath.Join(worktreePath, ".git")); statErr != nil {
			t.Fatalf("worktree .git missing: %v", statErr)
		}
	}

	ctx := context.Background()
	dbPath, pathErr := paths.StateDBPathForRoot(root)
	if pathErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", pathErr)
	}
	db, openErr := statestore.Open(ctx, dbPath)
	if openErr != nil {
		t.Fatalf("Open(state db) error: %v", openErr)
	}
	t.Cleanup(func() { _ = db.Close() })

	var repoKey string
	var remoteURL string
	if err := db.QueryRowContext(ctx, "SELECT repo_key, remote_url FROM repos WHERE repo_uid = ?", "github.com/tasuku43/sample").Scan(&repoKey, &remoteURL); err != nil {
		t.Fatalf("query repos: %v", err)
	}
	if repoKey != "tasuku43/sample" {
		t.Fatalf("repos.repo_key = %q, want %q", repoKey, "tasuku43/sample")
	}
	if remoteURL != repoSpec {
		t.Fatalf("repos.remote_url = %q, want %q", remoteURL, repoSpec)
	}

	var alias string
	var branch string
	var baseRef string
	if err := db.QueryRowContext(ctx, `
SELECT alias, branch, base_ref
FROM workspace_repos
WHERE workspace_id = ? AND repo_uid = ?
`, "MVP-020", "github.com/tasuku43/sample").Scan(&alias, &branch, &baseRef); err != nil {
		t.Fatalf("query workspace_repos: %v", err)
	}
	if alias != "sample" {
		t.Fatalf("workspace_repos.alias = %q, want %q", alias, "sample")
	}
	if branch != "MVP-020/test" {
		t.Fatalf("workspace_repos.branch = %q, want %q", branch, "MVP-020/test")
	}
	if baseRef != "" {
		t.Fatalf("workspace_repos.base_ref = %q, want empty", baseRef)
	}
}
