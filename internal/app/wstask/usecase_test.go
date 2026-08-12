package wstask

import (
	"errors"
	"strings"
	"testing"
)

type memoryPort struct {
	docs       map[string]DocumentSnapshot
	pathToKey  map[string]string
	beforeSave func()
}

func newMemoryPort() *memoryPort {
	return &memoryPort{
		docs:      map[string]DocumentSnapshot{},
		pathToKey: map[string]string{},
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

func (p *memoryPort) Save(snapshot DocumentSnapshot, content string) error {
	if p.beforeSave != nil {
		p.beforeSave()
		p.beforeSave = nil
	}
	key, ok := p.pathToKey[snapshot.Path]
	if !ok {
		return errors.New("unknown path")
	}
	current := p.docs[key]
	if current.Exists != snapshot.Exists || current.Content != snapshot.Content {
		return newConflict("workspace document changed during update")
	}
	current.Exists = true
	current.Content = content
	p.docs[key] = current
	return nil
}

func TestServiceAdd_CreatesTasksSectionAndPreservesOutsideContent(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.md",
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

func TestServiceList_AllowsExtendedTaskIDs(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:   "/root/workspaces/WS1/workspace.md",
		Exists: true,
		Content: "## Tasks\n\n" +
			"### TASK-001a Follow-up\nstatus: todo\n\n" +
			"### TASK-001-1 Split work\nstatus: doing\n",
	})

	result, err := NewService(port).List("/root", "WS1", "active")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Overview.Items) != 2 {
		t.Fatalf("items = %+v, want 2 items", result.Overview.Items)
	}
	if result.Overview.Items[0].ID != "TASK-001a" || result.Overview.Items[1].ID != "TASK-001-1" {
		t.Fatalf("task IDs = %+v, want extended IDs", result.Overview.Items)
	}
}

func TestServiceStatus_AllowsExtendedTaskID(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-001-1 Split work\nstatus: todo\n",
	})

	result, err := NewService(port).Status("/root", "WS1", "TASK-001-1", StatusDone)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.Task.ID != "TASK-001-1" || result.Task.Status != StatusDone {
		t.Fatalf("task = %+v, want extended ID done", result.Task)
	}
	if !strings.Contains(port.docs["active:WS1"].Content, "### TASK-001-1 Split work\nstatus: done") {
		t.Fatalf("saved content did not update extended task ID: %q", port.docs["active:WS1"].Content)
	}
}

func TestServiceAdd_IgnoresExtendedTaskIDsWhenChoosingNextNumericID(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:   "/root/workspaces/WS1/workspace.md",
		Exists: true,
		Content: "## Tasks\n\n" +
			"### TASK-001a Follow-up\nstatus: todo\n\n" +
			"### TASK-001-1 Split work\nstatus: todo\n\n" +
			"### TASK-009 Existing numeric\nstatus: done\n",
	})

	result, err := NewService(port).Add("/root", "WS1", "Next numeric", "")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if result.Task.ID != "TASK-010" {
		t.Fatalf("task id = %q, want TASK-010", result.Task.ID)
	}
}

func TestServiceAdd_YAMLPreservesDependencyGraph(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:   "/root/workspaces/WS1/workspace.yaml",
		Exists: true,
		Source: SourceYAML,
		Content: "schema_version: 1\n" +
			"tasks:\n" +
			"  - id: TASK-001\n" +
			"    title: Foundation\n" +
			"    status: done\n" +
			"    depends_on: []\n" +
			"  - id: TASK-002\n" +
			"    title: Integration\n" +
			"    status: todo\n" +
			"    depends_on: [TASK-001]\n",
	})

	result, err := NewService(port).Add("/root", "WS1", "Regression test", "")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if result.Task.ID != "TASK-003" {
		t.Fatalf("task id = %q, want TASK-003", result.Task.ID)
	}

	ws, err := ParseYAMLWorkspace([]byte(port.docs["active:WS1"].Content))
	if err != nil {
		t.Fatalf("ParseYAMLWorkspace() error = %v", err)
	}
	if got := ws.Tasks[1].DependsOn; len(got) != 1 || got[0] != "TASK-001" {
		t.Fatalf("TASK-002 depends_on = %v, want [TASK-001]", got)
	}
	if got := ws.Tasks[2].DependsOn; got == nil || len(got) != 0 {
		t.Fatalf("TASK-003 depends_on = %#v, want an explicit empty list", got)
	}
}

func TestServiceStatus_YAMLPreservesDependencyGraph(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:   "/root/workspaces/WS1/workspace.yaml",
		Exists: true,
		Source: SourceYAML,
		Content: "schema_version: 1\n" +
			"tasks:\n" +
			"  - id: TASK-001\n" +
			"    title: Foundation\n" +
			"    status: done\n" +
			"    depends_on: []\n" +
			"  - id: TASK-002\n" +
			"    title: Integration\n" +
			"    status: todo\n" +
			"    depends_on: [TASK-001]\n",
	})

	result, err := NewService(port).Status("/root", "WS1", "TASK-002", StatusDoing)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !result.Task.Changed || result.Task.Status != StatusDoing {
		t.Fatalf("task change = %+v", result.Task)
	}

	ws, err := ParseYAMLWorkspace([]byte(port.docs["active:WS1"].Content))
	if err != nil {
		t.Fatalf("ParseYAMLWorkspace() error = %v", err)
	}
	if got := ws.Tasks[1].DependsOn; len(got) != 1 || got[0] != "TASK-001" {
		t.Fatalf("TASK-002 depends_on = %v, want [TASK-001]", got)
	}
}

func TestServiceAdd_InvalidYAMLFailsClosed(t *testing.T) {
	port := newMemoryPort()
	const original = "schema_version: 1\ntasks:\n  - id: TASK-001\n    title: Broken\n    status: todo\n    depends_on: [TASK-999]\n"
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.yaml",
		Exists:  true,
		Source:  SourceYAML,
		Content: original,
	})

	_, err := NewService(port).Add("/root", "WS1", "Must not be written", "")
	if err == nil || !strings.Contains(err.Error(), "depends_on references missing task") {
		t.Fatalf("Add() error = %v, want dependency validation failure", err)
	}
	if got := port.docs["active:WS1"].Content; got != original {
		t.Fatalf("invalid workspace.yaml was modified\nwant:\n%s\ngot:\n%s", original, got)
	}
}

func TestServiceStatus_InvalidYAMLFailsClosed(t *testing.T) {
	port := newMemoryPort()
	const original = "schema_version: 1\ntasks:\n  - id: TASK-001\n    title: Broken\n    status: todo\n    depends_on: [TASK-999]\n"
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.yaml",
		Exists:  true,
		Source:  SourceYAML,
		Content: original,
	})

	_, err := NewService(port).Status("/root", "WS1", "TASK-001", StatusDoing)
	if err == nil || !strings.Contains(err.Error(), "depends_on references missing task") {
		t.Fatalf("Status() error = %v, want dependency validation failure", err)
	}
	if got := port.docs["active:WS1"].Content; got != original {
		t.Fatalf("invalid workspace.yaml was modified\nwant:\n%s\ngot:\n%s", original, got)
	}
}

func TestServiceView_DerivesCurrentAndNextFromTaskDescriptions(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:   "/root/workspaces/WS1/workspace.md",
		Exists: true,
		Content: "## Current State\n\nLegacy state ignored.\n\n## Next\n\nLegacy next ignored.\n\n## Tasks\n\n" +
			"### TASK-001 Done\nstatus: done\n\n" +
			"### TASK-002 Current title\nstatus: doing\n\nCurrent description line one.\nCurrent description line two.\n\n" +
			"### TASK-003 Next title\nstatus: todo\n\nNext description.\n",
	})

	result, err := NewService(port).View("/root", "WS1", "active")
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if result.CurrentState != "Current description line one.\nCurrent description line two." {
		t.Fatalf("CurrentState = %q", result.CurrentState)
	}
	if result.Next != "Next description." {
		t.Fatalf("Next = %q", result.Next)
	}
}

func TestServiceView_DerivedCurrentAndNextFallBackToTitles(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.md",
		Exists:  true,
		Content: "## Tasks\n\n### TASK-001 Current title\nstatus: doing\n\n### TASK-002 Next title\nstatus: todo\n",
	})

	result, err := NewService(port).View("/root", "WS1", "active")
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if result.CurrentState != "Current title" {
		t.Fatalf("CurrentState = %q", result.CurrentState)
	}
	if result.Next != "Next title" {
		t.Fatalf("Next = %q", result.Next)
	}
}

func TestServiceStatus_RemovesLegacyCurrentAndNextSectionsOnWrite(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:   "/root/workspaces/WS1/workspace.md",
		Exists: true,
		Content: "# Workspace\n\n## Notes\n\nKeep me.\n\n## Current State\n\nLegacy state.\n\n## Next\n\nLegacy next.\n\n## Tasks\n\n" +
			"### TASK-001 Current title\nstatus: doing\n\nCurrent detail.\n\n" +
			"### TASK-002 Next title\nstatus: todo\n\nNext detail.\n",
	})

	_, err := NewService(port).Status("/root", "WS1", "TASK-002", StatusDone)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	got := port.docs["active:WS1"].Content
	if strings.Contains(got, "## Current State") || strings.Contains(got, "## Next") {
		t.Fatalf("legacy state sections should be removed: %q", got)
	}
	if !strings.Contains(got, "## Notes\n\nKeep me.") {
		t.Fatalf("freeform content outside managed sections should be preserved: %q", got)
	}
}

func TestServiceStatus_RefusesToOverwriteConcurrentDocumentChange(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.md",
		Exists:  true,
		Content: "## Current State\n\nInitial state.\n\n## Tasks\n\n### TASK-001 First\nstatus: todo\n",
	})
	port.beforeSave = func() {
		snapshot := port.docs["active:WS1"]
		snapshot.Content = "## Current State\n\nEdited elsewhere.\n\n## Tasks\n\n### TASK-001 First\nstatus: todo\n"
		port.docs["active:WS1"] = snapshot
	}

	_, err := NewService(port).Status("/root", "WS1", "TASK-001", StatusDone)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Status() error = %v, want conflict", err)
	}
	got := port.docs["active:WS1"].Content
	if strings.Contains(got, "status: done") {
		t.Fatalf("concurrent change was overwritten: %q", got)
	}
	if !strings.Contains(got, "Edited elsewhere.") {
		t.Fatalf("concurrent change was not preserved: %q", got)
	}
}

func TestServiceApplyStatuses_IsAtomicAndPreservesYAMLDependencies(t *testing.T) {
	port := newMemoryPort()
	const original = "schema_version: 1\ntasks:\n" +
		"  - id: TASK-001\n    title: Foundation\n    status: done\n    depends_on: []\n" +
		"  - id: TASK-002\n    title: Integration\n    status: todo\n    depends_on: [TASK-001]\n"
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.yaml",
		Exists:  true,
		Source:  SourceYAML,
		Content: original,
	})
	service := NewService(port)
	snapshot, err := service.Snapshot("/root", "WS1", "active")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	updated, err := service.ApplyStatuses("/root", "WS1", "active", snapshot.Revision, []BatchStatusChange{
		{TaskID: "TASK-001", From: StatusDone, To: StatusTodo},
		{TaskID: "TASK-002", From: StatusTodo, To: StatusDoing},
	})
	if err != nil {
		t.Fatalf("ApplyStatuses() error = %v", err)
	}
	if updated.Revision == snapshot.Revision || len(updated.Items) != 2 || updated.Items[0].Status != StatusTodo || updated.Items[1].Status != StatusDoing {
		t.Fatalf("updated = %#v", updated)
	}
	workspace, err := ParseYAMLWorkspace([]byte(port.docs["active:WS1"].Content))
	if err != nil {
		t.Fatalf("ParseYAMLWorkspace() error = %v", err)
	}
	if got := workspace.Tasks[1].DependsOn; len(got) != 1 || got[0] != "TASK-001" {
		t.Fatalf("TASK-002 depends_on = %#v", got)
	}
}

func TestServiceApplyStatuses_RejectsStaleOrMismatchedBatchWithoutWrite(t *testing.T) {
	port := newMemoryPort()
	const original = "## Tasks\n\n### TASK-001 One\nstatus: todo\n\n### TASK-002 Two\nstatus: doing\n"
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.md",
		Exists:  true,
		Source:  SourceMD,
		Content: original,
	})
	service := NewService(port)
	snapshot, err := service.Snapshot("/root", "WS1", "active")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	_, err = service.ApplyStatuses("/root", "WS1", "active", snapshot.Revision, []BatchStatusChange{
		{TaskID: "TASK-001", From: StatusTodo, To: StatusDone},
		{TaskID: "TASK-002", From: StatusTodo, To: StatusDone},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("mismatched ApplyStatuses() error = %v", err)
	}
	if got := port.docs["active:WS1"].Content; got != original {
		t.Fatalf("document changed after rejected batch: %q", got)
	}

	port.docs["active:WS1"] = DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.md",
		Exists:  true,
		Source:  SourceMD,
		Content: strings.Replace(original, "status: todo", "status: blocked", 1),
	}
	_, err = service.ApplyStatuses("/root", "WS1", "active", snapshot.Revision, []BatchStatusChange{
		{TaskID: "TASK-001", From: StatusTodo, To: StatusDone},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale ApplyStatuses() error = %v", err)
	}
}

func TestServiceList_FailsOnDuplicateTaskIDs(t *testing.T) {
	port := newMemoryPort()
	port.set("active", "WS1", DocumentSnapshot{
		Path:    "/root/workspaces/WS1/workspace.md",
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
		Path:    "/root/workspaces/WS1/workspace.md",
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
		Path:    "/root/workspaces/WS1/workspace.md",
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
		Path:    "/root/workspaces/WS1/workspace.md",
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
