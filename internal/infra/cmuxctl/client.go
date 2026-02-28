package cmuxctl

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return stdout, ee.Stderr, err
		}
		return stdout, nil, err
	}
	return stdout, nil, nil
}

type Client struct {
	Runner     Runner
	SocketPath string
	Password   string
}

type Capabilities struct {
	Methods map[string]struct{}
}

type Workspace struct {
	ID       string
	Ref      string
	Index    int
	Title    string
	Selected bool
}

type Pane struct {
	ID      string
	Ref     string
	Index   int
	Focused bool
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

type Notification struct {
	WorkspaceID string
	SurfaceID   string
	Title       string
	Subtitle    string
	Body        string
	CreatedAt   int64
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
	if err := c.runJSON(ctx, &payload, "list-workspaces"); err != nil {
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
	args := []string{"new-workspace"}
	if command != "" {
		args = append(args, "--command", command)
	}
	stdout, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return "", commandError("new-workspace", stderr, err)
	}
	raw := strings.TrimSpace(string(stdout))
	if !strings.HasPrefix(raw, "OK ") {
		return "", fmt.Errorf("cmux new-workspace: unexpected response: %q", raw)
	}
	id := strings.TrimSpace(strings.TrimPrefix(raw, "OK "))
	if id == "" {
		return "", fmt.Errorf("cmux new-workspace: empty workspace id")
	}
	return id, nil
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
	_, stderr, err := c.run(ctx, false, false, "rename-workspace", "--workspace", workspace, title)
	if err != nil {
		return commandError("rename-workspace", stderr, err)
	}
	return nil
}

func (c *Client) SelectWorkspace(ctx context.Context, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, stderr, err := c.run(ctx, false, false, "select-workspace", "--workspace", workspace)
	if err != nil {
		return commandError("select-workspace", stderr, err)
	}
	return nil
}

func (c *Client) SetStatus(ctx context.Context, workspace string, label string, text string, icon string, color string) error {
	workspace = strings.TrimSpace(workspace)
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	icon = strings.TrimSpace(icon)
	color = strings.TrimSpace(color)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if label == "" {
		return fmt.Errorf("label is required")
	}
	if text == "" {
		return fmt.Errorf("text is required")
	}
	args := []string{"set-status", label, text, "--workspace", workspace}
	if icon != "" {
		args = append(args, "--icon", icon)
	}
	if color != "" {
		args = append(args, "--color", color)
	}
	_, stderr, err := c.run(ctx, false, false, args...)
	if err != nil {
		return commandError("set-status", stderr, err)
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

func (c *Client) ListPanes(ctx context.Context, workspace string) ([]Pane, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	var payload struct {
		Panes []struct {
			ID      string `json:"id"`
			Ref     string `json:"ref"`
			Index   int    `json:"index"`
			Focused bool   `json:"focused"`
		} `json:"panes"`
	}
	if err := c.runJSON(ctx, &payload, "list-panes", "--workspace", workspace); err != nil {
		return nil, err
	}
	out := make([]Pane, 0, len(payload.Panes))
	for _, row := range payload.Panes {
		out = append(out, Pane{
			ID:      strings.TrimSpace(row.ID),
			Ref:     strings.TrimSpace(row.Ref),
			Index:   row.Index,
			Focused: row.Focused,
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
