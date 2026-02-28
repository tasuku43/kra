package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type cmuxListClient = cmuxRuntimeClient

var newCMUXListClient = func() cmuxListClient { return newCMUXRuntimeClient() }

func (c *CLI) runCMUXList(args []string) int {
	outputFormat := "human"
	workspaceID := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXListUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printCMUXListUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printCMUXListUsage(c.Err)
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
			fmt.Fprintf(c.Err, "unknown flag for cmux list: %q\n", args[0])
			c.printCMUXListUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printCMUXListUsage(c.Err)
		return exitUsage
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for cmux list: %q\n", strings.Join(args, " "))
		c.printCMUXListUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeCMUXSimpleError("cmux.list", outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeCMUXSimpleError("cmux.list", outputFormat, "internal_error", workspaceID, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeCMUXSimpleError("cmux.list", outputFormat, "internal_error", workspaceID, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}

	store := newCMUXMapStore(root)
	mapping, err := store.Load()
	if err != nil {
		return c.writeCMUXSimpleError("cmux.list", outputFormat, "internal_error", workspaceID, fmt.Sprintf("load cmux mapping: %v", err), exitError)
	}

	runtimeChecked := false
	prunedCount := 0
	runtimeErr := ""
	if cmuxList, listErr := newCMUXListClient().ListWorkspaces(context.Background()); listErr != nil {
		runtimeErr = fmt.Sprintf("list cmux workspaces: %v", listErr)
	} else {
		runtimeChecked = true
		reconciled, _, pruned, recErr := reconcileCMUXMappingWithRuntime(store, mapping, cmuxList, true)
		if recErr != nil {
			return c.writeCMUXSimpleError("cmux.list", outputFormat, "internal_error", workspaceID, fmt.Sprintf("save cmux mapping: %v", recErr), exitError)
		}
		mapping = reconciled
		prunedCount = pruned
	}

	type row struct {
		WorkspaceID string `json:"workspace_id"`
		CMUXID      string `json:"cmux_workspace_id"`
		Ordinal     int    `json:"ordinal"`
		Title       string `json:"title"`
		LastUsedAt  string `json:"last_used_at,omitempty"`
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
				LastUsedAt:  e.LastUsedAt,
			})
		}
	}

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "cmux.list",
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"items":           rows,
				"runtime_checked": runtimeChecked,
				"pruned_count":    prunedCount,
			},
		})
		return exitOK
	}
	if runtimeErr != "" {
		fmt.Fprintf(c.Err, "cmux list: %s (showing state only)\n", runtimeErr)
	}
	if prunedCount > 0 {
		fmt.Fprintf(c.Out, "pruned stale cmux mappings: %d\n", prunedCount)
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
		fmt.Fprintf(c.Out, "  [%d] %s  %s\n", r.Ordinal, r.CMUXID, r.Title)
	}
	return exitOK
}

func writeCMUXSimpleError(c *CLI, action string, format string, code string, workspaceID string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      action,
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	if workspaceID != "" {
		fmt.Fprintf(c.Err, "%s (%s): %s\n", strings.ReplaceAll(action, ".", " "), workspaceID, message)
	} else {
		fmt.Fprintf(c.Err, "%s: %s\n", strings.ReplaceAll(action, ".", " "), message)
	}
	return exitCode
}

func (c *CLI) writeCMUXSimpleError(action string, format string, code string, workspaceID string, message string, exitCode int) int {
	return writeCMUXSimpleError(c, action, format, code, workspaceID, message, exitCode)
}
