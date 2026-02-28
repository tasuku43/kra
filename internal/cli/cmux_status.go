package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type cmuxStatusClient interface {
	ListWorkspaces(ctx context.Context) ([]cmuxctl.Workspace, error)
}

var newCMUXStatusClient = func() cmuxStatusClient { return cmuxctl.NewClient() }

func (c *CLI) runCMUXStatus(args []string) int {
	outputFormat := "human"
	workspaceID := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXStatusUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printCMUXStatusUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printCMUXStatusUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--workspace=") {
				workspaceID = strings.TrimSpace(strings.TrimPrefix(args[0], "--workspace="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for cmux status: %q\n", args[0])
			c.printCMUXStatusUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printCMUXStatusUsage(c.Err)
		return exitUsage
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for cmux status: %q\n", strings.Join(args, " "))
		c.printCMUXStatusUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeCMUXSimpleError("cmux.status", outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeCMUXSimpleError("cmux.status", outputFormat, "internal_error", workspaceID, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeCMUXSimpleError("cmux.status", outputFormat, "internal_error", workspaceID, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}
	mapping, err := newCMUXMapStore(root).Load()
	if err != nil {
		return c.writeCMUXSimpleError("cmux.status", outputFormat, "internal_error", workspaceID, fmt.Sprintf("load cmux mapping: %v", err), exitError)
	}

	cmuxList, err := newCMUXStatusClient().ListWorkspaces(context.Background())
	if err != nil {
		return c.writeCMUXSimpleError("cmux.status", outputFormat, "cmux_list_failed", workspaceID, fmt.Sprintf("list cmux workspaces: %v", err), exitError)
	}
	exists := map[string]bool{}
	for _, row := range cmuxList {
		id := strings.TrimSpace(row.ID)
		if id != "" {
			exists[id] = true
		}
	}

	type row struct {
		WorkspaceID string `json:"workspace_id"`
		CMUXID      string `json:"cmux_workspace_id"`
		Ordinal     int    `json:"ordinal"`
		Title       string `json:"title"`
		Exists      bool   `json:"exists"`
	}
	rows := make([]row, 0)
	workspaceIDs := make([]string, 0, len(mapping.Workspaces))
	for wsID := range mapping.Workspaces {
		if workspaceID != "" && wsID != workspaceID {
			continue
		}
		workspaceIDs = append(workspaceIDs, wsID)
	}
	sort.Strings(workspaceIDs)
	for _, wsID := range workspaceIDs {
		ws := mapping.Workspaces[wsID]
		for _, e := range ws.Entries {
			rows = append(rows, row{
				WorkspaceID: wsID,
				CMUXID:      e.CMUXWorkspaceID,
				Ordinal:     e.Ordinal,
				Title:       e.TitleSnapshot,
				Exists:      exists[e.CMUXWorkspaceID],
			})
		}
	}

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "cmux.status",
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"items": rows,
			},
		})
		return exitOK
	}

	if len(rows) == 0 {
		fmt.Fprintln(c.Out, "no cmux mappings")
		return exitOK
	}
	current := ""
	for _, r := range rows {
		if r.WorkspaceID != current {
			current = r.WorkspaceID
			fmt.Fprintf(c.Out, "%s:\n", current)
		}
		status := "missing"
		if r.Exists {
			status = "ok"
		}
		fmt.Fprintf(c.Out, "  [%d] %s  %s  (%s)\n", r.Ordinal, r.CMUXID, r.Title, status)
	}
	return exitOK
}
