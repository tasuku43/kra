package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/paths"
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

func TestCLI_WS_List_SelectFlagRejected(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list", "--select"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
	}
	if !strings.Contains(err.String(), "unknown flag for ws list") {
		t.Fatalf("stderr missing unknown flag error: %q", err.String())
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

	code := c.Run([]string{"ws", "list", "--tree"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "IMPORTED") {
		t.Fatalf("stdout missing imported workspace: %q", out.String())
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
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("create ws dir: %v", err)
	}
	meta := newWorkspaceMetaFileForCreate("WS1", "", "", 123)
	meta.ReposRestore = []workspaceMetaRepoRestore{{
		RepoUID:   "github.com/o/r",
		RepoKey:   "o/r",
		RemoteURL: "https://example.com/o/r.git",
		Alias:     "r",
		Branch:    "main",
		BaseRef:   "origin/main",
	}}
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list", "--tree"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	if !strings.Contains(out.String(), "state:missing") {
		t.Fatalf("stdout should include missing repo state: %q", out.String())
	}
}

func TestCLI_WS_List_FallbackToFilesystem_WhenStateDBCorrupted(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	if err := os.MkdirAll(filepath.Join(env.Root, "workspaces", "FS-ONLY"), 0o755); err != nil {
		t.Fatalf("create workspace dir: %v", err)
	}
	meta := newWorkspaceMetaFileForCreate("FS-ONLY", "filesystem workspace", "", 1700000000)
	if err := writeWorkspaceMetaFile(filepath.Join(env.Root, "workspaces", "FS-ONLY"), meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	dbPath, err := paths.StateDBPathForRoot(env.Root)
	if err != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("corrupted"), 0o644); err != nil {
		t.Fatalf("write corrupted db: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	got := out.String()
	if !strings.Contains(got, "FS-ONLY") {
		t.Fatalf("stdout missing FS workspace: %q", got)
	}
	if !strings.Contains(got, "filesystem workspace") {
		t.Fatalf("stdout missing workspace title: %q", got)
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
		code := c.Run([]string{"ws", "--act", "close", "WS_ARCHIVED"})
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
		code := c.Run([]string{"ws", "--act", "close", "WS_ARCHIVED"})
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

func TestRenderWSListSummaryRow_AlignsTitleColumn(t *testing.T) {
	rowA := wsListRow{
		ID:        "WS_A",
		RepoCount: 1,
		Risk:      workspacerisk.WorkspaceRiskClean,
		Title:     "first title",
	}
	rowB := wsListRow{
		ID:        "WORKSPACE-B-LONG",
		RepoCount: 12,
		Risk:      workspacerisk.WorkspaceRiskDiverged,
		Title:     "second title",
	}

	lineA := renderWSListSummaryRow(rowA, 16, 10, 8, 120, false)
	lineB := renderWSListSummaryRow(rowB, 16, 10, 8, 120, false)

	colA := strings.Index(lineA, "first title")
	colB := strings.Index(lineB, "second title")
	if colA <= 0 || colB <= 0 {
		t.Fatalf("title columns not found: lineA=%q lineB=%q", lineA, lineB)
	}
	if colA != colB {
		t.Fatalf("title start columns differ: got %d and %d", colA, colB)
	}
}

func TestRenderWSListSummaryRow_TruncatesTitleWithEllipsis(t *testing.T) {
	row := wsListRow{
		ID:        "WS1",
		RepoCount: 1,
		Risk:      workspacerisk.WorkspaceRiskClean,
		Title:     "abcdefghijklmnopqrstuvwxyz",
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
		ID:        "WS1",
		RepoCount: 7,
		Risk:      workspacerisk.WorkspaceRiskClean,
		Title:     "desc",
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
		t.Fatalf("summary row must keep ID|risk|repos|title order: %q", line)
	}
}

func TestCLI_WS_List_ShowsLogicalWorkStateTodoAndInProgress(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WIP", "repos", "r"), 0o755); err != nil {
		t.Fatalf("create WIP repo dir: %v", err)
	}

	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	if err := os.MkdirAll(filepath.Join(root, "workspaces", "TODO"), 0o755); err != nil {
		t.Fatalf("create TODO workspace dir: %v", err)
	}
	if err := writeWorkspaceMetaFile(filepath.Join(root, "workspaces", "TODO"), newWorkspaceMetaFileForCreate("TODO", "", "", 100)); err != nil {
		t.Fatalf("write TODO workspace meta: %v", err)
	}
	wipMeta := newWorkspaceMetaFileForCreate("WIP", "", "", 101)
	wipMeta.ReposRestore = []workspaceMetaRepoRestore{{
		RepoUID:   "github.com/o/r",
		RepoKey:   "o/r",
		RemoteURL: "https://example.com/o/r.git",
		Alias:     "r",
		Branch:    "main",
		BaseRef:   "origin/main",
	}}
	if err := writeWorkspaceMetaFile(filepath.Join(root, "workspaces", "WIP"), wipMeta); err != nil {
		t.Fatalf("write WIP workspace meta: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "list"})
	if code != exitOK {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "TODO") || !strings.Contains(got, "todo") {
		t.Fatalf("stdout should include todo workspace state: %q", got)
	}
	if !strings.Contains(got, "WIP") || !strings.Contains(got, "in-progress") {
		t.Fatalf("stdout should include in-progress workspace state: %q", got)
	}
}

func TestCLI_WS_List_TSVIncludesWorkStateColumn(t *testing.T) {
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
	code := c.Run([]string{"ws", "list", "--format", "tsv"})
	if code != exitOK {
		t.Fatalf("ws list --format tsv exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.HasPrefix(got, "id\tstatus\tupdated_at\trepo_count\trisk\twork_state\ttitle\n") {
		t.Fatalf("tsv header missing work_state: %q", got)
	}
	if !strings.Contains(got, "\ttodo\t") {
		t.Fatalf("tsv should include logical work_state token: %q", got)
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
