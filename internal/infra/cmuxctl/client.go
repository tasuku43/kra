package cmuxctl

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/procexec"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return procexec.RunOutput(ctx, "", name, cmuxCommandTimeout, args...)
}

const cmuxCommandTimeout = 15 * time.Second

type Client struct {
	Runner     Runner
	SocketPath string
	Password   string
}

type Capabilities struct {
	Methods map[string]struct{}
}

// Supports reports whether capabilities explicitly advertise method. Some
// cmux releases omit implemented Todo methods from this list; callers may use
// a read-only method probe as a fallback for those families.
func (c Capabilities) Supports(method string) bool {
	_, ok := c.Methods[strings.TrimSpace(method)]
	return ok
}

type Workspace struct {
	ID       string
	Ref      string
	Index    int
	Title    string
	Selected bool
}

type Window struct {
	ID                  string
	Index               int
	Key                 bool
	SelectedWorkspaceID string
	WorkspaceCount      int
}

type WorkspaceCreateOptions struct {
	OperationID               string
	Title                     string
	CWD                       string
	Focus                     bool
	WindowID                  string
	GroupID                   string
	GroupPlacement            string
	GroupReferenceWorkspaceID string
}

type WorkspaceCreateResult struct {
	WorkspaceID  string
	WorkspaceRef string
	WindowID     string
	WindowRef    string
	GroupID      string
	GroupRef     string
	SurfaceID    string
	SurfaceRef   string
}

type WorkspaceGroupPreferences struct {
	Collapsed   bool
	Pinned      bool
	CustomColor string
	IconSymbol  string
}

type WorkspaceGroup struct {
	ID                  string
	Ref                 string
	Name                string
	WindowID            string
	WindowRef           string
	AnchorWorkspaceID   string
	AnchorWorkspaceRef  string
	MemberWorkspaceIDs  []string
	MemberWorkspaceRefs []string
	MemberCount         int
	Preferences         WorkspaceGroupPreferences
}

type WorkspaceGroupList struct {
	WindowID  string
	WindowRef string
	Groups    []WorkspaceGroup
}

type WorkspaceGroupCreateOptions struct {
	Name              string
	CWD               string
	Window            string
	ChildWorkspaceIDs []string
}

type TodoItem struct {
	ID     string `json:"id"`
	Index  int    `json:"index"`
	Text   string `json:"text"`
	State  string `json:"state"`
	Origin string `json:"origin"`
}

type TodoItemInput struct {
	Text   string `json:"text"`
	State  string `json:"state,omitempty"`
	Origin string `json:"origin,omitempty"`
}

type TodoProgress struct {
	Completed          int    `json:"completed"`
	Total              int    `json:"total"`
	FirstUncheckedText string `json:"first_unchecked_text"`
}

type TodoList struct {
	WorkspaceID  string       `json:"workspace_id"`
	WorkspaceRef string       `json:"workspace_ref"`
	WindowID     string       `json:"window_id"`
	WindowRef    string       `json:"window_ref"`
	Items        []TodoItem   `json:"items"`
	Progress     TodoProgress `json:"progress"`
}

type Pane struct {
	ID                 string
	Ref                string
	Index              int
	Focused            bool
	SelectedSurfaceID  string
	SelectedSurfaceRef string
}

type Surface struct {
	ID       string
	Ref      string
	Index    int
	Title    string
	Type     string
	Selected bool
	PaneID   string
}

type PaneCreateResult struct {
	WorkspaceRef string
	WindowRef    string
	PaneRef      string
	SurfaceRef   string
	Type         string
}

type MarkdownOpenResult struct {
	WorkspaceRef     string
	WindowRef        string
	PaneRef          string
	SurfaceRef       string
	SourcePaneRef    string
	SourceSurfaceRef string
	TargetPaneRef    string
	Path             string
}

type MoveSurfaceResult struct {
	WorkspaceRef string
	WindowRef    string
	PaneRef      string
	SurfaceRef   string
}

type Notification struct {
	WorkspaceID string
	SurfaceID   string
	Title       string
	Subtitle    string
	Body        string
	CreatedAt   int64
}

type NotifyOptions struct {
	Title     string
	Subtitle  string
	Body      string
	Workspace string
	Surface   string
}

func NewClient() *Client {
	return &Client{Runner: execRunner{}}
}

func (c *Client) Capabilities(ctx context.Context) (Capabilities, error) {
	var payload struct {
		Methods []string `json:"methods"`
	}
	if err := c.runJSON(ctx, &payload, "capabilities"); err != nil {
		return Capabilities{}, err
	}
	methods := make(map[string]struct{}, len(payload.Methods))
	for _, m := range payload.Methods {
		methods[strings.TrimSpace(m)] = struct{}{}
	}
	return Capabilities{Methods: methods}, nil
}

func (c *Client) Identify(ctx context.Context, workspace string, surface string) (map[string]any, error) {
	args := []string{"identify"}
	if strings.TrimSpace(workspace) != "" {
		args = append(args, "--workspace", strings.TrimSpace(workspace))
	}
	if strings.TrimSpace(surface) != "" {
		args = append(args, "--surface", strings.TrimSpace(surface))
	}
	var payload map[string]any
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) ListWindows(ctx context.Context) ([]Window, error) {
	var payload []struct {
		ID                  string `json:"id"`
		Index               int    `json:"index"`
		Key                 bool   `json:"key"`
		SelectedWorkspaceID string `json:"selected_workspace_id"`
		WorkspaceCount      int    `json:"workspace_count"`
	}
	if err := c.runJSON(ctx, &payload, "list-windows"); err != nil {
		return nil, err
	}
	out := make([]Window, 0, len(payload))
	for _, row := range payload {
		out = append(out, Window{
			ID:                  strings.TrimSpace(row.ID),
			Index:               row.Index,
			Key:                 row.Key,
			SelectedWorkspaceID: strings.TrimSpace(row.SelectedWorkspaceID),
			WorkspaceCount:      row.WorkspaceCount,
		})
	}
	return out, nil
}

func (c *Client) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	var payload struct {
		Workspaces []struct {
			ID       string `json:"id"`
			Ref      string `json:"ref"`
			Index    int    `json:"index"`
			Title    string `json:"title"`
			Selected bool   `json:"selected"`
		} `json:"workspaces"`
	}
	if err := c.runJSON(ctx, &payload, "workspace", "list"); err != nil {
		return nil, err
	}
	out := make([]Workspace, 0, len(payload.Workspaces))
	for _, row := range payload.Workspaces {
		out = append(out, Workspace{
			ID:       strings.TrimSpace(row.ID),
			Ref:      strings.TrimSpace(row.Ref),
			Index:    row.Index,
			Title:    strings.TrimSpace(row.Title),
			Selected: row.Selected,
		})
	}
	if len(out) > 0 {
		return out, nil
	}
	if fallback, err := c.listWorkspacesFromTree(ctx); err == nil && len(fallback) > 0 {
		return fallback, nil
	}
	return out, nil
}

func (c *Client) listWorkspacesFromTree(ctx context.Context) ([]Workspace, error) {
	var payload struct {
		Windows []struct {
			Workspaces []struct {
				ID       string `json:"id"`
				Ref      string `json:"ref"`
				Index    int    `json:"index"`
				Title    string `json:"title"`
				Selected bool   `json:"selected"`
				Current  bool   `json:"current"`
				Active   bool   `json:"active"`
			} `json:"workspaces"`
		} `json:"windows"`
	}
	if err := c.runJSON(ctx, &payload, "tree", "--all"); err != nil {
		return nil, err
	}
	out := make([]Workspace, 0)
	seen := make(map[string]struct{})
	for _, window := range payload.Windows {
		for _, row := range window.Workspaces {
			id := strings.TrimSpace(row.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, Workspace{
				ID:       id,
				Ref:      strings.TrimSpace(row.Ref),
				Index:    row.Index,
				Title:    strings.TrimSpace(row.Title),
				Selected: row.Selected || row.Current || row.Active,
			})
		}
	}
	return out, nil
}

func (c *Client) CreateWorkspace(ctx context.Context) (string, error) {
	return c.CreateWorkspaceWithCommand(ctx, "")
}

func (c *Client) CreateWorkspaceWithCommand(ctx context.Context, command string) (string, error) {
	command = strings.TrimSpace(command)
	args := []string{"workspace", "create"}
	if command != "" {
		args = append(args, "--command", command)
	}
	stdout, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return "", commandError("workspace create", stderr, err)
	}
	raw := strings.TrimSpace(string(stdout))
	if !strings.HasPrefix(raw, "OK ") {
		return "", fmt.Errorf("cmux workspace create: unexpected response: %q", raw)
	}
	id := strings.TrimSpace(strings.TrimPrefix(raw, "OK "))
	if id == "" {
		return "", fmt.Errorf("cmux workspace create: empty workspace id")
	}
	return id, nil
}

func (c *Client) CreateWorkspaceWithOptions(ctx context.Context, opts WorkspaceCreateOptions) (WorkspaceCreateResult, error) {
	operationID := strings.TrimSpace(opts.OperationID)
	title := strings.TrimSpace(opts.Title)
	cwd := strings.TrimSpace(opts.CWD)
	windowID := strings.TrimSpace(opts.WindowID)
	groupID := strings.TrimSpace(opts.GroupID)
	groupPlacement := strings.TrimSpace(opts.GroupPlacement)
	groupReferenceWorkspaceID := strings.TrimSpace(opts.GroupReferenceWorkspaceID)
	if groupID == "" && (groupPlacement != "" || groupReferenceWorkspaceID != "") {
		return WorkspaceCreateResult{}, fmt.Errorf("group id is required for group placement")
	}
	params := struct {
		OperationID               string `json:"operation_id,omitempty"`
		Title                     string `json:"title,omitempty"`
		CWD                       string `json:"cwd,omitempty"`
		Focus                     bool   `json:"focus"`
		WindowID                  string `json:"window_id,omitempty"`
		GroupID                   string `json:"group_id,omitempty"`
		GroupPlacement            string `json:"group_placement,omitempty"`
		GroupReferenceWorkspaceID string `json:"group_reference_workspace_id,omitempty"`
	}{
		OperationID:               operationID,
		Title:                     title,
		CWD:                       cwd,
		Focus:                     opts.Focus,
		WindowID:                  windowID,
		GroupID:                   groupID,
		GroupPlacement:            groupPlacement,
		GroupReferenceWorkspaceID: groupReferenceWorkspaceID,
	}
	var payload struct {
		WorkspaceID  string `json:"workspace_id"`
		WorkspaceRef string `json:"workspace_ref"`
		WindowID     string `json:"window_id"`
		WindowRef    string `json:"window_ref"`
		GroupID      string `json:"group_id"`
		GroupRef     string `json:"group_ref"`
		SurfaceID    string `json:"surface_id"`
		SurfaceRef   string `json:"surface_ref"`
	}
	if err := c.runRPCJSON(ctx, &payload, "workspace.create", params); err != nil {
		return WorkspaceCreateResult{}, err
	}
	result := WorkspaceCreateResult{
		WorkspaceID:  strings.TrimSpace(payload.WorkspaceID),
		WorkspaceRef: strings.TrimSpace(payload.WorkspaceRef),
		WindowID:     strings.TrimSpace(payload.WindowID),
		WindowRef:    strings.TrimSpace(payload.WindowRef),
		GroupID:      strings.TrimSpace(payload.GroupID),
		GroupRef:     strings.TrimSpace(payload.GroupRef),
		SurfaceID:    strings.TrimSpace(payload.SurfaceID),
		SurfaceRef:   strings.TrimSpace(payload.SurfaceRef),
	}
	if result.WorkspaceID == "" && result.WorkspaceRef == "" {
		return WorkspaceCreateResult{}, fmt.Errorf("cmux rpc workspace.create: empty workspace id")
	}
	return result, nil
}

func (c *Client) RenameWorkspace(ctx context.Context, workspace string, title string) error {
	workspace = strings.TrimSpace(workspace)
	title = strings.TrimSpace(title)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if title == "" {
		return fmt.Errorf("title is required")
	}
	_, stderr, err := c.run(ctx, false, false, "workspace", "rename", workspace, "--title", title)
	if err != nil {
		return commandError("workspace rename", stderr, err)
	}
	return nil
}

func (c *Client) SelectWorkspace(ctx context.Context, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, stderr, err := c.run(ctx, false, false, "workspace", "select", workspace)
	if err != nil {
		return commandError("workspace select", stderr, err)
	}
	return nil
}

func (c *Client) CloseWorkspace(ctx context.Context, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, stderr, err := c.run(ctx, false, false, "workspace", "close", workspace)
	if err != nil {
		return commandError("workspace close", stderr, err)
	}
	return nil
}

type workspaceGroupJSON struct {
	ID                  string   `json:"id"`
	Ref                 string   `json:"ref"`
	Name                string   `json:"name"`
	Collapsed           bool     `json:"is_collapsed"`
	Pinned              bool     `json:"is_pinned"`
	AnchorWorkspaceID   string   `json:"anchor_workspace_id"`
	AnchorWorkspaceRef  string   `json:"anchor_workspace_ref"`
	CustomColor         string   `json:"custom_color"`
	IconSymbol          string   `json:"icon_symbol"`
	MemberWorkspaceIDs  []string `json:"member_workspace_ids"`
	MemberWorkspaceRefs []string `json:"member_workspace_refs"`
	MemberCount         int      `json:"member_count"`
}

func (c *Client) ListWorkspaceGroups(ctx context.Context, window string) (WorkspaceGroupList, error) {
	window = strings.TrimSpace(window)
	args := []string{"workspace", "group", "list"}
	if window != "" {
		args = append(args, "--window", window)
	}
	var payload struct {
		WindowID  string               `json:"window_id"`
		WindowRef string               `json:"window_ref"`
		Groups    []workspaceGroupJSON `json:"groups"`
	}
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return WorkspaceGroupList{}, err
	}
	result := WorkspaceGroupList{
		WindowID:  strings.TrimSpace(payload.WindowID),
		WindowRef: strings.TrimSpace(payload.WindowRef),
		Groups:    make([]WorkspaceGroup, 0, len(payload.Groups)),
	}
	for _, group := range payload.Groups {
		result.Groups = append(result.Groups, workspaceGroupFromJSON(group, result.WindowID, result.WindowRef))
	}
	return result, nil
}

func (c *Client) CreateWorkspaceGroup(ctx context.Context, opts WorkspaceGroupCreateOptions) (WorkspaceGroup, error) {
	name := strings.TrimSpace(opts.Name)
	cwd := strings.TrimSpace(opts.CWD)
	window := strings.TrimSpace(opts.Window)
	if cwd == "" {
		return WorkspaceGroup{}, fmt.Errorf("cwd is required")
	}
	children := make([]string, 0, len(opts.ChildWorkspaceIDs))
	for _, child := range opts.ChildWorkspaceIDs {
		child = strings.TrimSpace(child)
		if child == "" {
			return WorkspaceGroup{}, fmt.Errorf("child workspace id is required")
		}
		children = append(children, child)
	}
	args := []string{"workspace", "group", "create"}
	if name != "" {
		args = append(args, "--name", name)
	}
	args = append(args, "--cwd", cwd)
	if len(children) > 0 {
		args = append(args, "--from", strings.Join(children, ","))
	}
	if window != "" {
		args = append(args, "--window", window)
	}
	var payload struct {
		Group workspaceGroupJSON `json:"group"`
	}
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return WorkspaceGroup{}, err
	}
	group := workspaceGroupFromJSON(payload.Group, "", "")
	if group.ID == "" && group.Ref == "" {
		return WorkspaceGroup{}, fmt.Errorf("cmux workspace group create: empty group id")
	}
	return group, nil
}

func (c *Client) AddWorkspaceToGroup(ctx context.Context, group string, workspace string) error {
	group = strings.TrimSpace(group)
	workspace = strings.TrimSpace(workspace)
	if group == "" {
		return fmt.Errorf("group is required")
	}
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, stderr, err := c.run(
		ctx,
		false,
		false,
		"workspace", "group", "add", "--group", group, "--workspace", workspace,
	)
	if err != nil {
		return commandError("workspace group add", stderr, err)
	}
	return nil
}

func (c *Client) RemoveWorkspaceFromGroup(ctx context.Context, group string, workspace string) error {
	group = strings.TrimSpace(group)
	workspace = strings.TrimSpace(workspace)
	if group == "" {
		return fmt.Errorf("group is required")
	}
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, stderr, err := c.run(ctx, false, false, "workspace", "group", "remove", "--workspace", workspace)
	if err != nil {
		return commandError("workspace group remove", stderr, err)
	}
	return nil
}

func (c *Client) SetWorkspaceGroupAnchor(ctx context.Context, group string, workspace string) error {
	group = strings.TrimSpace(group)
	workspace = strings.TrimSpace(workspace)
	if group == "" {
		return fmt.Errorf("group is required")
	}
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, stderr, err := c.run(
		ctx,
		false,
		false,
		"workspace", "group", "set-anchor", "--group", group, "--workspace", workspace,
	)
	if err != nil {
		return commandError("workspace group set-anchor", stderr, err)
	}
	return nil
}

func (c *Client) UngroupWorkspaceGroup(ctx context.Context, group string) error {
	group = strings.TrimSpace(group)
	if group == "" {
		return fmt.Errorf("group is required")
	}
	_, stderr, err := c.run(ctx, false, false, "workspace", "group", "ungroup", group)
	if err != nil {
		return commandError("workspace group ungroup", stderr, err)
	}
	return nil
}

func (c *Client) FocusWorkspaceGroup(ctx context.Context, group string) error {
	group = strings.TrimSpace(group)
	if group == "" {
		return fmt.Errorf("group is required")
	}
	_, stderr, err := c.run(ctx, false, false, "workspace", "group", "focus", group)
	if err != nil {
		return commandError("workspace group focus", stderr, err)
	}
	return nil
}

func workspaceGroupFromJSON(group workspaceGroupJSON, windowID string, windowRef string) WorkspaceGroup {
	memberIDs := make([]string, len(group.MemberWorkspaceIDs))
	for i, id := range group.MemberWorkspaceIDs {
		memberIDs[i] = strings.TrimSpace(id)
	}
	memberRefs := make([]string, len(group.MemberWorkspaceRefs))
	for i, ref := range group.MemberWorkspaceRefs {
		memberRefs[i] = strings.TrimSpace(ref)
	}
	return WorkspaceGroup{
		ID:                  strings.TrimSpace(group.ID),
		Ref:                 strings.TrimSpace(group.Ref),
		Name:                strings.TrimSpace(group.Name),
		WindowID:            strings.TrimSpace(windowID),
		WindowRef:           strings.TrimSpace(windowRef),
		AnchorWorkspaceID:   strings.TrimSpace(group.AnchorWorkspaceID),
		AnchorWorkspaceRef:  strings.TrimSpace(group.AnchorWorkspaceRef),
		MemberWorkspaceIDs:  memberIDs,
		MemberWorkspaceRefs: memberRefs,
		MemberCount:         group.MemberCount,
		Preferences: WorkspaceGroupPreferences{
			Collapsed:   group.Collapsed,
			Pinned:      group.Pinned,
			CustomColor: strings.TrimSpace(group.CustomColor),
			IconSymbol:  strings.TrimSpace(group.IconSymbol),
		},
	}
}

func (c *Client) ListTodos(ctx context.Context, workspace string) (TodoList, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return TodoList{}, fmt.Errorf("workspace is required")
	}
	var payload TodoList
	if err := c.runRPCJSON(ctx, &payload, "workspace.todo.list", struct {
		WorkspaceID string `json:"workspace_id"`
	}{WorkspaceID: workspace}); err != nil {
		return TodoList{}, err
	}
	normalizeTodoList(&payload)
	return payload, nil
}

func (c *Client) AddTodo(ctx context.Context, workspace string, input TodoItemInput) (TodoItem, error) {
	workspace = strings.TrimSpace(workspace)
	text := strings.TrimSpace(input.Text)
	if workspace == "" {
		return TodoItem{}, fmt.Errorf("workspace is required")
	}
	if text == "" {
		return TodoItem{}, fmt.Errorf("todo text is required")
	}
	params := struct {
		WorkspaceID string `json:"workspace_id"`
		Text        string `json:"text"`
		State       string `json:"state,omitempty"`
		Origin      string `json:"origin,omitempty"`
	}{
		WorkspaceID: workspace,
		Text:        text,
		State:       strings.TrimSpace(input.State),
		Origin:      strings.TrimSpace(input.Origin),
	}
	return c.runTodoItemMutation(ctx, "workspace.todo.add", params)
}

func (c *Client) SetTodoState(ctx context.Context, workspace string, itemID string, state string) (TodoItem, error) {
	workspace = strings.TrimSpace(workspace)
	itemID = strings.TrimSpace(itemID)
	state = strings.TrimSpace(state)
	if workspace == "" {
		return TodoItem{}, fmt.Errorf("workspace is required")
	}
	if itemID == "" {
		return TodoItem{}, fmt.Errorf("todo item id is required")
	}
	if state == "" {
		return TodoItem{}, fmt.Errorf("todo state is required")
	}
	params := struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
		State       string `json:"state"`
	}{WorkspaceID: workspace, ID: itemID, State: state}
	return c.runTodoItemMutation(ctx, "workspace.todo.set_state", params)
}

func (c *Client) EditTodo(ctx context.Context, workspace string, itemID string, text string) (TodoItem, error) {
	workspace = strings.TrimSpace(workspace)
	itemID = strings.TrimSpace(itemID)
	text = strings.TrimSpace(text)
	if workspace == "" {
		return TodoItem{}, fmt.Errorf("workspace is required")
	}
	if itemID == "" {
		return TodoItem{}, fmt.Errorf("todo item id is required")
	}
	if text == "" {
		return TodoItem{}, fmt.Errorf("todo text is required")
	}
	params := struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
		Text        string `json:"text"`
	}{WorkspaceID: workspace, ID: itemID, Text: text}
	return c.runTodoItemMutation(ctx, "workspace.todo.edit", params)
}

func (c *Client) RemoveTodo(ctx context.Context, workspace string, itemID string) error {
	workspace = strings.TrimSpace(workspace)
	itemID = strings.TrimSpace(itemID)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if itemID == "" {
		return fmt.Errorf("todo item id is required")
	}
	params := struct {
		WorkspaceID string `json:"workspace_id"`
		ID          string `json:"id"`
	}{WorkspaceID: workspace, ID: itemID}
	var payload struct {
		RemovedCount int `json:"removed_count"`
	}
	return c.runRPCJSON(ctx, &payload, "workspace.todo.remove", params)
}

func (c *Client) runTodoItemMutation(ctx context.Context, method string, params any) (TodoItem, error) {
	var payload struct {
		Item TodoItem `json:"item"`
	}
	if err := c.runRPCJSON(ctx, &payload, method, params); err != nil {
		return TodoItem{}, err
	}
	payload.Item.ID = strings.TrimSpace(payload.Item.ID)
	payload.Item.Text = strings.TrimSpace(payload.Item.Text)
	payload.Item.State = strings.TrimSpace(payload.Item.State)
	payload.Item.Origin = strings.TrimSpace(payload.Item.Origin)
	if payload.Item.ID == "" {
		return TodoItem{}, fmt.Errorf("cmux rpc %s: empty todo item id", method)
	}
	return payload.Item, nil
}

func normalizeTodoList(list *TodoList) {
	list.WorkspaceID = strings.TrimSpace(list.WorkspaceID)
	list.WorkspaceRef = strings.TrimSpace(list.WorkspaceRef)
	list.WindowID = strings.TrimSpace(list.WindowID)
	list.WindowRef = strings.TrimSpace(list.WindowRef)
	list.Progress.FirstUncheckedText = strings.TrimSpace(list.Progress.FirstUncheckedText)
	for i := range list.Items {
		list.Items[i].ID = strings.TrimSpace(list.Items[i].ID)
		list.Items[i].Text = strings.TrimSpace(list.Items[i].Text)
		list.Items[i].State = strings.TrimSpace(list.Items[i].State)
		list.Items[i].Origin = strings.TrimSpace(list.Items[i].Origin)
	}
}

func (c *Client) Log(ctx context.Context, message string, level string, source string, workspace string) error {
	message = strings.TrimSpace(message)
	level = strings.TrimSpace(level)
	source = strings.TrimSpace(source)
	workspace = strings.TrimSpace(workspace)
	if message == "" {
		return fmt.Errorf("message is required")
	}
	args := []string{"log"}
	if level != "" {
		args = append(args, "--level", level)
	}
	if source != "" {
		args = append(args, "--source", source)
	}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	args = append(args, message)
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("log", stderr, err)
	}
	return nil
}

func (c *Client) SendText(ctx context.Context, workspace string, surface string, text string) error {
	workspace = strings.TrimSpace(workspace)
	surface = strings.TrimSpace(surface)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if text == "" {
		return fmt.Errorf("text is required")
	}
	args := []string{"send", "--workspace", workspace}
	if surface != "" {
		args = append(args, "--surface", surface)
	}
	args = append(args, text)
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("send", stderr, err)
	}
	return nil
}

func (c *Client) Notify(ctx context.Context, opts NotifyOptions) error {
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		return fmt.Errorf("title is required")
	}
	args := []string{"notify", "--title", title}
	if subtitle := strings.TrimSpace(opts.Subtitle); subtitle != "" {
		args = append(args, "--subtitle", subtitle)
	}
	if body := strings.TrimSpace(opts.Body); body != "" {
		args = append(args, "--body", body)
	}
	if workspace := strings.TrimSpace(opts.Workspace); workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	if surface := strings.TrimSpace(opts.Surface); surface != "" {
		args = append(args, "--surface", surface)
	}
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("notify", stderr, err)
	}
	return nil
}

func (c *Client) ListPanes(ctx context.Context, workspace string) ([]Pane, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	var payload struct {
		Panes []struct {
			ID                 string `json:"id"`
			Ref                string `json:"ref"`
			Index              int    `json:"index"`
			Focused            bool   `json:"focused"`
			SelectedSurfaceID  string `json:"selected_surface_id"`
			SelectedSurfaceRef string `json:"selected_surface_ref"`
		} `json:"panes"`
	}
	if err := c.runJSON(ctx, &payload, "list-panes", "--workspace", workspace); err != nil {
		return nil, err
	}
	out := make([]Pane, 0, len(payload.Panes))
	for _, row := range payload.Panes {
		out = append(out, Pane{
			ID:                 strings.TrimSpace(row.ID),
			Ref:                strings.TrimSpace(row.Ref),
			Index:              row.Index,
			Focused:            row.Focused,
			SelectedSurfaceID:  strings.TrimSpace(row.SelectedSurfaceID),
			SelectedSurfaceRef: strings.TrimSpace(row.SelectedSurfaceRef),
		})
	}
	return out, nil
}

func (c *Client) ListPaneSurfaces(ctx context.Context, workspace string, pane string) ([]Surface, error) {
	workspace = strings.TrimSpace(workspace)
	pane = strings.TrimSpace(pane)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	if pane == "" {
		return nil, fmt.Errorf("pane is required")
	}
	var payload struct {
		Surfaces []struct {
			ID       string `json:"id"`
			Ref      string `json:"ref"`
			Index    int    `json:"index"`
			Title    string `json:"title"`
			Type     string `json:"type"`
			Selected bool   `json:"selected"`
			PaneID   string `json:"pane_id"`
		} `json:"surfaces"`
	}
	if err := c.runJSON(ctx, &payload, "list-pane-surfaces", "--workspace", workspace, "--pane", pane); err != nil {
		return nil, err
	}
	out := make([]Surface, 0, len(payload.Surfaces))
	for _, row := range payload.Surfaces {
		out = append(out, Surface{
			ID:       strings.TrimSpace(row.ID),
			Ref:      strings.TrimSpace(row.Ref),
			Index:    row.Index,
			Title:    strings.TrimSpace(row.Title),
			Type:     strings.TrimSpace(row.Type),
			Selected: row.Selected,
			PaneID:   strings.TrimSpace(row.PaneID),
		})
	}
	return out, nil
}

func (c *Client) CreatePane(ctx context.Context, workspace string, direction string) (PaneCreateResult, error) {
	workspace = strings.TrimSpace(workspace)
	direction = strings.TrimSpace(direction)
	if workspace == "" {
		return PaneCreateResult{}, fmt.Errorf("workspace is required")
	}
	if direction == "" {
		direction = "right"
	}
	args := []string{"new-pane", "--workspace", workspace, "--direction", direction}
	var payload struct {
		WorkspaceRef string `json:"workspace_ref"`
		WindowRef    string `json:"window_ref"`
		PaneRef      string `json:"pane_ref"`
		SurfaceRef   string `json:"surface_ref"`
		Type         string `json:"type"`
	}
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return PaneCreateResult{}, err
	}
	return PaneCreateResult{
		WorkspaceRef: strings.TrimSpace(payload.WorkspaceRef),
		WindowRef:    strings.TrimSpace(payload.WindowRef),
		PaneRef:      strings.TrimSpace(payload.PaneRef),
		SurfaceRef:   strings.TrimSpace(payload.SurfaceRef),
		Type:         strings.TrimSpace(payload.Type),
	}, nil
}

func (c *Client) CreateSplit(ctx context.Context, direction string, workspace string, surface string) (PaneCreateResult, error) {
	direction = strings.TrimSpace(direction)
	workspace = strings.TrimSpace(workspace)
	surface = strings.TrimSpace(surface)
	if direction == "" {
		return PaneCreateResult{}, fmt.Errorf("direction is required")
	}
	args := []string{"new-split", direction}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	if surface != "" {
		args = append(args, "--surface", surface)
	}
	var payload struct {
		WorkspaceRef string `json:"workspace_ref"`
		WindowRef    string `json:"window_ref"`
		PaneRef      string `json:"pane_ref"`
		SurfaceRef   string `json:"surface_ref"`
		Type         string `json:"type"`
	}
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return PaneCreateResult{}, err
	}
	return PaneCreateResult{
		WorkspaceRef: strings.TrimSpace(payload.WorkspaceRef),
		WindowRef:    strings.TrimSpace(payload.WindowRef),
		PaneRef:      strings.TrimSpace(payload.PaneRef),
		SurfaceRef:   strings.TrimSpace(payload.SurfaceRef),
		Type:         strings.TrimSpace(payload.Type),
	}, nil
}

func (c *Client) MarkdownOpen(ctx context.Context, path string, workspace string, surface string) (MarkdownOpenResult, error) {
	path = strings.TrimSpace(path)
	workspace = strings.TrimSpace(workspace)
	surface = strings.TrimSpace(surface)
	if path == "" {
		return MarkdownOpenResult{}, fmt.Errorf("path is required")
	}
	args := []string{"markdown", "open", path}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	if surface != "" {
		args = append(args, "--surface", surface)
	}
	var payload struct {
		WorkspaceRef     string `json:"workspace_ref"`
		WindowRef        string `json:"window_ref"`
		PaneRef          string `json:"pane_ref"`
		SurfaceRef       string `json:"surface_ref"`
		SourcePaneRef    string `json:"source_pane_ref"`
		SourceSurfaceRef string `json:"source_surface_ref"`
		TargetPaneRef    string `json:"target_pane_ref"`
		Path             string `json:"path"`
	}
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return MarkdownOpenResult{}, err
	}
	return MarkdownOpenResult{
		WorkspaceRef:     strings.TrimSpace(payload.WorkspaceRef),
		WindowRef:        strings.TrimSpace(payload.WindowRef),
		PaneRef:          strings.TrimSpace(payload.PaneRef),
		SurfaceRef:       strings.TrimSpace(payload.SurfaceRef),
		SourcePaneRef:    strings.TrimSpace(payload.SourcePaneRef),
		SourceSurfaceRef: strings.TrimSpace(payload.SourceSurfaceRef),
		TargetPaneRef:    strings.TrimSpace(payload.TargetPaneRef),
		Path:             strings.TrimSpace(payload.Path),
	}, nil
}

func (c *Client) MoveSurface(ctx context.Context, surface string, pane string, workspace string, after string, focus bool) (MoveSurfaceResult, error) {
	surface = strings.TrimSpace(surface)
	pane = strings.TrimSpace(pane)
	workspace = strings.TrimSpace(workspace)
	after = strings.TrimSpace(after)
	if surface == "" {
		return MoveSurfaceResult{}, fmt.Errorf("surface is required")
	}
	if pane == "" {
		return MoveSurfaceResult{}, fmt.Errorf("pane is required")
	}
	args := []string{"move-surface", "--surface", surface, "--pane", pane}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	if after != "" {
		args = append(args, "--after", after)
	}
	args = append(args, "--focus", strconv.FormatBool(focus))
	var payload struct {
		WorkspaceRef string `json:"workspace_ref"`
		WindowRef    string `json:"window_ref"`
		PaneRef      string `json:"pane_ref"`
		SurfaceRef   string `json:"surface_ref"`
	}
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return MoveSurfaceResult{}, err
	}
	return MoveSurfaceResult{
		WorkspaceRef: strings.TrimSpace(payload.WorkspaceRef),
		WindowRef:    strings.TrimSpace(payload.WindowRef),
		PaneRef:      strings.TrimSpace(payload.PaneRef),
		SurfaceRef:   strings.TrimSpace(payload.SurfaceRef),
	}, nil
}

func (c *Client) RenameTab(ctx context.Context, workspace string, surface string, title string) error {
	workspace = strings.TrimSpace(workspace)
	surface = strings.TrimSpace(surface)
	title = strings.TrimSpace(title)
	if surface == "" {
		return fmt.Errorf("surface is required")
	}
	if title == "" {
		return fmt.Errorf("title is required")
	}
	args := []string{"rename-tab", "--surface", surface}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	args = append(args, "--title", title)
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("rename-tab", stderr, err)
	}
	return nil
}

func (c *Client) TabAction(ctx context.Context, workspace string, tab string, action string, title string, url string) error {
	tab = strings.TrimSpace(tab)
	action = strings.TrimSpace(action)
	title = strings.TrimSpace(title)
	url = strings.TrimSpace(url)
	workspace = strings.TrimSpace(workspace)
	if action == "" {
		return fmt.Errorf("action is required")
	}
	args := []string{"tab-action", "--action", action}
	if tab != "" {
		args = append(args, "--tab", tab)
	}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	if title != "" {
		args = append(args, "--title", title)
	}
	if url != "" {
		args = append(args, "--url", url)
	}
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("tab-action", stderr, err)
	}
	return nil
}

func (c *Client) CloseSurface(ctx context.Context, workspace string, surface string) error {
	workspace = strings.TrimSpace(workspace)
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return fmt.Errorf("surface is required")
	}
	args := []string{"close-surface", "--surface", surface}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("close-surface", stderr, err)
	}
	return nil
}

func (c *Client) FocusPane(ctx context.Context, pane string, workspace string) error {
	pane = strings.TrimSpace(pane)
	workspace = strings.TrimSpace(workspace)
	if pane == "" {
		return fmt.Errorf("pane is required")
	}
	args := []string{"focus-pane", "--pane", pane}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("focus-pane", stderr, err)
	}
	return nil
}

func (c *Client) BrowserStateSave(ctx context.Context, workspace string, surface string, path string) error {
	_ = strings.TrimSpace(workspace) // browser subcommands route by surface; keep arg for adapter symmetry.
	surface = strings.TrimSpace(surface)
	path = strings.TrimSpace(path)
	if surface == "" {
		return fmt.Errorf("surface is required")
	}
	if path == "" {
		return fmt.Errorf("path is required")
	}
	args := []string{"browser", "--surface", surface, "state", "save", path}
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("browser state save", stderr, err)
	}
	return nil
}

func (c *Client) BrowserStateLoad(ctx context.Context, workspace string, surface string, path string) error {
	_ = strings.TrimSpace(workspace) // browser subcommands route by surface; keep arg for adapter symmetry.
	surface = strings.TrimSpace(surface)
	path = strings.TrimSpace(path)
	if surface == "" {
		return fmt.Errorf("surface is required")
	}
	if path == "" {
		return fmt.Errorf("path is required")
	}
	args := []string{"browser", "--surface", surface, "state", "load", path}
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("browser state load", stderr, err)
	}
	return nil
}

func (c *Client) ReadScreen(ctx context.Context, workspace string, surface string, lines int, scrollback bool) (string, error) {
	workspace = strings.TrimSpace(workspace)
	surface = strings.TrimSpace(surface)
	if workspace == "" {
		return "", fmt.Errorf("workspace is required")
	}
	if surface == "" {
		return "", fmt.Errorf("surface is required")
	}
	if lines < 1 {
		lines = 120
	}
	args := []string{
		"read-screen",
		"--workspace", workspace,
		"--surface", surface,
		"--lines", strconv.Itoa(lines),
	}
	if scrollback {
		args = append(args, "--scrollback")
	}
	var payload map[string]any
	if err := c.runJSON(ctx, &payload, args...); err != nil {
		return "", err
	}
	if text, ok := payload["text"].(string); ok {
		return text, nil
	}
	return "", nil
}

func (c *Client) ListNotifications(ctx context.Context) ([]Notification, error) {
	var payload any
	if err := c.runJSON(ctx, &payload, "list-notifications"); err != nil {
		return nil, err
	}
	rawItems := extractNotificationItemsAny(payload)
	out := make([]Notification, 0, len(rawItems))
	for _, item := range rawItems {
		out = append(out, Notification{
			WorkspaceID: firstNonEmptyString(item, "workspace_id", "tab_id", "workspace", "tab"),
			SurfaceID:   firstNonEmptyString(item, "surface_id", "surface"),
			Title:       firstNonEmptyString(item, "title"),
			Subtitle:    firstNonEmptyString(item, "subtitle"),
			Body:        firstNonEmptyString(item, "body", "message", "text"),
			CreatedAt:   firstNonZeroInt64(item, "created_at", "timestamp", "time"),
		})
	}
	return out, nil
}

func extractNotificationItemsAny(payload any) []map[string]any {
	if rows, ok := payload.([]any); ok {
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			if m, ok := row.(map[string]any); ok {
				out = append(out, m)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	m, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	return extractNotificationItems(m)
}

func extractNotificationItems(payload map[string]any) []map[string]any {
	candidates := []string{"notifications", "items", "list", "events"}
	for _, key := range candidates {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		rows, ok := raw.([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			if m, ok := row.(map[string]any); ok {
				out = append(out, m)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func firstNonEmptyString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstNonZeroInt64(item map[string]any, keys ...string) int64 {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case float64:
			if v > 0 {
				return int64(v)
			}
		case int64:
			if v > 0 {
				return v
			}
		case int:
			if v > 0 {
				return int64(v)
			}
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

func (c *Client) runJSON(ctx context.Context, dst any, args ...string) error {
	stdout, stderr, err := c.run(ctx, true, true, args...)
	if err != nil && isUnsupportedIDFormatError(stderr, err) {
		stdout, stderr, err = c.run(ctx, true, false, args...)
	}
	if err != nil {
		return commandError(strings.Join(args, " "), stderr, err)
	}
	if err := json.Unmarshal(stdout, dst); err != nil {
		return fmt.Errorf("decode cmux json output: %w", err)
	}
	return nil
}

func (c *Client) runRPCJSON(ctx context.Context, dst any, method string, params any) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("rpc method is required")
	}
	if params == nil {
		params = struct{}{}
	}
	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode cmux rpc %s params: %w", method, err)
	}
	stdout, stderr, err := c.run(ctx, false, false, "rpc", method, string(rawParams))
	if err != nil {
		return commandError("rpc "+method, stderr, err)
	}
	if err := json.Unmarshal(stdout, dst); err != nil {
		return fmt.Errorf("decode cmux rpc %s json output: %w", method, err)
	}
	return nil
}

func (c *Client) run(ctx context.Context, jsonOutput bool, idFormatBoth bool, args ...string) ([]byte, []byte, error) {
	base := make([]string, 0, len(args)+8)
	if strings.TrimSpace(c.SocketPath) != "" {
		base = append(base, "--socket", strings.TrimSpace(c.SocketPath))
	}
	if strings.TrimSpace(c.Password) != "" {
		base = append(base, "--password", strings.TrimSpace(c.Password))
	}
	if jsonOutput {
		base = append(base, "--json")
		if idFormatBoth {
			base = append(base, "--id-format", "both")
		}
	}
	base = append(base, args...)
	r := c.Runner
	if r == nil {
		r = execRunner{}
	}
	return r.Run(ctx, "cmux", base...)
}

func isUnsupportedIDFormatError(stderr []byte, err error) bool {
	msg := strings.ToLower(strings.TrimSpace(string(stderr)))
	if msg == "" && err != nil {
		msg = strings.ToLower(err.Error())
	}
	if !strings.Contains(msg, "id-format") {
		return false
	}
	return strings.Contains(msg, "unknown option") ||
		strings.Contains(msg, "unrecognized option") ||
		strings.Contains(msg, "unknown flag")
}

func commandError(command string, stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		return fmt.Errorf("cmux %s: %w", command, err)
	}
	return fmt.Errorf("cmux %s: %s: %w", command, msg, err)
}
