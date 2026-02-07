package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

var errWSGoSingleSelectionRequired = errors.New("ws go requires exactly one workspace selected")

func (c *CLI) runWSGo(args []string) int {
	var archivedScope bool
	var emitCD bool
	var uiOutput bool
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSGoUsage(c.Out)
			return exitOK
		case "--archived":
			archivedScope = true
			args = args[1:]
		case "--emit-cd":
			emitCD = true
			args = args[1:]
		case "--ui":
			uiOutput = true
			args = args[1:]
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws go: %q\n", args[0])
			c.printWSGoUsage(c.Err)
			return exitUsage
		}
	}

	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws go: %q\n", strings.Join(args[1:], " "))
		c.printWSGoUsage(c.Err)
		return exitUsage
	}
	if uiOutput && emitCD {
		fmt.Fprintln(c.Err, "--ui and --emit-cd cannot be used together")
		c.printWSGoUsage(c.Err)
		return exitUsage
	}

	directWorkspaceID := ""
	if len(args) == 1 {
		directWorkspaceID = args[0]
		if err := validateWorkspaceID(directWorkspaceID); err != nil {
			fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
			return exitUsage
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-go"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws go args=%q archived=%t emitCD=%t ui=%t", args, archivedScope, emitCD, uiOutput)

	ctx := context.Background()
	dbPath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve state db path: %v\n", err)
		return exitError
	}
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		fmt.Fprintf(c.Err, "resolve repo pool path: %v\n", err)
		return exitError
	}

	db, err := statestore.Open(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(c.Err, "open state store: %v\n", err)
		return exitError
	}
	defer func() { _ = db.Close() }()

	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		fmt.Fprintf(c.Err, "initialize settings: %v\n", err)
		return exitError
	}

	scope := "active"
	if archivedScope {
		scope = "archived"
	}
	useColorOut := writerSupportsColor(c.Out)
	selectedTargetPath := ""

	flow := workspaceSelectRiskResultFlowConfig{
		FlowName: "ws go",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			if directWorkspaceID != "" {
				selected := []workspaceFlowSelection{{ID: directWorkspaceID}}
				c.debugf("ws go direct mode selected=%v", workspaceFlowSelectionIDs(selected))
				return selected, nil
			}

			candidates, err := listWorkspaceCandidatesByStatus(ctx, db, scope)
			if err != nil {
				return nil, fmt.Errorf("list %s workspaces: %w", scope, err)
			}
			if len(candidates) == 0 {
				if scope == "archived" {
					return nil, errNoArchivedWorkspaces
				}
				return nil, errNoActiveWorkspaces
			}

			ids, err := c.promptWorkspaceSelectorSingle(scope, "go", candidates)
			if err != nil {
				return nil, err
			}
			c.debugf("ws go selector mode selected=%v", ids)
			return []workspaceFlowSelection{{ID: ids[0]}}, nil
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			targetPath, err := resolveWorkspaceGoTarget(ctx, db, root, scope, item.ID)
			if err != nil {
				return err
			}
			selectedTargetPath = targetPath
			return nil
		},
		PrintResult: func(done []string, total int, useColor bool) {
			if !uiOutput {
				return
			}
			printResultSection(c.Out, useColor, fmt.Sprintf("Destination: %s", selectedTargetPath))
		},
	}

	_, err = c.runWorkspaceSelectRiskResultFlow(flow, useColorOut)
	if err != nil {
		switch {
		case errors.Is(err, errNoActiveWorkspaces):
			fmt.Fprintln(c.Err, "no active workspaces available")
			return exitError
		case errors.Is(err, errNoArchivedWorkspaces):
			fmt.Fprintln(c.Err, "no archived workspaces available")
			return exitError
		case errors.Is(err, errSelectorCanceled):
			c.debugf("ws go selector canceled")
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		case errors.Is(err, errWSGoSingleSelectionRequired):
			fmt.Fprintf(c.Err, "ws go: %v\n", errWSGoSingleSelectionRequired)
			return exitError
		default:
			fmt.Fprintf(c.Err, "run ws go flow: %v\n", err)
			return exitError
		}
	}

	if !uiOutput {
		fmt.Fprintf(c.Out, "cd %s\n", shellSingleQuote(selectedTargetPath))
	}
	c.debugf("ws go destination=%s", selectedTargetPath)
	return exitOK
}

func listWorkspaceCandidatesByStatus(ctx context.Context, db *sql.DB, status string) ([]workspaceSelectorCandidate, error) {
	items, err := statestore.ListWorkspaces(ctx, db)
	if err != nil {
		return nil, err
	}

	out := make([]workspaceSelectorCandidate, 0, len(items))
	for _, it := range items {
		if it.Status != status {
			continue
		}
		out = append(out, workspaceSelectorCandidate{
			ID:          it.ID,
			Description: strings.TrimSpace(it.Description),
		})
	}
	return out, nil
}

func resolveWorkspaceGoTarget(ctx context.Context, db *sql.DB, root string, scope string, workspaceID string) (string, error) {
	status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, workspaceID)
	if err != nil {
		return "", fmt.Errorf("load workspace: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("workspace not found: %s", workspaceID)
	}
	if status != scope {
		return "", fmt.Errorf("workspace is not %s (status=%s): %s", scope, status, workspaceID)
	}

	targetPath := filepath.Join(root, "workspaces", workspaceID)
	if scope == "archived" {
		targetPath = filepath.Join(root, "archive", workspaceID)
	}
	fi, err := os.Stat(targetPath)
	if err != nil {
		return "", fmt.Errorf("stat target path: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("target path is not a directory: %s", targetPath)
	}
	return targetPath, nil
}

func shellSingleQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
