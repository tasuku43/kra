package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type cmuxOpenClient interface {
	Capabilities(ctx context.Context) (cmuxctl.Capabilities, error)
	CreateWorkspace(ctx context.Context) (string, error)
	RenameWorkspace(ctx context.Context, workspace string, title string) error
	SelectWorkspace(ctx context.Context, workspace string) error
	SendText(ctx context.Context, workspace string, surface string, text string) error
}

var newCMUXOpenClient = func() cmuxOpenClient { return cmuxctl.NewClient() }
var newCMUXMapStore = func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) }

func (c *CLI) runCMUXOpen(args []string) int {
	outputFormat := "human"
	workspaceID := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXOpenUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printCMUXOpenUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for cmux open: %q\n", args[0])
			c.printCMUXOpenUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printCMUXOpenUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for cmux open: %q\n", strings.Join(args[1:], " "))
		c.printCMUXOpenUsage(c.Err)
		return exitUsage
	}
	if len(args) == 1 {
		workspaceID = strings.TrimSpace(args[0])
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeCMUXOpenError(outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}

	if workspaceID == "" {
		if outputFormat == "json" {
			return c.writeCMUXOpenError(outputFormat, "invalid_argument", "", "workspace id is required in --format json mode", exitUsage)
		}
		selectedID, err := c.selectWorkspaceForCMUXOpen(root)
		if err != nil {
			return c.writeCMUXOpenError(outputFormat, "workspace_not_found", "", err.Error(), exitError)
		}
		workspaceID = selectedID
	}

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	fi, err := os.Stat(wsPath)
	if err != nil || !fi.IsDir() {
		return c.writeCMUXOpenError(outputFormat, "workspace_not_found", workspaceID, fmt.Sprintf("workspace not found: %s", workspaceID), exitError)
	}

	title := ""
	if meta, err := loadWorkspaceMetaFile(wsPath); err == nil {
		title = strings.TrimSpace(meta.Workspace.Title)
	}

	client := newCMUXOpenClient()
	ctx := context.Background()
	caps, err := client.Capabilities(ctx)
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "cmux_capability_missing", workspaceID, fmt.Sprintf("read cmux capabilities: %v", err), exitError)
	}
	required := []string{"workspace.create", "workspace.rename", "workspace.select", "surface.send_text"}
	for _, method := range required {
		if _, ok := caps.Methods[method]; !ok {
			return c.writeCMUXOpenError(outputFormat, "cmux_capability_missing", workspaceID, fmt.Sprintf("cmux capability missing: %s", method), exitError)
		}
	}

	cmuxWorkspaceID, err := client.CreateWorkspace(ctx)
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "cmux_create_failed", workspaceID, fmt.Sprintf("create cmux workspace: %v", err), exitError)
	}

	store := newCMUXMapStore(root)
	mapping, err := store.Load()
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "state_write_failed", workspaceID, fmt.Sprintf("load cmux mapping: %v", err), exitError)
	}
	ordinal, err := cmuxmap.AllocateOrdinal(&mapping, workspaceID)
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "state_write_failed", workspaceID, fmt.Sprintf("allocate cmux ordinal: %v", err), exitError)
	}
	cmuxTitle, err := cmuxmap.FormatWorkspaceTitle(workspaceID, title, ordinal)
	if err != nil {
		return c.writeCMUXOpenError(outputFormat, "cmux_rename_failed", workspaceID, fmt.Sprintf("format cmux workspace title: %v", err), exitError)
	}

	if err := client.RenameWorkspace(ctx, cmuxWorkspaceID, cmuxTitle); err != nil {
		return c.writeCMUXOpenError(outputFormat, "cmux_rename_failed", workspaceID, fmt.Sprintf("rename cmux workspace: %v", err), exitError)
	}
	if err := client.SelectWorkspace(ctx, cmuxWorkspaceID); err != nil {
		return c.writeCMUXOpenError(outputFormat, "cmux_select_failed", workspaceID, fmt.Sprintf("select cmux workspace: %v", err), exitError)
	}
	if err := client.SendText(ctx, cmuxWorkspaceID, "", fmt.Sprintf("cd %s\n", shellQuoteSingle(wsPath))); err != nil {
		return c.writeCMUXOpenError(outputFormat, "cmux_cwd_sync_failed", workspaceID, fmt.Sprintf("sync cmux cwd: %v", err), exitError)
	}

	ws := mapping.Workspaces[workspaceID]
	now := time.Now().UTC().Format(time.RFC3339)
	ws.Entries = append(ws.Entries, cmuxmap.Entry{
		CMUXWorkspaceID: cmuxWorkspaceID,
		Ordinal:         ordinal,
		TitleSnapshot:   cmuxTitle,
		CreatedAt:       now,
		LastUsedAt:      now,
	})
	mapping.Workspaces[workspaceID] = ws
	if err := store.Save(mapping); err != nil {
		return c.writeCMUXOpenError(outputFormat, "state_write_failed", workspaceID, fmt.Sprintf("save cmux mapping: %v", err), exitError)
	}

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "cmux.open",
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"kra_workspace_id":   workspaceID,
				"kra_workspace_path": wsPath,
				"cmux_workspace_id":  cmuxWorkspaceID,
				"ordinal":            ordinal,
				"title":              cmuxTitle,
				"cwd_synced":         true,
			},
		})
		return exitOK
	}

	fmt.Fprintln(c.Out, "opened cmux workspace")
	fmt.Fprintf(c.Out, "  kra: %s\n", workspaceID)
	fmt.Fprintf(c.Out, "  cmux: %s\n", cmuxWorkspaceID)
	fmt.Fprintf(c.Out, "  title: %s\n", cmuxTitle)
	fmt.Fprintf(c.Out, "  cwd: %s\n", wsPath)
	return exitOK
}

func (c *CLI) selectWorkspaceForCMUXOpen(root string) (string, error) {
	candidates, err := listWorkspaceCandidatesByStatus(context.Background(), root, "active")
	if err != nil {
		return "", fmt.Errorf("list workspaces: %w", err)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no active workspaces available")
	}
	ids, err := c.promptWorkspaceSelectorSingle("active", "open", candidates)
	if err != nil {
		return "", err
	}
	if len(ids) != 1 {
		return "", fmt.Errorf("cmux open requires exactly one workspace selected")
	}
	return strings.TrimSpace(ids[0]), nil
}

func (c *CLI) writeCMUXOpenError(format string, code string, workspaceID string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "cmux.open",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	if workspaceID != "" {
		fmt.Fprintf(c.Err, "cmux open (%s): %s\n", workspaceID, message)
	} else {
		fmt.Fprintf(c.Err, "cmux open: %s\n", message)
	}
	return exitCode
}

func shellQuoteSingle(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
