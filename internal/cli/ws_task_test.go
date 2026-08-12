package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
	"github.com/tasuku43/kra/internal/testutil"
)

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
	if !bytes.Contains(content, []byte("# Notes\n\noutside\n\n## Tasks\n\n### TASK-001 First task\nstatus: todo\n\nLine one\n")) {
		t.Fatalf("workspace.md missing canonical task block: %q", got)
	}
}

func TestCLI_WSTaskStatus_JSON_AllowsDoneToTodo(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 Start me\nstatus: done\n")
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
	content, readErr := os.ReadFile(filepath.Join(wsPath, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: todo")) {
		t.Fatalf("workspace.md should be updated to todo: %q", string(content))
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
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 2, Y: m.rows[3].Y - m.scroll})
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

func TestWSTaskTUI_MouseClickUsesRenderedTaskRowWithCurrentStateAndNext(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Current State\n\nLegacy state ignored.\n\n## Next\n\nLegacy next ignored.\n\n## Tasks\n\n### TASK-001 First\nstatus: doing\n\nInvestigating click hit testing.\n\n### TASK-002 Second\nstatus: todo\n\nPatch the rendered row calculation.\n\n### TASK-003 Third\nstatus: todo\n")

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

func TestWSTaskTUI_CurrentStateWrapsWithoutTruncation(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	longState := "日本語の長いカレントステートを画面幅に合わせて折り返し、途中で消さずに最後まで表示する"
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: doing\n\n"+longState+"\n\n### TASK-002 Next\nstatus: todo\n\n次の具体的な作業も折り返して表示する\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	m.width = 24
	m.rows = buildWSTaskTUIRows(m.model, m.width)
	view := m.View()
	for _, want := range []string{"日本語の長い", "後まで表示する", "次の具体的な作業"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q after wrapping: %q", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "日本語") && displayWidth(line) > m.width {
			t.Fatalf("wrapped current state line width = %d, want <= %d: %q", displayWidth(line), m.width, line)
		}
	}
	if m.rows[0].Y <= wsTaskTUIBodyTopY {
		t.Fatalf("task row Y = %d, want below wrapped current state", m.rows[0].Y)
	}
}

func TestWSTaskTUI_CurrentStateScrolls(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: doing\n\nline 1\nline 2\nline 3\nline 4\nline 5\nline 6\n\n### TASK-002 Next\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, false)
	m.height = 8
	view := m.View()
	if !strings.Contains(view, "line 1") {
		t.Fatalf("initial view missing first current-state line: %q", view)
	}
	scrolled, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	next := scrolled.(wsTaskTUIModel)
	if next.scroll == 0 {
		t.Fatalf("scroll = %d, want non-zero for long current state", next.scroll)
	}
	view = next.View()
	if strings.Contains(view, "line 1") {
		t.Fatalf("scrolled view should move past first current-state line: %q", view)
	}
}

func TestWSTaskTUI_CurrentStateRendersLightMarkdown(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: doing\n\n- **Done** `code` path\n  - nested item\n\n### TASK-002 Next\nstatus: todo\n")

	m := newWSTaskTUIModel(env.Root, wsTaskTarget{workspaceID: "WS1", scope: "active"}, wsTaskTUIOptions{}, true)
	view := m.View()
	if !strings.Contains(view, "•") {
		t.Fatalf("view missing bullet marker: %q", view)
	}
	if !strings.Contains(view, ansiBold+ansiBody+"Done") {
		t.Fatalf("view missing bold markdown: %q", view)
	}
	if !strings.Contains(view, ansiAccent+"code") {
		t.Fatalf("view missing inline code accent: %q", view)
	}
	plain := stripANSISequences(view)
	if !strings.Contains(plain, uiIndent+"  ● TASK-001") {
		t.Fatalf("view missing indented task row: %q", view)
	}
}

func TestWSTaskTUI_AllModeRendersCurrentStateWithoutTaskRows(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: doing\n\nWorking on the current-state Dock view.\n\n### TASK-002 Next\nstatus: todo\n")

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

func TestCLI_WSTaskRetiredDockCommand_IsUnknown(t *testing.T) {
	setKraHomeForTest(t)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "dock", args: []string{"ws", "task", "dock", "install"}, want: `unknown command: "ws task dock"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var err bytes.Buffer
			code := New(&out, &err).Run(tc.args)
			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d", code, exitUsage)
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", out.String())
			}
			if !strings.Contains(err.String(), tc.want) {
				t.Fatalf("stderr = %q, want %q", err.String(), tc.want)
			}
		})
	}
}

func TestCLI_WSTaskLauncher_UpdatesSelectedTask(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")
	writeWorkspaceTasksFile(t, wsPath, "## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-002 Second\nstatus: blocked\n")

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
