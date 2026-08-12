package cmuxctl

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestClientWorkspaceCanonicalMutationArgs(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Client) error
		want []string
	}{
		{
			name: "rename",
			run: func(c *Client) error {
				return c.RenameWorkspace(context.Background(), "workspace:2", "Build")
			},
			want: []string{"workspace", "rename", "workspace:2", "--title", "Build"},
		},
		{
			name: "select",
			run: func(c *Client) error {
				return c.SelectWorkspace(context.Background(), "workspace:2")
			},
			want: []string{"workspace", "select", "workspace:2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRunner{stdout: []byte("OK\n")}
			if err := tt.run(&Client{Runner: f}); err != nil {
				t.Fatalf("mutation error: %v", err)
			}
			if !reflect.DeepEqual(f.lastArgs, tt.want) {
				t.Fatalf("args = %v, want %v", f.lastArgs, tt.want)
			}
		})
	}
}

func TestClientCreateWorkspaceWithOptions_RPCParamsAndResult(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{
		"workspace_id":"ws-id","workspace_ref":"workspace:3",
		"window_id":"win-id","window_ref":"window:1",
		"group_id":"group-id","group_ref":"workspace_group:1",
		"surface_id":"surface-id","surface_ref":"surface:4"
	}`)}
	c := &Client{Runner: f, SocketPath: "/tmp/cmux.sock", Password: "secret"}

	got, err := c.CreateWorkspaceWithOptions(context.Background(), WorkspaceCreateOptions{
		OperationID:               "11111111-1111-1111-1111-111111111111",
		Title:                     "TASK-001 Build",
		CWD:                       "/tmp/project",
		WindowID:                  "win-id",
		GroupID:                   "group-id",
		GroupPlacement:            "end",
		GroupReferenceWorkspaceID: "anchor-id",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithOptions() error: %v", err)
	}
	if got.WorkspaceID != "ws-id" || got.WorkspaceRef != "workspace:3" || got.GroupID != "group-id" || got.SurfaceID != "surface-id" {
		t.Fatalf("result = %+v", got)
	}
	wantPrefix := []string{"--socket", "/tmp/cmux.sock", "--password", "secret", "rpc", "workspace.create"}
	if len(f.lastArgs) != len(wantPrefix)+1 || !reflect.DeepEqual(f.lastArgs[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("args = %v, want prefix %v + JSON", f.lastArgs, wantPrefix)
	}
	params := decodeRPCParams(t, f.lastArgs[len(f.lastArgs)-1])
	assertJSONValue(t, params, "operation_id", "11111111-1111-1111-1111-111111111111")
	assertJSONValue(t, params, "title", "TASK-001 Build")
	assertJSONValue(t, params, "cwd", "/tmp/project")
	assertJSONValue(t, params, "focus", false)
	assertJSONValue(t, params, "window_id", "win-id")
	assertJSONValue(t, params, "group_id", "group-id")
	assertJSONValue(t, params, "group_placement", "end")
	assertJSONValue(t, params, "group_reference_workspace_id", "anchor-id")
}

func TestClientCreateWorkspaceWithOptions_RejectsPlacementWithoutGroup(t *testing.T) {
	c := &Client{Runner: &fakeRunner{}}
	_, err := c.CreateWorkspaceWithOptions(context.Background(), WorkspaceCreateOptions{GroupPlacement: "end"})
	if err == nil || !strings.Contains(err.Error(), "group id") {
		t.Fatalf("error = %v, want group id validation", err)
	}
}

func TestClientListWindows_CanonicalArgsAndDecode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`[
		{"index":0,"id":" first-window-id ","key":true,"selected_workspace_id":" first-workspace-id ","workspace_count":2},
		{"index":1,"id":"second-window-id","key":false,"selected_workspace_id":null,"workspace_count":1}
	]`)}
	c := &Client{Runner: f}

	got, err := c.ListWindows(context.Background())
	if err != nil {
		t.Fatalf("ListWindows() error: %v", err)
	}
	wantArgs := []string{"--json", "--id-format", "both", "list-windows"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
	want := []Window{
		{ID: "first-window-id", Index: 0, Key: true, SelectedWorkspaceID: "first-workspace-id", WorkspaceCount: 2},
		{ID: "second-window-id", Index: 1, WorkspaceCount: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("windows = %+v, want %+v", got, want)
	}
}

func TestClientListWorkspaceGroups_CanonicalArgsAndDecode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{
		"window_id":"win-id","window_ref":"window:1","groups":[{
			"id":"group-id","ref":"workspace_group:1","name":"kra",
			"is_collapsed":true,"is_pinned":true,
			"anchor_workspace_id":"anchor-id","anchor_workspace_ref":"workspace:1",
			"custom_color":"#123456","icon_symbol":"folder",
			"member_workspace_ids":["anchor-id","child-id"],
			"member_workspace_refs":["workspace:1","workspace:2"],"member_count":2
		}]
	}`)}
	c := &Client{Runner: f}

	got, err := c.ListWorkspaceGroups(context.Background(), "window:1")
	if err != nil {
		t.Fatalf("ListWorkspaceGroups() error: %v", err)
	}
	wantArgs := []string{"--json", "--id-format", "both", "workspace", "group", "list", "--window", "window:1"}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
	if len(got.Groups) != 1 {
		t.Fatalf("groups = %+v", got.Groups)
	}
	group := got.Groups[0]
	if group.ID != "group-id" || group.Ref != "workspace_group:1" || group.WindowID != "win-id" || group.AnchorWorkspaceID != "anchor-id" || group.MemberCount != 2 {
		t.Fatalf("group = %+v", group)
	}
	if !group.Preferences.Collapsed || !group.Preferences.Pinned || group.Preferences.CustomColor != "#123456" || group.Preferences.IconSymbol != "folder" {
		t.Fatalf("preferences = %+v", group.Preferences)
	}
}

func TestClientCreateWorkspaceGroup_FreshAnchorAndChildOrder(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"group":{"id":"group-id","ref":"workspace_group:1","name":"kra","anchor_workspace_id":"anchor-id","member_workspace_ids":["anchor-id","child-2","child-1"],"member_count":3}}`)}
	c := &Client{Runner: f}

	got, err := c.CreateWorkspaceGroup(context.Background(), WorkspaceGroupCreateOptions{
		Name:              "kra",
		CWD:               "/tmp/kra-root",
		Window:            "window:1",
		ChildWorkspaceIDs: []string{"child-2", "child-1"},
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceGroup() error: %v", err)
	}
	wantArgs := []string{
		"--json", "--id-format", "both", "workspace", "group", "create",
		"--name", "kra", "--cwd", "/tmp/kra-root", "--from", "child-2,child-1", "--window", "window:1",
	}
	if !reflect.DeepEqual(f.lastArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", f.lastArgs, wantArgs)
	}
	if got.ID != "group-id" || got.WindowID != "" || !reflect.DeepEqual(got.MemberWorkspaceIDs, []string{"anchor-id", "child-2", "child-1"}) {
		t.Fatalf("group = %+v", got)
	}
}

func TestClientCreateWorkspaceGroup_RequiresCWD(t *testing.T) {
	_, err := (&Client{}).CreateWorkspaceGroup(context.Background(), WorkspaceGroupCreateOptions{Name: "kra"})
	if err == nil || !strings.Contains(err.Error(), "cwd") {
		t.Fatalf("error = %v, want cwd validation", err)
	}
}

func TestClientAddWorkspaceToGroup_CanonicalArgs(t *testing.T) {
	f := &fakeRunner{stdout: []byte("OK\n")}
	c := &Client{Runner: f}
	if err := c.AddWorkspaceToGroup(context.Background(), "workspace_group:1", "workspace:2"); err != nil {
		t.Fatalf("AddWorkspaceToGroup() error: %v", err)
	}
	want := []string{"workspace", "group", "add", "--group", "workspace_group:1", "--workspace", "workspace:2"}
	if !reflect.DeepEqual(f.lastArgs, want) {
		t.Fatalf("args = %v, want %v", f.lastArgs, want)
	}
}

func TestClientWorkspaceGroupCompensationArgs(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Client) error
		want []string
	}{
		{
			name: "remove",
			run: func(c *Client) error {
				return c.RemoveWorkspaceFromGroup(context.Background(), "workspace_group:1", "workspace:2")
			},
			want: []string{"workspace", "group", "remove", "--workspace", "workspace:2"},
		},
		{
			name: "set anchor",
			run: func(c *Client) error {
				return c.SetWorkspaceGroupAnchor(context.Background(), "workspace_group:1", "workspace:2")
			},
			want: []string{"workspace", "group", "set-anchor", "--group", "workspace_group:1", "--workspace", "workspace:2"},
		},
		{
			name: "ungroup",
			run: func(c *Client) error {
				return c.UngroupWorkspaceGroup(context.Background(), "workspace_group:1")
			},
			want: []string{"workspace", "group", "ungroup", "workspace_group:1"},
		},
		{
			name: "focus",
			run: func(c *Client) error {
				return c.FocusWorkspaceGroup(context.Background(), "workspace_group:1")
			},
			want: []string{"workspace", "group", "focus", "workspace_group:1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRunner{stdout: []byte("OK\n")}
			if err := tt.run(&Client{Runner: f}); err != nil {
				t.Fatalf("group mutation error: %v", err)
			}
			if !reflect.DeepEqual(f.lastArgs, tt.want) {
				t.Fatalf("args = %v, want %v", f.lastArgs, tt.want)
			}
		})
	}
}

func TestClientWorkspaceGroupCompensation_RequiresInputs(t *testing.T) {
	c := &Client{}
	if err := c.RemoveWorkspaceFromGroup(context.Background(), "", "workspace:2"); err == nil {
		t.Fatal("RemoveWorkspaceFromGroup() with empty group should fail")
	}
	if err := c.RemoveWorkspaceFromGroup(context.Background(), "workspace_group:1", ""); err == nil {
		t.Fatal("RemoveWorkspaceFromGroup() with empty workspace should fail")
	}
	if err := c.SetWorkspaceGroupAnchor(context.Background(), "", "workspace:2"); err == nil {
		t.Fatal("SetWorkspaceGroupAnchor() with empty group should fail")
	}
	if err := c.SetWorkspaceGroupAnchor(context.Background(), "workspace_group:1", ""); err == nil {
		t.Fatal("SetWorkspaceGroupAnchor() with empty workspace should fail")
	}
	if err := c.UngroupWorkspaceGroup(context.Background(), ""); err == nil {
		t.Fatal("UngroupWorkspaceGroup() with empty group should fail")
	}
	if err := c.FocusWorkspaceGroup(context.Background(), ""); err == nil {
		t.Fatal("FocusWorkspaceGroup() with empty group should fail")
	}
}

func TestClientListTodos_RPCDecode(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{
		"workspace_id":"ws-id","workspace_ref":"workspace:1","window_id":"win-id","window_ref":"window:1",
		"items":[{"id":"todo-id","index":0,"text":"Build","state":"in-progress","origin":"agent"}],
		"progress":{"completed":0,"total":1,"first_unchecked_text":"Build"}
	}`)}
	c := &Client{Runner: f}

	got, err := c.ListTodos(context.Background(), "ws-id")
	if err != nil {
		t.Fatalf("ListTodos() error: %v", err)
	}
	assertRPCCall(t, f.lastArgs, "workspace.todo.list", map[string]any{"workspace_id": "ws-id"})
	if len(got.Items) != 1 || got.Items[0].ID != "todo-id" || got.Items[0].State != "in-progress" || got.Progress.Total != 1 {
		t.Fatalf("todo list = %+v", got)
	}
}

func TestCapabilitiesSupportsTrimsMethod(t *testing.T) {
	caps := Capabilities{Methods: map[string]struct{}{"workspace.todo.list": {}}}
	if !caps.Supports(" workspace.todo.list ") || caps.Supports("workspace.todo.add") {
		t.Fatalf("unexpected Supports result: %#v", caps.Methods)
	}
}

func TestClientTodoMutations_RPCParamsAndDecode(t *testing.T) {
	itemResponse := []byte(`{"item":{"id":"todo-id","index":0,"text":"Build","state":"completed","origin":"agent"}}`)
	tests := []struct {
		name       string
		method     string
		wantParams map[string]any
		run        func(*Client) (TodoItem, error)
	}{
		{
			name:       "add",
			method:     "workspace.todo.add",
			wantParams: map[string]any{"workspace_id": "ws-id", "text": "Build", "state": "pending", "origin": "agent"},
			run: func(c *Client) (TodoItem, error) {
				return c.AddTodo(context.Background(), "ws-id", TodoItemInput{Text: "Build", State: "pending", Origin: "agent"})
			},
		},
		{
			name:       "set state",
			method:     "workspace.todo.set_state",
			wantParams: map[string]any{"workspace_id": "ws-id", "id": "todo-id", "state": "completed"},
			run: func(c *Client) (TodoItem, error) {
				return c.SetTodoState(context.Background(), "ws-id", "todo-id", "completed")
			},
		},
		{
			name:       "edit",
			method:     "workspace.todo.edit",
			wantParams: map[string]any{"workspace_id": "ws-id", "id": "todo-id", "text": "Build"},
			run: func(c *Client) (TodoItem, error) {
				return c.EditTodo(context.Background(), "ws-id", "todo-id", "Build")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRunner{stdout: itemResponse}
			got, err := tt.run(&Client{Runner: f})
			if err != nil {
				t.Fatalf("mutation error: %v", err)
			}
			assertRPCCall(t, f.lastArgs, tt.method, tt.wantParams)
			if got.ID != "todo-id" || got.State != "completed" {
				t.Fatalf("item = %+v", got)
			}
		})
	}
}

func TestClientRemoveTodo_RPCParams(t *testing.T) {
	f := &fakeRunner{stdout: []byte(`{"removed_count":1}`)}
	c := &Client{Runner: f}
	if err := c.RemoveTodo(context.Background(), "ws-id", "todo-id"); err != nil {
		t.Fatalf("RemoveTodo() error: %v", err)
	}
	assertRPCCall(t, f.lastArgs, "workspace.todo.remove", map[string]any{"workspace_id": "ws-id", "id": "todo-id"})
}

func TestClientNativeRPC_ErrorsIncludeMethod(t *testing.T) {
	t.Run("runner error", func(t *testing.T) {
		f := &fakeRunner{stderr: []byte("unsupported"), err: errors.New("exit status 1")}
		_, err := (&Client{Runner: f}).ListTodos(context.Background(), "ws-id")
		if err == nil || !containsAll(err.Error(), []string{"cmux rpc workspace.todo.list", "unsupported"}) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("decode error", func(t *testing.T) {
		f := &fakeRunner{stdout: []byte("not-json")}
		_, err := (&Client{Runner: f}).ListTodos(context.Background(), "ws-id")
		if err == nil || !strings.Contains(err.Error(), "decode cmux rpc workspace.todo.list") {
			t.Fatalf("error = %v", err)
		}
	})
}

func assertRPCCall(t *testing.T, args []string, method string, wantParams map[string]any) {
	t.Helper()
	if len(args) != 3 || args[0] != "rpc" || args[1] != method {
		t.Fatalf("args = %v, want rpc %s <JSON>", args, method)
	}
	got := decodeRPCParams(t, args[2])
	if !reflect.DeepEqual(got, wantParams) {
		t.Fatalf("params = %#v, want %#v", got, wantParams)
	}
}

func decodeRPCParams(t *testing.T, raw string) map[string]any {
	t.Helper()
	var params map[string]any
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatalf("decode RPC params %q: %v", raw, err)
	}
	return params
}

func assertJSONValue(t *testing.T, object map[string]any, key string, want any) {
	t.Helper()
	if got := object[key]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v (object: %#v)", key, got, want, object)
	}
}
