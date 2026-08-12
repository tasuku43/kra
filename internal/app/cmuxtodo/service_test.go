package cmuxtodo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
)

func TestServiceInitialProjectionPreservesForeignAndSavesUUIDBase(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(workspaceFile("WS1", "cmux-1"))
	documents := newMemoryDocuments([]Task{
		{ID: "TASK-001", Title: "first", Status: TaskDone},
		{ID: "TASK-002", Title: "second", Status: TaskDoing},
	})
	client := newMemoryClient("cmux-1", []TodoItem{{
		ID: "foreign-1", Text: "TASK-001 first", State: TodoPending, Origin: "user",
	}})
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}
	if result.ForeignCount != 1 || result.ManagedCount != 2 || !result.Changed {
		t.Fatalf("result = %#v", result)
	}
	if store.saveCalls != 1 || !store.released {
		t.Fatalf("save/released = %d/%v", store.saveCalls, store.released)
	}
	if len(client.items) != 3 || client.items[0].ID != "foreign-1" || client.items[0].Origin != "user" {
		t.Fatalf("live Todo = %#v", client.items)
	}
	// Managed candidates are added doing, blocked, todo, done while KRA file
	// order is retained within a state.
	if client.items[1].Text != "TASK-002 second" || client.items[2].Text != "TASK-001 first" {
		t.Fatalf("managed creation order = %#v", client.items)
	}
	base := store.file.Todos["WS1"]
	if base.CMUXWorkspaceID != "cmux-1" || base.SyncedAt != "2026-08-13T00:00:00Z" || len(base.Items) != 2 {
		t.Fatalf("saved base = %#v", base)
	}
	for _, item := range base.Items {
		if item.CMUXItemID == "" || item.CMUXItemID == "foreign-1" {
			t.Fatalf("managed base adopted foreign item: %#v", item)
		}
	}
}

func TestServiceInvalidDocumentFailsBeforeLiveTodoRead(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(workspaceFile("WS1", "cmux-1"))
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", nil)
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("error = %v, want ErrInvalidDocument", err)
	}
	if client.listCalls != 0 || len(client.mutations) != 0 || store.saveCalls != 0 {
		t.Fatalf("side effects: list=%d mutations=%#v save=%d", client.listCalls, client.mutations, store.saveCalls)
	}
}

func TestServiceCapacityFailsWithoutPartialProjection(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(workspaceFile("WS1", "cmux-1"))
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "new", Status: TaskDoing}})
	items := make([]TodoItem, MaxTodoItems)
	for i := range items {
		items[i] = TodoItem{ID: fmt.Sprintf("foreign-%02d", i), Text: "foreign", State: TodoPending, Origin: "user"}
	}
	client := newMemoryClient("cmux-1", items)
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrCapacity) || !hasConflict(result.Conflicts, ConflictCapacity) {
		t.Fatalf("result/error = %#v / %v", result, err)
	}
	if len(client.mutations) != 0 || store.saveCalls != 0 || documents.applyCalls != 0 {
		t.Fatalf("partial side effects: client=%#v save=%d apply=%d", client.mutations, store.saveCalls, documents.applyCalls)
	}
}

func TestServiceTodoListWorkspaceIdentityFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		workspaceID   string
		workspaceAt   map[int]string
		wantErrorPart string
	}{
		{name: "initial response omits identity", workspaceID: "", wantErrorPart: "omitted workspace id"},
		{name: "initial response has wrong identity", workspaceID: "cmux-other", wantErrorPart: "does not match"},
		{name: "pre-mutation response changes identity", workspaceID: "cmux-1", workspaceAt: map[int]string{2: "cmux-other"}, wantErrorPart: "does not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryStore(workspaceFile("WS1", "cmux-1"))
			documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}})
			client := newMemoryClient(tt.workspaceID, nil)
			for call, workspaceID := range tt.workspaceAt {
				client.listWorkspaceAt[call] = workspaceID
			}
			service := testService(store, client, documents)

			_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
			if err == nil || !strings.Contains(err.Error(), tt.wantErrorPart) {
				t.Fatalf("error = %v, want %q", err, tt.wantErrorPart)
			}
			if len(client.mutations) != 0 || store.saveCalls != 0 || documents.applyCalls != 0 {
				t.Fatalf("identity mismatch side effects: mutations/save/apply = %#v/%d/%d", client.mutations, store.saveCalls, documents.applyCalls)
			}
		})
	}
}

func TestServiceResolvesWorkspaceAndRootAnchorTargets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		request   Request
		file      cmuxmap.File
		wantCMUX  string
		wantScope string
	}{
		{
			name:      "workspace uses first active mapping",
			request:   Request{Root: "/kra", WorkspaceID: "WS1", DryRun: true},
			file:      workspaceFile("WS1", "cmux-workspace"),
			wantCMUX:  "cmux-workspace",
			wantScope: "active",
		},
		{
			name:    "root uses group anchor",
			request: Request{Root: "/kra", Target: TargetRoot, DryRun: true},
			file: func() cmuxmap.File {
				file := workspaceFile(RootWorkspaceID, "legacy-root-entry")
				file.RootGroup = &cmuxmap.GroupMapping{CMUXGroupID: "group-1", AnchorWorkspaceID: "root-anchor"}
				return file
			}(),
			wantCMUX:  "root-anchor",
			wantScope: "root",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryStore(tt.file)
			documents := newMemoryDocuments(nil)
			client := newMemoryClient(tt.wantCMUX, nil)
			service := testService(store, client, documents)

			result, err := service.Reconcile(context.Background(), tt.request)
			if err != nil {
				t.Fatalf("Reconcile() error: %v", err)
			}
			if result.CMUXWorkspaceID != tt.wantCMUX || client.lastWorkspace != tt.wantCMUX {
				t.Fatalf("cmux target = %q/%q, want %q", result.CMUXWorkspaceID, client.lastWorkspace, tt.wantCMUX)
			}
			if documents.lastScope != tt.wantScope {
				t.Fatalf("document scope = %q, want %q", documents.lastScope, tt.wantScope)
			}
			if store.saveCalls != 0 {
				t.Fatalf("dry-run save calls = %d", store.saveCalls)
			}
		})
	}
}

func TestServiceRootTargetPersistsBaseAgainstGroupAnchor(t *testing.T) {
	t.Parallel()
	file := workspaceFile(RootWorkspaceID, "legacy-root-entry")
	file.RootGroup = &cmuxmap.GroupMapping{
		CMUXWindowID:      "window-1",
		CMUXGroupID:       "group-1",
		AnchorWorkspaceID: "root-anchor",
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "root inbox", Status: TaskBlocked}})
	client := newMemoryClient("root-anchor", nil)
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", Target: TargetRoot})
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}
	if result.WorkspaceID != RootWorkspaceID || result.CMUXWorkspaceID != "root-anchor" || documents.lastScope != "root" {
		t.Fatalf("root result/scope = %#v / %q", result, documents.lastScope)
	}
	base, ok := store.file.Todos[RootWorkspaceID]
	if !ok || base.CMUXWorkspaceID != "root-anchor" || len(base.Items) != 1 {
		t.Fatalf("root base = %#v, exists=%v", base, ok)
	}
	if base.Items[0].Text != "⛔ TASK-001 root inbox" || base.Items[0].CMUXItemID != client.items[0].ID {
		t.Fatalf("root base item/live = %#v / %#v", base.Items[0], client.items)
	}
}

func TestServiceRemoteOnlyStatePullIsAtomicAndAdvancesBase(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 work")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 work", State: TodoCompleted, Origin: "agent"}})
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}
	if documents.applyCalls != 1 || documents.snapshot.Tasks[0].Status != TaskDone {
		t.Fatalf("document = %#v, apply calls=%d", documents.snapshot, documents.applyCalls)
	}
	if len(client.mutations) != 0 {
		t.Fatalf("remote mutations = %#v, want none", client.mutations)
	}
	base := store.file.Todos["WS1"].Items[0]
	if base.KRAStatus != string(TaskDone) || base.CMUXState != string(TodoCompleted) {
		t.Fatalf("base = %#v", base)
	}
	if len(result.Operations) != 1 || result.Operations[0].Kind != OperationPullStatus {
		t.Fatalf("operations = %#v", result.Operations)
	}
}

func TestServiceBaseSaveFailureRollsBackLocalAndPreservesRemoteWithoutCAS(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 original")},
	}
	store := newMemoryStore(file)
	store.saveErr = errors.New("disk full")
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "local", Status: TaskDoing}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 original", State: TodoCompleted, Origin: "agent"}})
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{
		Root: "/kra", WorkspaceID: "WS1", Prefer: PreferCMUX,
	})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "save cmux Todo base") || !strings.Contains(err.Error(), "no compare-and-set") {
		t.Fatalf("error = %v", err)
	}
	if documents.applyCalls != 2 || documents.snapshot.Tasks[0].Status != TaskDoing {
		t.Fatalf("local rollback = %#v, apply calls=%d", documents.snapshot, documents.applyCalls)
	}
	if len(client.items) != 1 || client.items[0].Text != "TASK-001 local" || client.items[0].State != TodoCompleted || client.editCalls != 1 {
		t.Fatalf("remote preservation/calls = %#v/%d", client.items, client.editCalls)
	}
	if got := store.file.Todos["WS1"].Items[0]; got.KRAStatus != string(TaskTodo) || got.Text != "TASK-001 original" {
		t.Fatalf("base changed despite save failure: %#v", got)
	}
	if result.Changed || len(result.Operations) != 0 {
		t.Fatalf("failed compensated transaction reported committed operations: %#v", result)
	}
}

func TestServiceLocalApplyFailurePreservesRemoteMarkerEditWithoutCAS(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskBlocked, "⛔ TASK-001 work")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskBlocked}})
	documents.applyErrAt[1] = errors.New("CAS conflict")
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "⛔ TASK-001 work", State: TodoCompleted, Origin: "agent"}})
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "apply KRA task statuses") || !strings.Contains(err.Error(), "no compare-and-set") {
		t.Fatalf("error = %v", err)
	}
	if client.items[0].Text != "TASK-001 work" || client.items[0].State != TodoCompleted || client.editCalls != 1 {
		t.Fatalf("remote marker preservation/calls = %#v/%d", client.items[0], client.editCalls)
	}
	if store.saveCalls != 0 || documents.snapshot.Tasks[0].Status != TaskBlocked {
		t.Fatalf("save/local = %d/%#v", store.saveCalls, documents.snapshot)
	}
}

func TestServiceLaterRemoteMutationFailurePreservesEarlierMutationWithoutCAS(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 original")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "renamed", Status: TaskDoing}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 original", State: TodoPending, Origin: "agent"}})
	client.stateErrAt[1] = errors.New("cmux unavailable")
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "set cmux Todo state") || !strings.Contains(err.Error(), "no compare-and-set") {
		t.Fatalf("error = %v", err)
	}
	if client.items[0].Text != "TASK-001 renamed" || client.items[0].State != TodoPending || client.editCalls != 1 || client.stateCalls != 1 {
		t.Fatalf("earlier edit preservation/calls = %#v/%d/%d", client.items[0], client.editCalls, client.stateCalls)
	}
	if store.saveCalls != 0 || documents.applyCalls != 0 {
		t.Fatalf("save/apply = %d/%d", store.saveCalls, documents.applyCalls)
	}
}

func TestServiceConcurrentTodoChangeBeforeMutationFailsWithoutOverwrite(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 old")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "new", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 old", State: TodoPending, Origin: "agent"}})
	client.listHook = func(call int, client *memoryClient) {
		if call == 2 {
			client.items = append(client.items, TodoItem{ID: "foreign-new", Text: "user note", State: TodoPending, Origin: "user"})
		}
	}
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrConcurrentTodo) || !hasConflict(result.Conflicts, ConflictConcurrentTodo) {
		t.Fatalf("result/error = %#v / %v", result, err)
	}
	if client.items[0].Text != "TASK-001 old" || len(client.mutations) != 0 || store.saveCalls != 0 {
		t.Fatalf("concurrent Todo was overwritten: items=%#v mutations=%#v save=%d", client.items, client.mutations, store.saveCalls)
	}
}

func TestServiceConcurrentForeignChangeAfterMutationPreservesAllRemoteFields(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 old")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "new", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 old", State: TodoPending, Origin: "agent"}})
	client.listHook = func(call int, client *memoryClient) {
		if call == 3 {
			client.items = append(client.items, TodoItem{ID: "foreign-new", Text: "user note", State: TodoPending, Origin: "user"})
		}
	}
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !errors.Is(err, ErrConcurrentTodo) || !strings.Contains(err.Error(), "no compare-and-set") {
		t.Fatalf("error = %v, want recovery wrapping ErrConcurrentTodo", err)
	}
	if client.items[0].Text != "TASK-001 new" || client.editCalls != 1 {
		t.Fatalf("owned text preservation/calls = %#v/%d", client.items, client.editCalls)
	}
	if len(client.items) != 2 || client.items[1].ID != "foreign-new" || client.items[1].Text != "user note" {
		t.Fatalf("foreign concurrent item was not preserved: %#v", client.items)
	}
	if store.saveCalls != 0 {
		t.Fatalf("base saved after failed verification")
	}
}

func TestServiceRemoteRecoveryWithoutCASReportsManualRepair(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 old")},
	}
	store := newMemoryStore(file)
	store.saveErr = errors.New("disk full")
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "new", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 old", State: TodoPending, Origin: "agent"}})
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "manual repair required") {
		t.Fatalf("error = %v, want ErrRecovery with repair guidance", err)
	}
	if client.items[0].Text != "TASK-001 new" {
		t.Fatalf("test did not leave the expected repair state: %#v", client.items[0])
	}
	if client.editCalls != 1 {
		t.Fatalf("remote recovery issued a second edit: %d calls", client.editCalls)
	}
}

func TestServiceBaseSaveFailurePreservesNewItemWithUnobservableAttachments(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(workspaceFile("WS1", "cmux-1"))
	store.saveErr = errors.New("disk full")
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "foreign-1", Text: "keep me", State: TodoPending, Origin: "user"}})
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "new-uuid-1") {
		t.Fatalf("error = %v", err)
	}
	if len(client.items) != 2 || client.items[0].ID != "foreign-1" || client.items[1].ID != "new-uuid-1" {
		t.Fatalf("live Todo should preserve foreign and uncertain added item: %#v", client.items)
	}
	if client.removeCalls != 0 || documents.applyCalls != 0 {
		t.Fatalf("remove/apply calls = %d/%d", client.removeCalls, documents.applyCalls)
	}
}

func TestServiceSuccessfulAddWithUntrustworthyResponseIDReportsRecovery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		initial       []TodoItem
		response      TodoItem
		wantItemCount int
	}{
		{
			name:          "empty id",
			response:      TodoItem{Text: "TASK-001 work", State: TodoPending, Origin: "agent"},
			wantItemCount: 1,
		},
		{
			name:          "duplicate id",
			initial:       []TodoItem{{ID: "foreign-1", Text: "keep", State: TodoPending, Origin: "user"}},
			response:      TodoItem{ID: "foreign-1", Text: "TASK-001 work", State: TodoPending, Origin: "agent"},
			wantItemCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryStore(workspaceFile("WS1", "cmux-1"))
			documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}})
			client := newMemoryClient("cmux-1", tt.initial)
			client.addResultAt[1] = tt.response
			service := testService(store, client, documents)

			_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
			if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "no trustworthy") {
				t.Fatalf("error = %v", err)
			}
			if len(client.items) != tt.wantItemCount || client.addCalls != 1 || client.removeCalls != 0 {
				t.Fatalf("items/add/remove = %#v/%d/%d", client.items, client.addCalls, client.removeCalls)
			}
			if store.saveCalls != 0 || documents.applyCalls != 0 {
				t.Fatalf("save/apply = %d/%d", store.saveCalls, documents.applyCalls)
			}
		})
	}
}

func TestServiceSuccessfulMutationWithUnexpectedPayloadReportsRecovery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		file      cmuxmap.File
		tasks     []Task
		items     []TodoItem
		configure func(*memoryClient)
		assert    func(*testing.T, *memoryClient)
	}{
		{
			name:  "add",
			file:  workspaceFile("WS1", "cmux-1"),
			tasks: []Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}},
			configure: func(client *memoryClient) {
				client.addResultAt[1] = TodoItem{ID: "reported-other", Text: "wrong", State: TodoCompleted, Origin: "user"}
			},
			assert: func(t *testing.T, client *memoryClient) {
				if len(client.items) != 1 || client.items[0].ID != "new-uuid-1" || client.removeCalls != 0 {
					t.Fatalf("unexpected add effect was not preserved: %#v/remove=%d", client.items, client.removeCalls)
				}
			},
		},
		{
			name: "edit",
			file: func() cmuxmap.File {
				file := workspaceFile("WS1", "cmux-1")
				file.Todos["WS1"] = cmuxmap.TodoMapping{
					CMUXWorkspaceID: "cmux-1",
					Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 old")},
				}
				return file
			}(),
			tasks: []Task{{ID: "TASK-001", Title: "new", Status: TaskTodo}},
			items: []TodoItem{{ID: "item-1", Text: "TASK-001 old", State: TodoPending, Origin: "agent"}},
			configure: func(client *memoryClient) {
				client.editResultAt[1] = TodoItem{ID: "item-1", Text: "wrong response", State: TodoPending, Origin: "agent"}
			},
			assert: func(t *testing.T, client *memoryClient) {
				if client.items[0].Text != "TASK-001 new" || client.editCalls != 1 {
					t.Fatalf("unexpected edit effect/calls = %#v/%d", client.items[0], client.editCalls)
				}
			},
		},
		{
			name: "state",
			file: func() cmuxmap.File {
				file := workspaceFile("WS1", "cmux-1")
				file.Todos["WS1"] = cmuxmap.TodoMapping{
					CMUXWorkspaceID: "cmux-1",
					Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 work")},
				}
				return file
			}(),
			tasks: []Task{{ID: "TASK-001", Title: "work", Status: TaskDoing}},
			items: []TodoItem{{ID: "item-1", Text: "TASK-001 work", State: TodoPending, Origin: "agent"}},
			configure: func(client *memoryClient) {
				client.stateResultAt[1] = TodoItem{ID: "item-1", Text: "TASK-001 work", State: TodoCompleted, Origin: "agent"}
			},
			assert: func(t *testing.T, client *memoryClient) {
				if client.items[0].State != TodoInProgress || client.stateCalls != 1 {
					t.Fatalf("unexpected state effect/calls = %#v/%d", client.items[0], client.stateCalls)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryStore(tt.file)
			documents := newMemoryDocuments(tt.tasks)
			client := newMemoryClient("cmux-1", tt.items)
			tt.configure(client)
			service := testService(store, client, documents)

			_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
			if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "untrustworthy payload") {
				t.Fatalf("error = %v", err)
			}
			if strings.Count(err.Error(), ErrRecovery.Error()) != 1 {
				t.Fatalf("nested recovery error was not flattened: %v", err)
			}
			tt.assert(t, client)
			if store.saveCalls != 0 || documents.applyCalls != 0 {
				t.Fatalf("save/apply = %d/%d", store.saveCalls, documents.applyCalls)
			}
		})
	}
}

func TestServiceBaseSaveFailurePreservesPushedStateWithoutCAS(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 work")},
	}
	store := newMemoryStore(file)
	store.saveErr = errors.New("disk full")
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskDoing}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 work", State: TodoPending, Origin: "agent"}})
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "no compare-and-set") {
		t.Fatalf("error = %v", err)
	}
	if client.items[0].State != TodoInProgress || client.stateCalls != 1 {
		t.Fatalf("state/calls = %q/%d, want in-progress/1", client.items[0].State, client.stateCalls)
	}
}

func TestServiceLocalRollbackCASFailureReportsRecovery(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 work")},
	}
	store := newMemoryStore(file)
	store.saveErr = errors.New("disk full")
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}})
	documents.applyErrAt[2] = errors.New("rollback CAS conflict")
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 work", State: TodoCompleted, Origin: "agent"}})
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "restore KRA task statuses") {
		t.Fatalf("error = %v", err)
	}
	if !errors.Is(err, store.saveErr) {
		t.Fatalf("RecoveryError lost primary cause: %v", err)
	}
	if documents.snapshot.Tasks[0].Status != TaskDone {
		t.Fatalf("test did not retain repair state: %#v", documents.snapshot)
	}
}

func TestServiceRemoteRecoveryDoesNotOverwriteConcurrentOwnedEdit(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 old")},
	}
	store := newMemoryStore(file)
	store.saveErr = errors.New("disk full")
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "new", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 old", State: TodoPending, Origin: "agent"}})
	client.listHook = func(call int, client *memoryClient) {
		if call == 4 { // read-only recovery audit
			client.items[0].Text = "user concurrent edit"
		}
	}
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "no compare-and-set") {
		t.Fatalf("error = %v", err)
	}
	if client.items[0].Text != "user concurrent edit" {
		t.Fatalf("concurrent edit was overwritten: %#v", client.items[0])
	}
	if client.editCalls != 1 {
		t.Fatalf("remote recovery issued a racy edit: %d calls", client.editCalls)
	}
}

func TestServicePreflightConflictHasNoWrites(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 work")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskDoing}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 work", State: TodoCompleted, Origin: "agent"}})
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrConflict) || !hasConflict(result.Conflicts, ConflictState) {
		t.Fatalf("result/error = %#v / %v", result, err)
	}
	if len(result.Operations) != 0 || result.Changed {
		t.Fatalf("conflict reported committed operations: %#v", result)
	}
	if len(client.mutations) != 0 || documents.applyCalls != 0 || store.saveCalls != 0 {
		t.Fatalf("preflight conflict wrote state: %#v / %d / %d", client.mutations, documents.applyCalls, store.saveCalls)
	}
}

func TestServiceRPCErrorAfterMutationPreservesAmbiguousEffectAndReportsRecovery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		file      cmuxmap.File
		tasks     []Task
		items     []TodoItem
		configure func(*memoryClient)
		assert    func(*testing.T, *memoryClient)
	}{
		{
			name:  "add",
			file:  workspaceFile("WS1", "cmux-1"),
			tasks: []Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}},
			configure: func(client *memoryClient) {
				client.addAppliedErrAt[1] = errors.New("lost add response")
			},
			assert: func(t *testing.T, client *memoryClient) {
				if len(client.items) != 1 || client.removeCalls != 0 {
					t.Fatalf("ambiguous add was undone: items/remove = %#v/%d", client.items, client.removeCalls)
				}
			},
		},
		{
			name: "edit",
			file: func() cmuxmap.File {
				file := workspaceFile("WS1", "cmux-1")
				file.Todos["WS1"] = cmuxmap.TodoMapping{
					CMUXWorkspaceID: "cmux-1",
					Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 old")},
				}
				return file
			}(),
			tasks: []Task{{ID: "TASK-001", Title: "new", Status: TaskTodo}},
			items: []TodoItem{{ID: "item-1", Text: "TASK-001 old", State: TodoPending, Origin: "agent"}},
			configure: func(client *memoryClient) {
				client.editAppliedErrAt[1] = errors.New("lost edit response")
			},
			assert: func(t *testing.T, client *memoryClient) {
				if client.items[0].Text != "TASK-001 new" || client.editCalls != 1 {
					t.Fatalf("ambiguous edit was undone: item/calls = %#v/%d", client.items[0], client.editCalls)
				}
			},
		},
		{
			name: "state",
			file: func() cmuxmap.File {
				file := workspaceFile("WS1", "cmux-1")
				file.Todos["WS1"] = cmuxmap.TodoMapping{
					CMUXWorkspaceID: "cmux-1",
					Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 work")},
				}
				return file
			}(),
			tasks: []Task{{ID: "TASK-001", Title: "work", Status: TaskDoing}},
			items: []TodoItem{{ID: "item-1", Text: "TASK-001 work", State: TodoPending, Origin: "agent"}},
			configure: func(client *memoryClient) {
				client.stateAppliedErrAt[1] = errors.New("lost state response")
			},
			assert: func(t *testing.T, client *memoryClient) {
				if client.items[0].State != TodoInProgress || client.stateCalls != 1 {
					t.Fatalf("ambiguous state was undone: item/calls = %#v/%d", client.items[0], client.stateCalls)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryStore(tt.file)
			documents := newMemoryDocuments(tt.tasks)
			client := newMemoryClient("cmux-1", tt.items)
			tt.configure(client)
			service := testService(store, client, documents)

			_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
			if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "causally ambiguous") {
				t.Fatalf("error = %v, want recovery failure without undo", err)
			}
			tt.assert(t, client)
			if store.saveCalls != 0 {
				t.Fatalf("base save calls = %d", store.saveCalls)
			}
		})
	}
}

func TestServiceUnrecognizedMutationEffectReportsRecovery(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 work")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskDoing}})
	client := newMemoryClient("cmux-1", []TodoItem{{ID: "item-1", Text: "TASK-001 work", State: TodoPending, Origin: "agent"}})
	client.stateErrAt[1] = errors.New("state RPC failed")
	client.listHook = func(call int, client *memoryClient) {
		if call == 3 { // post-error effect inspection
			client.items[0].Text = "concurrent text"
		}
	}
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if !errors.Is(err, ErrRecovery) || !strings.Contains(err.Error(), "causally ambiguous") {
		t.Fatalf("error = %v", err)
	}
	if client.items[0].Text != "concurrent text" {
		t.Fatalf("unrecognized state was overwritten: %#v", client.items[0])
	}
}

func TestServiceRemoteDeletionPreferKRARecreatesAndRekeysBase(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-1")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-1",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "deleted-id", TaskTodo, "TASK-001 work")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", nil)
	service := testService(store, client, documents)

	_, err := service.Reconcile(context.Background(), Request{
		Root: "/kra", WorkspaceID: "WS1", Prefer: PreferKRA,
	})
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}
	base := store.file.Todos["WS1"].Items[0]
	if base.CMUXItemID == "" || base.CMUXItemID == "deleted-id" || base.CMUXItemID != client.items[0].ID {
		t.Fatalf("recreated base = %#v, live=%#v", base, client.items)
	}
}

func TestServiceDryRunReportsPlanWithoutMutation(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(workspaceFile("WS1", "cmux-1"))
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "new", Status: TaskTodo}})
	client := newMemoryClient("cmux-1", nil)
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1", DryRun: true})
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}
	if !result.DryRun || len(result.Operations) != 1 || result.Operations[0].Kind != OperationAdd {
		t.Fatalf("result = %#v", result)
	}
	if len(client.items) != 0 || len(client.mutations) != 0 || store.saveCalls != 0 || documents.applyCalls != 0 {
		t.Fatalf("dry-run side effects: items=%#v mutations=%#v save=%d apply=%d", client.items, client.mutations, store.saveCalls, documents.applyCalls)
	}
}

func TestServiceChangedRuntimeTreatsOldLedgerItemAsForeign(t *testing.T) {
	t.Parallel()
	file := workspaceFile("WS1", "cmux-new")
	file.Todos["WS1"] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: "cmux-old",
		Items:           []cmuxmap.TodoItemBase{todoBase("TASK-001", "same-looking-id", TaskTodo, "TASK-001 work")},
	}
	store := newMemoryStore(file)
	documents := newMemoryDocuments([]Task{{ID: "TASK-001", Title: "work", Status: TaskTodo}})
	client := newMemoryClient("cmux-new", []TodoItem{{ID: "same-looking-id", Text: "TASK-001 work", State: TodoPending, Origin: "user"}})
	service := testService(store, client, documents)

	result, err := service.Reconcile(context.Background(), Request{Root: "/kra", WorkspaceID: "WS1"})
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}
	if result.ForeignCount != 1 || len(client.items) != 2 {
		t.Fatalf("result/live = %#v / %#v", result, client.items)
	}
	if got := store.file.Todos["WS1"].Items[0].CMUXItemID; got == "same-looking-id" {
		t.Fatalf("old ledger item was adopted: %q", got)
	}
}

type memoryStore struct {
	file      cmuxmap.File
	loadErr   error
	saveErr   error
	lockErr   error
	loadCalls int
	saveCalls int
	locked    bool
	released  bool
	lastSaved cmuxmap.File
}

func newMemoryStore(file cmuxmap.File) *memoryStore {
	return &memoryStore{file: cloneMappingFile(file)}
}

func (s *memoryStore) AcquireLock(context.Context) (func(), error) {
	if s.lockErr != nil {
		return nil, s.lockErr
	}
	s.locked = true
	return func() {
		s.locked = false
		s.released = true
	}, nil
}

func (s *memoryStore) Load() (cmuxmap.File, error) {
	s.loadCalls++
	if s.loadErr != nil {
		return cmuxmap.File{}, s.loadErr
	}
	return cloneMappingFile(s.file), nil
}

func (s *memoryStore) Save(file cmuxmap.File) error {
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.file = cloneMappingFile(file)
	s.lastSaved = cloneMappingFile(file)
	return nil
}

type memoryDocuments struct {
	snapshot    TaskSnapshot
	listCalls   int
	applyCalls  int
	applyErrAt  map[int]error
	listErrAt   map[int]error
	lastScope   string
	lastChanges []StatusChange
}

func newMemoryDocuments(tasks []Task) *memoryDocuments {
	return &memoryDocuments{
		snapshot:   TaskSnapshot{Tasks: append([]Task(nil), tasks...), Revision: "revision-1"},
		applyErrAt: make(map[int]error),
		listErrAt:  make(map[int]error),
	}
}

func (d *memoryDocuments) List(_ context.Context, _ string, _ string, scope string) (TaskSnapshot, error) {
	d.listCalls++
	d.lastScope = scope
	if err := d.listErrAt[d.listCalls]; err != nil {
		return TaskSnapshot{}, err
	}
	return cloneTaskSnapshot(d.snapshot), nil
}

func (d *memoryDocuments) ApplyStatuses(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	expectedRevision string,
	changes []StatusChange,
) (TaskSnapshot, error) {
	d.applyCalls++
	if err := d.applyErrAt[d.applyCalls]; err != nil {
		return TaskSnapshot{}, err
	}
	if expectedRevision != d.snapshot.Revision {
		return TaskSnapshot{}, fmt.Errorf("stale revision %q, current %q", expectedRevision, d.snapshot.Revision)
	}
	byID := make(map[string]int, len(d.snapshot.Tasks))
	for i, task := range d.snapshot.Tasks {
		byID[task.ID] = i
	}
	for _, change := range changes {
		index, ok := byID[change.TaskID]
		if !ok {
			return TaskSnapshot{}, fmt.Errorf("task %s not found", change.TaskID)
		}
		if d.snapshot.Tasks[index].Status != change.From {
			return TaskSnapshot{}, fmt.Errorf("task %s status %s, want %s", change.TaskID, d.snapshot.Tasks[index].Status, change.From)
		}
	}
	for _, change := range changes {
		d.snapshot.Tasks[byID[change.TaskID]].Status = change.To
	}
	d.snapshot.Revision = fmt.Sprintf("revision-%d", d.applyCalls+1)
	d.lastChanges = append([]StatusChange(nil), changes...)
	return cloneTaskSnapshot(d.snapshot), nil
}

type memoryClient struct {
	workspace         string
	items             []TodoItem
	nextID            int
	listCalls         int
	addCalls          int
	editCalls         int
	stateCalls        int
	removeCalls       int
	lastWorkspace     string
	mutations         []string
	listHook          func(int, *memoryClient)
	listErrAt         map[int]error
	listWorkspaceAt   map[int]string
	addErrAt          map[int]error
	addAppliedErrAt   map[int]error
	addResultAt       map[int]TodoItem
	editErrAt         map[int]error
	editAppliedErrAt  map[int]error
	editResultAt      map[int]TodoItem
	stateErrAt        map[int]error
	stateAppliedErrAt map[int]error
	stateResultAt     map[int]TodoItem
	removeErrAt       map[int]error
}

func newMemoryClient(workspace string, items []TodoItem) *memoryClient {
	return &memoryClient{
		workspace:         workspace,
		items:             append([]TodoItem(nil), items...),
		nextID:            1,
		listErrAt:         make(map[int]error),
		listWorkspaceAt:   make(map[int]string),
		addErrAt:          make(map[int]error),
		addAppliedErrAt:   make(map[int]error),
		addResultAt:       make(map[int]TodoItem),
		editErrAt:         make(map[int]error),
		editAppliedErrAt:  make(map[int]error),
		editResultAt:      make(map[int]TodoItem),
		stateErrAt:        make(map[int]error),
		stateAppliedErrAt: make(map[int]error),
		stateResultAt:     make(map[int]TodoItem),
		removeErrAt:       make(map[int]error),
	}
}

func (c *memoryClient) ListTodos(_ context.Context, workspaceID string) (TodoList, error) {
	c.listCalls++
	c.lastWorkspace = workspaceID
	if c.listHook != nil {
		c.listHook(c.listCalls, c)
	}
	if err := c.listErrAt[c.listCalls]; err != nil {
		return TodoList{}, err
	}
	responseWorkspaceID := c.workspace
	if override, ok := c.listWorkspaceAt[c.listCalls]; ok {
		responseWorkspaceID = override
	}
	return TodoList{WorkspaceID: responseWorkspaceID, Items: append([]TodoItem(nil), c.items...)}, nil
}

func (c *memoryClient) AddTodo(_ context.Context, workspaceID string, input TodoInput) (TodoItem, error) {
	c.addCalls++
	c.lastWorkspace = workspaceID
	if err := c.addErrAt[c.addCalls]; err != nil {
		return TodoItem{}, err
	}
	item := TodoItem{
		ID:     fmt.Sprintf("new-uuid-%d", c.nextID),
		Text:   input.Text,
		State:  input.State,
		Origin: input.Origin,
	}
	c.nextID++
	c.items = append(c.items, item)
	c.mutations = append(c.mutations, "add:"+item.ID)
	if err := c.addAppliedErrAt[c.addCalls]; err != nil {
		return TodoItem{}, err
	}
	if result, ok := c.addResultAt[c.addCalls]; ok {
		return result, nil
	}
	return item, nil
}

func (c *memoryClient) SetTodoState(_ context.Context, workspaceID string, itemID string, state TodoState) (TodoItem, error) {
	c.stateCalls++
	c.lastWorkspace = workspaceID
	if err := c.stateErrAt[c.stateCalls]; err != nil {
		return TodoItem{}, err
	}
	index := findTodo(c.items, itemID)
	if index < 0 {
		return TodoItem{}, fmt.Errorf("item %s not found", itemID)
	}
	c.items[index].State = state
	c.mutations = append(c.mutations, "state:"+itemID+":"+string(state))
	if err := c.stateAppliedErrAt[c.stateCalls]; err != nil {
		return TodoItem{}, err
	}
	if result, ok := c.stateResultAt[c.stateCalls]; ok {
		return result, nil
	}
	return c.items[index], nil
}

func (c *memoryClient) EditTodo(_ context.Context, workspaceID string, itemID string, text string) (TodoItem, error) {
	c.editCalls++
	c.lastWorkspace = workspaceID
	if err := c.editErrAt[c.editCalls]; err != nil {
		return TodoItem{}, err
	}
	index := findTodo(c.items, itemID)
	if index < 0 {
		return TodoItem{}, fmt.Errorf("item %s not found", itemID)
	}
	c.items[index].Text = text
	c.mutations = append(c.mutations, "edit:"+itemID+":"+text)
	if err := c.editAppliedErrAt[c.editCalls]; err != nil {
		return TodoItem{}, err
	}
	if result, ok := c.editResultAt[c.editCalls]; ok {
		return result, nil
	}
	return c.items[index], nil
}

func (c *memoryClient) RemoveTodo(_ context.Context, workspaceID string, itemID string) error {
	c.removeCalls++
	c.lastWorkspace = workspaceID
	if err := c.removeErrAt[c.removeCalls]; err != nil {
		return err
	}
	index := findTodo(c.items, itemID)
	if index < 0 {
		return nil
	}
	c.items = append(c.items[:index], c.items[index+1:]...)
	c.mutations = append(c.mutations, "remove:"+itemID)
	return nil
}

func testService(store *memoryStore, client *memoryClient, documents *memoryDocuments) *Service {
	service := NewService(
		func(string) MappingStore { return store },
		func() Client { return client },
		documents,
	)
	service.Now = func() time.Time { return time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC) }
	return service
}

func workspaceFile(workspaceID string, cmuxWorkspaceID string) cmuxmap.File {
	return cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			workspaceID: {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{{
					CMUXWorkspaceID: cmuxWorkspaceID,
					Ordinal:         1,
				}},
			},
		},
		Todos: map[string]cmuxmap.TodoMapping{},
	}
}

func cloneMappingFile(in cmuxmap.File) cmuxmap.File {
	out := cmuxmap.File{
		Version:    in.Version,
		Workspaces: make(map[string]cmuxmap.WorkspaceMapping, len(in.Workspaces)),
		Todos:      make(map[string]cmuxmap.TodoMapping, len(in.Todos)),
	}
	if in.RootGroup != nil {
		group := *in.RootGroup
		out.RootGroup = &group
	}
	for key, workspace := range in.Workspaces {
		workspace.Entries = append([]cmuxmap.Entry(nil), workspace.Entries...)
		out.Workspaces[key] = workspace
	}
	for key, todo := range in.Todos {
		todo.Items = append([]cmuxmap.TodoItemBase(nil), todo.Items...)
		out.Todos[key] = todo
	}
	return out
}

func cloneTaskSnapshot(in TaskSnapshot) TaskSnapshot {
	return TaskSnapshot{Tasks: append([]Task(nil), in.Tasks...), Revision: in.Revision}
}

func TestMemoryDocumentFixturePreservesExactBatchOrder(t *testing.T) {
	t.Parallel()
	documents := newMemoryDocuments([]Task{
		{ID: "A", Title: "a", Status: TaskTodo},
		{ID: "B", Title: "b", Status: TaskDoing},
	})
	changes := []StatusChange{
		{TaskID: "B", From: TaskDoing, To: TaskDone},
		{TaskID: "A", From: TaskTodo, To: TaskBlocked},
	}
	_, err := documents.ApplyStatuses(context.Background(), "", "", "", "revision-1", changes)
	if err != nil {
		t.Fatalf("ApplyStatuses() error: %v", err)
	}
	if !reflect.DeepEqual(documents.lastChanges, changes) {
		t.Fatalf("changes = %#v, want %#v", documents.lastChanges, changes)
	}
}
