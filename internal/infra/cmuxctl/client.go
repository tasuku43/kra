package cmuxctl

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	stdout, stderr, err := c.run(ctx, false, args...)
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
	_, stderr, err := c.run(ctx, false, "rename-workspace", "--workspace", workspace, title)
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
	_, stderr, err := c.run(ctx, false, "select-workspace", "--workspace", workspace)
	if err != nil {
		return commandError("select-workspace", stderr, err)
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
	_, stderr, err := c.run(ctx, false, args...)
	if err != nil {
		return commandError("send", stderr, err)
	}
	return nil
}

func (c *Client) runJSON(ctx context.Context, dst any, args ...string) error {
	stdout, stderr, err := c.run(ctx, true, args...)
	if err != nil {
		return commandError(strings.Join(args, " "), stderr, err)
	}
	if err := json.Unmarshal(stdout, dst); err != nil {
		return fmt.Errorf("decode cmux json output: %w", err)
	}
	return nil
}

func (c *Client) run(ctx context.Context, jsonOutput bool, args ...string) ([]byte, []byte, error) {
	base := make([]string, 0, len(args)+8)
	if strings.TrimSpace(c.SocketPath) != "" {
		base = append(base, "--socket", strings.TrimSpace(c.SocketPath))
	}
	if strings.TrimSpace(c.Password) != "" {
		base = append(base, "--password", strings.TrimSpace(c.Password))
	}
	if jsonOutput {
		base = append(base, "--json")
	}
	base = append(base, args...)
	r := c.Runner
	if r == nil {
		r = execRunner{}
	}
	return r.Run(ctx, "cmux", base...)
}

func commandError(command string, stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		return fmt.Errorf("cmux %s: %w", command, err)
	}
	return fmt.Errorf("cmux %s: %s: %w", command, msg, err)
}
