package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type agentRunOptions struct {
	workspaceID string
	repoKey     string
	agentKind   string
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

	opts, err = c.completeAgentRunOptionsInteractive(root, wd, opts)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve run options: %v\n", err)
		return exitError
	}
	execDir := filepath.Join(root, "workspaces", opts.workspaceID)
	scope := "workspace"
	if strings.TrimSpace(opts.repoKey) != "" {
		scope = "repo"
		execDir = filepath.Join(execDir, "repos", opts.repoKey)
	}
	cmdName := strings.TrimSpace(opts.agentKind)
	if cmdName == "" {
		fmt.Fprintf(c.Err, "resolve run options: --kind is required\n")
		return exitUsage
	}

	nowTS := time.Now()
	records, err := loadAgentRuntimeSessions(root)
	if err == nil {
		for _, record := range records {
			if record.WorkspaceID != opts.workspaceID {
				continue
			}
			if record.ExecutionScope != scope {
				continue
			}
			if strings.TrimSpace(record.RepoKey) != strings.TrimSpace(opts.repoKey) {
				continue
			}
			if strings.TrimSpace(strings.ToLower(record.Kind)) != strings.TrimSpace(strings.ToLower(opts.agentKind)) {
				continue
			}
			if record.RuntimeState != "running" && record.RuntimeState != "idle" {
				continue
			}
			fmt.Fprintf(
				c.Err,
				"warning: active session exists for workspace=%s kind=%s location=%s (session=%s)\n",
				opts.workspaceID,
				opts.agentKind,
				scopeLabel(scope, opts.repoKey),
				record.SessionID,
			)
			break
		}
	}

	if strings.TrimSpace(os.Getenv("KRA_AGENT_RUN_DRY_RUN")) == "1" {
		pid := os.Getpid()
		sessionID := newAgentRuntimeSessionID(nowTS, pid)
		runtimeRecord := agentRuntimeSessionRecord{
			SessionID:      sessionID,
			RootPath:       root,
			WorkspaceID:    opts.workspaceID,
			ExecutionScope: scope,
			RepoKey:        strings.TrimSpace(opts.repoKey),
			Kind:           opts.agentKind,
			PID:            pid,
			StartedAt:      nowTS.Unix(),
			UpdatedAt:      nowTS.Unix(),
			Seq:            1,
			RuntimeState:   "running",
			ExitCode:       nil,
		}
		if err := saveAgentRuntimeSession(runtimeRecord); err != nil {
			fmt.Fprintf(c.Err, "save runtime session: %v\n", err)
			return exitError
		}
		zero := 0
		runtimeRecord.Seq++
		runtimeRecord.UpdatedAt = time.Now().Unix()
		runtimeRecord.RuntimeState = "exited"
		runtimeRecord.ExitCode = &zero
		_ = saveAgentRuntimeSession(runtimeRecord)
		fmt.Fprintf(c.Out, "agent started: workspace=%s kind=%s session=%s dir=%s\n", opts.workspaceID, opts.agentKind, sessionID, execDir)
		return exitOK
	}

	if err := ensureAgentBroker(root); err != nil {
		fmt.Fprintf(c.Err, "ensure broker: %v\n", err)
		return exitError
	}
	startResult, err := startSessionWithAgentBroker(root, agentBrokerStartRequest{
		WorkspaceID:    opts.workspaceID,
		ExecutionScope: scope,
		RepoKey:        strings.TrimSpace(opts.repoKey),
		Kind:           cmdName,
		ExecDir:        execDir,
	})
	if err != nil {
		fmt.Fprintf(c.Err, "start agent session via broker: %v\n", err)
		return exitError
	}

	sessionID := strings.TrimSpace(startResult.SessionID)
	if sessionID == "" {
		fmt.Fprintf(c.Err, "start agent session via broker: empty session id\n")
		return exitError
	}
	fmt.Fprintf(c.Out, "agent started: workspace=%s kind=%s session=%s dir=%s\n", opts.workspaceID, opts.agentKind, sessionID, execDir)
	return exitOK
}

func scopeLabel(scope string, repoKey string) string {
	if strings.TrimSpace(scope) == "repo" && strings.TrimSpace(repoKey) != "" {
		return "repo:" + strings.TrimSpace(repoKey)
	}
	return "workspace"
}

func parseAgentRunOptions(args []string) (agentRunOptions, error) {
	opts := agentRunOptions{}
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
		default:
			return agentRunOptions{}, fmt.Errorf("unknown flag for agent run: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentRunOptions{}, fmt.Errorf("unexpected args for agent run: %q", strings.Join(rest, " "))
	}
	return opts, nil
}

func (c *CLI) completeAgentRunOptionsInteractive(root string, wd string, opts agentRunOptions) (agentRunOptions, error) {
	interactive := cliInputIsTTY(c.In)
	if strings.TrimSpace(opts.workspaceID) == "" {
		if workspaceID, ok := inferWorkspaceIDFromCurrentDir(root, wd); ok {
			opts.workspaceID = workspaceID
		}
	}
	if strings.TrimSpace(opts.workspaceID) == "" {
		if !interactive {
			return agentRunOptions{}, fmt.Errorf("--workspace is required in non-interactive mode")
		}
		candidates, err := listWorkspaceCandidatesByStatus(context.Background(), root, "active")
		if err != nil {
			return agentRunOptions{}, fmt.Errorf("list active workspaces: %w", err)
		}
		selected, err := c.promptWorkspaceSelectorSingle("active", "run", candidates)
		if err != nil {
			return agentRunOptions{}, err
		}
		if len(selected) != 1 {
			return agentRunOptions{}, fmt.Errorf("workspace selection canceled")
		}
		opts.workspaceID = selected[0]
	}

	exists, active, err := workspaceActiveOnFilesystem(root, opts.workspaceID)
	if err != nil {
		return agentRunOptions{}, fmt.Errorf("check workspace: %w", err)
	}
	if !exists || !active {
		return agentRunOptions{}, fmt.Errorf("workspace is not active: %s", opts.workspaceID)
	}

	if strings.TrimSpace(opts.repoKey) == "" && interactive {
		candidates, err := listAgentRunTargetCandidates(root, opts.workspaceID)
		if err != nil {
			return agentRunOptions{}, fmt.Errorf("list workspace targets: %w", err)
		}
		selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "run", "Target:", "target", candidates, true)
		if err != nil {
			return agentRunOptions{}, err
		}
		if len(selected) != 1 {
			return agentRunOptions{}, fmt.Errorf("target selection canceled")
		}
		if selected[0] != "workspace" {
			opts.repoKey = selected[0]
		}
	}

	if strings.TrimSpace(opts.agentKind) == "" {
		if !interactive {
			return agentRunOptions{}, fmt.Errorf("--kind is required in non-interactive mode")
		}
		candidates := []workspaceSelectorCandidate{
			{ID: "codex", Description: "OpenAI Codex CLI"},
			{ID: "claude", Description: "Claude Code CLI"},
		}
		selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "run", "Agent kind:", "kind", candidates, true)
		if err != nil {
			return agentRunOptions{}, err
		}
		if len(selected) != 1 {
			return agentRunOptions{}, fmt.Errorf("kind selection canceled")
		}
		opts.agentKind = selected[0]
	}
	return opts, nil
}

func inferWorkspaceIDFromCurrentDir(root string, wd string) (string, bool) {
	root = filepath.Clean(strings.TrimSpace(root))
	wd = filepath.Clean(strings.TrimSpace(wd))
	if root == "" || wd == "" {
		return "", false
	}
	if workspaceID, ok := inferWorkspaceIDByRel(root, wd); ok {
		return workspaceID, true
	}
	rootEval, rootErr := filepath.EvalSymlinks(root)
	wdEval, wdErr := filepath.EvalSymlinks(wd)
	if rootErr != nil || wdErr != nil {
		return "", false
	}
	return inferWorkspaceIDByRel(filepath.Clean(rootEval), filepath.Clean(wdEval))
}

func inferWorkspaceIDByRel(root string, wd string) (string, bool) {
	rel, err := filepath.Rel(root, wd)
	if err != nil {
		return "", false
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "workspaces" {
		return "", false
	}
	workspaceID := strings.TrimSpace(parts[1])
	if workspaceID == "" {
		return "", false
	}
	return workspaceID, true
}

func cliInputIsTTY(in io.Reader) bool {
	inFile, ok := in.(*os.File)
	return ok && isatty.IsTerminal(inFile.Fd())
}

func listAgentRunTargetCandidates(root string, workspaceID string) ([]workspaceSelectorCandidate, error) {
	candidates := []workspaceSelectorCandidate{
		{ID: "workspace", Description: "run at workspace root"},
	}
	reposDir := filepath.Join(root, "workspaces", workspaceID, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		alias := strings.TrimSpace(e.Name())
		if alias == "" {
			continue
		}
		candidates = append(candidates, workspaceSelectorCandidate{
			ID:          alias,
			Description: "repo:" + alias,
		})
	}
	return candidates, nil
}
