package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
	"github.com/tasuku43/kra/internal/testutil"
)

type fakeWSTaskSyncClient struct {
	statuses   map[string][]cmuxctl.StatusEntry
	setCalls   []cmuxctl.StatusEntry
	clearCalls []struct {
		workspace string
		key       string
	}
}

func newFakeWSTaskSyncClient() *fakeWSTaskSyncClient {
	return &fakeWSTaskSyncClient{
		statuses: map[string][]cmuxctl.StatusEntry{},
	}
}

func (f *fakeWSTaskSyncClient) ListStatus(_ context.Context, workspace string) ([]cmuxctl.StatusEntry, error) {
	return append([]cmuxctl.StatusEntry{}, f.statuses[workspace]...), nil
}

func (f *fakeWSTaskSyncClient) SetStatus(_ context.Context, workspace string, label string, text string, icon string, color string) error {
	entry := cmuxctl.StatusEntry{
		Key:   label,
		Value: text,
		Icon:  icon,
		Color: color,
	}
	f.setCalls = append(f.setCalls, entry)
	rows := append([]cmuxctl.StatusEntry{}, f.statuses[workspace]...)
	replaced := false
	for i := range rows {
		if rows[i].Key == label {
			rows[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		rows = append([]cmuxctl.StatusEntry{entry}, rows...)
	}
	f.statuses[workspace] = rows
	return nil
}

func (f *fakeWSTaskSyncClient) ClearStatus(_ context.Context, workspace string, label string) error {
	f.clearCalls = append(f.clearCalls, struct {
		workspace string
		key       string
	}{workspace: workspace, key: label})
	rows := f.statuses[workspace]
	out := rows[:0]
	for _, row := range rows {
		if row.Key != label {
			out = append(out, row)
		}
	}
	f.statuses[workspace] = append([]cmuxctl.StatusEntry{}, out...)
	return nil
}

func useFakeWSTaskSyncClient(t *testing.T, client appports.CMUXTaskSyncClient) {
	t.Helper()
	orig := newCMUXTaskSyncClient
	newCMUXTaskSyncClient = func() appports.CMUXTaskSyncClient {
		return client
	}
	t.Cleanup(func() {
		newCMUXTaskSyncClient = orig
	})
}

func seedCMUXWorkspaceMapping(t *testing.T, root string, workspaceID string, cmuxWorkspaceID string) {
	t.Helper()
	store := cmuxmap.NewStore(root)
	file, err := store.Load()
	if err != nil {
		t.Fatalf("load cmux mapping: %v", err)
	}
	file.Workspaces[workspaceID] = cmuxmap.WorkspaceMapping{
		NextOrdinal: 2,
		Entries: []cmuxmap.Entry{
			{CMUXWorkspaceID: cmuxWorkspaceID, Ordinal: 1},
		},
	}
	if err := store.Save(file); err != nil {
		t.Fatalf("save cmux mapping: %v", err)
	}
}

func TestCLI_WSTaskList_JSON_EmptyWhenTasksFileMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "list", "--id", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws task list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.task.list" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["task_state"]; got != "empty" {
		t.Fatalf("task_state = %v, want %q", got, "empty")
	}
	counts, ok := resp.Result["counts"].(map[string]any)
	if !ok {
		t.Fatalf("result.counts missing: %+v", resp.Result)
	}
	if got := counts["total"]; got != float64(0) {
		t.Fatalf("counts.total = %v, want 0", got)
	}
}

func TestCLI_WSTaskAdd_JSON_CreatesTaskDocumentAndPreservesOutsideContent(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	if err := os.WriteFile(filepath.Join(wsPath, workspaceDocumentFilename), []byte("# Notes\n\noutside\n"), 0o644); err != nil {
		t.Fatalf("write workspace.md: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "add", "--id", "WS1", "--title", "First task", "--description", "Line one", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws task add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.task.add" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	got := string(content)
	if !bytes.Contains(content, []byte("# Notes\n\noutside\n\n## Current State\n\nThis workspace has not recorded current state yet.\n\n## Next\n\nRecord the next concrete step here before handing off or stopping.\n\n## Tasks\n\n### TASK-001 First task\nstatus: todo\n\nLine one\n")) {
		t.Fatalf("workspace.md missing canonical task block: %q", got)
	}
}

func TestCLI_WSTaskStatus_JSON_AllowsDoneToTodoWithoutSync(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Start me\nstatus: done\n")
	seedCMUXWorkspaceMapping(t, env.Root, "WS1", "cmux-1")
	syncClient := newFakeWSTaskSyncClient()
	syncClient.statuses["cmux-1"] = []cmuxctl.StatusEntry{
		{Key: "task:TASK-001", Value: "✔ TASK-001 Start me", Icon: "checklist", Color: "#64748b"},
	}
	useFakeWSTaskSyncClient(t, syncClient)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "status", "--id", "WS1", "TASK-001", "todo", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws task status exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.task.status" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	task, ok := resp.Result["task"].(map[string]any)
	if !ok {
		t.Fatalf("result.task missing: %+v", resp.Result)
	}
	if got := task["status"]; got != "todo" {
		t.Fatalf("task.status = %v, want %q", got, "todo")
	}
	if _, ok := resp.Result["sync"]; ok {
		t.Fatalf("result.sync should be absent after sync deprecation: %+v", resp.Result)
	}

	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: todo")) {
		t.Fatalf("workspace.md should be updated to todo: %q", string(content))
	}
	if len(syncClient.setCalls) != 0 || len(syncClient.clearCalls) != 0 {
		t.Fatalf("cmux sync calls = set:%+v clear:%+v, want none", syncClient.setCalls, syncClient.clearCalls)
	}
}

func TestCLI_WSTaskList_JSON_InvalidContractFailsClosed(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-001 Second\nstatus: doing\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "list", "--id", "WS1", "--format", "json"})
	if code != exitError {
		t.Fatalf("ws task list exit code = %d, want %d", code, exitError)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCLI_WSTaskList_HumanUsesTasksSectionHeading(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 todo\nstatus: todo\n\n### TASK-002 doing\nstatus: doing\n\n### TASK-003 blocked\nstatus: blocked\n\n### TASK-004 done\nstatus: done\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "list", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws task list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "Tasks:\n\n") {
		t.Fatalf("stdout should render Tasks section heading: %q", got)
	}
	for _, want := range []string{
		"  ○ TASK-001: todo",
		"  ● TASK-002: doing",
		"  ▲ TASK-003: blocked",
		"  ✔ TASK-004: done",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing row %q: %q", want, got)
		}
	}
	if strings.Contains(got, "tasks: 4 total") || strings.Contains(got, "Todo:") || strings.Contains(got, "Doing:") {
		t.Fatalf("stdout should use flat task list without summary/subsections: %q", got)
	}
}

func TestCLI_WSTaskView_Current_GroupsByStatus(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 調査する\nstatus: todo\n\n### TASK-002 実装方針をまとめる\nstatus: doing\n\n### TASK-003 テストを書く\nstatus: todo\n\n### TASK-004 API仕様の確認待ち\nstatus: blocked\n\n### TASK-000 POCする\nstatus: done\n")
	realRoot, realErr := filepath.EvalSymlinks(env.Root)
	if realErr != nil {
		t.Fatalf("eval root: %v", realErr)
	}
	realWSPath, realErr := filepath.EvalSymlinks(wsPath)
	if realErr != nil {
		t.Fatalf("eval workspace path: %v", realErr)
	}
	if err := paths.WriteCurrentContext(realRoot); err != nil {
		t.Fatalf("WriteCurrentContext(%q): %v", realRoot, err)
	}
	prevWD, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("get wd: %v", wdErr)
	}
	if err := os.Chdir(realWSPath); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--current", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	for _, want := range []string{
		"TASKS  WS1",
		"  ○ TASK-001  調査する",
		"  ● TASK-002  実装方針をまとめる",
		"  ○ TASK-003  テストを書く",
		"  ▲ TASK-004  API仕様の確認待ち",
		"  ✔ TASK-000  POCする",
		"updated:",
		"source: " + filepath.Join(realWSPath, workspaceDocumentFilename),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q: %q", want, got)
		}
	}
	if strings.Index(got, "TASK-001") > strings.Index(got, "TASK-002") ||
		strings.Index(got, "TASK-002") > strings.Index(got, "TASK-003") ||
		strings.Index(got, "TASK-003") > strings.Index(got, "TASK-004") ||
		strings.Index(got, "TASK-004") > strings.Index(got, "TASK-000") {
		t.Fatalf("tasks are not in file order: %q", got)
	}
}

func TestCLI_WSTaskView_CMUXCurrentResolvesMappedWorkspace(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 From cmux\nstatus: todo\n")
	seedCMUXWorkspaceMapping(t, env.Root, "WS1", "cmux-1")
	t.Setenv("CMUX_WORKSPACE_ID", "cmux-1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--cmux-current", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "TASKS  WS1") || !strings.Contains(got, "TASK-001  From cmux") {
		t.Fatalf("stdout missing cmux current task view: %q", got)
	}
}

func TestCLI_WSTaskView_CMUXCurrentResolvesRuntimeUUIDToMappedRef(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 From cmux uuid\nstatus: todo\n")
	seedCMUXWorkspaceMapping(t, env.Root, "WS1", "workspace:1")
	t.Setenv("CMUX_WORKSPACE_ID", "EA43F9F0-CB52-4C46-A52D-95D7E9884C4C")

	orig := listCMUXWorkspacesForWSTaskTarget
	listCMUXWorkspacesForWSTaskTarget = func(context.Context) ([]cmuxctl.Workspace, error) {
		return []cmuxctl.Workspace{{
			ID:  "EA43F9F0-CB52-4C46-A52D-95D7E9884C4C",
			Ref: "workspace:1",
		}}, nil
	}
	t.Cleanup(func() { listCMUXWorkspacesForWSTaskTarget = orig })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--cmux-current", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "TASKS  WS1") || !strings.Contains(got, "TASK-001  From cmux uuid") {
		t.Fatalf("stdout missing cmux uuid task view: %q", got)
	}
}

func TestCLI_WSTaskView_CMUXCurrentFallsBackToWorkspaceCWD(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 From cwd\nstatus: todo\n")
	t.Setenv("CMUX_WORKSPACE_ID", "EA43F9F0-CB52-4C46-A52D-95D7E9884C4C")

	origList := listCMUXWorkspacesForWSTaskTarget
	listCMUXWorkspacesForWSTaskTarget = func(context.Context) ([]cmuxctl.Workspace, error) {
		return nil, fmt.Errorf("cmux unavailable")
	}
	t.Cleanup(func() { listCMUXWorkspacesForWSTaskTarget = origList })
	prevWD, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("get wd: %v", wdErr)
	}
	if err := os.Chdir(wsPath); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--cmux-current", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "TASKS  WS1") || !strings.Contains(got, "TASK-001  From cwd") {
		t.Fatalf("stdout missing cwd fallback task view: %q", got)
	}
}

func TestCLI_WSTaskView_CMUXCurrentFallsBackToAllFromRootCWD(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 From root\nstatus: todo\n")
	t.Setenv("CMUX_WORKSPACE_ID", "7399E7AC-DD20-445C-8AEA-8E0A7990492C")

	origList := listCMUXWorkspacesForWSTaskTarget
	listCMUXWorkspacesForWSTaskTarget = func(context.Context) ([]cmuxctl.Workspace, error) {
		return nil, fmt.Errorf("cmux unavailable")
	}
	t.Cleanup(func() { listCMUXWorkspacesForWSTaskTarget = origList })
	prevWD, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("get wd: %v", wdErr)
	}
	if err := os.Chdir(env.Root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--cmux-current", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "TASKS  KRA_ROOT") || !strings.Contains(got, "WS1  WS1") || !strings.Contains(got, "TASK-001  From root") {
		t.Fatalf("stdout missing root all fallback task view: %q", got)
	}
}

func TestCLI_WSTaskView_CMUXCurrentRootMappingUsesAll(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 From root mapping\nstatus: todo\n")
	seedCMUXWorkspaceMapping(t, env.Root, rootCMUXMappingID, "workspace:root")
	t.Setenv("CMUX_WORKSPACE_ID", "workspace:root")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--cmux-current", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "TASKS  KRA_ROOT") || !strings.Contains(got, "TASK-001  From root mapping") {
		t.Fatalf("stdout missing root mapping task view: %q", got)
	}
}

func TestParseWSTaskTUIOptions_CMUXCurrent(t *testing.T) {
	opts, err := parseWSTaskTUIOptions([]string{"--cmux-current", "--refresh", "2s"})
	if err != nil {
		t.Fatalf("parseWSTaskTUIOptions() error: %v", err)
	}
	if !opts.target.useCMUXCurrent || opts.target.useCurrent || opts.target.useAll || opts.target.useSelect {
		t.Fatalf("unexpected target opts: %+v", opts.target)
	}
}

func TestCLI_WSTaskView_EmptyWhenTasksFileMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--id", "WS1", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "No structured tasks.") || !strings.Contains(got, "source: "+filepath.Join(wsPath, workspaceDocumentFilename)) {
		t.Fatalf("stdout missing empty state: %q", got)
	}
}

func TestCLI_WSTaskView_EmptyWhenNoStructuredTasks(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "# Notes\n\noutside\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--id", "WS1", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if got := out.String(); !strings.Contains(got, "No structured tasks.") {
		t.Fatalf("stdout missing empty state: %q", got)
	}
}

func TestCLI_WSTaskView_InvalidContractFailsClosed(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-001 Second\nstatus: doing\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--id", "WS1", "--no-color"})
	if code != exitError {
		t.Fatalf("ws task view exit code = %d, want %d", code, exitError)
	}
	if got := out.String(); !strings.Contains(got, "error:") || !strings.Contains(got, "duplicate task id") {
		t.Fatalf("stdout missing fail-closed error: %q", got)
	}
}

func TestCLI_WSTaskView_AllTodoOnlyListsActiveWorkspaceOpenTasks(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	ws1 := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	ws2 := seedWorkspaceMeta(t, env.Root, "active", "WS2")
	archived := seedWorkspaceMeta(t, env.Root, "archived", "OLD1")
	writeWorkspaceTasksFile(t, ws1, "## Tasks\n\n### TASK-001 First todo\nstatus: todo\n\n### TASK-002 First done\nstatus: done\n")
	writeWorkspaceTasksFile(t, ws2, "## Tasks\n\n### TASK-003 Second doing\nstatus: doing\n")
	writeWorkspaceTasksFile(t, archived, "## Tasks\n\n### TASK-004 Archived todo\nstatus: todo\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--all", "--todo-only", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	for _, want := range []string{
		"TASKS  KRA_ROOT",
		"WS1  WS1",
		"  ○ TASK-001  First todo",
		"WS2  WS2",
		"  ● TASK-003  Second doing",
		"source: " + env.Root,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{"First done", "Archived todo"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout should not contain %q: %q", unwanted, got)
		}
	}
}

func TestCLI_WSTaskView_AllIncludeDoneShowsDone(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	ws1 := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, ws1, "## Tasks\n\n### TASK-001 First done\nstatus: done\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "view", "--all", "--todo-only", "--include-done", "--no-color"})
	if code != exitOK {
		t.Fatalf("ws task view exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if got := out.String(); !strings.Contains(got, "WS1  WS1") || !strings.Contains(got, "  ✔ TASK-001  First done") {
		t.Fatalf("stdout missing done task: %q", got)
	}
}

func TestPrintWSTaskView_NoColor(t *testing.T) {
	var out bytes.Buffer
	printWSTaskView(&out, wstask.ViewModel{
		WorkspaceID: "WS1",
		Path:        "/root/workspaces/WS1/workspace.md",
		Groups: []wstask.ViewGroup{{
			Status: wstask.StatusDoing,
			Title:  "Doing",
			Items:  []wstask.Item{{ID: "TASK-001", Title: "Build", Status: wstask.StatusDoing}},
		}},
	}, time.Date(2026, 5, 16, 18, 42, 10, 0, time.Local), false)
	if got := out.String(); strings.Contains(got, "\x1b[") {
		t.Fatalf("no-color renderer emitted ANSI: %q", got)
	}
}

func TestWSTaskTUI_RendersProgressSummaryAndBlockedEmphasis(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: done\n\n### TASK-002 Second\nstatus: blocked\n\n### TASK-003 Third\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	got := m.View()
	for _, want := range []string{
		"Progress  1/3",
		"33%",
		"Todo 1",
		"Doing 0",
		"Blocked 1",
		"Done 1",
		"source: " + filepath.Join(wsPath, workspaceDocumentFilename),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("view missing %q: %q", want, got)
		}
	}
}

func TestWSTaskTUI_AllRendersWorkspaceProgressAndBlockedBadge(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: done\n\n### TASK-002 Second\nstatus: blocked\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{}, wsTaskTUIOptions{target: wsTaskTargetOptions{useAll: true}, includeDone: true}, false)
	got := m.View()
	for _, want := range []string{
		"Progress  1/2",
		"50%",
		"WS1  WS1\nProgress  1/2",
		"Blocked 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("view missing %q: %q", want, got)
		}
	}
}

func TestWSTaskTUI_DoneToggleHidesAndShowsDoneRows(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Finished\nstatus: done\n\n### TASK-002 Next\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	if got := m.View(); !strings.Contains(got, "TASK-001") || !strings.Contains(got, "Progress  1/2") {
		t.Fatalf("initial view should show done row and full progress: %q", got)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updated.(wsTaskTUIModel)
	if got := m.View(); strings.Contains(got, "TASK-001") || !strings.Contains(got, "TASK-002") || !strings.Contains(got, "Progress  1/2") {
		t.Fatalf("hidden-done view should hide done row but keep full progress: %q", got)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updated.(wsTaskTUIModel)
	if got := m.View(); !strings.Contains(got, "TASK-001") || !strings.Contains(got, "done shown") {
		t.Fatalf("shown-done view should restore done row: %q", got)
	}
}

func TestWSTaskTUI_TodoOnlyStartsWithDoneRowsHidden(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Finished\nstatus: done\n\n### TASK-002 Next\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{todoOnly: true}, false)
	if got := m.View(); strings.Contains(got, "TASK-001") || !strings.Contains(got, "TASK-002") || !strings.Contains(got, "Progress  1/2") {
		t.Fatalf("todo-only TUI should hide done rows but keep full progress: %q", got)
	}
}

func TestWSTaskTUI_ToggleRowUpdatesTasksFile(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	editing, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = editing.(wsTaskTUIModel)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok := updated.(wsTaskTUIModel)
	if !ok {
		t.Fatalf("updated model = %T, want wsTaskTUIModel", updated)
	}
	if next.rows[0].Item.Status != wstask.StatusDone {
		t.Fatalf("task status = %s, want done", next.rows[0].Item.Status)
	}
	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: done")) {
		t.Fatalf("workspace.md should be updated to done: %q", string(content))
	}
}

func TestWSTaskTUI_ReadModeDoesNotUpdateTasksFile(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok := updated.(wsTaskTUIModel)
	if !ok {
		t.Fatalf("updated model = %T, want wsTaskTUIModel", updated)
	}
	if next.rows[0].Item.Status != wstask.StatusTodo {
		t.Fatalf("task status = %s, want todo", next.rows[0].Item.Status)
	}
	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: todo")) || bytes.Contains(content, []byte("status: done")) {
		t.Fatalf("workspace.md should remain todo in read mode: %q", string(content))
	}
	if !strings.Contains(next.message, "press i") {
		t.Fatalf("message = %q, want edit hint", next.message)
	}
}

func TestWSTaskTUI_MouseWheelScrollsAndClickUsesVisibleRow(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-002 Second\nstatus: todo\n\n### TASK-003 Third\nstatus: todo\n\n### TASK-004 Fourth\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	m.height = 8
	editing, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = editing.(wsTaskTUIModel)
	scrolled, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	m = scrolled.(wsTaskTUIModel)
	if m.scroll == 0 {
		t.Fatalf("scroll = %d, want non-zero after wheel down", m.scroll)
	}
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 2, Y: wsTaskTUIContentTopY})
	next := updated.(wsTaskTUIModel)
	if next.rows[0].Item.Status != wstask.StatusTodo {
		t.Fatalf("first visible-unrelated task status = %s, want todo", next.rows[0].Item.Status)
	}
	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("### TASK-004 Fourth\nstatus: done")) {
		t.Fatalf("expected scrolled click to update first visible task, got %q", string(content))
	}
}

func TestWSTaskTUI_MouseClickUsesRenderedTaskRow(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-002 Second\nstatus: todo\n\n### TASK-003 Third\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	editing, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = editing.(wsTaskTUIModel)
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 2, Y: m.rows[1].Y})
	next := updated.(wsTaskTUIModel)
	if next.rows[1].Item.Status != wstask.StatusDone {
		t.Fatalf("clicked task status = %s, want done", next.rows[1].Item.Status)
	}
	if next.rows[2].Item.Status != wstask.StatusTodo {
		t.Fatalf("next task status = %s, want todo", next.rows[2].Item.Status)
	}
	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("### TASK-002 Second\nstatus: done")) {
		t.Fatalf("expected click to update TASK-002, got %q", string(content))
	}
	if bytes.Contains(content, []byte("### TASK-003 Third\nstatus: done")) {
		t.Fatalf("click updated the row below: %q", string(content))
	}
}

func TestWSTaskTUI_AllModeRendersCurrentStateWithoutTaskRows(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Current State\n\nWorking on the current-state Dock view.\n\n## Tasks\n\n### TASK-001 First\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{}, wsTaskTUIOptions{target: wsTaskTargetOptions{useAll: true}}, false)
	if len(m.rows) != 0 {
		t.Fatalf("rows = %+v, want no task rows in all mode", m.rows)
	}
	view := m.View()
	if !strings.Contains(view, "Working on the current-state Dock view.") {
		t.Fatalf("view missing current state: %q", view)
	}
	if strings.Contains(view, "TASK-001") {
		t.Fatalf("all mode should not render task rows: %q", view)
	}
	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: todo")) {
		t.Fatalf("workspace.md should not be updated: %q", string(content))
	}
}

func TestRenderWSTaskListRow_DoingUsesInfoColor(t *testing.T) {
	got := renderWSTaskListRow(wstask.Item{
		ID:     "TASK-001",
		Title:  "doing",
		Status: wstask.StatusDoing,
	}, true)

	if !strings.Contains(got, ansiBlue) {
		t.Fatalf("doing row should use info color, got %q", got)
	}
	if strings.Contains(got, ansiCyan) {
		t.Fatalf("doing row should not use accent color, got %q", got)
	}
}

func TestCLI_WSTaskAdd_JSON_RejectsArchivedWorkspace(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "archived", "WS1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "add", "--id", "WS1", "--title", "Nope", "--format", "json"})
	if code != exitError {
		t.Fatalf("ws task add exit code = %d, want %d", code, exitError)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCLI_WSTaskSync_JSON_DeprecatedNoOp(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-003 Draft docs\nstatus: todo\n\n### TASK-001 Build parser\nstatus: doing\n\n### TASK-004 Shipped\nstatus: done\n\n### TASK-002 Waiting review\nstatus: blocked\n")
	seedCMUXWorkspaceMapping(t, env.Root, "WS1", "cmux-1")
	syncClient := newFakeWSTaskSyncClient()
	syncClient.statuses["cmux-1"] = []cmuxctl.StatusEntry{
		{Key: "task:TASK-001", Value: "● TASK-001 Build parser", Icon: "checklist", Color: "#0ea5e9"},
		{Key: "task:TASK-003", Value: "○ TASK-003 Draft docs", Icon: "checklist", Color: "#ffffff"},
		{Key: "task:TASK-999", Value: "✔ TASK-999 Legacy", Icon: "checklist", Color: "#64748b"},
		{Key: "kra", Value: "kra:workspace", Icon: "tag", Color: "#4F46E5"},
	}
	useFakeWSTaskSyncClient(t, syncClient)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "sync", "--id", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws task sync exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.task.sync" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	syncResult, ok := resp.Result["sync"].(map[string]any)
	if !ok {
		t.Fatalf("result.sync missing: %+v", resp.Result)
	}
	if got := syncResult["state"]; got != "skipped" {
		t.Fatalf("sync.state = %v, want skipped", got)
	}
	if warning, _ := syncResult["warning"].(string); !strings.Contains(warning, "deprecated") {
		t.Fatalf("sync.warning = %v, want deprecated warning", syncResult["warning"])
	}
	if got := syncResult["set"]; got != float64(0) {
		t.Fatalf("sync.set = %v, want 0", got)
	}
	if got := syncResult["cleared"]; got != float64(0) {
		t.Fatalf("sync.cleared = %v, want 0", got)
	}
	if len(syncClient.setCalls) != 0 || len(syncClient.clearCalls) != 0 {
		t.Fatalf("cmux sync calls = set:%+v clear:%+v, want none", syncClient.setCalls, syncClient.clearCalls)
	}
}

func TestCLI_WSTaskSync_JSON_DoesNotClearMissingTaskNamespaceEntries(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n---\n")
	seedCMUXWorkspaceMapping(t, env.Root, "WS1", "cmux-1")
	syncClient := newFakeWSTaskSyncClient()
	syncClient.statuses["cmux-1"] = []cmuxctl.StatusEntry{
		{Key: "task:TASK-001", Value: "● TASK-001 Build parser", Icon: "checklist", Color: "#0ea5e9"},
		{Key: "task:TASK-002", Value: "✔ TASK-002 Legacy", Icon: "checklist", Color: "#64748b"},
		{Key: "kra", Value: "kra:workspace", Icon: "tag", Color: "#4F46E5"},
	}
	useFakeWSTaskSyncClient(t, syncClient)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "sync", "--id", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws task sync exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.task.sync" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	syncResult, ok := resp.Result["sync"].(map[string]any)
	if !ok {
		t.Fatalf("result.sync missing: %+v", resp.Result)
	}
	if got := syncResult["state"]; got != "skipped" {
		t.Fatalf("sync.state = %v, want skipped", got)
	}
	if warning, _ := syncResult["warning"].(string); !strings.Contains(warning, "deprecated") {
		t.Fatalf("sync.warning = %v, want deprecated warning", syncResult["warning"])
	}
	if len(syncClient.setCalls) != 0 || len(syncClient.clearCalls) != 0 {
		t.Fatalf("cmux sync calls = set:%+v clear:%+v, want none", syncClient.setCalls, syncClient.clearCalls)
	}
}

func TestCLI_WSTaskSync_JSON_All_Success(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath1 := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	wsPath2 := seedWorkspaceMeta(t, env.Root, "active", "WS2")
	writeWorkspaceTasksFile(t, wsPath1, "## Tasks\n\n### TASK-001 First\nstatus: todo\n")
	writeWorkspaceTasksFile(t, wsPath2, "## Tasks\n\n### TASK-002 Second\nstatus: doing\n")
	seedCMUXWorkspaceMapping(t, env.Root, "WS1", "cmux-1")
	seedCMUXWorkspaceMapping(t, env.Root, "WS2", "cmux-2")
	syncClient := newFakeWSTaskSyncClient()
	useFakeWSTaskSyncClient(t, syncClient)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "sync", "--all", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws task sync --all exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.task.sync" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["count"]; got != float64(2) {
		t.Fatalf("result.count = %v, want 2", got)
	}
	if got := resp.Result["set"]; got != float64(0) {
		t.Fatalf("result.set = %v, want 0", got)
	}
	if got := resp.Result["skipped"]; got != float64(2) {
		t.Fatalf("result.skipped = %v, want 2", got)
	}
	if got := resp.Result["failed"]; got != float64(0) {
		t.Fatalf("result.failed = %v, want 0", got)
	}
}

func TestCLI_WSTaskSync_JSON_All_RejectsID(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "sync", "--id", "WS1", "--all", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("ws task sync exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Error.Code != "invalid_argument" || !strings.Contains(resp.Error.Message, "--id and --all cannot be used together") {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCLI_WSTaskDockInstall_CreatesGlobalDock(t *testing.T) {
	setKraHomeForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	globalPath := filepath.Join(home, ".config", "cmux", "dock.json")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "dock", "install"})
	if code != exitOK {
		t.Fatalf("ws task dock install exit code=%d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	b, readErr := os.ReadFile(globalPath)
	if readErr != nil {
		t.Fatalf("read global dock: %v", readErr)
	}
	var dock cmuxDockConfig
	if err := json.Unmarshal(b, &dock); err != nil {
		t.Fatalf("unmarshal global dock: %v", err)
	}
	if len(dock.Controls) != 1 || dock.Controls[0].ID != "kra-tasks" {
		t.Fatalf("unexpected controls: %+v", dock.Controls)
	}
	if !strings.Contains(dock.Controls[0].Command, "kra ws status --cmux-current") {
		t.Fatalf("unexpected command: %q", dock.Controls[0].Command)
	}
}

func TestCLI_WSTaskDockInstall_PreservesExistingControlsAndUpdatesKraTasks(t *testing.T) {
	setKraHomeForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	globalPath := filepath.Join(home, ".config", "cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global dock dir: %v", err)
	}
	existing := `{
  "controls": [
    {
      "id": "custom",
      "title": "Custom",
      "command": "echo custom",
      "height": 100
    },
    {
      "id": "kra-tasks",
      "title": "Old",
      "command": "old command",
      "height": 123
    }
  ]
}
`
	if err := os.WriteFile(globalPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write global dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "dock", "install", "--global", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws task dock install exit code=%d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if !resp.OK || resp.Action != "ws.task.dock.install" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	b, readErr := os.ReadFile(globalPath)
	if readErr != nil {
		t.Fatalf("read global dock: %v", readErr)
	}
	var dock cmuxDockConfig
	if err := json.Unmarshal(b, &dock); err != nil {
		t.Fatalf("unmarshal global dock: %v", err)
	}
	if len(dock.Controls) != 2 {
		t.Fatalf("controls len=%d, want 2: %+v", len(dock.Controls), dock.Controls)
	}
	if dock.Controls[0].ID != "custom" || dock.Controls[0].Command != "echo custom" {
		t.Fatalf("custom control not preserved: %+v", dock.Controls[0])
	}
	if dock.Controls[1].ID != "kra-tasks" || dock.Controls[1].Title != "Status" || dock.Controls[1].Height != 420 {
		t.Fatalf("kra-tasks not updated: %+v", dock.Controls[1])
	}
}

func TestCLI_WSTaskDockInstall_InvalidGlobalDockFailsClosed(t *testing.T) {
	setKraHomeForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	globalPath := filepath.Join(home, ".config", "cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global dock dir: %v", err)
	}
	if err := os.WriteFile(globalPath, []byte("{not json\n"), 0o644); err != nil {
		t.Fatalf("write invalid global dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "dock", "install"})
	if code == exitOK {
		t.Fatalf("ws task dock install exit code=%d, want failure", code)
	}
	got, readErr := os.ReadFile(globalPath)
	if readErr != nil {
		t.Fatalf("read global dock: %v", readErr)
	}
	if string(got) != "{not json\n" {
		t.Fatalf("invalid global dock was modified: %q", string(got))
	}
}

func TestCLI_WSTaskLauncher_UpdatesSelectedTaskWithoutSync(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-002 Second\nstatus: blocked\n")
	seedCMUXWorkspaceMapping(t, env.Root, "WS1", "cmux-1")
	syncClient := newFakeWSTaskSyncClient()
	useFakeWSTaskSyncClient(t, syncClient)

	origTaskPrompt := promptWSTaskSelection
	origStatusPrompt := promptWSTaskStatusSelection
	promptWSTaskSelection = func(_ *CLI, _ string, _ []workspaceSelectorCandidate) ([]string, error) {
		return []string{"TASK-002"}, nil
	}
	promptWSTaskStatusSelection = func(_ *CLI, _ string, _ []workspaceSelectorCandidate) ([]string, error) {
		return []string{"done"}, nil
	}
	t.Cleanup(func() {
		promptWSTaskSelection = origTaskPrompt
		promptWSTaskStatusSelection = origStatusPrompt
	})

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "task", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws task launcher exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "task updated: TASK-002") {
		t.Fatalf("stdout = %q, want task updated result", out.String())
	}

	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("### TASK-002 Second\nstatus: done")) {
		t.Fatalf("workspace.md should be updated to done: %q", string(content))
	}
	if len(syncClient.setCalls) != 0 || len(syncClient.clearCalls) != 0 {
		t.Fatalf("cmux sync calls = set:%+v clear:%+v, want none", syncClient.setCalls, syncClient.clearCalls)
	}
}

func TestCLI_WSList_JSON_IncludesTaskSummary(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Build parser\nstatus: doing\n\n### TASK-002 Write tests\nstatus: todo\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "list", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	items, ok := resp.Result["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("result.items = %#v", resp.Result["items"])
	}
	row := items[0].(map[string]any)
	tasks := row["tasks"].(map[string]any)
	if got := tasks["summary"]; got != "counts" {
		t.Fatalf("tasks.summary = %v, want %q", got, "counts")
	}
	counts := tasks["counts"].(map[string]any)
	if got := counts["doing"]; got != float64(1) {
		t.Fatalf("counts.doing = %v, want 1", got)
	}
}

func TestCLI_WSDashboard_JSON_IncludesTaskSummaryAndDetail(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Build parser\nstatus: doing\n\n### TASK-002 Wait for review\nstatus: blocked\n\n### TASK-003 Ship it\nstatus: done\n")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--workspace", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	workspaces := resp.Result["workspaces"].([]any)
	row := workspaces[0].(map[string]any)
	tasks := row["tasks"].(map[string]any)
	if got := tasks["summary"]; got != "counts" {
		t.Fatalf("tasks.summary = %v, want %q", got, "counts")
	}

	detail := resp.Result["detail"].(map[string]any)
	detailTasks := detail["tasks"].(map[string]any)
	activeItems := detailTasks["active_items"].(map[string]any)
	doing := activeItems["doing"].([]any)
	blocked := activeItems["blocked"].([]any)
	if len(doing) != 1 || len(blocked) != 1 {
		t.Fatalf("active_items = %#v", activeItems)
	}
}

func writeWorkspaceTasksFile(t *testing.T, workspacePath string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspacePath, workspaceDocumentFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write workspace.md: %v", err)
	}
}
