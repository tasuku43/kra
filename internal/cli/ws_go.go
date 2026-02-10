package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tasuku43/gionx/internal/infra/paths"
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
			targetPath, err := resolveWorkspaceGoTarget(root, scope, item.ID)
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
	scope := "active"
	if archivedScope {
		scope = "archived"
	}
	targetPath, err := resolveWorkspaceGoTarget(root, scope, workspaceID)
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

func listWorkspaceCandidatesByStatus(ctx context.Context, root string, status string) ([]workspaceSelectorCandidate, error) {
	_ = ctx
	rows, err := listWorkspaceSelectorRowsFromFilesystem(root, status)
	if err != nil {
		return nil, err
	}
	out := make([]workspaceSelectorCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspaceSelectorCandidate{
			ID:    row.ID,
			Title: formatWorkspaceTitleWithLogicalState("", row.Title),
		})
	}
	return out, nil
}

type workspaceSelectorRow struct {
	ID        string
	Title     string
	UpdatedAt int64
}

func listWorkspaceSelectorRowsFromFilesystem(root string, status string) ([]workspaceSelectorRow, error) {
	baseDir := filepath.Join(root, "workspaces")
	if status == "archived" {
		baseDir = filepath.Join(root, "archive")
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	rows := make([]workspaceSelectorRow, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := strings.TrimSpace(e.Name())
		if err := validateWorkspaceID(id); err != nil {
			continue
		}

		wsPath := filepath.Join(baseDir, id)
		meta, metaErr := loadWorkspaceMetaFile(wsPath)
		title := ""
		updatedAt := int64(0)
		if metaErr == nil {
			title = strings.TrimSpace(meta.Workspace.Title)
			updatedAt = meta.Workspace.UpdatedAt
		}
		if updatedAt <= 0 {
			fi, statErr := os.Stat(wsPath)
			if statErr == nil {
				updatedAt = fi.ModTime().Unix()
			}
		}
		rows = append(rows, workspaceSelectorRow{
			ID:        id,
			Title:     title,
			UpdatedAt: updatedAt,
		})
	}

	slices.SortFunc(rows, func(a, b workspaceSelectorRow) int {
		if a.UpdatedAt != b.UpdatedAt {
			if a.UpdatedAt > b.UpdatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return rows, nil
}

func resolveWorkspaceGoTarget(root string, scope string, workspaceID string) (string, error) {
	targetPath := filepath.Join(root, "workspaces", workspaceID)
	if scope == "archived" {
		targetPath = filepath.Join(root, "archive", workspaceID)
	}
	otherScopePath := filepath.Join(root, "archive", workspaceID)
	otherScopeName := "archived"
	if scope == "archived" {
		otherScopePath = filepath.Join(root, "workspaces", workspaceID)
		otherScopeName = "active"
	}

	fi, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if other, otherErr := os.Stat(otherScopePath); otherErr == nil && other.IsDir() {
				return "", fmt.Errorf("workspace is not %s (status=%s): %s", scope, otherScopeName, workspaceID)
			}
			return "", fmt.Errorf("workspace not found: %s", workspaceID)
		}
		return "", fmt.Errorf("stat target path: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("target path is not a directory: %s", targetPath)
	}
	return targetPath, nil
}
