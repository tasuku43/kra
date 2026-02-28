package cmuxctl

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/core/cmuxstyle"
)

type fakeRunner struct {
	lastName string
	lastArgs []string
	stdout   []byte
	stderr   []byte
	err      error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.lastName = name
	f.lastArgs = append([]string{}, args...)
	return f.stdout, f.stderr, f.err
}

type fakeRunnerSequence struct {
	calls []struct {
		stdout []byte
		stderr []byte
		err    error
	}
	names [][]string
}

func (f *fakeRunnerSequence) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.names = append(f.names, append([]string{name}, args...))
	if len(f.calls) == 0 {
		return nil, nil, errors.New("unexpected call")
	}
	call := f.calls[0]
	f.calls = f.calls[1:]
	return call.stdout, call.stderr, call.err
}

func TestClientCapabilities_ParsesMethods(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"methods":["workspace.create","workspace.select"]}`)}
	c := &Client{Runner: f, SocketPath: "/tmp/cmux.sock"}

	got, err := c.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() error: %v", err)
	}
	wantArgs := []string{"--socket", "/tmp/cmux.sock", "--json", "--id-format", "both", "capabilities"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
	if _, ok := got.Methods["workspace.create"]; !ok {
		t.Fatalf("workspace.create capability missing: %v", got.Methods)
	}
}

func TestClientCreateWorkspace_ParsesOKResponse(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK 1234-ABCD\n")}
	c := &Client{Runner: f}

	got, err := c.CreateWorkspace(context.Background())
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if got != "1234-ABCD" {
		t.Fatalf("workspace id = %q, want %q", got, "1234-ABCD")
	}
	wantArgs := []string{"new-workspace"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientCreateWorkspaceWithCommand_BuildsArgs(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK ws-1\n")}
	c := &Client{Runner: f}

	got, err := c.CreateWorkspaceWithCommand(context.Background(), "cd '/tmp/ws-1'")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithCommand() error: %v", err)
	}
	if got != "ws-1" {
		t.Fatalf("workspace id = %q, want %q", got, "ws-1")
	}
	wantArgs := []string{"new-workspace", "--command", "cd '/tmp/ws-1'"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientListWorkspaces_JSONMode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"workspaces":[{"id":"id1","ref":"workspace:1","index":0,"title":"t","selected":true}]}`)}
	c := &Client{Runner: f}

	got, err := c.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces() error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "id1" || got[0].Ref != "workspace:1" || !got[0].Selected {
		t.Fatalf("unexpected workspaces: %+v", got)
	}
	wantArgs := []string{"--json", "--id-format", "both", "list-workspaces"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientListWorkspaces_FallsBackToTreeWhenCurrentWindowEmpty(t *testing.T) {
	f := &fakeRunnerSequence{
		calls: []struct {
			stdout []byte
			stderr []byte
			err    error
		}{
			{stdout: []byte(`{"workspaces":[]}`)},
			{stdout: []byte(`{"windows":[{"workspaces":[{"id":"id2","ref":"workspace:2","index":1,"title":"other","current":true}]}]}`)},
		},
	}
	c := &Client{Runner: f}

	got, err := c.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces() error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "id2" || got[0].Ref != "workspace:2" || !got[0].Selected {
		t.Fatalf("unexpected fallback workspaces: %+v", got)
	}
	if len(f.names) != 2 {
		t.Fatalf("call count = %d, want 2", len(f.names))
	}
	wantFirst := []string{"cmux", "--json", "--id-format", "both", "list-workspaces"}
	if !reflect.DeepEqual(f.names[0], wantFirst) {
		t.Fatalf("first call = %v, want %v", f.names[0], wantFirst)
	}
	wantSecond := []string{"cmux", "--json", "--id-format", "both", "tree", "--all"}
	if !reflect.DeepEqual(f.names[1], wantSecond) {
		t.Fatalf("second call = %v, want %v", f.names[1], wantSecond)
	}
}

func TestClientRunJSON_FallsBackWhenIDFormatUnsupported(t *testing.T) {
	f := &fakeRunnerSequence{
		calls: []struct {
			stdout []byte
			stderr []byte
			err    error
		}{
			{stderr: []byte("Unknown option: --id-format"), err: errors.New("exit status 1")},
			{stdout: []byte(`{"methods":["workspace.create"]}`)},
		},
	}
	c := &Client{Runner: f}

	got, err := c.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() error: %v", err)
	}
	if _, ok := got.Methods["workspace.create"]; !ok {
		t.Fatalf("capabilities missing workspace.create: %+v", got.Methods)
	}
	if len(f.names) != 2 {
		t.Fatalf("call count = %d, want 2", len(f.names))
	}
	wantFirst := []string{"cmux", "--json", "--id-format", "both", "capabilities"}
	if !reflect.DeepEqual(f.names[0], wantFirst) {
		t.Fatalf("first call = %v, want %v", f.names[0], wantFirst)
	}
	wantSecond := []string{"cmux", "--json", "capabilities"}
	if !reflect.DeepEqual(f.names[1], wantSecond) {
		t.Fatalf("second call = %v, want %v", f.names[1], wantSecond)
	}
}

func TestClientRenameWorkspace_RequiresInputs(t *testing.T) {
	c := &Client{}
	if err := c.RenameWorkspace(context.Background(), "", "title"); err == nil {
		t.Fatalf("RenameWorkspace() with empty workspace should fail")
	}
	if err := c.RenameWorkspace(context.Background(), "ws", ""); err == nil {
		t.Fatalf("RenameWorkspace() with empty title should fail")
	}
}

func TestClientSelectWorkspace_RequiresInput(t *testing.T) {
	c := &Client{}
	if err := c.SelectWorkspace(context.Background(), ""); err == nil {
		t.Fatalf("SelectWorkspace() with empty workspace should fail")
	}
}

func TestClientSetStatus_RequiresInputs(t *testing.T) {
	c := &Client{}
	if err := c.SetStatus(context.Background(), "", "kra", "managed by kra", "tag", cmuxstyle.WorkspaceLabelColor); err == nil {
		t.Fatalf("SetStatus() with empty workspace should fail")
	}
	if err := c.SetStatus(context.Background(), "ws-1", "", "managed by kra", "tag", cmuxstyle.WorkspaceLabelColor); err == nil {
		t.Fatalf("SetStatus() with empty label should fail")
	}
	if err := c.SetStatus(context.Background(), "ws-1", "kra", "", "tag", cmuxstyle.WorkspaceLabelColor); err == nil {
		t.Fatalf("SetStatus() with empty text should fail")
	}
}

func TestClientSetStatus_BuildsCommandArgs(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK\n")}
	c := &Client{Runner: f}

	if err := c.SetStatus(context.Background(), "ws-1", "kra", "managed by kra", "tag", cmuxstyle.WorkspaceLabelColor); err != nil {
		t.Fatalf("SetStatus() error: %v", err)
	}
	wantArgs := []string{"set-status", "kra", "managed by kra", "--workspace", "ws-1", "--icon", "tag", "--color", cmuxstyle.WorkspaceLabelColor}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientSendText_BuildsCommandArgs(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK\n")}
	c := &Client{Runner: f}

	if err := c.SendText(context.Background(), "ws-1", "surface:1", "cd /tmp\n"); err != nil {
		t.Fatalf("SendText() error: %v", err)
	}
	wantArgs := []string{"send", "--workspace", "ws-1", "--surface", "surface:1", "cd /tmp\n"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientCommandError_UsesStderr(t *testing.T) {
	f := &fakeRunner{
		stderr: []byte("boom"),
		err:    errors.New("exit status 1"),
	}
	c := &Client{Runner: f}
	err := c.SelectWorkspace(context.Background(), "ws-1")
	if err == nil {
		t.Fatalf("SelectWorkspace() error = nil, want non-nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, []string{"cmux select-workspace", "boom"}) {
		t.Fatalf("error message = %q, want command + stderr", got)
	}
}

func TestClientListPanes_JSONMode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"panes":[{"id":"pane-1","ref":"pane:1","index":0,"focused":true}]}`)}
	c := &Client{Runner: f}

	got, err := c.ListPanes(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListPanes() error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "pane-1" || got[0].Ref != "pane:1" || !got[0].Focused {
		t.Fatalf("unexpected panes: %+v", got)
	}
	wantArgs := []string{"--json", "--id-format", "both", "list-panes", "--workspace", "ws-1"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientListPaneSurfaces_JSONMode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"surfaces":[{"id":"sf-1","ref":"surface:1","index":0,"title":"agent","type":"browser","selected":true,"pane_id":"pane-1"}]}`)}
	c := &Client{Runner: f}

	got, err := c.ListPaneSurfaces(context.Background(), "ws-1", "pane-1")
	if err != nil {
		t.Fatalf("ListPaneSurfaces() error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "sf-1" || got[0].PaneID != "pane-1" || !got[0].Selected || got[0].Type != "browser" {
		t.Fatalf("unexpected surfaces: %+v", got)
	}
	wantArgs := []string{"--json", "--id-format", "both", "list-pane-surfaces", "--workspace", "ws-1", "--pane", "pane-1"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientFocusPane_BuildsCommandArgs(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK\n")}
	c := &Client{Runner: f}

	if err := c.FocusPane(context.Background(), "pane-1", "ws-1"); err != nil {
		t.Fatalf("FocusPane() error: %v", err)
	}
	wantArgs := []string{"focus-pane", "--pane", "pane-1", "--workspace", "ws-1"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientBrowserStateSave_BuildsCommandArgs(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK\n")}
	c := &Client{Runner: f}

	if err := c.BrowserStateSave(context.Background(), "ws-1", "surface:2", "/tmp/state.json"); err != nil {
		t.Fatalf("BrowserStateSave() error: %v", err)
	}
	wantArgs := []string{"browser", "--surface", "surface:2", "state", "save", "/tmp/state.json"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientBrowserStateLoad_BuildsCommandArgs(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK\n")}
	c := &Client{Runner: f}

	if err := c.BrowserStateLoad(context.Background(), "ws-1", "surface:2", "/tmp/state.json"); err != nil {
		t.Fatalf("BrowserStateLoad() error: %v", err)
	}
	wantArgs := []string{"browser", "--surface", "surface:2", "state", "load", "/tmp/state.json"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientReadScreen_JSONMode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"text":"hello"}`)}
	c := &Client{Runner: f}

	got, err := c.ReadScreen(context.Background(), "ws-1", "sf-1", 120, true)
	if err != nil {
		t.Fatalf("ReadScreen() error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("ReadScreen() text = %q, want %q", got, "hello")
	}
	wantArgs := []string{"--json", "--id-format", "both", "read-screen", "--workspace", "ws-1", "--surface", "sf-1", "--lines", "120", "--scrollback"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientListNotifications_JSONMode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"notifications":[{"workspace_id":"ws-1","surface_id":"sf-1","title":"Claude Code","body":"waiting","created_at":1730000000}]}`)}
	c := &Client{Runner: f}

	got, err := c.ListNotifications(context.Background())
	if err != nil {
		t.Fatalf("ListNotifications() error: %v", err)
	}
	if len(got) != 1 || got[0].WorkspaceID != "ws-1" || got[0].SurfaceID != "sf-1" || got[0].Body != "waiting" {
		t.Fatalf("unexpected notifications: %+v", got)
	}
	wantArgs := []string{"--json", "--id-format", "both", "list-notifications"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func TestClientListNotifications_JSONArrayMode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`[{"workspace_id":"ws-2","surface_id":"sf-2","title":"Claude Code","body":"waiting","created_at":1730000001}]`)}
	c := &Client{Runner: f}

	got, err := c.ListNotifications(context.Background())
	if err != nil {
		t.Fatalf("ListNotifications() error: %v", err)
	}
	if len(got) != 1 || got[0].WorkspaceID != "ws-2" || got[0].SurfaceID != "sf-2" || got[0].Body != "waiting" {
		t.Fatalf("unexpected notifications: %+v", got)
	}
	wantArgs := []string{"--json", "--id-format", "both", "list-notifications"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
