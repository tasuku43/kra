package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/tasuku43/kra/internal/infra/paths"
)

func (c *CLI) runWSSave(args []string) int {
	targetID := ""
	useCurrent := false
	selectMode := false
	passthrough := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printWSSaveUsage(c.Out)
			return exitOK
		case "--id":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSSaveUsage(c.Err)
				return exitUsage
			}
			targetID = strings.TrimSpace(args[i+1])
			i++
		case "--current":
			useCurrent = true
		case "--select":
			selectMode = true
		default:
			if strings.HasPrefix(arg, "--id=") {
				targetID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
				continue
			}
			passthrough = append(passthrough, arg)
			if wsSaveFlagNeedsValue(arg) && i+1 < len(args) {
				passthrough = append(passthrough, strings.TrimSpace(args[i+1]))
				i++
			}
		}
	}
	if targetID != "" && useCurrent {
		fmt.Fprintln(c.Err, "--id and --current cannot be used together")
		c.printWSSaveUsage(c.Err)
		return exitUsage
	}
	if selectMode && targetID != "" {
		fmt.Fprintln(c.Err, "--select and --id cannot be used together")
		c.printWSSaveUsage(c.Err)
		return exitUsage
	}
	if selectMode && useCurrent {
		fmt.Fprintln(c.Err, "--select and --current cannot be used together")
		c.printWSSaveUsage(c.Err)
		return exitUsage
	}
	if targetID != "" {
		if err := validateWorkspaceID(targetID); err != nil {
			fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
			return exitUsage
		}
	}
	if useCurrent {
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
		resolved, ok := detectWorkspaceFromCWD(root, wd)
		if !ok || resolved.Status != "active" {
			fmt.Fprintln(c.Err, "ws save --current requires current path under workspaces/<id>/...")
			return exitError
		}
		targetID = resolved.ID
	}
	if targetID != "" {
		passthrough = append(passthrough, "--workspace", targetID)
	}
	if !useCurrent && !selectMode && targetID == "" {
		fmt.Fprintln(c.Err, "ws save requires one of --id <id>, --current, or --select")
		c.printWSSaveUsage(c.Err)
		return exitUsage
	}
	return c.runCMUXSave(passthrough)
}

func wsSaveFlagNeedsValue(arg string) bool {
	switch arg {
	case "--format", "--workspace", "--label", "-l":
		return true
	default:
		return false
	}
}
