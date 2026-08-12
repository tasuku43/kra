package cmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeClient struct {
	capabilities   cmuxctl.Capabilities
	createID       string
	createIDs      []string
	createIndex    int
	createErr      error
	createCmds     []string
	closeErr       error
	closeErrBy     map[string]error
	closed         []string
	closeCtxErr    error
	renameErr      error
	renameErrBy    map[string]error
	renameHook     func()
	selectErr      error
	sendErr        error
	sendErrBy      map[string]error
	listPanesErr   error
	identifyErr    map[string]error
	identifyBy     map[string]map[string]any
	listWorkspaces []cmuxctl.Workspace
	listErr        error

	renameWorkspace string
	renameTitle     string
	selectWorkspace string
	notify          cmuxctl.NotifyOptions
	sentWorkspace   string
	sentSurface     string
	sentText        string
	calls           []string
}

func (f *fakeClient) Capabilities(context.Context) (cmuxctl.Capabilities, error) {
	return f.capabilities, nil
}

func (f *fakeClient) CreateWorkspaceWithCommand(_ context.Context, command string) (string, error) {
	f.createCmds = append(f.createCmds, command)
	if f.createErr != nil {
		return "", f.createErr
	}
	if f.createIndex < len(f.createIDs) {
		id := f.createIDs[f.createIndex]
		f.createIndex++
		return id, nil
	}
	return f.createID, nil
}

func (f *fakeClient) CloseWorkspace(ctx context.Context, workspace string) error {
	f.closed = append(f.closed, workspace)
	f.closeCtxErr = ctx.Err()
	if f.closeErrBy != nil {
		if err, ok := f.closeErrBy[workspace]; ok {
			return err
		}
	}
	return f.closeErr
}

func (f *fakeClient) RenameWorkspace(_ context.Context, workspace string, title string) error {
	f.renameWorkspace = workspace
	f.renameTitle = title
	if f.renameHook != nil {
		f.renameHook()
	}
	if f.renameErrBy != nil {
		if err, ok := f.renameErrBy[workspace]; ok {
			return err
		}
	}
	return f.renameErr
}

func (f *fakeClient) SelectWorkspace(_ context.Context, workspace string) error {
	f.calls = append(f.calls, "select:"+workspace)
	f.selectWorkspace = workspace
	return f.selectErr
}

func (f *fakeClient) Notify(_ context.Context, opts cmuxctl.NotifyOptions) error {
	f.calls = append(f.calls, "notify:"+opts.Workspace)
	f.notify = opts
	return nil
}

func (f *fakeClient) SendText(_ context.Context, workspace string, surface string, text string) error {
	f.calls = append(f.calls, "send:"+workspace)
	f.sentWorkspace = workspace
	f.sentSurface = surface
	f.sentText = text
	if f.sendErrBy != nil {
		if err, ok := f.sendErrBy[workspace]; ok {
			return err
		}
	}
	return f.sendErr
}

func (f *fakeClient) ListPanes(_ context.Context, workspace string) ([]cmuxctl.Pane, error) {
	if f.listPanesErr != nil {
		return nil, f.listPanesErr
	}
	return []cmuxctl.Pane{{
		ID:                "pane-" + workspace,
		Ref:               "pane:" + workspace,
		Focused:           true,
		SelectedSurfaceID: "surface-id-" + workspace,
	}}, nil
}

type fakeMappingStore struct {
	file      cmuxmap.File
	lockErr   error
	loadErr   error
	saveErr   error
	saveCalls int
}

func (s *fakeMappingStore) AcquireLock(context.Context) (func(), error) {
	if s.lockErr != nil {
		return nil, s.lockErr
	}
	return func() {}, nil
}

func (s *fakeMappingStore) Load() (cmuxmap.File, error) {
	if s.loadErr != nil {
		return cmuxmap.File{}, s.loadErr
	}
	return cloneMappingFile(s.file), nil
}

func (s *fakeMappingStore) Save(file cmuxmap.File) error {
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.file = cloneMappingFile(file)
	return nil
}

func cloneMappingFile(in cmuxmap.File) cmuxmap.File {
	out := cmuxmap.File{Version: in.Version, Workspaces: make(map[string]cmuxmap.WorkspaceMapping, len(in.Workspaces))}
	for id, mapping := range in.Workspaces {
		out.Workspaces[id] = cloneWorkspaceMapping(mapping)
	}
	return out
}

func (f *fakeClient) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	return f.listWorkspaces, f.listErr
}

func (f *fakeClient) Identify(_ context.Context, workspace string, _ string) (map[string]any, error) {
	if f.identifyErr != nil {
		if err, ok := f.identifyErr[workspace]; ok {
			return nil, err
		}
	}
	if f.identifyBy != nil {
		if payload, ok := f.identifyBy[workspace]; ok {
			return payload, nil
		}
	}
	return map[string]any{
		"workspace_id": workspace,
		"focused": map[string]any{
			"surface_ref": "surface:" + workspace,
		},
	}, nil
}

func TestShellQuoteCDPath_UsesHomeVariableWhenUnderHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		t.Skip("home dir is not available")
	}
	got := shellQuoteCDPath(filepath.Join(home, "work/root/workspaces/TEST-4084"))
	want := `"$HOME/work/root/workspaces/TEST-4084"`
	if got != want {
		t.Fatalf("shellQuoteCDPath() = %q, want %q", got, want)
	}
}

func TestEnsureWorkspace_NoSelect_CreatesAndPersistsMapping(t *testing.T) {
	root := t.TempDir()
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
			},
		},
		createID: "CMUX-ROOT-1",
	}

	svc := NewService(
		func() Client { return fake },
		func(root string) MappingStore { return cmuxmap.NewStore(root) },
	)
	svc.Now = func() time.Time { return time.Unix(1700000000, 0) }

	item, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
	}, false)
	if code != "" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if item.CMUXWorkspaceID != "CMUX-ROOT-1" || item.ReusedExisting {
		t.Fatalf("unexpected item: %+v", item)
	}
	if fake.selectWorkspace != "" {
		t.Fatalf("select should be skipped, got %q", fake.selectWorkspace)
	}
	if fake.notify.Workspace != "" {
		t.Fatalf("notify should be skipped without select: %+v", fake.notify)
	}

	mapping, err := cmuxmap.NewStore(root).Load()
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	ws := mapping.Workspaces["KRA_ROOT"]
	if len(ws.Entries) != 1 || ws.Entries[0].CMUXWorkspaceID != "CMUX-ROOT-1" {
		t.Fatalf("mapping = %+v", ws)
	}
}

func TestEnsureWorkspace_NoSelect_ReusesExistingMappingWithoutSelecting(t *testing.T) {
	root := t.TempDir()
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"KRA_ROOT": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-ROOT-1", Ordinal: 1, TitleSnapshot: "KRA_ROOT"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
			},
		},
	}

	svc := NewService(
		func() Client { return fake },
		func(root string) MappingStore { return cmuxmap.NewStore(root) },
	)

	item, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
	}, false)
	if code != "" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if item.CMUXWorkspaceID != "CMUX-ROOT-1" || !item.ReusedExisting {
		t.Fatalf("unexpected item: %+v", item)
	}
	if fake.selectWorkspace != "" {
		t.Fatalf("select should be skipped, got %q", fake.selectWorkspace)
	}
	if len(fake.createCmds) != 0 {
		t.Fatalf("create should not run: %+v", fake.createCmds)
	}
	if fake.notify.Workspace != "" {
		t.Fatalf("notify should be skipped without select: %+v", fake.notify)
	}
}

func TestEnsureWorkspace_Select_RecreatesWhenIdentifyDoesNotResolveRequestedWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"KRA_ROOT": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "workspace:12", Ordinal: 1, TitleSnapshot: "KRA_ROOT"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createID: "workspace:22",
		identifyBy: map[string]map[string]any{
			"workspace:12": {
				"focused": map[string]any{
					"workspace_ref": "workspace:5",
				},
				"caller": nil,
			},
		},
	}

	svc := NewService(
		func() Client { return fake },
		func(root string) MappingStore { return cmuxmap.NewStore(root) },
	)

	item, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
	}, true)
	if code != "" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if item.CMUXWorkspaceID != "workspace:22" || item.ReusedExisting {
		t.Fatalf("unexpected item: %+v", item)
	}
	if len(fake.createCmds) != 1 {
		t.Fatalf("create should run once to replace stale mapping: %+v", fake.createCmds)
	}
	if fake.selectWorkspace != "workspace:22" {
		t.Fatalf("select workspace = %q, want %q", fake.selectWorkspace, "workspace:22")
	}
	if fake.notify.Workspace != "" {
		t.Fatalf("notify should be skipped for EnsureWorkspace select path: %+v", fake.notify)
	}
	if strings.Join(fake.calls, ",") != "select:workspace:22" {
		t.Fatalf("call order = %v, want select only", fake.calls)
	}

	mapping, err := cmuxmap.NewStore(root).Load()
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	ws := mapping.Workspaces["KRA_ROOT"]
	if len(ws.Entries) != 1 || ws.Entries[0].CMUXWorkspaceID != "workspace:22" {
		t.Fatalf("mapping = %+v", ws)
	}
	if ws.NextOrdinal != 2 {
		t.Fatalf("next_ordinal = %d, want 2", ws.NextOrdinal)
	}
}

func TestEnsureWorkspace_Select_RelinksByRuntimeTitleWhenMappedIDIsStale(t *testing.T) {
	root := t.TempDir()
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"KRA_ROOT": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "workspace:12", Ordinal: 1, TitleSnapshot: "KRA_ROOT | KRA_ROOT"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		identifyErr: map[string]error{
			"workspace:12": errors.New("cmux identify: Error: not_found: Workspace not found"),
		},
		listWorkspaces: []cmuxctl.Workspace{
			{ID: "workspace:99", Ref: "workspace:99", Title: "KRA_ROOT | KRA_ROOT"},
		},
	}

	svc := NewService(
		func() Client { return fake },
		func(root string) MappingStore { return cmuxmap.NewStore(root) },
	)

	item, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
	}, true)
	if code != "" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if item.CMUXWorkspaceID != "workspace:99" || !item.ReusedExisting {
		t.Fatalf("unexpected item: %+v", item)
	}
	if len(fake.createCmds) != 0 {
		t.Fatalf("create should not run when title relink succeeds: %+v", fake.createCmds)
	}
	if fake.selectWorkspace != "workspace:99" {
		t.Fatalf("select workspace = %q, want %q", fake.selectWorkspace, "workspace:99")
	}

	mapping, err := cmuxmap.NewStore(root).Load()
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	ws := mapping.Workspaces["KRA_ROOT"]
	if len(ws.Entries) != 1 || ws.Entries[0].CMUXWorkspaceID != "workspace:99" {
		t.Fatalf("mapping = %+v", ws)
	}
}

func TestReconcileMappingWithRuntime_TreatsWorkspaceRefAsExisting(t *testing.T) {
	store := cmuxmap.NewStore(t.TempDir())
	mapping := cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "workspace:5", Ordinal: 1, TitleSnapshot: "WS1"},
				},
			},
		},
	}

	reconciled, exists, pruned, err := ReconcileMappingWithRuntime(store, mapping, []cmuxctl.Workspace{
		{ID: "28D4A739-A2FD-4C56-9AC6-D1C9DD05035B", Ref: "workspace:5", Title: "WS1"},
	}, true)
	if err != nil {
		t.Fatalf("ReconcileMappingWithRuntime() error: %v", err)
	}
	if pruned != 0 {
		t.Fatalf("pruned = %d, want 0", pruned)
	}
	if !exists["workspace:5"] {
		t.Fatalf("exists should include workspace ref handle: %+v", exists)
	}
	ws := reconciled.Workspaces["WS1"]
	if len(ws.Entries) != 1 || ws.Entries[0].CMUXWorkspaceID != "workspace:5" {
		t.Fatalf("reconciled mapping = %+v", ws)
	}
}

func TestOpen_NewWorkspaceMandatoryFailureClosesAndLeavesNoMapping(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantCode  string
		configure func(*fakeClient)
	}{
		{
			name:     "rename",
			wantCode: "cmux_rename_failed",
			configure: func(fake *fakeClient) {
				fake.renameErr = errors.New("rename failed")
			},
		},
		{
			name:     "command surface",
			command:  "make test",
			wantCode: "cmux_list_panes_failed",
			configure: func(fake *fakeClient) {
				fake.listPanesErr = errors.New("pane lookup failed")
			},
		},
		{
			name:     "command send",
			command:  "make test",
			wantCode: "cmux_send_command_failed",
			configure: func(fake *fakeClient) {
				fake.sendErr = errors.New("send failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			fake := &fakeClient{
				capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
					"workspace.create": {},
					"workspace.rename": {},
				}},
				createID: "CMUX-NEW",
			}
			tt.configure(fake)
			svc := NewService(
				func() Client { return fake },
				func(string) MappingStore { return cmuxmap.NewStore(root) },
			)

			result, code, msg := svc.Open(context.Background(), root, []OpenTarget{{
				WorkspaceID:   "WS1",
				WorkspacePath: filepath.Join(root, "workspaces", "WS1"),
				Title:         "Example",
			}}, 1, false, tt.command)
			if code != "" {
				t.Fatalf("Open() code=%q msg=%q", code, msg)
			}
			if len(result.Failures) != 1 || result.Failures[0].Code != tt.wantCode {
				t.Fatalf("failures = %+v, want code %q", result.Failures, tt.wantCode)
			}
			if got := strings.Join(fake.closed, ","); got != "CMUX-NEW" {
				t.Fatalf("closed = %v, want [CMUX-NEW]", fake.closed)
			}
			mapping, err := cmuxmap.NewStore(root).Load()
			if err != nil {
				t.Fatalf("load mapping: %v", err)
			}
			if _, ok := mapping.Workspaces["WS1"]; ok {
				t.Fatalf("failed target mapping persisted: %+v", mapping.Workspaces["WS1"])
			}
		})
	}
}

func TestOpen_CleanupFailureKeepsPrimaryCodeAndReportsWorkspaceID(t *testing.T) {
	root := t.TempDir()
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID:  "CMUX-ORPHAN",
		renameErr: errors.New("rename failed"),
		closeErr:  errors.New("close failed"),
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return cmuxmap.NewStore(root) },
	)

	result, code, msg := svc.Open(context.Background(), root, []OpenTarget{{
		WorkspaceID:   "WS1",
		WorkspacePath: root,
		Title:         "Example",
	}}, 1, false, "")
	if code != "" {
		t.Fatalf("Open() code=%q msg=%q", code, msg)
	}
	if len(result.Failures) != 1 || result.Failures[0].Code != "cmux_rename_failed" {
		t.Fatalf("failures = %+v", result.Failures)
	}
	failureMessage := result.Failures[0].Message
	for _, want := range []string{"rename failed", "CMUX-ORPHAN", "close failed", "close it manually"} {
		if !strings.Contains(failureMessage, want) {
			t.Fatalf("failure message %q does not contain %q", failureMessage, want)
		}
	}
}

func TestEnsureWorkspace_SelectFailureClosesNewWorkspace(t *testing.T) {
	root := t.TempDir()
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
			"workspace.select": {},
		}},
		createID:  "CMUX-NEW",
		selectErr: errors.New("select failed"),
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return cmuxmap.NewStore(root) },
	)

	_, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
	}, true)
	if code != "cmux_select_failed" || !strings.Contains(msg, "select failed") {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if got := strings.Join(fake.closed, ","); got != "CMUX-NEW" {
		t.Fatalf("closed = %v, want [CMUX-NEW]", fake.closed)
	}
}

func TestEnsureWorkspace_MappingSaveFailureClosesNewWorkspace(t *testing.T) {
	store := &fakeMappingStore{
		file: cmuxmap.File{
			Version:    cmuxmap.CurrentVersion,
			Workspaces: map[string]cmuxmap.WorkspaceMapping{},
		},
		saveErr: errors.New("disk full"),
	}
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID: "CMUX-NEW",
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return store },
	)

	_, code, msg := svc.EnsureWorkspace(context.Background(), "/root", OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: "/root",
		Title:         "KRA_ROOT",
	}, false)
	if code != "state_write_failed" || !strings.Contains(msg, "disk full") {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if got := strings.Join(fake.closed, ","); got != "CMUX-NEW" {
		t.Fatalf("closed = %v, want [CMUX-NEW]", fake.closed)
	}
}

func TestOpen_CleanupNotFoundIsSuccessfulCompensation(t *testing.T) {
	root := t.TempDir()
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID:  "CMUX-GONE",
		renameErr: errors.New("rename failed"),
		closeErr:  errors.New("not_found: workspace not found"),
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return cmuxmap.NewStore(root) },
	)

	result, code, msg := svc.Open(context.Background(), root, []OpenTarget{{
		WorkspaceID:   "WS1",
		WorkspacePath: root,
		Title:         "Example",
	}}, 1, false, "")
	if code != "" {
		t.Fatalf("Open() code=%q msg=%q", code, msg)
	}
	if len(result.Failures) != 1 || result.Failures[0].Code != "cmux_rename_failed" {
		t.Fatalf("failures = %+v", result.Failures)
	}
	if strings.Contains(result.Failures[0].Message, "cleanup failed") {
		t.Fatalf("not-found cleanup was reported as failure: %q", result.Failures[0].Message)
	}
}

func TestOpen_PartialFailurePersistsOnlySuccessfulTarget(t *testing.T) {
	root := t.TempDir()
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createIDs: []string{"CMUX-OK", "CMUX-FAILED"},
		renameErrBy: map[string]error{
			"CMUX-FAILED": errors.New("rename failed"),
		},
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return cmuxmap.NewStore(root) },
	)

	result, code, msg := svc.Open(context.Background(), root, []OpenTarget{
		{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "WS1"), Title: "One"},
		{WorkspaceID: "WS2", WorkspacePath: filepath.Join(root, "WS2"), Title: "Two"},
	}, 1, true, "")
	if code != "" {
		t.Fatalf("Open() code=%q msg=%q", code, msg)
	}
	if len(result.Results) != 1 || result.Results[0].CMUXWorkspaceID != "CMUX-OK" || len(result.Failures) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got := strings.Join(fake.closed, ","); got != "CMUX-FAILED" {
		t.Fatalf("closed = %v, want [CMUX-FAILED]", fake.closed)
	}
	mapping, err := cmuxmap.NewStore(root).Load()
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	if got := mapping.Workspaces["WS1"].Entries; len(got) != 1 || got[0].CMUXWorkspaceID != "CMUX-OK" {
		t.Fatalf("WS1 mapping = %+v", mapping.Workspaces["WS1"])
	}
	if _, ok := mapping.Workspaces["WS2"]; ok {
		t.Fatalf("failed WS2 mapping persisted: %+v", mapping.Workspaces["WS2"])
	}
}

func TestOpen_ReusedFailureDoesNotCloseAndRestoresMapping(t *testing.T) {
	root := t.TempDir()
	store := cmuxmap.NewStore(root)
	const oldLastUsed = "2026-01-02T03:04:05Z"
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS2": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{{
					CMUXWorkspaceID: "CMUX-EXISTING",
					Ordinal:         1,
					TitleSnapshot:   "WS2 | Existing",
					LastUsedAt:      oldLastUsed,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID: "CMUX-NEW",
		sendErrBy: map[string]error{
			"CMUX-EXISTING": errors.New("send failed"),
		},
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return store },
	)

	result, code, msg := svc.Open(context.Background(), root, []OpenTarget{
		{WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "WS1"), Title: "New"},
		{WorkspaceID: "WS2", WorkspacePath: filepath.Join(root, "WS2"), Title: "Existing"},
	}, 1, true, "make test")
	if code != "" {
		t.Fatalf("Open() code=%q msg=%q", code, msg)
	}
	if len(result.Results) != 1 || len(result.Failures) != 1 || result.Failures[0].Code != "cmux_send_command_failed" {
		t.Fatalf("result = %+v", result)
	}
	if len(fake.closed) != 0 {
		t.Fatalf("reused failure closed workspaces: %v", fake.closed)
	}
	mapping, err := store.Load()
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	if got := mapping.Workspaces["WS2"].Entries[0].LastUsedAt; got != oldLastUsed {
		t.Fatalf("WS2 last_used_at = %q, want %q", got, oldLastUsed)
	}
}

func TestOpen_MappingSaveFailureClosesOnlyNewResults(t *testing.T) {
	store := &fakeMappingStore{
		file: cmuxmap.File{
			Version: cmuxmap.CurrentVersion,
			Workspaces: map[string]cmuxmap.WorkspaceMapping{
				"WS1": {
					NextOrdinal: 2,
					Entries: []cmuxmap.Entry{{
						CMUXWorkspaceID: "CMUX-EXISTING",
						Ordinal:         1,
						TitleSnapshot:   "WS1 | Existing",
					}},
				},
			},
		},
		saveErr: errors.New("disk full"),
	}
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID: "CMUX-NEW",
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return store },
	)

	_, code, msg := svc.Open(context.Background(), "/root", []OpenTarget{
		{WorkspaceID: "WS1", WorkspacePath: "/root/WS1", Title: "Existing"},
		{WorkspaceID: "WS2", WorkspacePath: "/root/WS2", Title: "New"},
	}, 1, true, "")
	if code != "state_write_failed" || !strings.Contains(msg, "disk full") {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if got := strings.Join(fake.closed, ","); got != "CMUX-NEW" {
		t.Fatalf("closed = %v, want only [CMUX-NEW]", fake.closed)
	}
}

func TestOpen_MappingSaveFailurePreservesEveryCleanupIncident(t *testing.T) {
	store := &fakeMappingStore{
		file:    cmuxmap.File{Version: cmuxmap.CurrentVersion, Workspaces: map[string]cmuxmap.WorkspaceMapping{}},
		saveErr: errors.New("disk full"),
	}
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createIDs: []string{"CMUX-B", "CMUX-A"},
		renameErrBy: map[string]error{
			"CMUX-A": errors.New("rename A failed"),
		},
		closeErrBy: map[string]error{
			"CMUX-A": errors.New("close A failed"),
			"CMUX-B": errors.New("close B failed"),
		},
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return store },
	)

	_, code, msg := svc.Open(context.Background(), "/root", []OpenTarget{
		{WorkspaceID: "WS-B", WorkspacePath: "/root/WS-B", Title: "B"},
		{WorkspaceID: "WS-A", WorkspacePath: "/root/WS-A", Title: "A"},
	}, 1, true, "")
	if code != "state_write_failed" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	for _, want := range []string{"disk full", "CMUX-A", "close A failed", "CMUX-B", "close B failed", "close it manually"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q does not contain %q", msg, want)
		}
	}
}

func TestOpen_LegacyMappingReusesFirstReachableEntry(t *testing.T) {
	root := t.TempDir()
	store := cmuxmap.NewStore(root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 3,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-STALE", Ordinal: 1, TitleSnapshot: "WS1 | Old"},
					{CMUXWorkspaceID: "CMUX-LIVE", Ordinal: 2, TitleSnapshot: "WS1 | Live"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID: "CMUX-NEW",
		identifyErr: map[string]error{
			"CMUX-STALE": errors.New("not_found: workspace not found"),
		},
	}
	svc := NewService(func() Client { return fake }, func(string) MappingStore { return store })

	result, code, msg := svc.Open(context.Background(), root, []OpenTarget{{
		WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "WS1"), Title: "Live",
	}}, 1, false, "")
	if code != "" || len(result.Results) != 1 {
		t.Fatalf("code=%q msg=%q result=%+v", code, msg, result)
	}
	if got := result.Results[0]; !got.ReusedExisting || got.CMUXWorkspaceID != "CMUX-LIVE" {
		t.Fatalf("result = %+v, want reused CMUX-LIVE", got)
	}
	if len(fake.createCmds) != 0 {
		t.Fatalf("create calls = %v, want none", fake.createCmds)
	}
	mapping, err := store.Load()
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	if got := mapping.Workspaces["WS1"].Entries; len(got) != 1 || got[0].CMUXWorkspaceID != "CMUX-LIVE" {
		t.Fatalf("collapsed mapping = %+v", got)
	}
}

func TestOpen_LegacyMappingRelinksByTitleSnapshot(t *testing.T) {
	root := t.TempDir()
	store := cmuxmap.NewStore(root)
	const legacyTitle = "WS1 | Old title [1]"
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {NextOrdinal: 2, Entries: []cmuxmap.Entry{{
				CMUXWorkspaceID: "CMUX-STALE", Ordinal: 1, TitleSnapshot: legacyTitle,
			}}},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID: "CMUX-NEW",
		identifyErr: map[string]error{
			"CMUX-STALE": errors.New("not_found: workspace not found"),
		},
		listWorkspaces: []cmuxctl.Workspace{{ID: "CMUX-RELINKED", Title: legacyTitle}},
	}
	svc := NewService(func() Client { return fake }, func(string) MappingStore { return store })

	result, code, msg := svc.Open(context.Background(), root, []OpenTarget{{
		WorkspaceID: "WS1", WorkspacePath: filepath.Join(root, "WS1"), Title: "New title",
	}}, 1, false, "")
	if code != "" || len(result.Results) != 1 {
		t.Fatalf("code=%q msg=%q result=%+v", code, msg, result)
	}
	if got := result.Results[0]; !got.ReusedExisting || got.CMUXWorkspaceID != "CMUX-RELINKED" {
		t.Fatalf("result = %+v, want relinked runtime", got)
	}
	if len(fake.createCmds) != 0 {
		t.Fatalf("create calls = %v, want none", fake.createCmds)
	}
}

type lockingFakeClient struct {
	mu          sync.Mutex
	createCount int
	alive       map[string]bool
}

func newLockingFakeClient() *lockingFakeClient {
	return &lockingFakeClient{alive: map[string]bool{}}
}

func (f *lockingFakeClient) Capabilities(context.Context) (cmuxctl.Capabilities, error) {
	return cmuxctl.Capabilities{Methods: map[string]struct{}{
		"workspace.create": {},
		"workspace.rename": {},
	}}, nil
}

func (f *lockingFakeClient) CreateWorkspaceWithCommand(context.Context, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCount++
	id := fmt.Sprintf("CMUX-%d", f.createCount)
	f.alive[id] = true
	return id, nil
}

func (f *lockingFakeClient) CloseWorkspace(_ context.Context, workspace string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.alive, workspace)
	return nil
}

func (*lockingFakeClient) RenameWorkspace(context.Context, string, string) error { return nil }
func (*lockingFakeClient) SelectWorkspace(context.Context, string) error         { return nil }
func (*lockingFakeClient) Notify(context.Context, cmuxctl.NotifyOptions) error   { return nil }
func (*lockingFakeClient) SendText(context.Context, string, string, string) error {
	return nil
}
func (*lockingFakeClient) ListPanes(context.Context, string) ([]cmuxctl.Pane, error) {
	return []cmuxctl.Pane{{SelectedSurfaceID: "SURFACE"}}, nil
}
func (*lockingFakeClient) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	return nil, nil
}
func (f *lockingFakeClient) Identify(_ context.Context, workspace string, _ string) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.alive[workspace] {
		return nil, errors.New("not_found: workspace not found")
	}
	return map[string]any{"workspace_id": workspace}, nil
}

func TestOpen_ConcurrentTransactionsPreserveMappingAndPreventDuplicateCreate(t *testing.T) {
	tests := []struct {
		name            string
		targets         []OpenTarget
		wantWorkspaces  int
		wantCreateCount int
	}{
		{
			name: "different workspaces",
			targets: []OpenTarget{
				{WorkspaceID: "WS1", WorkspacePath: "/root/WS1", Title: "One"},
				{WorkspaceID: "WS2", WorkspacePath: "/root/WS2", Title: "Two"},
			},
			wantWorkspaces: 2, wantCreateCount: 2,
		},
		{
			name: "same workspace",
			targets: []OpenTarget{
				{WorkspaceID: "WS1", WorkspacePath: "/root/WS1", Title: "One"},
				{WorkspaceID: "WS1", WorkspacePath: "/root/WS1", Title: "One"},
			},
			wantWorkspaces: 1, wantCreateCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			client := newLockingFakeClient()
			svc := NewService(func() Client { return client }, func(string) MappingStore { return cmuxmap.NewStore(root) })
			start := make(chan struct{})
			results := make(chan string, len(tt.targets))
			var wg sync.WaitGroup
			for _, target := range tt.targets {
				target := target
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					_, code, msg := svc.Open(context.Background(), root, []OpenTarget{target}, 1, false, "")
					results <- code + ":" + msg
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			for result := range results {
				if result != ":" {
					t.Fatalf("Open() result = %q", result)
				}
			}
			mapping, err := cmuxmap.NewStore(root).Load()
			if err != nil {
				t.Fatalf("load mapping: %v", err)
			}
			if len(mapping.Workspaces) != tt.wantWorkspaces {
				t.Fatalf("mapping workspaces = %+v, want %d", mapping.Workspaces, tt.wantWorkspaces)
			}
			client.mu.Lock()
			createCount := client.createCount
			client.mu.Unlock()
			if createCount != tt.wantCreateCount {
				t.Fatalf("create count = %d, want %d", createCount, tt.wantCreateCount)
			}
		})
	}
}

func TestOpen_CompensationDetachesFromCanceledContext(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID:   "CMUX-NEW",
		renameHook: cancel,
		renameErr:  errors.New("rename failed"),
	}
	svc := NewService(
		func() Client { return fake },
		func(string) MappingStore { return cmuxmap.NewStore(root) },
	)
	result, code, msg := svc.Open(ctx, root, []OpenTarget{{
		WorkspaceID:   "WS1",
		WorkspacePath: root,
		Title:         "Example",
	}}, 1, false, "")
	if code != "" || len(result.Failures) != 1 {
		t.Fatalf("code=%q msg=%q result=%+v", code, msg, result)
	}
	if fake.closeCtxErr != nil {
		t.Fatalf("cleanup context inherited cancellation: %v", fake.closeCtxErr)
	}
}
