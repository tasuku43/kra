package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	opts, err = c.completeAgentStopOptionsInteractive(root, wd, opts, records)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			return exitOK
		}
		fmt.Fprintf(c.Err, "resolve stop target: %v\n", err)
		c.printAgentStopUsage(c.Err)
		return exitUsage
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

	stoppedByBroker := false
	if err := stopSessionWithAgentBroker(root, record.SessionID); err == nil {
		stoppedByBroker = true
	} else if err := terminateAgentPID(record.PID); err != nil {
		fmt.Fprintf(c.Err, "stop session process: %v\n", err)
		return exitError
	}

	if stoppedByBroker {
		if exitedRecord, ok := waitAgentRuntimeSessionState(root, record.SessionID, "exited", 2*time.Second); ok {
			record = exitedRecord
			fmt.Fprintf(c.Out, "agent stopped: session=%s\n", record.SessionID)
			return exitOK
		}
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

func waitAgentRuntimeSessionState(root string, sessionID string, targetState string, timeout time.Duration) (agentRuntimeSessionRecord, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		records, err := loadAgentRuntimeSessions(root)
		if err != nil {
			return agentRuntimeSessionRecord{}, false
		}
		for _, r := range records {
			if r.SessionID != sessionID {
				continue
			}
			if r.RuntimeState == targetState {
				return r, true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return agentRuntimeSessionRecord{}, false
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
	return opts, nil
}

func (c *CLI) completeAgentStopOptionsInteractive(root string, wd string, opts agentStopOptions, records []agentRuntimeSessionRecord) (agentStopOptions, error) {
	if strings.TrimSpace(opts.sessionID) != "" || strings.TrimSpace(opts.workspaceID) != "" {
		return opts, nil
	}
	if !cliInputIsTTY(c.In) {
		return agentStopOptions{}, fmt.Errorf("either --session or --workspace is required in non-interactive mode")
	}

	scope, err := resolveAgentContextScope(root, wd)
	if err != nil {
		return agentStopOptions{}, err
	}

	candidates := filterStoppableSessionsByScope(records, scope)
	if len(candidates) == 0 {
		return agentStopOptions{}, fmt.Errorf("no running/idle session found in current context")
	}

	selectorItems := make([]workspaceSelectorCandidate, 0, len(candidates))
	for _, record := range candidates {
		location := "workspace"
		if strings.TrimSpace(record.ExecutionScope) == "repo" {
			location = "repo:" + strings.TrimSpace(record.RepoKey)
		}
		selectorItems = append(selectorItems, workspaceSelectorCandidate{
			ID:          record.SessionID,
			Description: fmt.Sprintf("%s  %s  state:%s  updated:%s", record.Kind, location, record.RuntimeState, formatRelativeAge(record.UpdatedAt)),
		})
	}

	selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "stop", "Session to stop:", "session", selectorItems, true)
	if err != nil {
		return agentStopOptions{}, err
	}
	if len(selected) != 1 || strings.TrimSpace(selected[0]) == "" {
		return agentStopOptions{}, fmt.Errorf("session selection canceled")
	}
	opts.sessionID = strings.TrimSpace(selected[0])
	return opts, nil
}

type agentContextScope struct {
	workspaceID string
	repoKey     string
}

func resolveAgentContextScope(root string, wd string) (agentContextScope, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	wd = filepath.Clean(strings.TrimSpace(wd))
	if root == "" || wd == "" {
		return agentContextScope{}, fmt.Errorf("invalid context path")
	}

	rel, err := filepath.Rel(root, wd)
	if err != nil {
		return agentContextScope{}, fmt.Errorf("resolve current context scope: %w", err)
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return agentContextScope{}, fmt.Errorf("current directory is KRA_ROOT; run inside workspaces/<id> or workspaces/<id>/repos/<repo-key>")
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return agentContextScope{}, fmt.Errorf("current directory is outside KRA_ROOT; run inside workspaces/<id> or workspaces/<id>/repos/<repo-key>")
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "workspaces" {
		return agentContextScope{}, fmt.Errorf("current directory does not map to workspace scope; run inside workspaces/<id> or workspaces/<id>/repos/<repo-key>")
	}
	scope := agentContextScope{workspaceID: strings.TrimSpace(parts[1])}
	if scope.workspaceID == "" {
		return agentContextScope{}, fmt.Errorf("current workspace id is empty")
	}
	if len(parts) >= 4 && parts[2] == "repos" {
		scope.repoKey = strings.TrimSpace(parts[3])
	}
	return scope, nil
}

func filterStoppableSessionsByScope(records []agentRuntimeSessionRecord, scope agentContextScope) []agentRuntimeSessionRecord {
	out := make([]agentRuntimeSessionRecord, 0, len(records))
	for _, record := range records {
		if record.WorkspaceID != scope.workspaceID {
			continue
		}
		if strings.TrimSpace(scope.repoKey) != "" && strings.TrimSpace(record.RepoKey) != strings.TrimSpace(scope.repoKey) {
			continue
		}
		if record.RuntimeState == "exited" {
			continue
		}
		out = append(out, record)
	}

	slices.SortFunc(out, func(a, b agentRuntimeSessionRecord) int {
		if cmp := compareExecutionLocation(a, b); cmp != 0 {
			return cmp
		}
		if a.UpdatedAt != b.UpdatedAt {
			if a.UpdatedAt > b.UpdatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.SessionID, b.SessionID)
	})
	return out
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
