package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/paths"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_List_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "list", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws list") {
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

	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "IMPORTED"), 0o755); err != nil {
		t.Fatalf("create imported ws dir: %v", err)
	}

	setKraHomeForTest(t)
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext() error: %v", err)
	}

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

	setKraHomeForTest(t)
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext() error: %v", err)
	}

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

func TestRenderWSListSummaryRow_BulletPrefix(t *testing.T) {
	rowA := wsListRow{
		ID:        "WS_A",
		RepoCount: 1,
		Title:     "first title",
	}
	lineA := renderWSListSummaryRow(rowA, 120, false)
	if !strings.HasPrefix(lineA, "  • WS_A: ") {
		t.Fatalf("summary row should be bullet list item: %q", lineA)
	}
}

func TestRenderWSListSummaryRow_TruncatesTitleWithEllipsis(t *testing.T) {
	row := wsListRow{
		ID:        "WS1",
		RepoCount: 1,
		Title:     "abcdefghijklmnopqrstuvwxyz",
	}

	line := renderWSListSummaryRow(row, 20, false)
	if !strings.Contains(line, "…") {
		t.Fatalf("line should contain ellipsis when truncated: %q", line)
	}
	if w := displayWidth(line); w > 20 {
		t.Fatalf("line width = %d, want <= 20: %q", w, line)
	}
}

func TestRenderWSListSummaryRow_ShowsIDAndTitleOnly(t *testing.T) {
	row := wsListRow{
		ID:        "WS1",
		RepoCount: 7,
		Title:     "desc",
	}
	line := renderWSListSummaryRow(row, 120, false)
	idIdx := strings.Index(line, "WS1")
	descIdx := strings.Index(line, "WS1: desc")
	if idIdx < 0 || descIdx < 0 {
		t.Fatalf("summary row missing required tokens: %q", line)
	}
	if strings.Contains(line, "repos:7") {
		t.Fatalf("summary row should not include repos: %q", line)
	}
}

func TestCLI_WS_List_TSVUsesCompactColumns(t *testing.T) {
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
	if !strings.HasPrefix(got, "id\tstatus\tupdated_at\trepo_count\ttitle\n") {
		t.Fatalf("tsv header mismatch: %q", got)
	}
	if strings.Contains(got, "\trisk\t") || strings.Contains(got, "\twork_state\t") {
		t.Fatalf("tsv should not include risk/work_state columns: %q", got)
	}
}

func TestCLI_WS_List_JSON_Success(t *testing.T) {
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
	code := c.Run([]string{"ws", "list", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws list --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.list" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got, _ := resp.Result["scope"].(string); got != "active" {
		t.Fatalf("result.scope = %q, want %q", got, "active")
	}
	items, ok := resp.Result["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("result.items missing: %#v", resp.Result["items"])
	}
	found := false
	for _, raw := range items {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := row["id"].(string); id == "WS1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("WS1 not found in result.items: %#v", items)
	}
}

func TestCLI_WS_List_JSON_UsageError(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "list", "--format", "json", "--unknown"})
	if code != exitUsage {
		t.Fatalf("ws list --format json invalid arg exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "ws.list" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json usage error: %q", err.String())
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

func TestCountWorkspaceReposFromFilesystem_DeduplicatesMetaAndDirs(t *testing.T) {
	root := t.TempDir()
	wsID := "WS1"
	reposPath := filepath.Join(root, "workspaces", wsID, "repos")
	if err := os.MkdirAll(filepath.Join(reposPath, "repo-a"), 0o755); err != nil {
		t.Fatalf("mkdir repo-a: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(reposPath, "repo-c"), 0o755); err != nil {
		t.Fatalf("mkdir repo-c: %v", err)
	}

	meta := workspaceMetaFile{
		ReposRestore: []workspaceMetaRepoRestore{
			{Alias: "repo-a"},
			{Alias: "repo-b"},
		},
	}
	got, err := countWorkspaceReposFromFilesystem(root, "active", wsID, meta)
	if err != nil {
		t.Fatalf("countWorkspaceReposFromFilesystem() error = %v, want nil", err)
	}
	if got != 3 {
		t.Fatalf("repo count = %d, want 3", got)
	}
}
