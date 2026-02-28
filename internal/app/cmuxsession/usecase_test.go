package cmuxsession

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	sessionstore "github.com/tasuku43/kra/internal/cmuxsession"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeClient struct {
	identifyErr         error
	selectWorkspaceErr  error
	listPanesErr        error
	listPaneSurfacesErr error
	readScreenErr       error
	browserSaveErr      error
	browserLoadErr      error
	focusPaneErr        error

	panesByWorkspace    map[string][]cmuxctl.Pane
	surfacesByPane      map[string][]cmuxctl.Surface
	readScreenTextByKey map[string]string
}

func (f *fakeClient) Identify(context.Context, string, string) (map[string]any, error) {
	if f.identifyErr != nil {
		return nil, f.identifyErr
	}
	return map[string]any{"ok": true}, nil
}

func (f *fakeClient) SelectWorkspace(context.Context, string) error {
	return f.selectWorkspaceErr
}

func (f *fakeClient) ListPanes(_ context.Context, workspace string) ([]cmuxctl.Pane, error) {
	if f.listPanesErr != nil {
		return nil, f.listPanesErr
	}
	return append([]cmuxctl.Pane{}, f.panesByWorkspace[workspace]...), nil
}

func (f *fakeClient) ListPaneSurfaces(_ context.Context, _ string, pane string) ([]cmuxctl.Surface, error) {
	if f.listPaneSurfacesErr != nil {
		return nil, f.listPaneSurfacesErr
	}
	return append([]cmuxctl.Surface{}, f.surfacesByPane[pane]...), nil
}

func (f *fakeClient) ReadScreen(_ context.Context, workspace string, surface string, _ int, _ bool) (string, error) {
	if f.readScreenErr != nil {
		return "", f.readScreenErr
	}
	key := workspace + "/" + surface
	if text, ok := f.readScreenTextByKey[key]; ok {
		return text, nil
	}
	return "", nil
}

func (f *fakeClient) BrowserStateSave(_ context.Context, _ string, _ string, path string) error {
	if f.browserSaveErr != nil {
		return f.browserSaveErr
	}
	return os.WriteFile(path, []byte(`{"ok":true}`), 0o644)
}

func (f *fakeClient) BrowserStateLoad(context.Context, string, string, string) error {
	return f.browserLoadErr
}

func (f *fakeClient) FocusPane(context.Context, string, string) error {
	return f.focusPaneErr
}

func TestServiceSave_SavesSessionArtifactsAndIndex(t *testing.T) {
	root := t.TempDir()
	workspaceID := "WS1"
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			workspaceID: {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-WS-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeClient{
		panesByWorkspace: map[string][]cmuxctl.Pane{
			"CMUX-WS-1": {
				{ID: "pane-1", Ref: "pane:1", Focused: true},
			},
		},
		surfacesByPane: map[string][]cmuxctl.Surface{
			"pane-1": {
				{ID: "sf-1", Ref: "surface:1", Title: "term", Type: "terminal", Selected: true},
				{ID: "sf-2", Ref: "surface:2", Title: "browser", Type: "browser"},
			},
		},
		readScreenTextByKey: map[string]string{
			"CMUX-WS-1/sf-1": "hello from terminal",
			"CMUX-WS-1/sf-2": "browser text",
		},
	}
	svc := NewService(
		func() Client { return fake },
		func(root string) MappingStore { return cmuxmap.NewStore(root) },
		func(root string) SessionStore { return sessionstore.NewStore(root) },
	)
	now := time.Date(2026, 2, 28, 15, 4, 5, 0, time.UTC)
	svc.Now = func() time.Time { return now }

	res, code, msg := svc.Save(context.Background(), SaveRequest{
		Root:                root,
		WorkspaceID:         workspaceID,
		Label:               "review before merge",
		IncludeBrowserState: true,
	})
	if code != "" || msg != "" {
		t.Fatalf("Save() code/msg = %q/%q, want empty", code, msg)
	}
	if res.SessionID == "" || res.PaneCount != 1 || res.SurfaceCount != 2 {
		t.Fatalf("unexpected save result: %+v", res)
	}
	if !res.BrowserStateSaved {
		t.Fatalf("BrowserStateSaved = false, want true")
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want empty", res.Warnings)
	}

	sessionFile, err := sessionstore.NewStore(root).Load()
	if err != nil {
		t.Fatalf("load session store: %v", err)
	}
	wsSessions := sessionFile.Workspaces[workspaceID].Sessions
	if len(wsSessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(wsSessions))
	}
	sessionDir := filepath.Join(root, filepath.FromSlash(wsSessions[0].Path))
	if _, err := os.Stat(filepath.Join(sessionDir, "session.json")); err != nil {
		t.Fatalf("session.json missing: %v", err)
	}
}

func TestServiceResume_StrictFailsOnPartialRestore(t *testing.T) {
	root := t.TempDir()
	workspaceID := "WS1"
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			workspaceID: {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-WS-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	saveClient := &fakeClient{
		panesByWorkspace: map[string][]cmuxctl.Pane{
			"CMUX-WS-1": {
				{ID: "pane-1", Ref: "pane:1", Focused: true},
			},
		},
		surfacesByPane: map[string][]cmuxctl.Surface{
			"pane-1": {
				{ID: "sf-2", Ref: "surface:2", Title: "browser", Type: "browser"},
			},
		},
		readScreenTextByKey: map[string]string{
			"CMUX-WS-1/sf-2": "browser text",
		},
	}
	svc := NewService(
		func() Client { return saveClient },
		func(root string) MappingStore { return cmuxmap.NewStore(root) },
		func(root string) SessionStore { return sessionstore.NewStore(root) },
	)
	saveRes, code, msg := svc.Save(context.Background(), SaveRequest{
		Root:                root,
		WorkspaceID:         workspaceID,
		Label:               "for resume",
		IncludeBrowserState: true,
	})
	if code != "" || msg != "" {
		t.Fatalf("Save() code/msg = %q/%q, want empty", code, msg)
	}

	resumeClient := &fakeClient{
		focusPaneErr: errors.New("focus failed"),
	}
	svc.NewClient = func() Client { return resumeClient }
	svc.Now = func() time.Time { return time.Date(2026, 2, 28, 16, 0, 0, 0, time.UTC) }
	res, code, _ := svc.Resume(context.Background(), ResumeRequest{
		Root:        root,
		WorkspaceID: workspaceID,
		SessionID:   saveRes.SessionID,
		Strict:      true,
	})
	if code != "session_restore_partial" {
		t.Fatalf("Resume() code = %q, want %q", code, "session_restore_partial")
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("warnings should not be empty in strict partial restore")
	}
}
