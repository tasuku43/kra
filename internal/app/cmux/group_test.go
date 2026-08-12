package cmux

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type groupTestClient struct {
	*fakeClient

	mu sync.Mutex

	groupLists      []cmuxctl.WorkspaceGroupList
	groupListErr    error
	groupListErrs   []error
	groupListIndex  int
	groupListCalls  []string
	windows         []cmuxctl.Window
	windowListErr   error
	windowListCalls int

	createGroupResult cmuxctl.WorkspaceGroup
	createGroupErr    error
	createGroupCalls  []cmuxctl.WorkspaceGroupCreateOptions

	createWorkspaceResults []cmuxctl.WorkspaceCreateResult
	createWorkspaceErr     error
	createWorkspaceErrs    []error
	createWorkspaceIndex   int
	createWorkspaceCalls   []cmuxctl.WorkspaceCreateOptions
	createWorkspaceCtxErrs []error
	failCreateOnCanceled   bool

	addCalls       []groupMembershipCall
	removeCalls    []groupMembershipCall
	setAnchorCalls []groupMembershipCall
	ungroupCalls   []string
	ungroupErr     error
	focusCalls     []string
	events         []string
}

type groupMembershipCall struct {
	Group     string
	Workspace string
}

func newGroupTestClient() *groupTestClient {
	return &groupTestClient{fakeClient: &fakeClient{capabilities: workspaceGroupTestCapabilities()}}
}

func workspaceGroupTestCapabilities() cmuxctl.Capabilities {
	methods := map[string]struct{}{
		"workspace.create": {},
		"workspace.rename": {},
		"workspace.select": {},
	}
	for _, method := range requiredWorkspaceGroupMethods {
		methods[method] = struct{}{}
	}
	return cmuxctl.Capabilities{Methods: methods}
}

func (f *groupTestClient) ListWorkspaceGroups(_ context.Context, window string) (cmuxctl.WorkspaceGroupList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groupListCalls = append(f.groupListCalls, window)
	f.events = append(f.events, "list-groups:"+window)
	callIndex := len(f.groupListCalls) - 1
	if callIndex < len(f.groupListErrs) && f.groupListErrs[callIndex] != nil {
		return cmuxctl.WorkspaceGroupList{}, f.groupListErrs[callIndex]
	}
	if f.groupListErr != nil {
		return cmuxctl.WorkspaceGroupList{}, f.groupListErr
	}
	if len(f.groupLists) == 0 {
		return cmuxctl.WorkspaceGroupList{}, nil
	}
	index := f.groupListIndex
	if index >= len(f.groupLists) {
		index = len(f.groupLists) - 1
	} else {
		f.groupListIndex++
	}
	return f.groupLists[index], nil
}

func (f *groupTestClient) ListWindows(context.Context) ([]cmuxctl.Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.windowListCalls++
	f.events = append(f.events, "list-windows")
	if f.windowListErr != nil {
		return nil, f.windowListErr
	}
	return append([]cmuxctl.Window{}, f.windows...), nil
}

func (f *groupTestClient) CreateWorkspaceGroup(_ context.Context, opts cmuxctl.WorkspaceGroupCreateOptions) (cmuxctl.WorkspaceGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	opts.ChildWorkspaceIDs = append([]string{}, opts.ChildWorkspaceIDs...)
	f.createGroupCalls = append(f.createGroupCalls, opts)
	f.events = append(f.events, "create-group")
	return f.createGroupResult, f.createGroupErr
}

func (f *groupTestClient) CreateWorkspaceWithOptions(ctx context.Context, opts cmuxctl.WorkspaceCreateOptions) (cmuxctl.WorkspaceCreateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createWorkspaceCalls = append(f.createWorkspaceCalls, opts)
	f.createWorkspaceCtxErrs = append(f.createWorkspaceCtxErrs, ctx.Err())
	f.events = append(f.events, "create-workspace:"+opts.Title)
	if f.failCreateOnCanceled && ctx.Err() != nil {
		return cmuxctl.WorkspaceCreateResult{}, ctx.Err()
	}
	callIndex := len(f.createWorkspaceCalls) - 1
	if callIndex < len(f.createWorkspaceErrs) && f.createWorkspaceErrs[callIndex] != nil {
		return cmuxctl.WorkspaceCreateResult{}, f.createWorkspaceErrs[callIndex]
	}
	if f.createWorkspaceErr != nil {
		return cmuxctl.WorkspaceCreateResult{}, f.createWorkspaceErr
	}
	if len(f.createWorkspaceResults) == 0 {
		return cmuxctl.WorkspaceCreateResult{}, nil
	}
	index := f.createWorkspaceIndex
	if index >= len(f.createWorkspaceResults) {
		index = len(f.createWorkspaceResults) - 1
	} else {
		f.createWorkspaceIndex++
	}
	return f.createWorkspaceResults[index], nil
}

func (f *groupTestClient) AddWorkspaceToGroup(_ context.Context, group string, workspace string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalls = append(f.addCalls, groupMembershipCall{Group: group, Workspace: workspace})
	f.events = append(f.events, "add:"+workspace)
	return nil
}

func (f *groupTestClient) RemoveWorkspaceFromGroup(_ context.Context, group string, workspace string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCalls = append(f.removeCalls, groupMembershipCall{Group: group, Workspace: workspace})
	f.events = append(f.events, "remove:"+workspace)
	return nil
}

func (f *groupTestClient) SetWorkspaceGroupAnchor(_ context.Context, group string, workspace string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setAnchorCalls = append(f.setAnchorCalls, groupMembershipCall{Group: group, Workspace: workspace})
	f.events = append(f.events, "set-anchor:"+workspace)
	return nil
}

func (f *groupTestClient) UngroupWorkspaceGroup(_ context.Context, group string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ungroupCalls = append(f.ungroupCalls, group)
	f.events = append(f.events, "ungroup:"+group)
	return f.ungroupErr
}

func (f *groupTestClient) FocusWorkspaceGroup(_ context.Context, group string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.focusCalls = append(f.focusCalls, group)
	f.events = append(f.events, "focus:"+group)
	return nil
}

func (f *groupTestClient) CloseWorkspace(ctx context.Context, workspace string) error {
	f.mu.Lock()
	f.events = append(f.events, "close:"+workspace)
	f.mu.Unlock()
	return f.fakeClient.CloseWorkspace(ctx, workspace)
}

type groupTestStore struct {
	mu        sync.Mutex
	file      cmuxmap.File
	saveErr   error
	saveCalls int
}

func (s *groupTestStore) AcquireLock(context.Context) (func(), error) {
	return func() {}, nil
}

func (s *groupTestStore) Load() (cmuxmap.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneGroupTestMapping(s.file), nil
}

func (s *groupTestStore) Save(file cmuxmap.File) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.file = cloneGroupTestMapping(file)
	return nil
}

func (s *groupTestStore) snapshot() cmuxmap.File {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneGroupTestMapping(s.file)
}

func cloneGroupTestMapping(in cmuxmap.File) cmuxmap.File {
	out := cmuxmap.File{
		Version:    in.Version,
		Workspaces: make(map[string]cmuxmap.WorkspaceMapping, len(in.Workspaces)),
		Todos:      make(map[string]cmuxmap.TodoMapping, len(in.Todos)),
	}
	if in.RootGroup != nil {
		group := *in.RootGroup
		out.RootGroup = &group
	}
	for id, mapping := range in.Workspaces {
		out.Workspaces[id] = cloneWorkspaceMapping(mapping)
	}
	for id, todo := range in.Todos {
		todo.Items = append([]cmuxmap.TodoItemBase{}, todo.Items...)
		out.Todos[id] = todo
	}
	return out
}

func newGroupTestStore(file cmuxmap.File) *groupTestStore {
	if file.Version == 0 {
		file.Version = cmuxmap.CurrentVersion
	}
	if file.Workspaces == nil {
		file.Workspaces = map[string]cmuxmap.WorkspaceMapping{}
	}
	if file.Todos == nil {
		file.Todos = map[string]cmuxmap.TodoMapping{}
	}
	return &groupTestStore{file: cloneGroupTestMapping(file)}
}

func newWorkspaceGroupTestService(client *groupTestClient, store *groupTestStore) *Service {
	service := NewService(
		func() Client { return client },
		func(string) MappingStore { return store },
	)
	service.Now = func() time.Time { return time.Date(2026, 8, 13, 12, 34, 56, 0, time.UTC) }
	return service
}

func TestRootWorkspaceGroupNameUsesRootBasename(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "projects", "kra")
	if got := rootWorkspaceGroupName(root); got != "kra" {
		t.Fatalf("rootWorkspaceGroupName(%q) = %q, want %q", root, got, "kra")
	}
}

func TestWorkspaceGroupCapabilitiesRequireFocusAndUngroup(t *testing.T) {
	for _, method := range []string{"workspace.group.focus", "workspace.group.ungroup"} {
		caps := workspaceGroupTestCapabilities()
		delete(caps.Methods, method)
		if supportsWorkspaceGroups(caps) {
			t.Fatalf("supportsWorkspaceGroups() = true without %s", method)
		}
		if warning := workspaceGroupCapabilityWarning(caps); !strings.Contains(warning, method) {
			t.Fatalf("warning = %q, want missing method %q", warning, method)
		}
	}
}

func TestWorkspaceGroupFreshCreateRequiresStableWindowID(t *testing.T) {
	root := t.TempDir()
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowRef: "window:1"}}
	store := newGroupTestStore(cmuxmap.File{})

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" || !strings.Contains(message, "stable cmux workspace group window id") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if len(client.createGroupCalls) != 0 || len(client.closed) != 0 || len(client.ungroupCalls) != 0 {
		t.Fatalf("invalid window response caused side effects: create=%+v close=%v ungroup=%v", client.createGroupCalls, client.closed, client.ungroupCalls)
	}
	if store.saveCalls != 0 {
		t.Fatalf("mapping saves = %d, want 0", store.saveCalls)
	}
}

func TestWorkspaceGroupFreshGroupAndTask(t *testing.T) {
	root := t.TempDir()
	taskPath := filepath.Join(root, "workspaces", "WS1")
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", WindowRef: "window:1"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		ID:                  "group-id",
		Ref:                 "workspace_group:1",
		Name:                rootWorkspaceGroupName(root),
		AnchorWorkspaceID:   "anchor-id",
		AnchorWorkspaceRef:  "workspace:1",
		MemberWorkspaceIDs:  []string{"anchor-id"},
		MemberWorkspaceRefs: []string{"workspace:1"},
		MemberCount:         1,
	}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID:  "task-workspace-id",
		WorkspaceRef: "workspace:2",
		WindowID:     "window-id",
		WindowRef:    "window:1",
		GroupID:      "group-id",
		GroupRef:     "workspace_group:1",
	}}
	store := newGroupTestStore(cmuxmap.File{})

	result, code, message := newWorkspaceGroupTestService(client, store).Open(
		context.Background(),
		root,
		[]OpenTarget{{WorkspaceID: "WS1", WorkspacePath: taskPath, Title: "Build parser"}},
		1,
		false,
		"",
	)
	if code != "" {
		t.Fatalf("Open() code=%q message=%q", code, message)
	}
	if len(result.Failures) != 0 || len(result.Results) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got := result.Results[0]; got.CMUXWorkspaceID != "task-workspace-id" || got.ReusedExisting {
		t.Fatalf("opened item = %+v", got)
	}
	if len(client.createGroupCalls) != 1 {
		t.Fatalf("create group calls = %+v", client.createGroupCalls)
	}
	groupOpts := client.createGroupCalls[0]
	if groupOpts.Name != rootWorkspaceGroupName(root) || groupOpts.CWD != root || groupOpts.Window != "window-id" || len(groupOpts.ChildWorkspaceIDs) != 0 {
		t.Fatalf("create group options = %+v", groupOpts)
	}
	if len(client.createWorkspaceCalls) != 1 {
		t.Fatalf("create workspace calls = %+v", client.createWorkspaceCalls)
	}
	wantTitle, err := cmuxmap.FormatWorkspaceTitle("WS1", "Build parser", 1)
	if err != nil {
		t.Fatalf("format title: %v", err)
	}
	workspaceOpts := client.createWorkspaceCalls[0]
	if workspaceOpts.Title != wantTitle || workspaceOpts.CWD != taskPath || workspaceOpts.Focus || workspaceOpts.WindowID != "window-id" || workspaceOpts.GroupID != "group-id" || workspaceOpts.GroupPlacement != "end" {
		t.Fatalf("create workspace options = %+v", workspaceOpts)
	}
	if len(workspaceOpts.OperationID) != 36 {
		t.Fatalf("operation id = %q, want UUID", workspaceOpts.OperationID)
	}

	saved := store.snapshot()
	if saved.RootGroup == nil || saved.RootGroup.CMUXWindowID != "window-id" || saved.RootGroup.CMUXGroupID != "group-id" || saved.RootGroup.AnchorWorkspaceID != "anchor-id" {
		t.Fatalf("root group mapping = %+v", saved.RootGroup)
	}
	assertSingleGroupTestMapping(t, saved, rootMappingWorkspaceID, "anchor-id")
	assertSingleGroupTestMapping(t, saved, "WS1", "task-workspace-id")
	if len(client.closed) != 0 || len(client.addCalls) != 0 {
		t.Fatalf("unexpected cleanup/add: closed=%v adds=%+v", client.closed, client.addCalls)
	}
}

func TestWorkspaceGroupChildCreateRetriesWithSameOperationID(t *testing.T) {
	root := t.TempDir()
	taskPath := filepath.Join(root, "workspaces", "WS1")
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID:      "window-id",
		CMUXGroupID:       "group-id",
		AnchorWorkspaceID: "anchor-id",
		NameSnapshot:      "root",
		CreatedAt:         "2026-08-12T00:00:00Z",
		LastUsedAt:        "2026-08-12T00:00:00Z",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
		},
	})
	live := cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: "root", WindowID: "window-id",
		AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"}, MemberCount: 1,
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{live}}}
	client.createWorkspaceErrs = []error{context.DeadlineExceeded, nil}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "task-id", WindowID: "window-id", GroupID: "group-id",
	}}

	item, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: "WS1", WorkspacePath: taskPath, Title: "Build"},
		false,
	)
	if code != "" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if item.CMUXWorkspaceID != "task-id" || item.ReusedExisting {
		t.Fatalf("item = %+v", item)
	}
	if len(client.createWorkspaceCalls) != 2 {
		t.Fatalf("create workspace calls = %+v", client.createWorkspaceCalls)
	}
	if !reflect.DeepEqual(client.createWorkspaceCalls[0], client.createWorkspaceCalls[1]) {
		t.Fatalf("retry changed create options: first=%+v retry=%+v", client.createWorkspaceCalls[0], client.createWorkspaceCalls[1])
	}
	gotOpts := client.createWorkspaceCalls[0]
	if len(gotOpts.OperationID) != 36 || gotOpts.WindowID != "window-id" || gotOpts.GroupID != "group-id" {
		t.Fatalf("create workspace options = %+v", gotOpts)
	}
	assertSingleGroupTestMapping(t, store.snapshot(), "WS1", "task-id")
}

func TestWorkspaceCreateRetryRecoversLiveIDAfterInnerTimeout(t *testing.T) {
	client := newGroupTestClient()
	client.createWorkspaceErrs = []error{context.DeadlineExceeded, nil}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "recovered-id", WindowID: "window-id", GroupID: "group-id",
	}}
	opts := cmuxctl.WorkspaceCreateOptions{
		OperationID:    "11111111-1111-1111-1111-111111111111",
		Title:          "WS1 | Build",
		CWD:            "/tmp/root/workspaces/WS1",
		WindowID:       "window-id",
		GroupID:        "group-id",
		GroupPlacement: "end",
	}

	got, err := createWorkspaceWithIdempotencyRetry(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("createWorkspaceWithIdempotencyRetry() error: %v", err)
	}
	if got.WorkspaceID != "recovered-id" {
		t.Fatalf("result = %+v", got)
	}
	if len(client.createWorkspaceCalls) != 2 || !reflect.DeepEqual(client.createWorkspaceCalls[0], client.createWorkspaceCalls[1]) {
		t.Fatalf("create workspace calls = %+v", client.createWorkspaceCalls)
	}
	if len(client.createWorkspaceCtxErrs) != 2 || client.createWorkspaceCtxErrs[0] != nil || client.createWorkspaceCtxErrs[1] != nil {
		t.Fatalf("create workspace context errors = %v, want [<nil> <nil>]", client.createWorkspaceCtxErrs)
	}
}

func TestWorkspaceCreateRetryRecoversAfterMalformedSuccessPayload(t *testing.T) {
	client := newGroupTestClient()
	client.createWorkspaceErrs = []error{
		errors.New("decode cmux rpc workspace.create json output: unexpected end of JSON input"),
		nil,
	}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "recovered-id", WindowID: "window-id", GroupID: "group-id",
	}}
	opts := cmuxctl.WorkspaceCreateOptions{
		OperationID:    "11111111-1111-1111-1111-111111111111",
		Title:          "WS1 | Build",
		CWD:            "/tmp/root/workspaces/WS1",
		WindowID:       "window-id",
		GroupID:        "group-id",
		GroupPlacement: "end",
	}

	got, err := createWorkspaceWithIdempotencyRetry(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("createWorkspaceWithIdempotencyRetry() error: %v", err)
	}
	if got.WorkspaceID != "recovered-id" || len(client.createWorkspaceCalls) != 2 {
		t.Fatalf("result=%+v calls=%+v", got, client.createWorkspaceCalls)
	}
	if !reflect.DeepEqual(client.createWorkspaceCalls[0], client.createWorkspaceCalls[1]) {
		t.Fatalf("retry changed create options: first=%+v retry=%+v", client.createWorkspaceCalls[0], client.createWorkspaceCalls[1])
	}
}

func TestWorkspaceCreateDoesNotRetryAfterCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := newGroupTestClient()
	client.failCreateOnCanceled = true
	opts := cmuxctl.WorkspaceCreateOptions{
		OperationID:    "11111111-1111-1111-1111-111111111111",
		Title:          "WS1 | Build",
		CWD:            "/tmp/root/workspaces/WS1",
		WindowID:       "window-id",
		GroupID:        "group-id",
		GroupPlacement: "end",
	}

	_, err := createWorkspaceWithIdempotencyRetry(ctx, client, opts)
	if err == nil {
		t.Fatal("createWorkspaceWithIdempotencyRetry() error = nil")
	}
	for _, want := range []string{
		"result is uncertain",
		"operation_id=" + opts.OperationID,
		"caller context ended before retry: context canceled",
		"inspect cmux workspaces",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}
	if len(client.createWorkspaceCalls) != 1 {
		t.Fatalf("create workspace calls = %+v, want no retry", client.createWorkspaceCalls)
	}
	if len(client.createWorkspaceCtxErrs) != 1 || !errors.Is(client.createWorkspaceCtxErrs[0], context.Canceled) {
		t.Fatalf("create workspace context errors = %v", client.createWorkspaceCtxErrs)
	}
}

func TestWorkspaceCreateDoesNotRetryDeterministicRPCError(t *testing.T) {
	client := newGroupTestClient()
	client.createWorkspaceErr = errors.New("cmux rpc workspace.create: not_found: Group not found")
	opts := cmuxctl.WorkspaceCreateOptions{
		OperationID:    "11111111-1111-1111-1111-111111111111",
		Title:          "WS1 | Build",
		CWD:            "/tmp/root/workspaces/WS1",
		WindowID:       "window-id",
		GroupID:        "group-id",
		GroupPlacement: "end",
	}

	_, err := createWorkspaceWithIdempotencyRetry(context.Background(), client, opts)
	if err == nil || !strings.Contains(err.Error(), "Group not found") {
		t.Fatalf("createWorkspaceWithIdempotencyRetry() error = %v", err)
	}
	if len(client.createWorkspaceCalls) != 1 {
		t.Fatalf("create workspace calls = %+v, want no retry", client.createWorkspaceCalls)
	}
}

func TestWorkspaceGroupChildCreateUncertainResultReportsManualRepairIdentity(t *testing.T) {
	root := t.TempDir()
	taskPath := filepath.Join(root, "workspaces", "WS1")
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID:      "window-id",
		CMUXGroupID:       "group-id",
		AnchorWorkspaceID: "anchor-id",
		NameSnapshot:      "root",
		CreatedAt:         "2026-08-12T00:00:00Z",
		LastUsedAt:        "2026-08-12T00:00:00Z",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
		},
	})
	live := cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: "root", WindowID: "window-id",
		AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"}, MemberCount: 1,
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{live}}}
	client.createWorkspaceErrs = []error{context.DeadlineExceeded, errors.New("retry unavailable")}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: "WS1", WorkspacePath: taskPath, Title: "Build"},
		false,
	)
	if code != "cmux_create_failed" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if len(client.createWorkspaceCalls) != 2 || !reflect.DeepEqual(client.createWorkspaceCalls[0], client.createWorkspaceCalls[1]) {
		t.Fatalf("create workspace calls = %+v", client.createWorkspaceCalls)
	}
	operationID := client.createWorkspaceCalls[0].OperationID
	for _, want := range []string{
		"result is uncertain",
		"operation_id=" + operationID,
		"window_id=window-id",
		"group_id=group-id",
		taskPath,
		"inspect cmux workspaces",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
	if len(operationID) != 36 {
		t.Fatalf("operation id = %q, want UUID", operationID)
	}
	if len(client.closed) != 0 {
		t.Fatalf("uncertain workspace was closed without a known handle: %v", client.closed)
	}
	if _, exists := store.snapshot().Workspaces["WS1"]; exists {
		t.Fatal("uncertain workspace result was persisted")
	}
}

func TestWorkspaceGroupCreateErrorIsFailClosedAndDiagnostic(t *testing.T) {
	root := t.TempDir()
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", WindowRef: "window:1"}}
	client.createGroupErr = errors.New("response lost")
	store := newGroupTestStore(cmuxmap.File{})

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	for _, want := range []string{
		`window "window-id"`,
		`name "` + rootWorkspaceGroupName(root) + `"`,
		"result is uncertain",
		"ownership could not be proven",
		"inspect cmux Workspace Groups manually",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
	if len(client.createGroupCalls) != 1 {
		t.Fatalf("create group calls = %+v", client.createGroupCalls)
	}
	if len(client.ungroupCalls) != 0 || len(client.removeCalls) != 0 || len(client.closed) != 0 {
		t.Fatalf("uncertain group was cleaned without ownership proof: ungroup=%v remove=%+v close=%v", client.ungroupCalls, client.removeCalls, client.closed)
	}
	if store.saveCalls != 0 {
		t.Fatalf("save calls = %d, want 0", store.saveCalls)
	}
}

func TestWorkspaceGroupRepairAnchorCreateUsesLiveGroupWindow(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID:      "stale-window-id",
		CMUXGroupID:       "group-id",
		AnchorWorkspaceID: "missing-anchor-id",
		NameSnapshot:      "root",
		CreatedAt:         "2026-08-12T00:00:00Z",
		LastUsedAt:        "2026-08-12T00:00:00Z",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("missing-anchor-id", "root"),
		},
	})
	live := cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: "root", WindowID: "live-window-id",
		AnchorWorkspaceID: "live-anchor-id", MemberWorkspaceIDs: []string{"live-anchor-id"}, MemberCount: 1,
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{
		{WindowID: "stale-window-id"},
		{WindowID: "live-window-id", Groups: []cmuxctl.WorkspaceGroup{live}},
	}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "repair-anchor-id", WindowID: "live-window-id", GroupID: "group-id",
	}}

	item, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if item.CMUXWorkspaceID != "repair-anchor-id" {
		t.Fatalf("item = %+v", item)
	}
	if len(client.createWorkspaceCalls) != 1 {
		t.Fatalf("create workspace calls = %+v", client.createWorkspaceCalls)
	}
	gotOpts := client.createWorkspaceCalls[0]
	if gotOpts.WindowID != "live-window-id" || gotOpts.GroupID != "group-id" || len(gotOpts.OperationID) != 36 {
		t.Fatalf("repair create options = %+v", gotOpts)
	}
	if !reflect.DeepEqual(client.setAnchorCalls, []groupMembershipCall{{Group: "group-id", Workspace: "repair-anchor-id"}}) {
		t.Fatalf("set anchor calls = %+v", client.setAnchorCalls)
	}
	saved := store.snapshot()
	if saved.RootGroup == nil || saved.RootGroup.CMUXWindowID != "live-window-id" || saved.RootGroup.AnchorWorkspaceID != "repair-anchor-id" {
		t.Fatalf("root group mapping = %+v", saved.RootGroup)
	}
}

func TestWorkspaceGroupIdempotentReuse(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID:      "window-id",
		CMUXGroupID:       "group-id",
		AnchorWorkspaceID: "anchor-id",
		NameSnapshot:      "user name",
		CreatedAt:         "2026-08-12T00:00:00Z",
		LastUsedAt:        "2026-08-12T00:00:00Z",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "user name"),
			"WS1":                  testWorkspaceMapping("task-workspace-id", "WS1 | Build parser"),
		},
	})
	live := cmuxctl.WorkspaceGroup{
		ID: "group-id", Ref: "workspace_group:1", Name: "user name", WindowID: "window-id",
		AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id", "task-workspace-id", "foreign-id"}, MemberCount: 3,
		Preferences: cmuxctl.WorkspaceGroupPreferences{Collapsed: true, Pinned: true, CustomColor: "#123456", IconSymbol: "folder"},
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{
		{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{live}},
		{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{live}},
	}

	result, code, message := newWorkspaceGroupTestService(client, store).Open(
		context.Background(), root,
		[]OpenTarget{{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build parser"}},
		1, false, "",
	)
	if code != "" {
		t.Fatalf("Open() code=%q message=%q", code, message)
	}
	if len(result.Results) != 1 || !result.Results[0].ReusedExisting || result.Results[0].CMUXWorkspaceID != "task-workspace-id" {
		t.Fatalf("result = %+v", result)
	}
	if len(client.createGroupCalls) != 0 || len(client.createWorkspaceCalls) != 0 || len(client.addCalls) != 0 || len(client.removeCalls) != 0 || len(client.setAnchorCalls) != 0 || len(client.closed) != 0 {
		t.Fatalf("idempotent reuse mutated runtime: createGroup=%d createWorkspace=%d add=%v remove=%v anchor=%v close=%v",
			len(client.createGroupCalls), len(client.createWorkspaceCalls), client.addCalls, client.removeCalls, client.setAnchorCalls, client.closed)
	}
	saved := store.snapshot()
	if saved.RootGroup == nil || saved.RootGroup.NameSnapshot != "user name" || saved.RootGroup.CMUXGroupID != "group-id" {
		t.Fatalf("root group mapping = %+v", saved.RootGroup)
	}
}

func TestWorkspaceGroupRecoversMovedManagedGroupFromAmbientList(t *testing.T) {
	t.Run("reconciles saved group and updates window", func(t *testing.T) {
		root := t.TempDir()
		groupMapping := cmuxmap.GroupMapping{
			CMUXWindowID:      "stale-window-id",
			CMUXGroupID:       "managed-group-id",
			AnchorWorkspaceID: "root-anchor-id",
			NameSnapshot:      "saved root name",
			CreatedAt:         "2026-08-12T00:00:00Z",
			LastUsedAt:        "2026-08-12T00:00:00Z",
		}
		store := newGroupTestStore(cmuxmap.File{
			RootGroup: &groupMapping,
			Workspaces: map[string]cmuxmap.WorkspaceMapping{
				rootMappingWorkspaceID: testWorkspaceMapping("root-anchor-id", "saved root name"),
			},
		})
		managed := cmuxctl.WorkspaceGroup{
			ID: "managed-group-id", Name: "live root name", WindowID: "moved-window-id",
			AnchorWorkspaceID:  "temporary-anchor-id",
			MemberWorkspaceIDs: []string{"temporary-anchor-id", "root-anchor-id", "managed-member-id"},
			MemberCount:        3,
		}
		foreign := cmuxctl.WorkspaceGroup{
			ID: "foreign-group-id", Name: "foreign",
			AnchorWorkspaceID:  "foreign-anchor-id",
			MemberWorkspaceIDs: []string{"foreign-anchor-id", "foreign-member-id"},
			MemberCount:        2,
		}
		client := newGroupTestClient()
		client.identifyErr = map[string]error{
			"root-anchor-id": errors.New("not_found: workspace not found in stale window"),
		}
		client.groupLists = []cmuxctl.WorkspaceGroupList{
			{WindowID: "stale-window-id"},
			{WindowID: "moved-window-id", Groups: []cmuxctl.WorkspaceGroup{foreign, managed}},
		}

		item, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
			context.Background(), root,
			OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
			false,
		)
		if code != "" {
			t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
		}
		if item.CMUXWorkspaceID != "root-anchor-id" || !item.ReusedExisting {
			t.Fatalf("root item = %+v", item)
		}
		if !reflect.DeepEqual(client.groupListCalls, []string{"stale-window-id", ""}) {
			t.Fatalf("group list calls = %v", client.groupListCalls)
		}
		if !reflect.DeepEqual(client.setAnchorCalls, []groupMembershipCall{{Group: "managed-group-id", Workspace: "root-anchor-id"}}) {
			t.Fatalf("set anchor calls = %+v", client.setAnchorCalls)
		}
		if !reflect.DeepEqual(client.focusCalls, []string{"managed-group-id"}) {
			t.Fatalf("focus calls = %v", client.focusCalls)
		}
		if len(client.createGroupCalls) != 0 || len(client.createWorkspaceCalls) != 0 || len(client.addCalls) != 0 || len(client.removeCalls) != 0 || len(client.ungroupCalls) != 0 || len(client.closed) != 0 {
			t.Fatalf("moved/foreign groups were unexpectedly mutated: createGroup=%d createWorkspace=%d add=%+v remove=%+v ungroup=%v close=%v",
				len(client.createGroupCalls), len(client.createWorkspaceCalls), client.addCalls, client.removeCalls, client.ungroupCalls, client.closed)
		}
		saved := store.snapshot()
		if saved.RootGroup == nil {
			t.Fatal("root group mapping is nil")
		}
		if saved.RootGroup.CMUXWindowID != "moved-window-id" || saved.RootGroup.CMUXGroupID != "managed-group-id" || saved.RootGroup.AnchorWorkspaceID != "root-anchor-id" || saved.RootGroup.NameSnapshot != "live root name" {
			t.Fatalf("root group mapping = %+v", saved.RootGroup)
		}
		assertSingleGroupTestMapping(t, saved, rootMappingWorkspaceID, "root-anchor-id")
	})

	t.Run("ambient lookup failure is fail closed", func(t *testing.T) {
		root := t.TempDir()
		groupMapping := cmuxmap.GroupMapping{
			CMUXWindowID:      "stale-window-id",
			CMUXGroupID:       "managed-group-id",
			AnchorWorkspaceID: "root-anchor-id",
			NameSnapshot:      "saved root name",
		}
		store := newGroupTestStore(cmuxmap.File{
			RootGroup: &groupMapping,
			Workspaces: map[string]cmuxmap.WorkspaceMapping{
				rootMappingWorkspaceID: testWorkspaceMapping("root-anchor-id", "saved root name"),
			},
		})
		client := newGroupTestClient()
		client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "stale-window-id"}}
		client.groupListErrs = []error{nil, errors.New("ambient unavailable")}

		_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
			context.Background(), root,
			OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
			false,
		)
		if code != "cmux_group_list_failed" || !strings.Contains(message, "ambient unavailable") {
			t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
		}
		if !reflect.DeepEqual(client.groupListCalls, []string{"stale-window-id", ""}) {
			t.Fatalf("group list calls = %v", client.groupListCalls)
		}
		assertNoGroupTestMutation(t, client)
		if store.saveCalls != 0 {
			t.Fatalf("save calls = %d, want 0", store.saveCalls)
		}
		if saved := store.snapshot(); !reflect.DeepEqual(saved.RootGroup, &groupMapping) {
			t.Fatalf("root group mapping changed: got %+v want %+v", saved.RootGroup, groupMapping)
		}
	})
}

func TestWorkspaceGroupRecoversMovedManagedGroupFromThirdWindow(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID:      "saved-window-id",
		CMUXGroupID:       "managed-group-id",
		AnchorWorkspaceID: "root-anchor-id",
		NameSnapshot:      "saved root name",
		CreatedAt:         "2026-08-12T00:00:00Z",
		LastUsedAt:        "2026-08-12T00:00:00Z",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("root-anchor-id", "saved root name"),
		},
	})
	managed := cmuxctl.WorkspaceGroup{
		ID: "managed-group-id", Name: "live root name", WindowID: "third-window-id",
		AnchorWorkspaceID:  "temporary-anchor-id",
		MemberWorkspaceIDs: []string{"temporary-anchor-id", "root-anchor-id", "managed-member-id"},
		MemberCount:        3,
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{
		{WindowID: "saved-window-id"},
		{WindowID: "ambient-window-id"},
		{WindowID: "third-window-id", Groups: []cmuxctl.WorkspaceGroup{managed}},
	}
	client.windows = []cmuxctl.Window{
		{ID: "saved-window-id"},
		{ID: "ambient-window-id", Key: true},
		{ID: "third-window-id"},
	}

	item, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if item.CMUXWorkspaceID != "root-anchor-id" || !item.ReusedExisting {
		t.Fatalf("root item = %+v", item)
	}
	if !reflect.DeepEqual(client.groupListCalls, []string{"saved-window-id", "", "third-window-id"}) {
		t.Fatalf("group list calls = %v", client.groupListCalls)
	}
	if client.windowListCalls != 1 {
		t.Fatalf("window list calls = %d, want 1", client.windowListCalls)
	}
	if !reflect.DeepEqual(client.setAnchorCalls, []groupMembershipCall{{Group: "managed-group-id", Workspace: "root-anchor-id"}}) {
		t.Fatalf("set anchor calls = %+v", client.setAnchorCalls)
	}
	if !reflect.DeepEqual(client.focusCalls, []string{"managed-group-id"}) {
		t.Fatalf("focus calls = %v", client.focusCalls)
	}
	if len(client.createGroupCalls) != 0 || len(client.createWorkspaceCalls) != 0 || len(client.addCalls) != 0 || len(client.removeCalls) != 0 || len(client.ungroupCalls) != 0 || len(client.closed) != 0 {
		t.Fatalf("third-window recovery mutated foreign/provisional runtimes: createGroup=%d createWorkspace=%d add=%+v remove=%+v ungroup=%v close=%v",
			len(client.createGroupCalls), len(client.createWorkspaceCalls), client.addCalls, client.removeCalls, client.ungroupCalls, client.closed)
	}
	saved := store.snapshot()
	if saved.RootGroup == nil || saved.RootGroup.CMUXWindowID != "third-window-id" || saved.RootGroup.CMUXGroupID != "managed-group-id" || saved.RootGroup.AnchorWorkspaceID != "root-anchor-id" || saved.RootGroup.NameSnapshot != "live root name" {
		t.Fatalf("root group mapping = %+v", saved.RootGroup)
	}
	assertSingleGroupTestMapping(t, saved, rootMappingWorkspaceID, "root-anchor-id")
}

func TestWorkspaceGroupAllWindowScanFailuresAreFailClosed(t *testing.T) {
	tests := []struct {
		name           string
		windowListErr  error
		groupListErrs  []error
		wantMessage    string
		wantGroupCalls []string
	}{
		{
			name:           "window enumeration failure",
			windowListErr:  errors.New("window enumeration unavailable"),
			wantMessage:    "window enumeration unavailable",
			wantGroupCalls: []string{"saved-window-id", ""},
		},
		{
			name:           "third window group list failure",
			groupListErrs:  []error{nil, nil, errors.New("third window unavailable")},
			wantMessage:    "third window unavailable",
			wantGroupCalls: []string{"saved-window-id", "", "third-window-id"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			groupMapping := cmuxmap.GroupMapping{
				CMUXWindowID:      "saved-window-id",
				CMUXGroupID:       "managed-group-id",
				AnchorWorkspaceID: "root-anchor-id",
				NameSnapshot:      "saved root name",
			}
			store := newGroupTestStore(cmuxmap.File{
				RootGroup: &groupMapping,
				Workspaces: map[string]cmuxmap.WorkspaceMapping{
					rootMappingWorkspaceID: testWorkspaceMapping("root-anchor-id", "saved root name"),
				},
			})
			client := newGroupTestClient()
			client.groupLists = []cmuxctl.WorkspaceGroupList{
				{WindowID: "saved-window-id"},
				{WindowID: "ambient-window-id"},
				{WindowID: "third-window-id"},
			}
			client.groupListErrs = tt.groupListErrs
			client.windows = []cmuxctl.Window{
				{ID: "saved-window-id"},
				{ID: "ambient-window-id", Key: true},
				{ID: "third-window-id"},
			}
			client.windowListErr = tt.windowListErr

			_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
				context.Background(), root,
				OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
				false,
			)
			if code != "cmux_group_list_failed" || !strings.Contains(message, tt.wantMessage) {
				t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
			}
			if !reflect.DeepEqual(client.groupListCalls, tt.wantGroupCalls) {
				t.Fatalf("group list calls = %v, want %v", client.groupListCalls, tt.wantGroupCalls)
			}
			if client.windowListCalls != 1 {
				t.Fatalf("window list calls = %d, want 1", client.windowListCalls)
			}
			assertNoGroupTestMutation(t, client)
			if store.saveCalls != 0 {
				t.Fatalf("save calls = %d, want 0", store.saveCalls)
			}
			if saved := store.snapshot(); !reflect.DeepEqual(saved.RootGroup, &groupMapping) {
				t.Fatalf("root group mapping changed: got %+v want %+v", saved.RootGroup, groupMapping)
			}
		})
	}
}

func TestWorkspaceGroupLegacyRootMigrationRoutesToRootWindow(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{Workspaces: map[string]cmuxmap.WorkspaceMapping{
		rootMappingWorkspaceID: testWorkspaceMapping("legacy-anchor-id", "KRA_ROOT"),
	}})
	client := newGroupTestClient()
	client.identifyBy = map[string]map[string]any{
		"legacy-anchor-id": testIdentifyCallerPayload("legacy-anchor-id", "root-window-id"),
	}
	client.groupLists = []cmuxctl.WorkspaceGroupList{
		{WindowID: "ambient-window-id", WindowRef: "window:1"},
		{WindowID: "root-window-id", WindowRef: "window:2"},
	}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		ID: "group-id", Ref: "workspace_group:1", Name: rootWorkspaceGroupName(root),
		AnchorWorkspaceID: "provisional-anchor-id", MemberWorkspaceIDs: []string{"provisional-anchor-id", "legacy-anchor-id"}, MemberCount: 2,
	}

	item, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if item.CMUXWorkspaceID != "legacy-anchor-id" || !item.ReusedExisting {
		t.Fatalf("root item = %+v", item)
	}
	if !reflect.DeepEqual(client.groupListCalls, []string{"", "root-window-id"}) {
		t.Fatalf("group list calls = %v", client.groupListCalls)
	}
	if len(client.createGroupCalls) != 1 || client.createGroupCalls[0].Window != "root-window-id" || !reflect.DeepEqual(client.createGroupCalls[0].ChildWorkspaceIDs, []string{"legacy-anchor-id"}) {
		t.Fatalf("create group calls = %+v", client.createGroupCalls)
	}
	if !reflect.DeepEqual(client.setAnchorCalls, []groupMembershipCall{{Group: "group-id", Workspace: "legacy-anchor-id"}}) {
		t.Fatalf("set anchor calls = %+v", client.setAnchorCalls)
	}
	if !reflect.DeepEqual(client.removeCalls, []groupMembershipCall{{Group: "group-id", Workspace: "provisional-anchor-id"}}) {
		t.Fatalf("remove calls = %+v", client.removeCalls)
	}
	if !reflect.DeepEqual(client.closed, []string{"provisional-anchor-id"}) {
		t.Fatalf("closed workspaces = %v", client.closed)
	}
	if !reflect.DeepEqual(client.focusCalls, []string{"group-id"}) {
		t.Fatalf("focus calls = %v", client.focusCalls)
	}
	if len(client.createWorkspaceCalls) != 0 || len(client.ungroupCalls) != 0 {
		t.Fatalf("unexpected create/ungroup: create=%+v ungroup=%v", client.createWorkspaceCalls, client.ungroupCalls)
	}
	saved := store.snapshot()
	if saved.RootGroup == nil || saved.RootGroup.CMUXWindowID != "root-window-id" || saved.RootGroup.AnchorWorkspaceID != "legacy-anchor-id" || saved.RootGroup.CMUXGroupID != "group-id" {
		t.Fatalf("root group mapping = %+v", saved.RootGroup)
	}
	assertSingleGroupTestMapping(t, saved, rootMappingWorkspaceID, "legacy-anchor-id")
}

func TestWorkspaceGroupLegacyRootLocationFailureIsFailClosed(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{Workspaces: map[string]cmuxmap.WorkspaceMapping{
		rootMappingWorkspaceID: testWorkspaceMapping("legacy-anchor-id", "KRA_ROOT"),
	}})
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "ambient-window-id"}}
	client.identifyBy = map[string]map[string]any{
		"legacy-anchor-id": {
			"caller": map[string]any{"workspace_id": "legacy-anchor-id"},
		},
	}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_identify_failed" || !strings.Contains(message, "caller.window_id") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	assertNoGroupTestMutation(t, client)
	if store.saveCalls != 0 {
		t.Fatalf("save calls = %d, want 0", store.saveCalls)
	}
}

func TestWorkspaceGroupForeignMembershipIsNonDestructive(t *testing.T) {
	t.Run("legacy root already in foreign group", func(t *testing.T) {
		root := t.TempDir()
		store := newGroupTestStore(cmuxmap.File{Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("legacy-anchor-id", "KRA_ROOT"),
		}})
		client := newGroupTestClient()
		client.identifyBy = map[string]map[string]any{
			"legacy-anchor-id": testIdentifyCallerPayload("legacy-anchor-id", "root-window-id"),
		}
		client.groupLists = []cmuxctl.WorkspaceGroupList{
			{WindowID: "ambient-window-id"},
			{WindowID: "root-window-id", Groups: []cmuxctl.WorkspaceGroup{{
				ID: "foreign-group-id", AnchorWorkspaceID: "foreign-anchor-id",
				MemberWorkspaceIDs: []string{"foreign-anchor-id", "legacy-anchor-id"}, MemberCount: 2,
			}}},
		}

		_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
			context.Background(), root,
			OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
			false,
		)
		if code != "cmux_group_conflict" || !strings.Contains(message, "foreign-group-id") {
			t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
		}
		if !reflect.DeepEqual(client.groupListCalls, []string{"", "root-window-id"}) {
			t.Fatalf("group list calls = %v", client.groupListCalls)
		}
		assertNoGroupTestMutation(t, client)
		if store.saveCalls != 0 {
			t.Fatalf("save calls = %d, want 0", store.saveCalls)
		}
	})

	t.Run("mapped task already in foreign group", func(t *testing.T) {
		root := t.TempDir()
		groupMapping := cmuxmap.GroupMapping{CMUXWindowID: "window-id", CMUXGroupID: "managed-group-id", AnchorWorkspaceID: "anchor-id"}
		store := newGroupTestStore(cmuxmap.File{
			RootGroup: &groupMapping,
			Workspaces: map[string]cmuxmap.WorkspaceMapping{
				rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
				"WS1":                  testWorkspaceMapping("task-id", "WS1 | Build"),
			},
		})
		managed := cmuxctl.WorkspaceGroup{ID: "managed-group-id", AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"}}
		foreign := cmuxctl.WorkspaceGroup{ID: "foreign-group-id", AnchorWorkspaceID: "foreign-anchor-id", MemberWorkspaceIDs: []string{"foreign-anchor-id", "task-id"}}
		client := newGroupTestClient()
		client.groupLists = []cmuxctl.WorkspaceGroupList{
			{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed, foreign}},
			{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed, foreign}},
		}

		_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
			context.Background(), root,
			OpenTarget{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"},
			false,
		)
		if code != "cmux_group_conflict" || !strings.Contains(message, "foreign-group-id") {
			t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
		}
		assertNoGroupTestMutation(t, client)
		assertSingleGroupTestMapping(t, store.snapshot(), "WS1", "task-id")
	})
}

func TestWorkspaceGroupSaveFailureCompensatesFreshRuntimes(t *testing.T) {
	root := t.TempDir()
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: rootWorkspaceGroupName(root), AnchorWorkspaceID: "anchor-id",
		MemberWorkspaceIDs: []string{"anchor-id"}, MemberCount: 1,
	}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "task-id", WindowID: "window-id", GroupID: "group-id",
	}}
	store := newGroupTestStore(cmuxmap.File{})
	store.saveErr = errors.New("disk full")

	result, code, message := newWorkspaceGroupTestService(client, store).Open(
		context.Background(), root,
		[]OpenTarget{{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"}},
		1, false, "",
	)
	if code != "state_write_failed" || !strings.Contains(message, "disk full") {
		t.Fatalf("Open() result=%+v code=%q message=%q", result, code, message)
	}
	if !reflect.DeepEqual(client.closed, []string{"task-id", "anchor-id"}) {
		t.Fatalf("closed workspaces = %v, want child then anchor", client.closed)
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"group-id"}) || len(client.removeCalls) != 0 {
		t.Fatalf("fresh rollback group cleanup: ungroup=%v remove=%+v", client.ungroupCalls, client.removeCalls)
	}
	if saved := store.snapshot(); saved.RootGroup != nil || len(saved.Workspaces) != 0 {
		t.Fatalf("failed save changed stored mapping: %+v", saved)
	}
	joined := strings.Join(client.events, ",")
	if !strings.Contains(joined, "close:task-id,ungroup:group-id,close:anchor-id") {
		t.Fatalf("event order = %s", joined)
	}
}

func TestWorkspaceGroupFreshRollbackUngroupsAfterChildCloseFailure(t *testing.T) {
	root := t.TempDir()
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: rootWorkspaceGroupName(root), AnchorWorkspaceID: "anchor-id",
		MemberWorkspaceIDs: []string{"anchor-id"}, MemberCount: 1,
	}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "task-id", WindowID: "window-id", GroupID: "group-id",
	}}
	client.closeErrBy = map[string]error{"task-id": errors.New("child close unavailable")}
	store := newGroupTestStore(cmuxmap.File{})
	store.saveErr = errors.New("disk full")

	_, code, message := newWorkspaceGroupTestService(client, store).Open(
		context.Background(), root,
		[]OpenTarget{{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"}},
		1, false, "",
	)
	if code != "state_write_failed" {
		t.Fatalf("Open() code=%q message=%q", code, message)
	}
	for _, want := range []string{"child close unavailable", "cmux workspace task-id", "workspace may remain open"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"group-id"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if !reflect.DeepEqual(client.closed, []string{"task-id", "anchor-id"}) {
		t.Fatalf("closed workspaces = %v", client.closed)
	}
	joined := strings.Join(client.events, ",")
	if !strings.Contains(joined, "close:task-id,ungroup:group-id,close:anchor-id") {
		t.Fatalf("event order = %s", joined)
	}
}

func TestWorkspaceGroupFreshRollbackLeavesAnchorWhenUngroupFails(t *testing.T) {
	root := t.TempDir()
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: rootWorkspaceGroupName(root), AnchorWorkspaceID: "anchor-id",
		MemberWorkspaceIDs: []string{"anchor-id"}, MemberCount: 1,
	}
	client.ungroupErr = errors.New("ungroup unavailable")
	store := newGroupTestStore(cmuxmap.File{})
	store.saveErr = errors.New("disk full")

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "state_write_failed" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	for _, want := range []string{
		"cleanup failed for cmux group group-id",
		"ungroup unavailable",
		"ungroup it manually",
		"provisional anchor anchor-id was left open",
		"close the anchor manually",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"group-id"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if len(client.closed) != 0 {
		t.Fatalf("anchor was closed after failed ungroup: %v", client.closed)
	}
}

func TestWorkspaceGroupSaveFailureRetainsReusedMembershipInExistingGroup(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID:      "window-id",
		CMUXGroupID:       "group-id",
		AnchorWorkspaceID: "anchor-id",
		NameSnapshot:      "root",
		CreatedAt:         "2026-08-12T00:00:00Z",
		LastUsedAt:        "2026-08-12T00:00:00Z",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
			"WS1":                  testWorkspaceMapping("reused-id", "WS1 | Build"),
		},
	})
	store.saveErr = errors.New("disk full")
	managed := cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: "root", WindowID: "window-id",
		AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"},
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{
		{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed}},
		{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed}},
	}

	_, code, message := newWorkspaceGroupTestService(client, store).Open(
		context.Background(), root,
		[]OpenTarget{{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"}},
		1, false, "",
	)
	if code != "state_write_failed" || !strings.Contains(message, "disk full") {
		t.Fatalf("Open() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.addCalls, []groupMembershipCall{{Group: "group-id", Workspace: "reused-id"}}) {
		t.Fatalf("add calls = %+v", client.addCalls)
	}
	if len(client.removeCalls) != 0 || len(client.ungroupCalls) != 0 || len(client.closed) != 0 {
		t.Fatalf("reused workspace/group was destructively compensated: remove=%+v ungroup=%v close=%v",
			client.removeCalls, client.ungroupCalls, client.closed)
	}
}

func TestWorkspaceGroupSaveFailureDissolvesFreshGroupWithReusedMember(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{Workspaces: map[string]cmuxmap.WorkspaceMapping{
		"WS1": testWorkspaceMapping("reused-id", "WS1 | Build"),
	}})
	store.saveErr = errors.New("disk full")
	managed := cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: rootWorkspaceGroupName(root), WindowID: "window-id",
		AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"},
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{
		{WindowID: "window-id"},
		{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed}},
	}
	client.createGroupResult = managed

	_, code, message := newWorkspaceGroupTestService(client, store).Open(
		context.Background(), root,
		[]OpenTarget{{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"}},
		1, false, "",
	)
	if code != "state_write_failed" || !strings.Contains(message, "disk full") {
		t.Fatalf("Open() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.addCalls, []groupMembershipCall{{Group: "group-id", Workspace: "reused-id"}}) {
		t.Fatalf("add calls = %+v", client.addCalls)
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"group-id"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if !reflect.DeepEqual(client.closed, []string{"anchor-id"}) || len(client.removeCalls) != 0 {
		t.Fatalf("cleanup close=%v remove=%+v; reused workspace must be preserved", client.closed, client.removeCalls)
	}
}

func TestWorkspaceGroupReusedCommandFailureDoesNotAddMembership(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID:      "window-id",
		CMUXGroupID:       "group-id",
		AnchorWorkspaceID: "anchor-id",
		NameSnapshot:      "root",
		CreatedAt:         "2026-08-12T00:00:00Z",
		LastUsedAt:        "2026-08-12T00:00:00Z",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
			"WS1":                  testWorkspaceMapping("reused-id", "WS1 | Build"),
		},
	})
	managed := cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: "root", WindowID: "window-id",
		AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"},
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed}}}
	client.sendErr = errors.New("send failed")

	result, code, message := newWorkspaceGroupTestService(client, store).Open(
		context.Background(), root,
		[]OpenTarget{{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"}},
		1, false, "echo ok",
	)
	if code != "" || message != "" || len(result.Failures) != 1 || result.Failures[0].Code != "cmux_send_command_failed" {
		t.Fatalf("Open() result=%+v code=%q message=%q", result, code, message)
	}
	if len(client.addCalls) != 0 || len(client.removeCalls) != 0 {
		t.Fatalf("membership changed around failed command: add=%+v remove=%+v", client.addCalls, client.removeCalls)
	}
}

func TestWorkspaceGroupRootSaveFailureClosesFreshAnchorOnce(t *testing.T) {
	root := t.TempDir()
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: rootWorkspaceGroupName(root), AnchorWorkspaceID: "anchor-id",
		MemberWorkspaceIDs: []string{"anchor-id"}, MemberCount: 1,
	}
	store := newGroupTestStore(cmuxmap.File{})
	store.saveErr = errors.New("disk full")

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "state_write_failed" || !strings.Contains(message, "disk full") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.closed, []string{"anchor-id"}) {
		t.Fatalf("closed workspaces = %v, want fresh anchor exactly once", client.closed)
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"group-id"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if saved := store.snapshot(); saved.RootGroup != nil || len(saved.Workspaces) != 0 {
		t.Fatalf("failed save changed stored mapping: %+v", saved)
	}
}

func TestWorkspaceGroupSaveFailureUngroupsButPreservesLegacyRoot(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{Workspaces: map[string]cmuxmap.WorkspaceMapping{
		rootMappingWorkspaceID: testWorkspaceMapping("legacy-anchor-id", "KRA_ROOT"),
	}})
	store.saveErr = errors.New("disk full")
	client := newGroupTestClient()
	client.identifyBy = map[string]map[string]any{
		"legacy-anchor-id": testIdentifyCallerPayload("legacy-anchor-id", "window-id"),
	}
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		ID: "group-id", Name: rootWorkspaceGroupName(root), AnchorWorkspaceID: "provisional-anchor-id",
		MemberWorkspaceIDs: []string{"provisional-anchor-id", "legacy-anchor-id"}, MemberCount: 2,
	}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "state_write_failed" || !strings.Contains(message, "disk full") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"group-id"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if !reflect.DeepEqual(client.closed, []string{"provisional-anchor-id"}) {
		t.Fatalf("closed workspaces = %v; legacy root must stay open", client.closed)
	}
	if saved := store.snapshot(); saved.RootGroup != nil {
		t.Fatalf("failed save persisted root group: %+v", saved.RootGroup)
	}
	assertSingleGroupTestMapping(t, store.snapshot(), rootMappingWorkspaceID, "legacy-anchor-id")
}

func TestWorkspaceGroupWrongReturnedGroupClosesOnlyCreatedWorkspace(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID: "window-id", CMUXGroupID: "managed-group-id", AnchorWorkspaceID: "anchor-id", NameSnapshot: "root",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
		},
	})
	managed := cmuxctl.WorkspaceGroup{
		ID: "managed-group-id", Name: "root", WindowID: "window-id", AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"},
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed}}}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "provisional-task-id", WindowID: "window-id", GroupID: "foreign-group-id",
	}}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"},
		false,
	)
	if code != "cmux_group_setup_failed" || !strings.Contains(message, "unexpected group") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.closed, []string{"provisional-task-id"}) {
		t.Fatalf("closed workspaces = %v", client.closed)
	}
	if len(client.removeCalls) != 0 || len(client.ungroupCalls) != 0 || len(client.addCalls) != 0 || len(client.setAnchorCalls) != 0 {
		t.Fatalf("foreign/managed groups were mutated: remove=%+v ungroup=%v add=%+v anchor=%+v",
			client.removeCalls, client.ungroupCalls, client.addCalls, client.setAnchorCalls)
	}
	if _, exists := store.snapshot().Workspaces["WS1"]; exists {
		t.Fatal("failed provisional workspace was persisted")
	}
}

func TestWorkspaceGroupMissingReturnedGroupClosesCreatedWorkspace(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID: "window-id", CMUXGroupID: "managed-group-id", AnchorWorkspaceID: "anchor-id", NameSnapshot: "root",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
		},
	})
	managed := cmuxctl.WorkspaceGroup{
		ID: "managed-group-id", Name: "root", WindowID: "window-id", AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"},
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed}}}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{{
		WorkspaceID: "provisional-task-id", WindowID: "window-id",
	}}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"},
		false,
	)
	if code != "cmux_group_setup_failed" || !strings.Contains(message, "unexpected group") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.closed, []string{"provisional-task-id"}) {
		t.Fatalf("closed workspaces = %v", client.closed)
	}
	if _, exists := store.snapshot().Workspaces["WS1"]; exists {
		t.Fatal("unverified provisional workspace was persisted")
	}
}

func TestWorkspaceGroupCreateWrongResponseDoesNotCleanForeignGroup(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{})
	foreign := cmuxctl.WorkspaceGroup{
		ID: "foreign-group-id", Ref: "workspace_group:1", Name: "user group",
		AnchorWorkspaceID: "foreign-anchor-id", MemberWorkspaceIDs: []string{"foreign-anchor-id", "foreign-member-id"}, MemberCount: 2,
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{foreign}}}
	// Simulate an inconsistent cmux response that points at a group which was
	// already present in the pre-create snapshot. None of its identities are
	// safe compensation targets.
	client.createGroupResult = foreign

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" || !strings.Contains(message, "pre-existing group") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if len(client.createGroupCalls) != 1 {
		t.Fatalf("create group calls = %+v", client.createGroupCalls)
	}
	if len(client.closed) != 0 || len(client.removeCalls) != 0 || len(client.setAnchorCalls) != 0 || len(client.ungroupCalls) != 0 || len(client.focusCalls) != 0 {
		t.Fatalf("foreign group was used or cleaned: close=%v remove=%+v anchor=%+v ungroup=%v focus=%v",
			client.closed, client.removeCalls, client.setAnchorCalls, client.ungroupCalls, client.focusCalls)
	}
	if saved := store.snapshot(); saved.RootGroup != nil || len(saved.Workspaces) != 0 {
		t.Fatalf("wrong group response was persisted: %+v", saved)
	}
}

func TestWorkspaceGroupCreateMissingIDDoesNotCloseForeignAnchor(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{})
	foreign := cmuxctl.WorkspaceGroup{
		ID: "foreign-group-id", AnchorWorkspaceID: "foreign-anchor-id",
		MemberWorkspaceIDs: []string{"foreign-anchor-id"}, MemberCount: 1,
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{foreign}}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{AnchorWorkspaceID: "foreign-anchor-id"}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" || !strings.Contains(message, "reused anchor") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if len(client.closed) != 0 || len(client.ungroupCalls) != 0 || len(client.removeCalls) != 0 || len(client.setAnchorCalls) != 0 {
		t.Fatalf("foreign anchor was cleaned or mutated: close=%v ungroup=%v remove=%+v anchor=%+v",
			client.closed, client.ungroupCalls, client.removeCalls, client.setAnchorCalls)
	}
}

func TestWorkspaceGroupCreateNewIDWithForeignAnchorUngroupsOnlyNewGroup(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{})
	foreign := cmuxctl.WorkspaceGroup{
		ID: "foreign-group-id", AnchorWorkspaceID: "foreign-anchor-id",
		MemberWorkspaceIDs: []string{"foreign-anchor-id"}, MemberCount: 1,
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{foreign}}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{ID: "created-group-id", AnchorWorkspaceID: "foreign-anchor-id"}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" || !strings.Contains(message, "reused anchor") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"created-group-id"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if len(client.closed) != 0 {
		t.Fatalf("foreign anchor was closed: %v", client.closed)
	}
}

func TestWorkspaceGroupCreateMissingAnchorUngroupsCreatedGroup(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{})
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{ID: "created-group-id"}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" || !strings.Contains(message, "omitted group or anchor") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"created-group-id"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if len(client.closed) != 0 {
		t.Fatalf("closed workspaces = %v", client.closed)
	}
}

func TestWorkspaceGroupCreateRefOnlyResponseClosesProvisionalAnchor(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{})
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		Ref:               "workspace_group:1",
		AnchorWorkspaceID: "provisional-anchor-id",
	}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" || !strings.Contains(message, "stable id") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.closed, []string{"provisional-anchor-id"}) {
		t.Fatalf("closed workspaces = %v", client.closed)
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"workspace_group:1"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if saved := store.snapshot(); saved.RootGroup != nil {
		t.Fatalf("ref-only group response was persisted: %+v", saved.RootGroup)
	}
}

func TestWorkspaceGroupInvalidCreateLeavesAnchorWhenUngroupFails(t *testing.T) {
	root := t.TempDir()
	store := newGroupTestStore(cmuxmap.File{})
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id"}}
	client.createGroupResult = cmuxctl.WorkspaceGroup{
		Ref:               "workspace_group:1",
		AnchorWorkspaceID: "provisional-anchor-id",
	}
	client.ungroupErr = errors.New("ungroup unavailable")

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: rootMappingWorkspaceID, WorkspacePath: root, Title: rootMappingWorkspaceID},
		false,
	)
	if code != "cmux_group_create_failed" {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	for _, want := range []string{
		"cleanup failed for cmux group workspace_group:1",
		"ungroup unavailable",
		"ungroup it manually",
		"provisional anchor provisional-anchor-id was left open",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
	if !reflect.DeepEqual(client.ungroupCalls, []string{"workspace_group:1"}) {
		t.Fatalf("ungroup calls = %v", client.ungroupCalls)
	}
	if len(client.closed) != 0 {
		t.Fatalf("anchor was closed after failed ungroup: %v", client.closed)
	}
}

func TestWorkspaceGroupChildRefOnlyResponseIsClosedAndNotPersisted(t *testing.T) {
	root := t.TempDir()
	groupMapping := cmuxmap.GroupMapping{
		CMUXWindowID: "window-id", CMUXGroupID: "managed-group-id", AnchorWorkspaceID: "anchor-id", NameSnapshot: "root",
	}
	store := newGroupTestStore(cmuxmap.File{
		RootGroup: &groupMapping,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			rootMappingWorkspaceID: testWorkspaceMapping("anchor-id", "root"),
		},
	})
	managed := cmuxctl.WorkspaceGroup{
		ID: "managed-group-id", Name: "root", WindowID: "window-id", AnchorWorkspaceID: "anchor-id", MemberWorkspaceIDs: []string{"anchor-id"},
	}
	client := newGroupTestClient()
	client.groupLists = []cmuxctl.WorkspaceGroupList{{WindowID: "window-id", Groups: []cmuxctl.WorkspaceGroup{managed}}}
	client.createWorkspaceResults = []cmuxctl.WorkspaceCreateResult{
		{WorkspaceRef: "workspace:2", WindowID: "window-id", GroupID: "managed-group-id"},
	}

	_, code, message := newWorkspaceGroupTestService(client, store).EnsureWorkspace(
		context.Background(), root,
		OpenTarget{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "workspaces", "WS1"), Title: "Build"},
		false,
	)
	if code != "cmux_create_failed" || !strings.Contains(message, "stable workspace id") {
		t.Fatalf("EnsureWorkspace() code=%q message=%q", code, message)
	}
	if !reflect.DeepEqual(client.closed, []string{"workspace:2"}) {
		t.Fatalf("closed workspaces = %v", client.closed)
	}
	if _, exists := store.snapshot().Workspaces["WS1"]; exists {
		t.Fatal("ref-only workspace response was persisted")
	}
}

func testIdentifyCallerPayload(workspaceID string, windowID string) map[string]any {
	return map[string]any{
		"caller": map[string]any{
			"workspace_id": workspaceID,
			"window_id":    windowID,
		},
	}
}

func testWorkspaceMapping(workspaceID string, title string) cmuxmap.WorkspaceMapping {
	return cmuxmap.WorkspaceMapping{
		NextOrdinal: 2,
		Entries: []cmuxmap.Entry{{
			CMUXWorkspaceID: workspaceID,
			Ordinal:         1,
			TitleSnapshot:   title,
			CreatedAt:       "2026-08-12T00:00:00Z",
			LastUsedAt:      "2026-08-12T00:00:00Z",
		}},
	}
}

func assertSingleGroupTestMapping(t *testing.T, mapping cmuxmap.File, workspaceID string, wantCMUXID string) {
	t.Helper()
	workspace, ok := mapping.Workspaces[workspaceID]
	if !ok || len(workspace.Entries) != 1 || workspace.Entries[0].CMUXWorkspaceID != wantCMUXID {
		t.Fatalf("mapping[%s] = %+v, want cmux %s", workspaceID, workspace, wantCMUXID)
	}
}

func assertNoGroupTestMutation(t *testing.T, client *groupTestClient) {
	t.Helper()
	if len(client.createGroupCalls) != 0 || len(client.createWorkspaceCalls) != 0 || len(client.addCalls) != 0 || len(client.removeCalls) != 0 || len(client.setAnchorCalls) != 0 || len(client.ungroupCalls) != 0 || len(client.focusCalls) != 0 || len(client.closed) != 0 {
		t.Fatalf("unexpected runtime mutation: createGroup=%d createWorkspace=%d add=%+v remove=%+v anchor=%+v ungroup=%v focus=%v close=%v",
			len(client.createGroupCalls), len(client.createWorkspaceCalls), client.addCalls, client.removeCalls, client.setAnchorCalls, client.ungroupCalls, client.focusCalls, client.closed)
	}
}
