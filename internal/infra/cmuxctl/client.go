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

type StatusEntry struct {
	Key   string
	Value string
	Icon  string
	Color string
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

func (c *Client) CloseWorkspace(ctx context.Context, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	_, stderr, err := c.run(ctx, false, false, "close-workspace", "--workspace", workspace)
	if err != nil {
		return commandError("close-workspace", stderr, err)
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

func (c *Client) ClearStatus(ctx context.Context, workspace string, label string) error {
	workspace = strings.TrimSpace(workspace)
	label = strings.TrimSpace(label)
	if workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if label == "" {
		return fmt.Errorf("label is required")
	}
	_, stderr, err := c.run(ctx, false, false, "clear-status", label, "--workspace", workspace)
	if err != nil {
		return commandError("clear-status", stderr, err)
	}
	return nil
}

func (c *Client) ListStatus(ctx context.Context, workspace string) ([]StatusEntry, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	stdout, stderr, err := c.run(ctx, false, false, "list-status", "--workspace", workspace)
	if err != nil {
		return nil, commandError("list-status", stderr, err)
	}
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" || trimmed == "No status entries" {
		return nil, nil
	}
	lines := strings.Split(trimmed, "\n")
	entries := make([]StatusEntry, 0, len(lines))
	for _, line := range lines {
		entry, ok, perr := parseStatusEntryLine(line)
		if perr != nil {
			return nil, perr
		}
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
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

func (c *Client) CreateSurface(ctx context.Context, workspace string, pane string) (PaneCreateResult, error) {
	workspace = strings.TrimSpace(workspace)
	pane = strings.TrimSpace(pane)
	if workspace == "" {
		return PaneCreateResult{}, fmt.Errorf("workspace is required")
	}
	if pane == "" {
		return PaneCreateResult{}, fmt.Errorf("pane is required")
	}
	args := []string{"new-surface", "--workspace", workspace, "--pane", pane}
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

func parseStatusEntryLine(line string) (StatusEntry, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return StatusEntry{}, false, nil
	}
	key, rest, ok := strings.Cut(line, "=")
	if !ok {
		return StatusEntry{}, false, fmt.Errorf("parse list-status line: invalid entry: %q", line)
	}
	key = strings.TrimSpace(key)
	rest = strings.TrimSpace(rest)
	if key == "" || rest == "" {
		return StatusEntry{}, false, fmt.Errorf("parse list-status line: invalid entry: %q", line)
	}

	entry := StatusEntry{Key: key}
	cut := len(rest)
	for _, marker := range []string{" icon=", " color="} {
		if idx := strings.Index(rest, marker); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	entry.Value = strings.TrimSpace(rest[:cut])
	if entry.Value == "" {
		return StatusEntry{}, false, fmt.Errorf("parse list-status line: invalid entry: %q", line)
	}
	if cut < len(rest) {
		tokens := strings.Fields(strings.TrimSpace(rest[cut:]))
		for _, token := range tokens {
			switch {
			case strings.HasPrefix(token, "icon="):
				entry.Icon = strings.TrimSpace(strings.TrimPrefix(token, "icon="))
			case strings.HasPrefix(token, "color="):
				entry.Color = strings.TrimSpace(strings.TrimPrefix(token, "color="))
			}
		}
	}
	return entry, true, nil
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
