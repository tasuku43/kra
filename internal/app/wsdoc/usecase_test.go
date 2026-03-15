package wsdoc

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxdocs"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeClient struct {
	capabilities        cmuxctl.Capabilities
	identifyErr         error
	workspaces          []cmuxctl.Workspace
	listWorkspacesErr   error
	createWorkspaceID   string
	createWorkspaceErr  error
	renameWorkspaceErr  error
	listPanes           map[string][]cmuxctl.Pane
	listPanesErr        map[string]error
	listPaneSurfaces    map[string][]cmuxctl.Surface
	listPaneSurfaceErrs map[string]error
	createPaneResult    cmuxctl.PaneCreateResult
	createPaneErr       error
	createSplitResult   cmuxctl.PaneCreateResult
	createSplitErr      error
	markdownResult      cmuxctl.MarkdownOpenResult
	markdownErr         error
	moveResult          cmuxctl.MoveSurfaceResult
	moveErr             error

	renameWorkspaceCalls []struct {
		Workspace string
		Title     string
	}
	renameCalls []struct {
		Workspace string
		Surface   string
		Title     string
	}
	moveCalls []struct {
		Surface   string
		Pane      string
		Workspace string
		After     string
		Focus     bool
	}
	closeCalls []struct {
		Workspace string
		Surface   string
	}
}

func (f *fakeClient) Capabilities(context.Context) (cmuxctl.Capabilities, error) {
	return f.capabilities, nil
}

func (f *fakeClient) Identify(context.Context, string, string) (map[string]any, error) {
	if f.identifyErr != nil {
		return nil, f.identifyErr
	}
	return map[string]any{"ok": true}, nil
}

func (f *fakeClient) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	if f.listWorkspacesErr != nil {
		return nil, f.listWorkspacesErr
	}
	return append([]cmuxctl.Workspace{}, f.workspaces...), nil
}

func (f *fakeClient) CreateWorkspace(context.Context) (string, error) {
	return f.createWorkspaceID, f.createWorkspaceErr
}

func (f *fakeClient) RenameWorkspace(_ context.Context, workspace string, title string) error {
	f.renameWorkspaceCalls = append(f.renameWorkspaceCalls, struct {
		Workspace string
		Title     string
	}{Workspace: workspace, Title: title})
	return f.renameWorkspaceErr
}

func (f *fakeClient) ListPanes(_ context.Context, workspace string) ([]cmuxctl.Pane, error) {
	if err := f.listPanesErr[workspace]; err != nil {
		return nil, err
	}
	return append([]cmuxctl.Pane{}, f.listPanes[workspace]...), nil
}

func (f *fakeClient) ListPaneSurfaces(_ context.Context, _ string, pane string) ([]cmuxctl.Surface, error) {
	if err := f.listPaneSurfaceErrs[pane]; err != nil {
		return nil, err
	}
	return append([]cmuxctl.Surface{}, f.listPaneSurfaces[pane]...), nil
}

func (f *fakeClient) CreatePane(context.Context, string, string) (cmuxctl.PaneCreateResult, error) {
	return f.createPaneResult, f.createPaneErr
}

func (f *fakeClient) CreateSplit(context.Context, string, string, string) (cmuxctl.PaneCreateResult, error) {
	return f.createSplitResult, f.createSplitErr
}

func (f *fakeClient) MarkdownOpen(context.Context, string, string, string) (cmuxctl.MarkdownOpenResult, error) {
	return f.markdownResult, f.markdownErr
}

func (f *fakeClient) MoveSurface(_ context.Context, surface string, pane string, workspace string, after string, focus bool) (cmuxctl.MoveSurfaceResult, error) {
	f.moveCalls = append(f.moveCalls, struct {
		Surface   string
		Pane      string
		Workspace string
		After     string
		Focus     bool
	}{Surface: surface, Pane: pane, Workspace: workspace, After: after, Focus: focus})
	return f.moveResult, f.moveErr
}

func (f *fakeClient) RenameTab(_ context.Context, workspace string, surface string, title string) error {
	f.renameCalls = append(f.renameCalls, struct {
		Workspace string
		Surface   string
		Title     string
	}{Workspace: workspace, Surface: surface, Title: title})
	return nil
}

func (f *fakeClient) CloseSurface(_ context.Context, workspace string, surface string) error {
	f.closeCalls = append(f.closeCalls, struct {
		Workspace string
		Surface   string
	}{Workspace: workspace, Surface: surface})
	return nil
}

func TestServiceOpenCreatesStageWorkspaceAndDocsPane(t *testing.T) {
	root := t.TempDir()
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"markdown.open":    {},
			"workspace.list":   {},
			"workspace.create": {},
			"workspace.rename": {},
			"pane.create":      {},
			"pane.list":        {},
			"pane.surfaces":    {},
			"surface.move":     {},
			"surface.close":    {},
		}},
		createWorkspaceID: "workspace:stage",
		listPanes: map[string][]cmuxctl.Pane{
			"workspace:stage": {{Ref: "pane:stage", Index: 0, Focused: true}},
			"CMUX-1":          {{Ref: "pane:docs", Index: 0}},
		},
		listPanesErr: map[string]error{},
		listPaneSurfaces: map[string][]cmuxctl.Surface{
			"pane:stage": {
				{Ref: "surface:stage", Index: 0, Selected: true},
			},
			"pane:docs": {
				{Ref: "surface:bootstrap", Index: 0, Title: "Terminal"},
			},
		},
		listPaneSurfaceErrs: map[string]error{},
		createPaneResult: cmuxctl.PaneCreateResult{
			PaneRef:    "pane:docs",
			SurfaceRef: "surface:bootstrap",
		},
		markdownResult: cmuxctl.MarkdownOpenResult{
			PaneRef:    "pane:temp",
			SurfaceRef: "surface:viewer",
		},
		moveResult: cmuxctl.MoveSurfaceResult{
			PaneRef:    "pane:docs",
			SurfaceRef: "surface:viewer",
		},
	}

	svc := NewService(
		func() Client { return fake },
		func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
		func(root string) cmuxdocs.Store { return cmuxdocs.NewStore(root) },
	)
	svc.Now = func() time.Time { return time.Unix(1700000000, 0) }

	result, code, msg := svc.Open(context.Background(), OpenRequest{
		Root:        root,
		WorkspaceID: "WS1",
		Path:        filepath.Join(root, "workspaces", "WS1", "notes", "README.md"),
		ScopePath:   ".",
		Focus:       true,
	})
	if code != "" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if result.DocsPaneRef != "pane:docs" || result.ViewerSurfaceRef != "surface:viewer" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(fake.renameWorkspaceCalls) != 1 || fake.renameWorkspaceCalls[0].Title != stageWorkspaceName {
		t.Fatalf("rename workspace calls = %+v", fake.renameWorkspaceCalls)
	}
	if len(fake.moveCalls) != 1 || fake.moveCalls[0].After != "surface:bootstrap" || !fake.moveCalls[0].Focus {
		t.Fatalf("move calls = %+v", fake.moveCalls)
	}
	if len(fake.closeCalls) != 1 || fake.closeCalls[0].Workspace != "CMUX-1" || fake.closeCalls[0].Surface != "surface:bootstrap" {
		t.Fatalf("close calls = %+v", fake.closeCalls)
	}
	if len(fake.renameCalls) != 1 || fake.renameCalls[0].Title != "docs:README.md" {
		t.Fatalf("rename calls = %+v", fake.renameCalls)
	}

	state, err := cmuxdocs.NewStore(root).Load()
	if err != nil {
		t.Fatalf("load docs state: %v", err)
	}
	if state.Stage.WorkspaceRef != "workspace:stage" || state.Stage.SurfaceRef != "surface:stage" {
		t.Fatalf("stage state = %+v", state.Stage)
	}
	slot := state.Workspaces["WS1"]
	if slot.PaneRef != "pane:docs" || slot.LastViewerRef != "surface:viewer" {
		t.Fatalf("slot = %+v", slot)
	}
}

func TestServiceOpenReusesStoredStageAndDocsPane(t *testing.T) {
	root := t.TempDir()
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}
	if err := cmuxdocs.NewStore(root).Save(cmuxdocs.File{
		Version: cmuxdocs.CurrentVersion,
		Stage: cmuxdocs.Stage{
			WorkspaceRef: "workspace:stage",
			PaneRef:      "pane:stage",
			SurfaceRef:   "surface:stage",
		},
		Workspaces: map[string]cmuxdocs.Slot{
			"WS1": {
				CMUXWorkspaceID: "CMUX-1",
				PaneRef:         "pane:docs",
				LastViewerRef:   "surface:old",
			},
		},
	}); err != nil {
		t.Fatalf("save docs state: %v", err)
	}

	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"markdown.open":    {},
			"workspace.list":   {},
			"workspace.create": {},
			"workspace.rename": {},
			"pane.create":      {},
			"pane.list":        {},
			"pane.surfaces":    {},
			"surface.move":     {},
			"surface.close":    {},
		}},
		listPanes:    map[string][]cmuxctl.Pane{},
		listPanesErr: map[string]error{},
		listPaneSurfaces: map[string][]cmuxctl.Surface{
			"pane:stage": {
				{Ref: "surface:stage", Index: 0, Selected: true},
			},
			"pane:docs": {
				{Ref: "surface:old", Index: 0, Title: "docs:old.md"},
				{Ref: "surface:other", Index: 1, Title: "other"},
			},
		},
		listPaneSurfaceErrs: map[string]error{},
		markdownResult: cmuxctl.MarkdownOpenResult{
			PaneRef:    "pane:temp",
			SurfaceRef: "surface:new",
		},
		moveResult: cmuxctl.MoveSurfaceResult{
			PaneRef:    "pane:docs",
			SurfaceRef: "surface:new",
		},
	}

	svc := NewService(
		func() Client { return fake },
		func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
		func(root string) cmuxdocs.Store { return cmuxdocs.NewStore(root) },
	)

	result, code, msg := svc.Open(context.Background(), OpenRequest{
		Root:        root,
		WorkspaceID: "WS1",
		Path:        filepath.Join(root, "workspaces", "WS1", "notes", "README.md"),
		ScopePath:   ".",
		Focus:       false,
	})
	if code != "" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if result.DocsPaneRef != "pane:docs" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(fake.renameWorkspaceCalls) != 0 {
		t.Fatalf("rename workspace calls = %+v", fake.renameWorkspaceCalls)
	}
	if len(fake.moveCalls) != 1 || fake.moveCalls[0].After != "surface:old" || fake.moveCalls[0].Focus {
		t.Fatalf("move calls = %+v", fake.moveCalls)
	}
	if len(fake.closeCalls) != 0 {
		t.Fatalf("close calls should be empty: %+v", fake.closeCalls)
	}
}

func TestServiceOpenRediscoversDocsPaneWhenStoredPaneIsStale(t *testing.T) {
	root := t.TempDir()
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}
	if err := cmuxdocs.NewStore(root).Save(cmuxdocs.File{
		Version: cmuxdocs.CurrentVersion,
		Stage: cmuxdocs.Stage{
			WorkspaceRef: "workspace:stage",
			PaneRef:      "pane:stage",
			SurfaceRef:   "surface:stage",
		},
		Workspaces: map[string]cmuxdocs.Slot{
			"WS1": {
				CMUXWorkspaceID: "CMUX-1",
				PaneRef:         "pane:stale",
				LastViewerRef:   "surface:stale",
			},
		},
	}); err != nil {
		t.Fatalf("save docs state: %v", err)
	}

	fake := &fakeClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"markdown.open":    {},
			"workspace.list":   {},
			"workspace.create": {},
			"workspace.rename": {},
			"pane.create":      {},
			"pane.list":        {},
			"pane.surfaces":    {},
			"surface.move":     {},
			"surface.close":    {},
		}},
		listPanes: map[string][]cmuxctl.Pane{
			"CMUX-1": {
				{Ref: "pane:1"},
				{Ref: "pane:2"},
			},
		},
		listPanesErr: map[string]error{},
		listPaneSurfaces: map[string][]cmuxctl.Surface{
			"pane:stage": {
				{Ref: "surface:stage", Index: 0, Selected: true},
			},
			"pane:1": {
				{Ref: "surface:4", Title: "docs:README.md", Index: 0},
			},
			"pane:2": {
				{Ref: "surface:8", Title: "other", Index: 0},
			},
		},
		listPaneSurfaceErrs: map[string]error{
			"pane:stale": errors.New("pane not found"),
		},
		markdownResult: cmuxctl.MarkdownOpenResult{
			PaneRef:    "pane:temp",
			SurfaceRef: "surface:10",
		},
		moveResult: cmuxctl.MoveSurfaceResult{
			PaneRef:    "pane:1",
			SurfaceRef: "surface:10",
		},
	}

	svc := NewService(
		func() Client { return fake },
		func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
		func(root string) cmuxdocs.Store { return cmuxdocs.NewStore(root) },
	)

	result, code, msg := svc.Open(context.Background(), OpenRequest{
		Root:        root,
		WorkspaceID: "WS1",
		Path:        filepath.Join(root, "workspaces", "WS1", "notes", "README.md"),
		ScopePath:   ".",
		Focus:       false,
	})
	if code != "" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if result.DocsPaneRef != "pane:1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(fake.moveCalls) != 1 || fake.moveCalls[0].Pane != "pane:1" || fake.moveCalls[0].After != "surface:4" || fake.moveCalls[0].Focus {
		t.Fatalf("move calls = %+v", fake.moveCalls)
	}
	if len(fake.closeCalls) != 0 {
		t.Fatalf("close calls should be empty on rediscovery: %+v", fake.closeCalls)
	}
}
