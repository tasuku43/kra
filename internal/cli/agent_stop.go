package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type agentStopOptions struct {
	workspaceID string
	status      string
}

func (c *CLI) runAgentStop(args []string) int {
	opts, err := parseAgentStopOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printAgentStopUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printAgentStopUsage(c.Err)
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

	records, err := loadAgentActivities(root)
	if err != nil {
		fmt.Fprintf(c.Err, "load agent activities: %v\n", err)
		return exitError
	}

	idx := -1
	for i := range records {
		if records[i].WorkspaceID == opts.workspaceID {
			idx = i
			break
		}
	}
	if idx < 0 {
		fmt.Fprintf(c.Err, "agent activity not found for workspace: %s\n", opts.workspaceID)
		return exitError
	}
	if records[idx].Status != "running" {
		fmt.Fprintf(c.Err, "agent activity is not running for workspace: %s (status=%s)\n", opts.workspaceID, records[idx].Status)
		return exitError
	}

	records[idx].Status = opts.status
	records[idx].LastHeartbeatAt = time.Now().Unix()
	if err := saveAgentActivities(root, records); err != nil {
		fmt.Fprintf(c.Err, "save agent activities: %v\n", err)
		return exitError
	}

	fmt.Fprintf(c.Out, "agent stopped: workspace=%s status=%s\n", opts.workspaceID, opts.status)
	return exitOK
}

func parseAgentStopOptions(args []string) (agentStopOptions, error) {
	opts := agentStopOptions{status: "failed"}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return agentStopOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--workspace="):
			opts.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
			rest = rest[1:]
		case arg == "--workspace":
			if len(rest) < 2 {
				return agentStopOptions{}, fmt.Errorf("--workspace requires a value")
			}
			opts.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--status="):
			opts.status = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(arg, "--status=")))
			rest = rest[1:]
		case arg == "--status":
			if len(rest) < 2 {
				return agentStopOptions{}, fmt.Errorf("--status requires a value")
			}
			opts.status = strings.TrimSpace(strings.ToLower(rest[1]))
			rest = rest[2:]
		default:
			return agentStopOptions{}, fmt.Errorf("unknown flag for agent stop: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentStopOptions{}, fmt.Errorf("unexpected args for agent stop: %q", strings.Join(rest, " "))
	}
	if opts.workspaceID == "" {
		return agentStopOptions{}, fmt.Errorf("--workspace is required")
	}
	switch opts.status {
	case "succeeded", "failed", "unknown":
	default:
		return agentStopOptions{}, fmt.Errorf("unsupported --status: %q (supported: succeeded, failed, unknown)", opts.status)
	}
	return opts, nil
}
