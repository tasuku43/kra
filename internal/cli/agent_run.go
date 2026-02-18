package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/infra/paths"
	"golang.org/x/term"
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

	opts, err = c.completeAgentRunOptionsInteractive(root, opts)
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
	cmd := exec.Command(cmdName)
	cmd.Dir = execDir
	cmd.Env = append(os.Environ(), "KRA_AGENT_WORKSPACE="+opts.workspaceID)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(c.Err, "start agent process: %v\n", err)
		return exitError
	}
	defer func() { _ = ptmx.Close() }()

	nowTS := time.Now()
	sessionID := newAgentRuntimeSessionID(nowTS, cmd.Process.Pid)
	runtimeRecord := agentRuntimeSessionRecord{
		SessionID:      sessionID,
		RootPath:       root,
		WorkspaceID:    opts.workspaceID,
		ExecutionScope: scope,
		RepoKey:        strings.TrimSpace(opts.repoKey),
		Kind:           opts.agentKind,
		PID:            cmd.Process.Pid,
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
	fmt.Fprintf(c.Out, "agent started: workspace=%s kind=%s session=%s dir=%s\n", opts.workspaceID, opts.agentKind, sessionID, execDir)
	if strings.TrimSpace(os.Getenv("KRA_AGENT_RUN_DRY_RUN")) == "1" {
		zero := 0
		runtimeRecord.Seq++
		runtimeRecord.UpdatedAt = time.Now().Unix()
		runtimeRecord.RuntimeState = "exited"
		runtimeRecord.ExitCode = &zero
		_ = saveAgentRuntimeSession(runtimeRecord)
		return exitOK
	}

	var mu sync.Mutex
	updateRuntime := func(state string, exitCode *int) {
		mu.Lock()
		defer mu.Unlock()
		runtimeRecord.Seq++
		runtimeRecord.UpdatedAt = time.Now().Unix()
		if strings.TrimSpace(state) != "" {
			runtimeRecord.RuntimeState = state
		}
		runtimeRecord.ExitCode = exitCode
		_ = saveAgentRuntimeSession(runtimeRecord)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			if cmd.Process == nil {
				return
			}
			_ = cmd.Process.Signal(sig)
		}
	}()

	restoreTerminal, resizeCleanup := setupPTYBridgeTerminal(c.In, c.Out, ptmx)
	defer restoreTerminal()
	defer resizeCleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		buf := make([]byte, 4096)
		lastPersist := time.Now()
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				_, _ = c.Out.Write(buf[:n])
				if time.Since(lastPersist) >= time.Second {
					updateRuntime("running", nil)
					lastPersist = time.Now()
				}
			}
			if readErr != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	go func() {
		_, _ = io.Copy(ptmx, c.In)
	}()

	waitErr := cmd.Wait()
	cancel()
	_ = ptmx.Close()
	<-copyDone
	exitCode := 0
	finalState := "exited"
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			finalState = "unknown"
		}
	}
	updateRuntime(finalState, &exitCode)
	if finalState == "unknown" {
		fmt.Fprintf(c.Err, "wait agent process: %v\n", waitErr)
		return exitError
	}
	return exitCode
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

func (c *CLI) completeAgentRunOptionsInteractive(root string, opts agentRunOptions) (agentRunOptions, error) {
	interactive := cliInputIsTTY(c.In)
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

func cliInputIsTTY(in io.Reader) bool {
	inFile, ok := in.(*os.File)
	return ok && isatty.IsTerminal(inFile.Fd())
}

func setupPTYBridgeTerminal(in io.Reader, out io.Writer, ptmx *os.File) (restore func(), cleanup func()) {
	restore = func() {}
	cleanup = func() {}

	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if !inOK || !outOK {
		return restore, cleanup
	}
	if !isatty.IsTerminal(inFile.Fd()) || !isatty.IsTerminal(outFile.Fd()) {
		return restore, cleanup
	}

	oldState, err := term.MakeRaw(int(inFile.Fd()))
	if err == nil {
		restore = func() {
			_ = term.Restore(int(inFile.Fd()), oldState)
		}
	}

	_ = pty.InheritSize(inFile, ptmx)
	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)
	go func() {
		for range winchCh {
			_ = pty.InheritSize(inFile, ptmx)
		}
	}()
	cleanup = func() {
		signal.Stop(winchCh)
		close(winchCh)
	}
	return restore, cleanup
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
