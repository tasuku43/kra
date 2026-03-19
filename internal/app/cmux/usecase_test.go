package cmux

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/core/cmuxstyle"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeClient struct {
	capabilities cmuxctl.Capabilities
	createID     string
	createErr    error
	createCmds   []string
	renameErr    error
	selectErr    error
	setStatusErr error
	identifyErr  map[string]error
	identifyBy   map[string]map[string]any

	renameWorkspace string
	renameTitle     string
	selectWorkspace string
	statusWorkspace string
	statusLabel     string
	statusText      string
	statusIcon      string
	statusColor     string
}

func (f *fakeClient) Capabilities(context.Context) (cmuxctl.Capabilities, error) {
	return f.capabilities, nil
}

func (f *fakeClient) CreateWorkspaceWithCommand(_ context.Context, command string) (string, error) {
	f.createCmds = append(f.createCmds, command)
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.createID, nil
}

func (f *fakeClient) RenameWorkspace(_ context.Context, workspace string, title string) error {
	f.renameWorkspace = workspace
	f.renameTitle = title
	return f.renameErr
}

func (f *fakeClient) SelectWorkspace(_ context.Context, workspace string) error {
	f.selectWorkspace = workspace
	return f.selectErr
}

func (f *fakeClient) SetStatus(_ context.Context, workspace string, label string, text string, icon string, color string) error {
	f.statusWorkspace = workspace
	f.statusLabel = label
	f.statusText = text
	f.statusIcon = icon
	f.statusColor = color
	return f.setStatusErr
}

func (f *fakeClient) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	return nil, nil
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
	return map[string]any{"workspace_id": workspace}, nil
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
		func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
	)
	svc.Now = func() time.Time { return time.Unix(1700000000, 0) }

	item, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
		StatusText:    "kra:root",
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
	if fake.statusWorkspace != "CMUX-ROOT-1" || fake.statusLabel != "kra" || fake.statusText != "kra:root" || fake.statusIcon != "tag" || fake.statusColor != cmuxstyle.WorkspaceLabelColor {
		t.Fatalf("status args = workspace=%q label=%q text=%q icon=%q color=%q", fake.statusWorkspace, fake.statusLabel, fake.statusText, fake.statusIcon, fake.statusColor)
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
		func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
	)

	item, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
		StatusText:    "kra:root",
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
		func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
	)

	item, code, msg := svc.EnsureWorkspace(context.Background(), root, OpenTarget{
		WorkspaceID:   "KRA_ROOT",
		WorkspacePath: root,
		Title:         "KRA_ROOT",
		StatusText:    "kra:root",
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
