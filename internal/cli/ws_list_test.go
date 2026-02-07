package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_List_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws list") {
		t.Fatalf("stdout missing ws list usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_List_ImportsWorkspaceDirAndPrintsIt(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "IMPORTED"), 0o755); err != nil {
		t.Fatalf("create imported ws dir: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "IMPORTED") {
		t.Fatalf("stdout missing imported workspace: %q", out.String())
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

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces WHERE id = 'IMPORTED'").Scan(&count); err != nil {
		t.Fatalf("query imported workspace: %v", err)
	}
	if count != 1 {
		t.Fatalf("imported workspace row count = %d, want 1", count)
	}
}

func TestCLI_WS_List_MarksMissingRepoWorktree(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WS1"), 0o755); err != nil {
		t.Fatalf("create ws dir: %v", err)
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
	if _, err := statestore.CreateWorkspace(ctx, db, statestore.CreateWorkspaceInput{
		ID:          "WS1",
		Description: "",
		SourceURL:   "",
		Now:         123,
	}); err != nil {
		t.Fatalf("CreateWorkspace error: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO repos (repo_uid, repo_key, remote_url, created_at, updated_at)
VALUES ('github.com/o/r', 'o/r', 'https://example.com/o/r.git', 1, 1)
`); err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspace_repos (
  workspace_id, repo_uid, repo_key, alias, branch, base_ref, repo_spec_input, missing_at, created_at, updated_at
) VALUES (
  'WS1', 'github.com/o/r', 'o/r', 'r', 'main', '', 'github.com/o/r', NULL, 1, 1
)
`); err != nil {
		t.Fatalf("insert workspace_repo: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	var missingAt *int64
	if qErr := db.QueryRowContext(ctx, `
SELECT missing_at
FROM workspace_repos
WHERE workspace_id = 'WS1' AND repo_uid = 'github.com/o/r'
`).Scan(&missingAt); qErr != nil {
		t.Fatalf("query missing_at: %v", qErr)
	}
	if missingAt == nil || *missingAt == 0 {
		t.Fatalf("missing_at not set: %v", missingAt)
	}
}

func TestCLI_WS_List_DefaultScopeShowsActiveOnlyAndNoSelectionMarker(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS_ACTIVE"})
		if code != exitOK {
			t.Fatalf("ws create WS_ACTIVE exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS_ARCHIVED"})
		if code != exitOK {
			t.Fatalf("ws create WS_ARCHIVED exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "WS_ARCHIVED"})
		if code != exitOK {
			t.Fatalf("ws close WS_ARCHIVED exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "Workspaces(active):") {
		t.Fatalf("stdout missing active heading: %q", got)
	}
	if !strings.Contains(got, "Workspaces(active):\n\n") {
		t.Fatalf("ws list should keep one blank line after heading: %q", got)
	}
	if !strings.Contains(got, "WS_ACTIVE") {
		t.Fatalf("stdout missing active workspace: %q", got)
	}
	if strings.Contains(got, "WS_ARCHIVED") {
		t.Fatalf("stdout should not include archived workspace in default scope: %q", got)
	}
	if strings.Contains(got, "[ ]") || strings.Contains(got, "[x]") {
		t.Fatalf("stdout must not include selector markers: %q", got)
	}
}

func TestCLI_WS_List_ArchivedScopeShowsArchivedOnly(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS_ACTIVE"})
		if code != exitOK {
			t.Fatalf("ws create WS_ACTIVE exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS_ARCHIVED"})
		if code != exitOK {
			t.Fatalf("ws create WS_ARCHIVED exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "WS_ARCHIVED"})
		if code != exitOK {
			t.Fatalf("ws close WS_ARCHIVED exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "list", "--archived"})
	if code != exitOK {
		t.Fatalf("ws list --archived exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "Workspaces(archived):") {
		t.Fatalf("stdout missing archived heading: %q", got)
	}
	if !strings.Contains(got, "WS_ARCHIVED") {
		t.Fatalf("stdout missing archived workspace: %q", got)
	}
	if strings.Contains(got, "WS_ACTIVE") {
		t.Fatalf("stdout should not include active workspace in archived scope: %q", got)
	}
}

func TestCLI_WS_List_SummaryDoesNotShowTextualRiskTags(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create WS1 exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	got := out.String()
	if strings.Contains(got, "[clean]") || strings.Contains(got, "[dirty]") || strings.Contains(got, "[unknown]") {
		t.Fatalf("summary should not include textual risk tags: %q", got)
	}
}

func TestRenderWSListSummaryRow_AlignsDescriptionColumn(t *testing.T) {
	rowA := wsListRow{
		ID:          "WS_A",
		RepoCount:   1,
		Risk:        workspacerisk.WorkspaceRiskClean,
		Description: "first description",
	}
	rowB := wsListRow{
		ID:          "WORKSPACE-B-LONG",
		RepoCount:   12,
		Risk:        workspacerisk.WorkspaceRiskDiverged,
		Description: "second description",
	}

	lineA := renderWSListSummaryRow(rowA, 16, 10, 8, 120, false)
	lineB := renderWSListSummaryRow(rowB, 16, 10, 8, 120, false)

	colA := strings.Index(lineA, "first description")
	colB := strings.Index(lineB, "second description")
	if colA <= 0 || colB <= 0 {
		t.Fatalf("description columns not found: lineA=%q lineB=%q", lineA, lineB)
	}
	if colA != colB {
		t.Fatalf("description start columns differ: got %d and %d", colA, colB)
	}
}

func TestRenderWSListSummaryRow_TruncatesDescriptionWithEllipsis(t *testing.T) {
	row := wsListRow{
		ID:          "WS1",
		RepoCount:   1,
		Risk:        workspacerisk.WorkspaceRiskClean,
		Description: "abcdefghijklmnopqrstuvwxyz",
	}

	line := renderWSListSummaryRow(row, 10, 10, 8, 44, false)
	if !strings.Contains(line, "…") {
		t.Fatalf("line should contain ellipsis when truncated: %q", line)
	}
	if w := displayWidth(line); w > 44 {
		t.Fatalf("line width = %d, want <= 44: %q", w, line)
	}
}

func TestRenderWSListSummaryRow_FixedColumnOrderContract(t *testing.T) {
	row := wsListRow{
		ID:          "WS1",
		RepoCount:   7,
		Risk:        workspacerisk.WorkspaceRiskClean,
		Description: "desc",
	}
	line := renderWSListSummaryRow(row, 10, 1, 8, 120, false)
	idIdx := strings.Index(line, "WS1")
	riskIdx := strings.Index(line, "*")
	repoIdx := strings.Index(line, "repos:7")
	descIdx := strings.Index(line, "desc")
	if idIdx < 0 || riskIdx < 0 || repoIdx < 0 || descIdx < 0 {
		t.Fatalf("summary row missing required tokens: %q", line)
	}
	if !(idIdx < riskIdx && riskIdx < repoIdx && repoIdx < descIdx) {
		t.Fatalf("summary row must keep ID|risk|repos|description order: %q", line)
	}
}

func TestPrintWSListHuman_EmptyRowsUsesIndentedNone(t *testing.T) {
	var out bytes.Buffer
	printWSListHuman(&out, nil, "active", false, false)
	got := out.String()

	if !strings.Contains(got, "Workspaces(active):\n\n") {
		t.Fatalf("missing heading spacing contract: %q", got)
	}
	if !strings.Contains(got, "\n  (none)\n") {
		t.Fatalf("empty state should be indented with shared indent: %q", got)
	}
}
