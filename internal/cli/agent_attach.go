package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/kra/internal/infra/paths"
	"golang.org/x/term"
)

type agentAttachOptions struct {
	sessionID string
}

type agentAttachMode struct {
	forceRedraw   bool
	writeBoundary bool
	flushInput    bool
	restoreShell  bool
	clearOnEnter  bool
	fullscreen    bool
	localDetach   bool
}

var errAgentAttachDetached = errors.New("attach detached by user")

var defaultAgentAttachMode = agentAttachMode{
	forceRedraw:   true,
	writeBoundary: false,
	flushInput:    true,
	restoreShell:  true,
	clearOnEnter:  false,
	fullscreen:    true,
	localDetach:   true,
}

func (c *CLI) runAgentAttach(args []string) int {
	return c.runAgentAttachWithMode(args, defaultAgentAttachMode)
}

func (c *CLI) runAgentAttachWithMode(args []string, mode agentAttachMode) int {
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
			if isTerminalWriter(c.Out) {
				writeAttachSessionBoundary(c.Out)
			}
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
		fmt.Fprintf(c.Err, "session is not active: %s\n", record.SessionID)
		return exitError
	}

	if err := pingAgentBroker(root); err != nil {
		fmt.Fprintf(c.Err, "broker is not available; start a session first with kra agent run: %v\n", err)
		return exitError
	}

	conn, err := attachSessionWithAgentBroker(
		root,
		record.SessionID,
		terminalCols(c.In, c.Out),
		terminalRows(c.In, c.Out),
		mode.forceRedraw,
	)
	if err != nil {
		fmt.Fprintf(c.Err, "attach session via broker: %v\n", err)
		return exitError
	}
	defer func() { _ = conn.Close() }()

	if err := proxyAgentAttachIO(root, record.SessionID, conn, c.In, c.Out, mode); err != nil {
		if errors.Is(err, errAgentAttachDetached) {
			fmt.Fprintf(c.Out, "detached: session=%s\n", record.SessionID)
			return exitOK
		}
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

func proxyAgentAttachIO(root string, sessionID string, conn *net.UnixConn, in io.Reader, out io.Writer, mode agentAttachMode) error {
	if conn == nil {
		return fmt.Errorf("broker connection is nil")
	}
	if mode.fullscreen && isTerminalWriter(out) {
		writeAttachTerminalEnter(out)
	} else if mode.clearOnEnter && isTerminalWriter(out) {
		writeAttachTerminalClear(out)
	}
	if !mode.fullscreen && mode.writeBoundary && isTerminalWriter(out) {
		writeAttachSessionBoundary(out)
	}
	if mode.flushInput && isTerminalReader(in) {
		flushTerminalInputBuffer(in)
	}

	restore, err := maybeEnterRawMode(in, out)
	if err != nil {
		return err
	}
	if mode.flushInput && isTerminalReader(in) {
		defer flushTerminalInputBuffer(in)
	}
	if restore != nil {
		defer restore()
	}
	if mode.restoreShell && isTerminalWriter(out) {
		defer writeAttachTerminalRestore(out)
	}

	stopResizeWatcher := startAttachResizeWatcher(root, sessionID, in, out)
	defer stopResizeWatcher()

	readErrCh := make(chan error, 1)
	go func() {
		_, readErr := io.Copy(out, conn)
		readErrCh <- readErr
	}()

	inputResCh := make(chan attachInputResult, 1)
	go func() {
		inputResCh <- forwardAttachInput(conn, in, mode.localDetach)
	}()

	select {
	case inputRes := <-inputResCh:
		if inputRes.detached {
			_ = conn.Close()
			return errAgentAttachDetached
		}
		if isAgentAttachIOError(inputRes.err) {
			_ = conn.Close()
			return inputRes.err
		}
		readErr := <-readErrCh
		if isAgentAttachIOError(readErr) {
			return readErr
		}
		return nil
	case readErr := <-readErrCh:
		_ = conn.Close()
		if isAgentAttachIOError(readErr) {
			return readErr
		}
		return nil
	}
}

type attachInputResult struct {
	detached bool
	err      error
}

func forwardAttachInput(conn *net.UnixConn, in io.Reader, localDetach bool) attachInputResult {
	buf := make([]byte, 4096)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			start := 0
			if localDetach {
				for i, b := range chunk {
					if b != 0x03 { // Ctrl-C -> local detach
						continue
					}
					if i > start {
						if werr := writeAllUnixConn(conn, chunk[start:i]); isAgentAttachIOError(werr) {
							return attachInputResult{err: werr}
						}
					}
					return attachInputResult{detached: true}
				}
			}
			if start < len(chunk) {
				if werr := writeAllUnixConn(conn, chunk[start:]); isAgentAttachIOError(werr) {
					return attachInputResult{err: werr}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return attachInputResult{}
			}
			return attachInputResult{err: err}
		}
	}
}

func writeAllUnixConn(conn *net.UnixConn, b []byte) error {
	for len(b) > 0 {
		n, err := conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func startAttachResizeWatcher(root string, sessionID string, in io.Reader, out io.Writer) func() {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(sessionID) == "" {
		return func() {}
	}
	if !isTerminalReader(in) && !isTerminalWriter(out) {
		return func() {}
	}
	cols, rows := terminalSize(in, out)
	if cols > 0 && rows > 0 {
		_ = resizeSessionWithAgentBroker(root, sessionID, cols, rows)
	}

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-sigch:
				cols, rows := terminalSize(in, out)
				if cols > 0 && rows > 0 {
					_ = resizeSessionWithAgentBroker(root, sessionID, cols, rows)
				}
			}
		}
	}()
	return func() {
		signal.Stop(sigch)
		close(done)
	}
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

func isTerminalWriter(out io.Writer) bool {
	outFile, ok := out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(outFile.Fd())
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

func writeAttachSessionBoundary(out io.Writer) {
	_, _ = io.WriteString(out, "\r\n")
}

func writeAttachTerminalClear(out io.Writer) {
	_, _ = io.WriteString(out, "\r\x1b[2J\x1b[H")
}

func writeAttachTerminalEnter(out io.Writer) {
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[2J\x1b[H")
}

func writeAttachTerminalRestore(out io.Writer) {
	_, _ = io.WriteString(out, "\x1b[0m\x1b[?25h\x1b[?7h\x1b[?2004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1006l\x1b[?1049l")
}
