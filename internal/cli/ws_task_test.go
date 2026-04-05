package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
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
	if err := os.WriteFile(filepath.Join(wsPath, "tasks.md"), []byte("# Notes\n\noutside\n"), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
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

	content, readErr := os.ReadFile(filepath.Join(wsPath, "tasks.md"))
	if readErr != nil {
		t.Fatalf("read tasks.md: %v", readErr)
	}
	got := string(content)
	if !bytes.Contains(content, []byte("# Notes\n\noutside\n\n## Tasks\n\n### TASK-001 First task\nstatus: todo\n\nLine one\n")) {
		t.Fatalf("tasks.md missing canonical task block: %q", got)
	}
}

func TestCLI_WSTaskStatus_JSON_AllowsDoneToTodoAndClearsCMUXTaskStatus(t *testing.T) {
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
	syncResult, ok := resp.Result["sync"].(map[string]any)
	if !ok {
		t.Fatalf("result.sync missing: %+v", resp.Result)
	}
	if got := syncResult["set"]; got != float64(1) {
		t.Fatalf("sync.set = %v, want 1", got)
	}

	content, readErr := os.ReadFile(filepath.Join(wsPath, "tasks.md"))
	if readErr != nil {
		t.Fatalf("read tasks.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: todo")) {
		t.Fatalf("tasks.md should be updated to todo: %q", string(content))
	}
	if len(syncClient.setCalls) != 1 || syncClient.setCalls[0].Key != "task:TASK-001" || syncClient.setCalls[0].Value != "○ TASK-001 Start me" {
		t.Fatalf("set calls = %+v, want todo pill for task:TASK-001", syncClient.setCalls)
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

func TestCLI_WSTaskSync_JSON_ReconcilesCMUXTaskNamespace(t *testing.T) {
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
	if got := syncResult["set"]; got != float64(4) {
		t.Fatalf("sync.set = %v, want 4", got)
	}
	if got := syncResult["cleared"]; got != float64(3) {
		t.Fatalf("sync.cleared = %v, want 3", got)
	}
	if len(syncClient.setCalls) != 4 {
		t.Fatalf("set calls = %+v, want 4 calls", syncClient.setCalls)
	}
	if syncClient.setCalls[0].Key != "task:TASK-002" || syncClient.setCalls[1].Key != "task:TASK-004" || syncClient.setCalls[2].Key != "task:TASK-001" || syncClient.setCalls[3].Key != "task:TASK-003" {
		t.Fatalf("set calls = %+v, want reverse replay order", syncClient.setCalls)
	}
	if got := syncClient.statuses["cmux-1"]; len(got) != 5 || got[0].Key != "task:TASK-003" || got[1].Key != "task:TASK-001" || got[2].Key != "task:TASK-004" || got[3].Key != "task:TASK-002" {
		t.Fatalf("statuses = %+v, want rendered order TASK-003/TASK-001/TASK-004/TASK-002", got)
	}
	if got := syncClient.statuses["cmux-1"][0]; got.Value != "○ TASK-003 Draft docs" || got.Color != "#ffffff" {
		t.Fatalf("todo pill = %+v, want markdown-leading task with white color", got)
	}
	if len(syncClient.clearCalls) != 3 {
		t.Fatalf("clear calls = %+v, want 3 calls", syncClient.clearCalls)
	}
}

func TestCLI_WSTaskSync_JSON_ClearsMissingTaskNamespaceEntries(t *testing.T) {
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
	if got := syncResult["set"]; got != float64(0) {
		t.Fatalf("sync.set = %v, want 0", got)
	}
	if got := syncResult["cleared"]; got != float64(2) {
		t.Fatalf("sync.cleared = %v, want 2", got)
	}
	if len(syncClient.setCalls) != 0 {
		t.Fatalf("set calls = %+v, want none", syncClient.setCalls)
	}
	if len(syncClient.clearCalls) != 2 {
		t.Fatalf("clear calls = %+v, want 2 calls", syncClient.clearCalls)
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
	if got := resp.Result["set"]; got != float64(2) {
		t.Fatalf("result.set = %v, want 2", got)
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

func TestCLI_WSTaskLauncher_UpdatesSelectedTaskAndSyncs(t *testing.T) {
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

	content, readErr := os.ReadFile(filepath.Join(wsPath, "tasks.md"))
	if readErr != nil {
		t.Fatalf("read tasks.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("### TASK-002 Second\nstatus: done")) {
		t.Fatalf("tasks.md should be updated to done: %q", string(content))
	}
	if len(syncClient.setCalls) != 2 {
		t.Fatalf("set calls = %+v, want two task pills", syncClient.setCalls)
	}
	if syncClient.setCalls[0].Key != "task:TASK-002" || syncClient.setCalls[0].Value != "✔ TASK-002 Second" {
		t.Fatalf("first set call = %+v, want reverse replay to start from tail task", syncClient.setCalls[0])
	}
	if syncClient.setCalls[1].Key != "task:TASK-001" || syncClient.setCalls[1].Value != "○ TASK-001 First" {
		t.Fatalf("second set call = %+v, want reverse replay to end at head task", syncClient.setCalls[1])
	}
	if got := syncClient.statuses["cmux-1"]; len(got) != 2 || got[0].Key != "task:TASK-001" || got[1].Key != "task:TASK-002" {
		t.Fatalf("statuses = %+v, want rendered order TASK-001/TASK-002", got)
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
	if err := os.WriteFile(filepath.Join(workspacePath, "tasks.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}
}
