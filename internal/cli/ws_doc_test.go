package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	appwsdoc "github.com/tasuku43/kra/internal/app/wsdoc"
	"github.com/tasuku43/kra/internal/cmuxdocs"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeCLIWSDocClient struct {
	capabilities      cmuxctl.Capabilities
	listPanes         map[string][]cmuxctl.Pane
	listPaneSurfaces  map[string][]cmuxctl.Surface
	createPaneResult  cmuxctl.PaneCreateResult
	markdownResult    cmuxctl.MarkdownOpenResult
	moveResult        cmuxctl.MoveSurfaceResult
	closeSurfaceCalls []string
}

func (f *fakeCLIWSDocClient) Capabilities(_ context.Context) (cmuxctl.Capabilities, error) {
	return f.capabilities, nil
}

func (f *fakeCLIWSDocClient) Identify(context.Context, string, string) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

func (f *fakeCLIWSDocClient) ListPanes(_ context.Context, workspace string) ([]cmuxctl.Pane, error) {
	return append([]cmuxctl.Pane{}, f.listPanes[workspace]...), nil
}

func (f *fakeCLIWSDocClient) ListPaneSurfaces(_ context.Context, _ string, pane string) ([]cmuxctl.Surface, error) {
	return append([]cmuxctl.Surface{}, f.listPaneSurfaces[pane]...), nil
}

func (f *fakeCLIWSDocClient) CreatePane(context.Context, string, string) (cmuxctl.PaneCreateResult, error) {
	return f.createPaneResult, nil
}

func (f *fakeCLIWSDocClient) CreateSplit(context.Context, string, string, string) (cmuxctl.PaneCreateResult, error) {
	return cmuxctl.PaneCreateResult{}, nil
}

func (f *fakeCLIWSDocClient) MarkdownOpen(context.Context, string, string, string) (cmuxctl.MarkdownOpenResult, error) {
	return f.markdownResult, nil
}

func (f *fakeCLIWSDocClient) MoveSurface(context.Context, string, string, string, string, bool) (cmuxctl.MoveSurfaceResult, error) {
	return f.moveResult, nil
}

func (f *fakeCLIWSDocClient) RenameTab(context.Context, string, string, string) error {
	return nil
}

func (f *fakeCLIWSDocClient) CloseSurface(_ context.Context, _ string, surface string) error {
	f.closeSurfaceCalls = append(f.closeSurfaceCalls, surface)
	return nil
}

func TestCLIWSDocOpenJSONRequiresExplicitFileWhenMultipleCandidates(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(filepath.Join(wsPath, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "notes", "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "notes", "b.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatalf("write b.md: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"ws", "doc", "open", "--id", "WS1", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}

	var resp testJSONResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if resp.Error.Code != "non_interactive_selection_required" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLIWSDocOpenJSONSuccess(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(filepath.Join(wsPath, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "notes", "README.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
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

	fake := &fakeCLIWSDocClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"markdown.open": {},
			"pane.create":   {},
			"pane.list":     {},
			"pane.surfaces": {},
			"surface.move":  {},
			"surface.close": {},
		}},
		listPanes: map[string][]cmuxctl.Pane{
			"workspace:root": {{Ref: "pane:stage", Index: 0, Focused: true}},
			"CMUX-1":         {{Ref: "pane:9", Index: 0}},
		},
		listPaneSurfaces: map[string][]cmuxctl.Surface{
			"pane:stage": {
				{Ref: "surface:stage", Index: 0, Selected: true},
			},
			"pane:9": {
				{Ref: "surface:11", Title: "Terminal", Index: 0},
			},
		},
		createPaneResult: cmuxctl.PaneCreateResult{
			PaneRef:    "pane:9",
			SurfaceRef: "surface:11",
		},
		markdownResult: cmuxctl.MarkdownOpenResult{
			PaneRef:    "pane:temp",
			SurfaceRef: "surface:21",
		},
		moveResult: cmuxctl.MoveSurfaceResult{
			PaneRef:    "pane:9",
			SurfaceRef: "surface:21",
		},
	}

	prev := newWSDocService
	newWSDocService = func() *appwsdoc.Service {
		return appwsdoc.NewService(
			func() appwsdoc.Client { return fake },
			func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
			func(root string) cmuxdocs.Store { return cmuxdocs.NewStore(root) },
			func(context.Context, string) (string, string, string) { return "workspace:root", "", "" },
		)
	}
	t.Cleanup(func() { newWSDocService = prev })

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"ws", "doc", "open", "--id", "WS1", "notes/README.md", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, errBuf.String(), out.String())
	}

	var resp testJSONResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if !resp.OK || resp.Action != "ws.doc.open" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if _, ok := resp.Result["anchor_surface_ref"]; ok {
		t.Fatalf("anchor_surface_ref should not be returned anymore: %+v", resp.Result)
	}
	if got, _ := resp.Result["docs_pane_ref"].(string); got != "pane:9" {
		t.Fatalf("docs_pane_ref = %q, want %q", got, "pane:9")
	}
	state, err := cmuxdocs.NewStore(root).Load()
	if err != nil {
		t.Fatalf("load docs state: %v", err)
	}
	if state.Stage.WorkspaceRef != "workspace:root" {
		t.Fatalf("stage state = %+v", state.Stage)
	}
	if slot := state.Workspaces["WS1"]; slot.PaneRef != "pane:9" || slot.LastViewerRef != "surface:21" {
		t.Fatalf("slot = %+v", slot)
	}
}
