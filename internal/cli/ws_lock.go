package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/infra/paths"
)

func (c *CLI) runWSLock(args []string) int {
	return c.runWSPurgeGuardSet(args, true)
}

func (c *CLI) runWSUnlock(args []string) int {
	return c.runWSPurgeGuardSet(args, false)
}

func (c *CLI) runWSPurgeGuardSet(args []string, enabled bool) int {
	outputFormat := "human"
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			if enabled {
				c.printWSLockUsage(c.Out)
			} else {
				c.printWSUnlockUsage(c.Out)
			}
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
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
			fmt.Fprintf(c.Err, "unknown flag: %q\n", args[0])
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		return exitUsage
	}
	if len(args) != 1 {
		if enabled {
			c.printWSLockUsage(c.Err)
		} else {
			c.printWSUnlockUsage(c.Err)
		}
		return exitUsage
	}
	workspaceID := strings.TrimSpace(args[0])
	if err := validateWorkspaceID(workspaceID); err != nil {
		fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
		return exitUsage
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	wsPath, ok, err := resolveWorkspacePathByID(root, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve workspace path: %v\n", err)
		return exitError
	}
	if !ok {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      wsGuardAction(enabled),
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "not_found",
					Message: fmt.Sprintf("workspace not found: %s", workspaceID),
				},
			})
		} else {
			fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		}
		return exitError
	}
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		fmt.Fprintf(c.Err, "load %s: %v\n", workspaceMetaFilename, err)
		return exitError
	}
	now := time.Now().Unix()
	setWorkspaceMetaPurgeGuard(&meta, enabled, now)
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		fmt.Fprintf(c.Err, "update %s: %v\n", workspaceMetaFilename, err)
		return exitError
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      wsGuardAction(enabled),
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"purge_guard_enabled": enabled,
			},
		})
		return exitOK
	}
	useColorOut := writerSupportsColor(c.Out)
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	printResultSection(c.Out, useColorOut, fmt.Sprintf("purge guard %s: %s", state, workspaceID))
	return exitOK
}

func resolveWorkspacePathByID(root string, workspaceID string) (string, bool, error) {
	activePath := filepath.Join(root, "workspaces", workspaceID)
	if fi, err := os.Stat(activePath); err == nil && fi.IsDir() {
		return activePath, true, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	if fi, err := os.Stat(archivePath); err == nil && fi.IsDir() {
		return archivePath, true, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	return "", false, nil
}

func wsGuardAction(enabled bool) string {
	if enabled {
		return "ws.lock"
	}
	return "ws.unlock"
}
