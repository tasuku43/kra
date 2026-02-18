package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/infra/paths"
	"golang.org/x/term"
)

type agentAttachOptions struct {
	sessionID string
}

func (c *CLI) runAgentAttach(args []string) int {
	opts, err := parseAgentAttachOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printAgentAttachUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printAgentAttachUsage(c.Err)
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

	opts, err = c.completeAgentAttachOptionsInteractive(root, wd, opts, records)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			return exitOK
		}
		fmt.Fprintf(c.Err, "resolve attach target: %v\n", err)
		c.printAgentAttachUsage(c.Err)
		return exitUsage
	}

	record, err := resolveAgentAttachTarget(records, opts.sessionID)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if record.RuntimeState == "exited" {
		fmt.Fprintf(c.Err, "session is not running: %s\n", record.SessionID)
		return exitError
	}

	if err := pingAgentBroker(root); err != nil {
		fmt.Fprintf(c.Err, "broker is not available; start a session first with kra agent run: %v\n", err)
		return exitError
	}

	conn, err := attachSessionWithAgentBroker(root, record.SessionID, terminalCols(c.In, c.Out), terminalRows(c.In, c.Out))
	if err != nil {
		fmt.Fprintf(c.Err, "attach session via broker: %v\n", err)
		return exitError
	}
	defer func() { _ = conn.Close() }()

	if err := proxyAgentAttachIO(conn, c.In, c.Out); err != nil {
		fmt.Fprintf(c.Err, "attach session stream: %v\n", err)
		return exitError
	}
	return exitOK
}

func parseAgentAttachOptions(args []string) (agentAttachOptions, error) {
	opts := agentAttachOptions{}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return agentAttachOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--session="):
			opts.sessionID = strings.TrimSpace(strings.TrimPrefix(arg, "--session="))
			rest = rest[1:]
		case arg == "--session":
			if len(rest) < 2 {
				return agentAttachOptions{}, fmt.Errorf("--session requires a value")
			}
			opts.sessionID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return agentAttachOptions{}, fmt.Errorf("unknown flag for agent attach: %q", arg)
		}
	}
	if len(rest) > 0 {
		return agentAttachOptions{}, fmt.Errorf("unexpected args for agent attach: %q", strings.Join(rest, " "))
	}
	return opts, nil
}

func (c *CLI) completeAgentAttachOptionsInteractive(root string, wd string, opts agentAttachOptions, records []agentRuntimeSessionRecord) (agentAttachOptions, error) {
	if strings.TrimSpace(opts.sessionID) != "" {
		return opts, nil
	}
	if !cliInputIsTTY(c.In) {
		return agentAttachOptions{}, fmt.Errorf("--session is required in non-interactive mode")
	}

	scope, err := resolveAgentContextScope(root, wd)
	if err != nil {
		return agentAttachOptions{}, err
	}
	candidates := filterAttachableSessionsByScope(records, scope)
	if len(candidates) == 0 {
		return agentAttachOptions{}, fmt.Errorf("no attachable session found in current context")
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

	title := formatAgentAttachSelectorTitle(scope)
	selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "attach", title, "session", selectorItems, true)
	if err != nil {
		return agentAttachOptions{}, err
	}
	if len(selected) != 1 || strings.TrimSpace(selected[0]) == "" {
		return agentAttachOptions{}, fmt.Errorf("session selection canceled")
	}
	opts.sessionID = strings.TrimSpace(selected[0])
	return opts, nil
}

func filterAttachableSessionsByScope(records []agentRuntimeSessionRecord, scope agentContextScope) []agentRuntimeSessionRecord {
	return filterStoppableSessionsByScope(records, scope)
}

func resolveAgentAttachTarget(records []agentRuntimeSessionRecord, sessionID string) (agentRuntimeSessionRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return agentRuntimeSessionRecord{}, fmt.Errorf("session id is required")
	}
	for _, record := range records {
		if record.SessionID == sessionID {
			return record, nil
		}
	}
	return agentRuntimeSessionRecord{}, fmt.Errorf("agent session not found: %s", sessionID)
}

func formatAgentAttachSelectorTitle(scope agentContextScope) string {
	workspaceID := strings.TrimSpace(scope.workspaceID)
	repoKey := strings.TrimSpace(scope.repoKey)
	if workspaceID == "" {
		return "Session to attach:"
	}
	if repoKey == "" {
		return fmt.Sprintf("Session to attach (workspace: %s):", workspaceID)
	}
	return fmt.Sprintf("Session to attach (workspace: %s repo:%s):", workspaceID, repoKey)
}

func proxyAgentAttachIO(conn *net.UnixConn, in io.Reader, out io.Writer) error {
	if conn == nil {
		return fmt.Errorf("broker connection is nil")
	}

	restore, err := maybeEnterRawMode(in, out)
	if err != nil {
		return err
	}
	if restore != nil {
		defer restore()
	}

	terminalInput := isTerminalReader(in)
	writeErrCh := make(chan error, 1)
	go func() {
		_, writeErr := io.Copy(conn, in)
		writeErrCh <- writeErr
	}()

	_, readErr := io.Copy(out, conn)
	_ = conn.Close()

	writeErr := error(nil)
	if terminalInput {
		select {
		case writeErr = <-writeErrCh:
		default:
		}
	} else {
		writeErr = <-writeErrCh
	}
	if isAgentAttachIOError(readErr) {
		return readErr
	}
	if isAgentAttachIOError(writeErr) {
		return writeErr
	}
	return nil
}

func maybeEnterRawMode(in io.Reader, out io.Writer) (func(), error) {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if !inOK || !outOK {
		return nil, nil
	}
	if !isatty.IsTerminal(inFile.Fd()) || !isatty.IsTerminal(outFile.Fd()) {
		return nil, nil
	}
	state, err := term.MakeRaw(int(inFile.Fd()))
	if err != nil {
		return nil, fmt.Errorf("set terminal raw mode: %w", err)
	}
	return func() {
		_ = term.Restore(int(inFile.Fd()), state)
	}, nil
}

func isTerminalReader(in io.Reader) bool {
	inFile, ok := in.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(inFile.Fd())
}

func terminalCols(in io.Reader, out io.Writer) int {
	cols, _ := terminalSize(in, out)
	return cols
}

func terminalRows(in io.Reader, out io.Writer) int {
	_, rows := terminalSize(in, out)
	return rows
}

func terminalSize(in io.Reader, out io.Writer) (int, int) {
	if outFile, ok := out.(*os.File); ok && isatty.IsTerminal(outFile.Fd()) {
		cols, rows, err := term.GetSize(int(outFile.Fd()))
		if err == nil && cols > 0 && rows > 0 {
			return cols, rows
		}
	}
	if inFile, ok := in.(*os.File); ok && isatty.IsTerminal(inFile.Fd()) {
		cols, rows, err := term.GetSize(int(inFile.Fd()))
		if err == nil && cols > 0 && rows > 0 {
			return cols, rows
		}
	}
	return 0, 0
}

func isAgentAttachIOError(err error) bool {
	if err == nil || errors.Is(err, io.EOF) {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return false
	}
	return !strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
}
