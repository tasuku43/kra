//go:build experimental

package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type agentRunOptions struct {
	workspaceID string
	repoKey     string
	agentKind   string
	taskSummary string
	instruction string
	status      string
	logPath     string
}

func (c *CLI) runAgentRun(args []string) int {
	opts, err := parseAgentRunOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printAgentRunUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printAgentRunUsage(c.Err)
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

	now := time.Now().Unix()
	newRecord := agentActivityRecord{
		WorkspaceID:        opts.workspaceID,
		RepoKey:            opts.repoKey,
		AgentKind:          opts.agentKind,
		TaskSummary:        opts.taskSummary,
		InstructionSummary: opts.instruction,
		StartedAt:          now,
		LastHeartbeatAt:    now,
		Status:             opts.status,
		LogPath:            opts.logPath,
	}

	updated := false
	for i := range records {
		if records[i].WorkspaceID != opts.workspaceID {
			continue
		}
		records[i] = newRecord
		updated = true
		break
	}
	if !updated {
		records = append(records, newRecord)
	}
	if err := saveAgentActivities(root, records); err != nil {
		fmt.Fprintf(c.Err, "save agent activities: %v\n", err)
		return exitError
	}

	fmt.Fprintf(c.Out, "agent started: workspace=%s kind=%s\n", opts.workspaceID, opts.agentKind)
	return exitOK
}

func parseAgentRunOptions(args []string) (agentRunOptions, error) {
	opts := agentRunOptions{status: "running"}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return agentRunOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--workspace="):
			opts.workspaceID = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
			rest = rest[1:]
		case arg == "--workspace":
			if len(rest) < 2 {
				return agentRunOptions{}, fmt.Errorf("--workspace requires a value")
			}
			opts.workspaceID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--repo="):
			opts.repoKey = strings.TrimSpace(strings.TrimPrefix(arg, "--repo="))
			rest = rest[1:]
		case arg == "--repo":
			if len(rest) < 2 {
				return agentRunOptions{}, fmt.Errorf("--repo requires a value")
			}
			opts.repoKey = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--kind="):
			opts.agentKind = strings.TrimSpace(strings.TrimPrefix(arg, "--kind="))
			rest = rest[1:]
		case arg == "--kind":
			if len(rest) < 2 {
				return agentRunOptions{}, fmt.Errorf("--kind requires a value")
			}
			opts.agentKind = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--task="):
			opts.taskSummary = strings.TrimSpace(strings.TrimPrefix(arg, "--task="))
			rest = rest[1:]
		case arg == "--task":
			if len(rest) < 2 {
				return agentRunOptions{}, fmt.Errorf("--task requires a value")
			}
			opts.taskSummary = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--instruction="):
			opts.instruction = strings.TrimSpace(strings.TrimPrefix(arg, "--instruction="))
			rest = rest[1:]
		case arg == "--instruction":
			if len(rest) < 2 {
				return agentRunOptions{}, fmt.Errorf("--instruction requires a value")
			}
			opts.instruction = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--status="):
			opts.status = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(arg, "--status=")))
			rest = rest[1:]
		case arg == "--status":
			if len(rest) < 2 {
				return agentRunOptions{}, fmt.Errorf("--status requires a value")
			}
			opts.status = strings.TrimSpace(strings.ToLower(rest[1]))
			rest = rest[2:]
		case strings.HasPrefix(arg, "--log-path="):
			opts.logPath = strings.TrimSpace(strings.TrimPrefix(arg, "--log-path="))
			rest = rest[1:]
		case arg == "--log-path":
			if len(rest) < 2 {
				return agentRunOptions{}, fmt.Errorf("--log-path requires a value")
			}
			opts.logPath = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return agentRunOptions{}, fmt.Errorf("unknown flag for agent run: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentRunOptions{}, fmt.Errorf("unexpected args for agent run: %q", strings.Join(rest, " "))
	}
	if opts.workspaceID == "" {
		return agentRunOptions{}, fmt.Errorf("--workspace is required")
	}
	if opts.agentKind == "" {
		return agentRunOptions{}, fmt.Errorf("--kind is required")
	}
	switch opts.status {
	case "running", "waiting_user", "thinking", "blocked":
	default:
		return agentRunOptions{}, fmt.Errorf("unsupported --status: %q (supported: running, waiting_user, thinking, blocked)", opts.status)
	}
	return opts, nil
}
