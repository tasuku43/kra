package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type agentStopOptions struct {
	sessionID   string
	workspaceID string
	repoKey     string
	kind        string
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

	records, err := loadAgentRuntimeSessions(root)
	if err != nil {
		fmt.Fprintf(c.Err, "load agent runtime sessions: %v\n", err)
		return exitError
	}

	record, err := resolveAgentStopTarget(records, opts)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if record.RuntimeState == "exited" {
		fmt.Fprintf(c.Out, "agent already stopped: session=%s\n", record.SessionID)
		return exitOK
	}

	if err := terminateAgentPID(record.PID); err != nil {
		fmt.Fprintf(c.Err, "stop session process: %v\n", err)
		return exitError
	}

	record.RuntimeState = "exited"
	record.UpdatedAt = time.Now().Unix()
	record.Seq++
	record.ExitCode = nil
	if err := saveAgentRuntimeSession(record); err != nil {
		fmt.Fprintf(c.Err, "save runtime session: %v\n", err)
		return exitError
	}

	fmt.Fprintf(c.Out, "agent stopped: session=%s\n", record.SessionID)
	return exitOK
}

func parseAgentStopOptions(args []string) (agentStopOptions, error) {
	opts := agentStopOptions{}
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
		case strings.HasPrefix(arg, "--session="):
			opts.sessionID = strings.TrimSpace(strings.TrimPrefix(arg, "--session="))
			rest = rest[1:]
		case arg == "--session":
			if len(rest) < 2 {
				return agentStopOptions{}, fmt.Errorf("--session requires a value")
			}
			opts.sessionID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--repo="):
			opts.repoKey = strings.TrimSpace(strings.TrimPrefix(arg, "--repo="))
			rest = rest[1:]
		case arg == "--repo":
			if len(rest) < 2 {
				return agentStopOptions{}, fmt.Errorf("--repo requires a value")
			}
			opts.repoKey = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--kind="):
			opts.kind = strings.TrimSpace(strings.TrimPrefix(arg, "--kind="))
			rest = rest[1:]
		case arg == "--kind":
			if len(rest) < 2 {
				return agentStopOptions{}, fmt.Errorf("--kind requires a value")
			}
			opts.kind = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return agentStopOptions{}, fmt.Errorf("unknown flag for agent stop: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentStopOptions{}, fmt.Errorf("unexpected args for agent stop: %q", strings.Join(rest, " "))
	}
	if opts.sessionID == "" && opts.workspaceID == "" {
		return agentStopOptions{}, fmt.Errorf("either --session or --workspace is required")
	}
	return opts, nil
}

func resolveAgentStopTarget(records []agentRuntimeSessionRecord, opts agentStopOptions) (agentRuntimeSessionRecord, error) {
	if opts.sessionID != "" {
		for _, r := range records {
			if r.SessionID == opts.sessionID {
				return r, nil
			}
		}
		return agentRuntimeSessionRecord{}, fmt.Errorf("agent session not found: %s", opts.sessionID)
	}

	candidates := make([]agentRuntimeSessionRecord, 0, len(records))
	for _, r := range records {
		if r.WorkspaceID != opts.workspaceID {
			continue
		}
		if opts.repoKey != "" && r.RepoKey != opts.repoKey {
			continue
		}
		if opts.kind != "" && r.Kind != opts.kind {
			continue
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		return agentRuntimeSessionRecord{}, fmt.Errorf("agent session not found for workspace: %s", opts.workspaceID)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	slices.SortFunc(candidates, func(a, b agentRuntimeSessionRecord) int {
		if a.UpdatedAt != b.UpdatedAt {
			if a.UpdatedAt > b.UpdatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.SessionID, b.SessionID)
	})
	return candidates[0], nil
}

func terminateAgentPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	if !isAgentPIDAlive(pid) {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !isAgentPIDAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := process.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	return nil
}

func isAgentPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}
