package wstask

import (
	"context"
	"errors"
	"testing"
)

type memoryPort struct {
	docs      map[string]DocumentSnapshot
	pathToKey map[string]string
}

type memorySyncPort struct {
	targets  map[string][]string
	statuses map[string][]SyncStatusEntry
	setCalls []struct {
		workspace string
		entry     SyncStatusEntry
	}
	clearCalls []struct {
		workspace string
		key       string
	}
}

func newMemoryPort() *memoryPort {
	return &memoryPort{
		docs:      map[string]DocumentSnapshot{},
		pathToKey: map[string]string{},
	}
}

func newMemorySyncPort() *memorySyncPort {
	return &memorySyncPort{
		targets:  map[string][]string{},
		statuses: map[string][]SyncStatusEntry{},
	}
}

func (p *memoryPort) key(scope string, workspaceID string) string {
	return scope + ":" + workspaceID
}

func (p *memoryPort) set(scope string, workspaceID string, snapshot DocumentSnapshot) {
	key := p.key(scope, workspaceID)
	p.docs[key] = snapshot
	p.pathToKey[snapshot.Path] = key
}

func (p *memoryPort) Load(root string, workspaceID string, scope string) (DocumentSnapshot, error) {
	snapshot, ok := p.docs[p.key(scope, workspaceID)]
	if !ok {
		return DocumentSnapshot{}, ErrWorkspaceNotFound
	}
	return snapshot, nil
}

func (p *memoryPort) Save(path string, content string) error {
	key, ok := p.pathToKey[path]
	if !ok {
		return errors.New("unknown path")
	}
	snapshot := p.docs[key]
	snapshot.Exists = true
	snapshot.Content = content
	p.docs[key] = snapshot
	return nil
}

func (p *memorySyncPort) key(root string, workspaceID string) string {
	return root + ":" + workspaceID
}

func (p *memorySyncPort) ListCMUXWorkspaceIDs(root string, workspaceID string) ([]string, error) {
	return append([]string{}, p.targets[p.key(root, workspaceID)]...), nil
}

func (p *memorySyncPort) ListStatuses(_ context.Context, cmuxWorkspaceID string) ([]SyncStatusEntry, error) {
	return append([]SyncStatusEntry{}, p.statuses[cmuxWorkspaceID]...), nil
}

func (p *memorySyncPort) SetStatus(_ context.Context, cmuxWorkspaceID string, entry SyncStatusEntry) error {
	p.setCalls = append(p.setCalls, struct {
		workspace string
		entry     SyncStatusEntry
	}{workspace: cmuxWorkspaceID, entry: entry})
	rows := append([]SyncStatusEntry{}, p.statuses[cmuxWorkspaceID]...)
	replaced := false
	for i := range rows {
		if rows[i].Key == entry.Key {
			rows[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		rows = append([]SyncStatusEntry{entry}, rows...)
	}
	p.statuses[cmuxWorkspaceID] = rows
	return nil
}

func (p *memorySyncPort) ClearStatus(_ context.Context, cmuxWorkspaceID string, key string) error {
	p.clearCalls = append(p.clearCalls, struct {
		workspace string
		key       string
	}{workspace: cmuxWorkspaceID, key: key})
	rows := p.statuses[cmuxWorkspaceID]
	out := rows[:0]
	for _, row := range rows {
		if row.Key != key {
			out = append(out, row)
		}
	}
	p.statuses[cmuxWorkspaceID] = append([]SyncStatusEntry{}, out...)
	return nil
}

func TestServiceAdd_CreatesTasksSectionAndPreservesOutsideContent(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "# Memo\n\nKeep this.\n\n## Notes\n\nstill here\n",
	})

	svc := NewService(port)
	result, err := svc.Add("/root", "WS1", "First task", "Line one")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if result.Task.ID != "TASK-001" {
		t.Fatalf("task id = %q, want %q", result.Task.ID, "TASK-001")
	}

	got := port.docs["active:WS1"].Content
	want := "# Memo\n\nKeep this.\n\n## Notes\n\nstill here\n\n## Tasks\n\n### TASK-001 First task\nstatus: todo\n\nLine one\n"
	if got != want {
		t.Fatalf("saved content mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestServiceList_FailsOnDuplicateTaskIDs(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-001 Second\nstatus: doing\n",
	})

	_, err := NewService(port).List("/root", "WS1", "active")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("List() error = %v, want conflict", err)
	}
}

func TestServiceOverview_DegradesOnInvalidContract(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-001 Broken\nno-status-here\n",
	})

	result, err := NewService(port).Overview("/root", "WS1", "active")
	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if result.Overview.Summary != SummaryInvalid {
		t.Fatalf("summary = %q, want %q", result.Overview.Summary, SummaryInvalid)
	}
	if result.Overview.Warning == "" {
		t.Fatalf("warning should not be empty")
	}
}

func TestServiceStart_AllowsBlockedToDoingAndRejectsDone(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-001 Waiting\nstatus: blocked\n\n### TASK-002 Finished\nstatus: done\n",
	})

	svc := NewService(port)
	okResult, err := svc.Start("/root", "WS1", "TASK-001")
	if err != nil {
		t.Fatalf("Start(blocked) error = %v", err)
	}
	if !okResult.Task.Changed || okResult.Task.Status != StatusDoing {
		t.Fatalf("changed=%v status=%s, want true/doing", okResult.Task.Changed, okResult.Task.Status)
	}

	_, err = svc.Start("/root", "WS1", "TASK-002")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Start(done) error = %v, want conflict", err)
	}
}

func TestServiceStatus_AllowsDoneToTodo(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-001 Finished\nstatus: done\n",
	})

	result, err := NewService(port).Status("/root", "WS1", "TASK-001", StatusTodo)
	if err != nil {
		t.Fatalf("Status(done->todo) error = %v", err)
	}
	if !result.Task.Changed || result.Task.Status != StatusTodo {
		t.Fatalf("changed=%v status=%s, want true/todo", result.Task.Changed, result.Task.Status)
	}
}

func TestServiceSync_ReconcilesTaskNamespace(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-003 Draft docs\nstatus: todo\n\n### TASK-001 Build parser\nstatus: doing\n\n### TASK-004 Shipped\nstatus: done\n\n### TASK-002 Waiting review\nstatus: blocked\n",
	})
	syncPort := newMemorySyncPort()
	syncPort.targets[syncPort.key("/root", "WS1")] = []string{"cmux-1"}
	syncPort.statuses["cmux-1"] = []SyncStatusEntry{
		{Key: "task:TASK-001", Value: "● TASK-001 Build parser", Icon: "checklist", Color: "#0ea5e9"},
		{Key: "task:TASK-003", Value: "○ TASK-003 Draft docs", Icon: "checklist", Color: "#ffffff"},
		{Key: "task:TASK-999", Value: "✔ TASK-999 Legacy", Icon: "checklist", Color: "#64748b"},
		{Key: "kra", Value: "kra:workspace", Icon: "tag", Color: "#4F46E5"},
	}

	result, err := NewService(port, syncPort).Sync(context.Background(), "/root", "WS1")
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.State != SyncStateApplied || result.Targets != 1 || result.SetCount != 4 || result.ClearCount != 3 {
		t.Fatalf("sync result = %+v", result)
	}
	if len(syncPort.setCalls) != 4 {
		t.Fatalf("set calls = %+v, want 4 calls", syncPort.setCalls)
	}
	if syncPort.setCalls[0].entry.Key != "task:TASK-002" || syncPort.setCalls[1].entry.Key != "task:TASK-004" || syncPort.setCalls[2].entry.Key != "task:TASK-001" || syncPort.setCalls[3].entry.Key != "task:TASK-003" {
		t.Fatalf("set calls = %+v, want reverse replay TASK-002/TASK-004/TASK-001/TASK-003", syncPort.setCalls)
	}
	if got := syncPort.statuses["cmux-1"]; len(got) != 5 || got[0].Key != "task:TASK-003" || got[1].Key != "task:TASK-001" || got[2].Key != "task:TASK-004" || got[3].Key != "task:TASK-002" {
		t.Fatalf("statuses = %+v, want rendered order TASK-003/TASK-001/TASK-004/TASK-002", got)
	}
	if got := syncPort.statuses["cmux-1"][0]; got.Value != "○ TASK-003 Draft docs" || got.Color != "#ffffff" {
		t.Fatalf("todo sync entry = %+v, want white todo pill", got)
	}
	if len(syncPort.clearCalls) != 3 {
		t.Fatalf("clear calls = %+v, want 3 calls", syncPort.clearCalls)
	}
	if syncPort.clearCalls[0].key != "task:TASK-001" || syncPort.clearCalls[1].key != "task:TASK-003" || syncPort.clearCalls[2].key != "task:TASK-999" {
		t.Fatalf("clear calls = %+v, want all existing task pills cleared first", syncPort.clearCalls)
	}
}

func TestServiceSync_ClearsAllTaskNamespaceWhenStructuredTasksAreEmpty(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "## Tasks\n\n---\n",
	})
	syncPort := newMemorySyncPort()
	syncPort.targets[syncPort.key("/root", "WS1")] = []string{"cmux-1"}
	syncPort.statuses["cmux-1"] = []SyncStatusEntry{
		{Key: "task:TASK-001", Value: "● TASK-001 Build parser", Icon: "checklist", Color: "#0ea5e9"},
		{Key: "task:TASK-002", Value: "○ TASK-002 Draft docs", Icon: "checklist", Color: "#ffffff"},
		{Key: "kra", Value: "kra:workspace", Icon: "tag", Color: "#4F46E5"},
	}

	result, err := NewService(port, syncPort).Sync(context.Background(), "/root", "WS1")
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.State != SyncStateApplied || result.Targets != 1 || result.SetCount != 0 || result.ClearCount != 2 {
		t.Fatalf("sync result = %+v", result)
	}
	if len(syncPort.setCalls) != 0 {
		t.Fatalf("set calls = %+v, want none", syncPort.setCalls)
	}
	if len(syncPort.clearCalls) != 2 {
		t.Fatalf("clear calls = %+v, want 2 calls", syncPort.clearCalls)
	}
	if syncPort.clearCalls[0].key != "task:TASK-001" || syncPort.clearCalls[1].key != "task:TASK-002" {
		t.Fatalf("clear calls = %+v, want TASK-001/TASK-002", syncPort.clearCalls)
	}
}

func TestServiceSync_FailsClosedOnInvalidTaskLikeBlock(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-001\nstatus: doing\n",
	})
	syncPort := newMemorySyncPort()
	syncPort.targets[syncPort.key("/root", "WS1")] = []string{"cmux-1"}
	syncPort.statuses["cmux-1"] = []SyncStatusEntry{
		{Key: "task:TASK-001", Value: "● TASK-001 Legacy", Icon: "checklist", Color: "#0ea5e9"},
	}

	_, err := NewService(port, syncPort).Sync(context.Background(), "/root", "WS1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Sync() error = %v, want conflict", err)
	}
	if len(syncPort.setCalls) != 0 {
		t.Fatalf("set calls = %+v, want none", syncPort.setCalls)
	}
	if len(syncPort.clearCalls) != 0 {
		t.Fatalf("clear calls = %+v, want none", syncPort.clearCalls)
	}
}

func TestServiceSync_SkipsWhenNoMappedCMUXWorkspace(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/tasks.md",
		Exists:  false,
		Content: "",
	})

	result, err := NewService(port, newMemorySyncPort()).Sync(context.Background(), "/root", "WS1")
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.State != SyncStateSkipped || result.WarningText == "" {
		t.Fatalf("sync result = %+v, want skipped with warning", result)
	}
}
