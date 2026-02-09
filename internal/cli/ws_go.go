package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/gionx/internal/infra/paths"
	"github.com/tasuku43/gionx/internal/infra/statestore"
)

var errWSGoSingleSelectionRequired = errors.New("ws go requires exactly one workspace selected")

func (c *CLI) runWSGo(args []string) int {
	var archivedScope bool
	var uiOutput bool
	outputFormat := "human"
	idFromFlag := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSGoUsage(c.Out)
			return exitOK
		case "--archived":
			archivedScope = true
			args = args[1:]
		case "--ui":
			uiOutput = true
			args = args[1:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSGoUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--id":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSGoUsage(c.Err)
				return exitUsage
			}
			idFromFlag = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--id=") {
				idFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--id="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws go: %q\n", args[0])
			c.printWSGoUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSGoUsage(c.Err)
		return exitUsage
	}

	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws go: %q\n", strings.Join(args[1:], " "))
		c.printWSGoUsage(c.Err)
		return exitUsage
	}
	if idFromFlag != "" && len(args) == 1 {
		fmt.Fprintln(c.Err, "--id and positional <id> cannot be used together")
		c.printWSGoUsage(c.Err)
		return exitUsage
	}
	if idFromFlag == "" && len(args) == 0 {
		fmt.Fprintln(c.Err, "ws go requires --id <id> or positional <id>")
		c.printWSGoUsage(c.Err)
		return exitUsage
	}
	if len(args) == 0 && idFromFlag != "" {
		args = []string{idFromFlag}
	}
	if outputFormat == "json" && uiOutput {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "go",
			Error: &cliJSONError{
				Code:    "invalid_argument",
				Message: "--ui cannot be used with --format json",
			},
		})
		return exitUsage
	}

	directWorkspaceID := args[0]
	if err := validateWorkspaceID(directWorkspaceID); err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "go",
				WorkspaceID: directWorkspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("invalid workspace id: %v", err),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
		return exitUsage
	}
	if outputFormat == "json" {
		return c.runWSGoJSON(directWorkspaceID, archivedScope)
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
	c.debugf("run ws go args=%q archived=%t ui=%t", args, archivedScope, uiOutput)

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
			selected := []workspaceFlowSelection{{ID: directWorkspaceID}}
			c.debugf("ws go direct mode selected=%v", workspaceFlowSelectionIDs(selected))
			return selected, nil
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

	if err := emitShellActionCD(selectedTargetPath); err != nil {
		fmt.Fprintf(c.Err, "write shell action: %v\n", err)
		return exitError
	}

	c.debugf("ws go destination=%s", selectedTargetPath)
	return exitOK
}

func (c *CLI) runWSGoJSON(workspaceID string, archivedScope bool) int {
	wd, err := os.Getwd()
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("get working dir: %v", err),
			},
		})
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("resolve GIONX_ROOT: %v", err),
			},
		})
		return exitError
	}
	ctx := context.Background()
	dbPath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("resolve state db path: %v", err),
			},
		})
		return exitError
	}
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("resolve repo pool path: %v", err),
			},
		})
		return exitError
	}
	db, err := statestore.Open(ctx, dbPath)
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("open state store: %v", err),
			},
		})
		return exitError
	}
	defer func() { _ = db.Close() }()
	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("initialize settings: %v", err),
			},
		})
		return exitError
	}
	scope := "active"
	if archivedScope {
		scope = "archived"
	}
	targetPath, err := resolveWorkspaceGoTarget(ctx, db, root, scope, workspaceID)
	if err != nil {
		code := "internal_error"
		msg := err.Error()
		switch {
		case strings.Contains(msg, "workspace not found"):
			code = "workspace_not_found"
		case strings.Contains(msg, "workspace is not"):
			code = "conflict"
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: msg,
			},
		})
		return exitError
	}
	if err := emitShellActionCD(targetPath); err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "go",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("write shell action: %v", err),
			},
		})
		return exitError
	}
	_ = writeCLIJSON(c.Out, cliJSONResponse{
		OK:          true,
		Action:      "go",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"target_path": targetPath,
		},
	})
	return exitOK
}

func listWorkspaceCandidatesByStatus(ctx context.Context, db *sql.DB, root string, status string) ([]workspaceSelectorCandidate, error) {
	items, err := statestore.ListWorkspaces(ctx, db)
	if err != nil {
		return nil, err
	}

	out := make([]workspaceSelectorCandidate, 0, len(items))
	for _, it := range items {
		if it.Status != status {
			continue
		}
		title := strings.TrimSpace(it.Title)
		if status == "active" {
			repos, err := statestore.ListWorkspaceRepos(ctx, db, it.ID)
			if err != nil {
				return nil, err
			}
			workState := deriveLogicalWorkState(ctx, root, it.ID, it.Status, repos)
			title = formatWorkspaceTitleWithLogicalState(workState, title)
		}
		out = append(out, workspaceSelectorCandidate{
			ID:    it.ID,
			Title: title,
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
