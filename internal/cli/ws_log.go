package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tasuku43/kra/internal/infra/paths"
)

var runCMUXLogCommand = func(message string) error {
	cmd := exec.Command("cmux", "log", "--level", "info", message)
	if out, err := cmd.CombinedOutput(); err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("%s: %w", trimmed, err)
	}
	return nil
}

func (c *CLI) runWSLog(args []string) int {
	workspaceID := ""
	useCurrent := false
	rest := append([]string{}, args...)

parseFlags:
	for len(rest) > 0 {
		arg := strings.TrimSpace(rest[0])
		switch {
		case arg == "--":
			rest = rest[1:]
			break parseFlags
		case arg == "-h" || arg == "--help" || arg == "help":
			c.printWSLogUsage(c.Out)
			return exitOK
		case arg == "--id":
			if len(rest) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSLogUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case arg == "--current":
			useCurrent = true
			rest = rest[1:]
		default:
			if strings.HasPrefix(arg, "--id=") {
				workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
				rest = rest[1:]
				continue
			}
			if strings.HasPrefix(arg, "--current=") {
				v := strings.TrimSpace(strings.TrimPrefix(arg, "--current="))
				if v != "" {
					fmt.Fprintln(c.Err, "--current does not take a value")
					c.printWSLogUsage(c.Err)
					return exitUsage
				}
				useCurrent = true
				rest = rest[1:]
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(c.Err, "unknown flag for ws log: %q\n", arg)
				c.printWSLogUsage(c.Err)
				return exitUsage
			}
			break parseFlags
		}
	}

	if workspaceID != "" && useCurrent {
		fmt.Fprintln(c.Err, "--id and --current cannot be used together")
		c.printWSLogUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
			return exitUsage
		}
	}

	message := strings.Join(rest, " ")
	if strings.TrimSpace(message) == "" {
		fmt.Fprintln(c.Err, "log message is required")
		c.printWSLogUsage(c.Err)
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

	if workspaceID == "" {
		current, ok := detectWorkspaceFromCWD(root, wd)
		if !ok {
			if useCurrent {
				fmt.Fprintln(c.Err, "ws log --current requires current path under workspaces/<id>/... or archive/<id>/...")
				return exitError
			}
			fmt.Fprintln(c.Err, "ws log requires --id <id> or current path under workspaces/<id>/... or archive/<id>/...")
			c.printWSLogUsage(c.Err)
			return exitUsage
		}
		workspaceID = current.ID
	}

	workspacePath, ok, err := resolveWorkspacePathByID(root, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve workspace path: %v\n", err)
		return exitError
	}
	if !ok {
		fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		return exitError
	}

	logPath := filepath.Join(workspacePath, "log.txt")
	if err := appendWorkspaceLogLine(logPath, message); err != nil {
		fmt.Fprintf(c.Err, "append workspace log: %v\n", err)
		return exitError
	}

	c.tryCMUXLogMirror(message)

	useColorOut := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColorOut,
		styleSuccess(fmt.Sprintf("log appended: %s", logPath), useColorOut),
		styleMuted(fmt.Sprintf("workspace: %s", workspaceID), useColorOut),
	)
	return exitOK
}

func appendWorkspaceLogLine(path string, message string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(message + "\n"); err != nil {
		return err
	}
	return nil
}

func (c *CLI) tryCMUXLogMirror(message string) {
	if strings.TrimSpace(os.Getenv("CMUX_WORKSPACE_ID")) == "" {
		return
	}
	if err := runCMUXLogCommand(message); err != nil {
		c.debugf("ws log cmux mirror failed err=%v", err)
	}
}
