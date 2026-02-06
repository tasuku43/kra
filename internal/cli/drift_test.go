package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/statestore"
	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Create_InvalidRoot_Errors(t *testing.T) {
	_ = testutil.NewEnv(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "does not look like a gionx root") {
		t.Fatalf("stderr missing root error: %q", errBuf.String())
	}
}

func TestCLI_WS_Create_FilesystemCollision_DoesNotInsertDBRow(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "MVP-020")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("create existing ws dir: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "workspace directory already exists") {
		t.Fatalf("stderr missing collision error: %q", errBuf.String())
	}

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := statestore.EnsureSettings(ctx, db, env.Root, env.RepoPoolPath()); err != nil {
		t.Fatalf("EnsureSettings error: %v", err)
	}

	var status string
	qErr := db.QueryRowContext(ctx, "SELECT status FROM workspaces WHERE id = ?", "MVP-020").Scan(&status)
	if !errors.Is(qErr, sql.ErrNoRows) {
		t.Fatalf("expected no workspaces row, got err=%v status=%q", qErr, status)
	}
}

func TestCLI_WS_Create_Purged_AllowsNewGeneration(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := statestore.EnsureSettings(ctx, db, env.Root, env.RepoPoolPath()); err != nil {
		t.Fatalf("EnsureSettings error: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspace_events (workspace_id, workspace_generation, event_type, at, meta)
VALUES ('MVP-020', 1, 'purged', 1, '{}')
`); err != nil {
		t.Fatalf("insert purged event: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--no-prompt", "MVP-020"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}

	var gen int
	if err := db.QueryRowContext(ctx, "SELECT generation FROM workspaces WHERE id = ?", "MVP-020").Scan(&gen); err != nil {
		t.Fatalf("query generation: %v", err)
	}
	if gen != 2 {
		t.Fatalf("generation = %d, want %d", gen, 2)
	}

	var eventType string
	var eventGen int
	if err := db.QueryRowContext(ctx, `
SELECT event_type, workspace_generation
FROM workspace_events
WHERE workspace_id = ?
ORDER BY id DESC
LIMIT 1
`, "MVP-020").Scan(&eventType, &eventGen); err != nil {
		t.Fatalf("query last event: %v", err)
	}
	if eventType != "created" || eventGen != 2 {
		t.Fatalf("last event = (%q, %d), want (%q, %d)", eventType, eventGen, "created", 2)
	}
}

func TestCLI_Init_UsesDifferentStateDBPerRoot(t *testing.T) {
	testutil.RequireCommand(t, "git")
	setGitIdentity(t)

	env := testutil.NewEnv(t)

	var out1 bytes.Buffer
	var err1 bytes.Buffer
	c1 := New(&out1, &err1)

	code := c1.Run([]string{"init"})
	if code != exitOK {
		t.Fatalf("first init exit code = %d, want %d (stderr=%q)", code, exitOK, err1.String())
	}

	otherRoot := t.TempDir()
	t.Setenv("GIONX_ROOT", otherRoot)

	var out2 bytes.Buffer
	var err2 bytes.Buffer
	c2 := New(&out2, &err2)

	code = c2.Run([]string{"init"})
	if code != exitOK {
		t.Fatalf("second init exit code = %d, want %d (stderr=%q)", code, exitOK, err2.String())
	}
	if err2.Len() != 0 {
		t.Fatalf("second init stderr not empty: %q", err2.String())
	}

	ctx := context.Background()
	dbPath1 := env.StateDBPath()
	db, err := statestore.Open(ctx, dbPath1)
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var gotRoot string
	if err := db.QueryRowContext(ctx, "SELECT root_path FROM settings WHERE id = 1").Scan(&gotRoot); err != nil {
		t.Fatalf("query settings: %v", err)
	}
	if gotRoot != env.Root {
		t.Fatalf("settings.root_path = %q, want %q", gotRoot, env.Root)
	}

	otherEnv := testutil.Env{
		Root:      otherRoot,
		DataHome:  env.DataHome,
		CacheHome: env.CacheHome,
	}
	dbPath2 := otherEnv.StateDBPath()
	if dbPath1 == dbPath2 {
		t.Fatalf("state db path should differ by root: %q", dbPath1)
	}

	db2, err := statestore.Open(ctx, dbPath2)
	if err != nil {
		t.Fatalf("Open(other state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })

	var gotRoot2 string
	if err := db2.QueryRowContext(ctx, "SELECT root_path FROM settings WHERE id = 1").Scan(&gotRoot2); err != nil {
		t.Fatalf("query other settings: %v", err)
	}
	if gotRoot2 != otherRoot {
		t.Fatalf("other settings.root_path = %q, want %q", gotRoot2, otherRoot)
	}
}

func TestCLI_Init_SettingsDrift_ErrorsOnDifferentRepoPool(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)

	var out1 bytes.Buffer
	var err1 bytes.Buffer
	c1 := New(&out1, &err1)

	code := c1.Run([]string{"init"})
	if code != exitOK {
		t.Fatalf("first init exit code = %d, want %d (stderr=%q)", code, exitOK, err1.String())
	}

	newCacheHome := filepath.Join(t.TempDir(), "other-xdg-cache")
	t.Setenv("XDG_CACHE_HOME", newCacheHome)

	var out2 bytes.Buffer
	var err2 bytes.Buffer
	c2 := New(&out2, &err2)

	code = c2.Run([]string{"init"})
	if code != exitError {
		t.Fatalf("second init exit code = %d, want %d (stderr=%q)", code, exitError, err2.String())
	}
	if !strings.Contains(err2.String(), "settings already initialized") {
		t.Fatalf("stderr missing settings drift error: %q", err2.String())
	}

	ctx := context.Background()
	db, err := statestore.Open(ctx, env.StateDBPath())
	if err != nil {
		t.Fatalf("Open(state db) error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var gotRepoPool string
	if err := db.QueryRowContext(ctx, "SELECT repo_pool_path FROM settings WHERE id = 1").Scan(&gotRepoPool); err != nil {
		t.Fatalf("query settings: %v", err)
	}
	if gotRepoPool != env.RepoPoolPath() {
		t.Fatalf("settings.repo_pool_path = %q, want %q", gotRepoPool, env.RepoPoolPath())
	}
}
