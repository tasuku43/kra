package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeCMUXSessionClient struct {
	identifyErr        error
	selectWorkspaceErr error
	listPanesErr       error
	listSurfacesErr    error
	readScreenErr      error
	browserSaveErr     error
	browserLoadErr     error
	focusPaneErr       error

	panes            []cmuxctl.Pane
	surfacesByPane   map[string][]cmuxctl.Surface
	readScreenBySurf map[string]string
}

func (f *fakeCMUXSessionClient) Identify(context.Context, string, string) (map[string]any, error) {
	if f.identifyErr != nil {
		return nil, f.identifyErr
	}
	return map[string]any{"ok": true}, nil
}

func (f *fakeCMUXSessionClient) SelectWorkspace(context.Context, string) error {
	return f.selectWorkspaceErr
}

func (f *fakeCMUXSessionClient) ListPanes(context.Context, string) ([]cmuxctl.Pane, error) {
	if f.listPanesErr != nil {
		return nil, f.listPanesErr
	}
	return append([]cmuxctl.Pane{}, f.panes...), nil
}

func (f *fakeCMUXSessionClient) ListPaneSurfaces(_ context.Context, _ string, pane string) ([]cmuxctl.Surface, error) {
	if f.listSurfacesErr != nil {
		return nil, f.listSurfacesErr
	}
	return append([]cmuxctl.Surface{}, f.surfacesByPane[pane]...), nil
}

func (f *fakeCMUXSessionClient) ReadScreen(_ context.Context, _ string, surface string, _ int, _ bool) (string, error) {
	if f.readScreenErr != nil {
		return "", f.readScreenErr
	}
	return f.readScreenBySurf[surface], nil
}

func (f *fakeCMUXSessionClient) BrowserStateSave(_ context.Context, _ string, _ string, path string) error {
	if f.browserSaveErr != nil {
		return f.browserSaveErr
	}
	return os.WriteFile(path, []byte(`{"ok":true}`), 0o644)
}

func (f *fakeCMUXSessionClient) BrowserStateLoad(context.Context, string, string, string) error {
	return f.browserLoadErr
}

func (f *fakeCMUXSessionClient) FocusPane(context.Context, string, string) error {
	return f.focusPaneErr
}

func TestCLI_WSSave_JSON_Success(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-WS-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeCMUXSessionClient{
		panes: []cmuxctl.Pane{
			{ID: "pane-1", Focused: true},
		},
		surfacesByPane: map[string][]cmuxctl.Surface{
			"pane-1": {
				{ID: "surface-1", Type: "terminal", Title: "term"},
				{ID: "surface-2", Type: "browser", Title: "browser"},
			},
		},
		readScreenBySurf: map[string]string{
			"surface-1": "hello",
			"surface-2": "browser",
		},
	}
	prevClient := newCMUXSessionClient
	newCMUXSessionClient = func() cmuxSessionClient { return fake }
	t.Cleanup(func() { newCMUXSessionClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "save", "--format", "json", "--id", "WS1", "-l", "review"})
	if code != exitOK {
		t.Fatalf("ws save --format json exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.save" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if got := resp.Result["session_id"]; got == nil || got == "" {
		t.Fatalf("session_id missing: %+v", resp.Result)
	}
}

func TestCLI_WSSave_RequiresTargetMode(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "save", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("ws save --format json exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "ws save requires one of --id <id>, --current, or --select") {
		t.Fatalf("stderr missing target mode message: %q", err.String())
	}
}

func TestCLI_WSResume_JSON_RequiresLatest(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WS1"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-WS-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "resume", "--format", "json", "--id", "WS1"})
	if code != exitUsage {
		t.Fatalf("ws resume --format json exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "ws.resume" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_WSResume_RequiresTargetMode(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "resume"})
	if code != exitUsage {
		t.Fatalf("ws resume exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "ws resume requires one of --id <id>, --current, or --select") {
		t.Fatalf("stderr missing target mode message: %q", err.String())
	}
}

func TestCLI_WSResume_JSON_StrictPartialFailure(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := cmuxmap.NewStore(root).Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-WS-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeCMUXSessionClient{
		panes: []cmuxctl.Pane{
			{ID: "pane-1", Focused: true},
		},
		surfacesByPane: map[string][]cmuxctl.Surface{
			"pane-1": {
				{ID: "surface-2", Type: "browser"},
			},
		},
		readScreenBySurf: map[string]string{
			"surface-2": "browser",
		},
	}
	prevClient := newCMUXSessionClient
	newCMUXSessionClient = func() cmuxSessionClient { return fake }
	t.Cleanup(func() { newCMUXSessionClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "save", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws save --format json exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}

	fake.focusPaneErr = errors.New("focus failed")
	out.Reset()
	err.Reset()
	code = c.Run([]string{"ws", "resume", "--format", "json", "--id", "WS1", "--latest", "--strict"})
	if code != exitError {
		t.Fatalf("ws resume --strict exit code = %d, want %d (stderr=%q out=%q)", code, exitError, err.String(), out.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Error.Code != "session_restore_partial" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}
