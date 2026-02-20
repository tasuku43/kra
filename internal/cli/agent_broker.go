package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/tasuku43/kra/internal/infra/paths"
)

const (
	agentBrokerActionPing   = "ping"
	agentBrokerActionStart  = "start"
	agentBrokerActionStop   = "stop"
	agentBrokerActionAttach = "attach"
	agentBrokerActionResize = "resize"
	agentBrokerActionList   = "sessions"
	agentBrokerActionInput  = "input"
	agentBrokerActionScreen = "screen_snapshot"
	agentBrokerActionClaim  = "claim_control"

	agentBrokerDialTimeout       = 300 * time.Millisecond
	agentBrokerStartupTimeout    = 4 * time.Second
	agentBrokerIdleTimeout       = 60 * time.Second
	agentBrokerAttachWriteTTL    = 2 * time.Second
	agentRuntimeWriteInterval    = 1 * time.Second
	agentRuntimeIdleAfterSilence = 3 * time.Second
	agentBrokerEmbeddedEnvKey    = "KRA_AGENT_BROKER_EMBEDDED"
	agentScreenStateMaxLines     = 2000
)

type agentBrokerStartRequest struct {
	WorkspaceID    string
	ExecutionScope string
	RepoKey        string
	Kind           string
	ExecDir        string
	Cols           int
	Rows           int
}

type agentBrokerStartResult struct {
	SessionID string
	PID       int
}

type agentBrokerRequest struct {
	Action string `json:"action"`

	WorkspaceID    string `json:"workspace_id,omitempty"`
	ExecutionScope string `json:"execution_scope,omitempty"`
	RepoKey        string `json:"repo_key,omitempty"`
	Kind           string `json:"kind,omitempty"`
	ExecDir        string `json:"exec_dir,omitempty"`
	Cols           int    `json:"cols,omitempty"`
	Rows           int    `json:"rows,omitempty"`

	SessionID   string `json:"session_id,omitempty"`
	ForceRedraw bool   `json:"force_redraw,omitempty"`
	Input       string `json:"input,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	AttachMode  string `json:"attach_mode,omitempty"`
}

type agentBrokerResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	SessionID string                      `json:"session_id,omitempty"`
	PID       int                         `json:"pid,omitempty"`
	Sessions  []agentRuntimeSessionRecord `json:"sessions,omitempty"`
	InputOK   *bool                       `json:"input_ok,omitempty"`
	ResizeOK  *bool                       `json:"resize_ok,omitempty"`
	ScreenSeq int64                       `json:"screen_seq,omitempty"`
	ScreenAt  int64                       `json:"screen_at,omitempty"`
	Screen    string                      `json:"screen,omitempty"`
}

type agentBrokerAttachment struct {
	conn     *net.UnixConn
	paused   bool
	clientID string
}

type agentBrokerAttachmentTarget struct {
	attachID string
	conn     *net.UnixConn
}

type agentBrokerSession struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	ptmx          *os.File
	record        agentRuntimeSessionRecord
	attachments   map[string]*agentBrokerAttachment
	outputHistory []byte
	nextAttachID  int64
	seqParser     *agentTerminalSequenceParser
	lastWriteAt   time.Time
	lastOutputAt  time.Time
	screenSeq     int64
	screenLines   []string
	screenPartial string
	screenAt      int64
}

func (s *agentBrokerSession) snapshot() agentRuntimeSessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.record
}

func (s *agentBrokerSession) update(mut func(*agentRuntimeSessionRecord)) {
	s.mu.Lock()
	mut(&s.record)
	record := s.record
	s.mu.Unlock()
	_ = saveAgentRuntimeSession(record)
}

func (s *agentBrokerSession) addAttachment(conn *net.UnixConn, paused bool, clientID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attachments == nil {
		s.attachments = map[string]*agentBrokerAttachment{}
	}
	s.nextAttachID++
	attachID := fmt.Sprintf("a-%d", s.nextAttachID)
	s.attachments[attachID] = &agentBrokerAttachment{
		conn:     conn,
		paused:   paused,
		clientID: strings.TrimSpace(clientID),
	}
	return attachID
}

func (s *agentBrokerSession) acquireControl(clientID string, mode string) (bool, bool) {
	return true, false
}

func (s *agentBrokerSession) claimControl(clientID string) (bool, bool) {
	return true, false
}

func (s *agentBrokerSession) canResize(clientID string) bool {
	return false
}

func (s *agentBrokerSession) hasAttachedClientLocked(clientID string) bool {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return false
	}
	for _, attachment := range s.attachments {
		if attachment == nil {
			continue
		}
		if strings.TrimSpace(attachment.clientID) == clientID {
			return true
		}
	}
	return false
}

func (s *agentBrokerSession) replayOutputHistory(attachID string) error {
	attachID = strings.TrimSpace(attachID)
	if attachID == "" {
		return fmt.Errorf("attach id is required")
	}
	sent := 0
	for {
		chunk, conn, done := s.replayChunk(attachID, sent)
		if done {
			return nil
		}
		if conn == nil {
			return fmt.Errorf("attach connection closed during replay")
		}
		if len(chunk) == 0 {
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(agentBrokerAttachWriteTTL))
		if err := writeAllUnixConn(conn, chunk); err != nil {
			_ = conn.SetWriteDeadline(time.Time{})
			s.removeAttachment(attachID)
			return err
		}
		_ = conn.SetWriteDeadline(time.Time{})
		sent += len(chunk)
	}
}

func (s *agentBrokerSession) replayChunk(attachID string, sent int) ([]byte, *net.UnixConn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attachment := s.attachments[attachID]
	if attachment == nil || attachment.conn == nil {
		return nil, nil, true
	}
	if sent < len(s.outputHistory) {
		chunk := append([]byte(nil), s.outputHistory[sent:]...)
		return chunk, attachment.conn, false
	}
	attachment.paused = false
	return nil, attachment.conn, true
}

func (s *agentBrokerSession) appendOutputAndSnapshotWritable(payload []byte, now time.Time) ([]agentBrokerAttachmentTarget, *agentRuntimeSessionRecord, []agentRuntimeSignalEvent) {
	s.mu.Lock()
	stateChanged := false
	signalEvents := make([]agentRuntimeSignalEvent, 0)
	if len(payload) > 0 {
		s.lastOutputAt = now
		s.outputHistory = append(s.outputHistory, payload...)
		s.updateScreenLocked(payload, now.Unix())
		currentState := s.record.RuntimeState
		nextState := currentState
		if currentState != "exited" && currentState != "running" {
			nextState = "running"
		}
		if s.seqParser != nil {
			seqEvents := s.seqParser.Feed(payload)
			if len(seqEvents) > 0 {
				signalEvents = append(signalEvents, seqEvents...)
				nextState = applyRuntimeStateHints(nextState, seqEvents)
			}
		}
		if nextState != currentState {
			s.record.RuntimeState = nextState
			stateChanged = true
		}
	}
	out := make([]agentBrokerAttachmentTarget, 0, len(s.attachments))
	for attachID, attachment := range s.attachments {
		if attachment == nil || attachment.conn == nil || attachment.paused {
			continue
		}
		out = append(out, agentBrokerAttachmentTarget{
			attachID: attachID,
			conn:     attachment.conn,
		})
	}
	var snapshot *agentRuntimeSessionRecord
	if len(payload) > 0 && (stateChanged || s.lastWriteAt.IsZero() || now.Sub(s.lastWriteAt) >= agentRuntimeWriteInterval) {
		s.record.Seq++
		s.record.UpdatedAt = now.Unix()
		record := s.record
		snapshot = &record
		s.lastWriteAt = now
	}
	s.mu.Unlock()
	return out, snapshot, signalEvents
}

func (s *agentBrokerSession) updateScreenLocked(payload []byte, at int64) {
	text := sanitizeTerminalOutput(payload)
	if text == "" {
		return
	}
	full := s.screenPartial + text
	parts := strings.Split(full, "\n")
	s.screenPartial = parts[len(parts)-1]
	if len(parts) > 1 {
		s.screenLines = append(s.screenLines, parts[:len(parts)-1]...)
		if len(s.screenLines) > agentScreenStateMaxLines {
			s.screenLines = append([]string(nil), s.screenLines[len(s.screenLines)-agentScreenStateMaxLines:]...)
		}
	}
	s.screenSeq++
	s.screenAt = at
}

func (s *agentBrokerSession) screenSnapshot(rows int) (int64, int64, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rows <= 0 {
		rows = 40
	}
	lines := append([]string(nil), s.screenLines...)
	if strings.TrimSpace(s.screenPartial) != "" {
		lines = append(lines, s.screenPartial)
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	return s.screenSeq, s.screenAt, strings.Join(lines, "\n")
}

func (s *agentBrokerSession) markIdleOnSilence(now time.Time) *agentRuntimeSessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record.RuntimeState != "running" {
		return nil
	}
	if s.lastOutputAt.IsZero() {
		return nil
	}
	if now.Sub(s.lastOutputAt) < agentRuntimeIdleAfterSilence {
		return nil
	}
	s.record.RuntimeState = "idle"
	s.record.Seq++
	s.record.UpdatedAt = now.Unix()
	record := s.record
	s.lastWriteAt = now
	return &record
}

func (s *agentBrokerSession) removeAttachment(attachID string) {
	attachID = strings.TrimSpace(attachID)
	if attachID == "" {
		return
	}
	s.mu.Lock()
	attachment := s.attachments[attachID]
	delete(s.attachments, attachID)
	s.mu.Unlock()
	if attachment != nil && attachment.conn != nil {
		_ = attachment.conn.Close()
	}
}

func (s *agentBrokerSession) closeAllAttachments() {
	s.mu.Lock()
	conns := make([]*net.UnixConn, 0, len(s.attachments))
	for _, attachment := range s.attachments {
		if attachment != nil && attachment.conn != nil {
			conns = append(conns, attachment.conn)
		}
	}
	s.attachments = map[string]*agentBrokerAttachment{}
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

type agentBrokerServer struct {
	rootPath string

	mu         sync.Mutex
	lastActive time.Time
	sessions   map[string]*agentBrokerSession
}

var (
	embeddedAgentBrokerMu      sync.Mutex
	embeddedAgentBrokerRunning = map[string]bool{}
)

func newAgentBrokerServer(rootPath string) *agentBrokerServer {
	return &agentBrokerServer{
		rootPath:   strings.TrimSpace(rootPath),
		lastActive: time.Now(),
		sessions:   map[string]*agentBrokerSession{},
	}
}

func (s *agentBrokerServer) refreshSessionStates(now time.Time) {
	s.mu.Lock()
	sessions := make([]*agentBrokerSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()

	for _, session := range sessions {
		if session == nil {
			continue
		}
		if snapshot := session.markIdleOnSilence(now); snapshot != nil {
			_ = saveAgentRuntimeSession(*snapshot)
		}
	}
}

func (s *agentBrokerServer) touch(now time.Time) {
	s.mu.Lock()
	s.lastActive = now
	s.mu.Unlock()
}

func (s *agentBrokerServer) shouldExitForIdle(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) > 0 {
		s.lastActive = now
		return false
	}
	return now.Sub(s.lastActive) >= agentBrokerIdleTimeout
}

func (s *agentBrokerServer) addSession(session *agentBrokerSession) {
	record := session.snapshot()
	s.mu.Lock()
	s.sessions[record.SessionID] = session
	s.lastActive = time.Now()
	s.mu.Unlock()
}

func (s *agentBrokerServer) getSession(sessionID string) (*agentBrokerSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	return session, ok
}

func (s *agentBrokerServer) deleteSession(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.lastActive = time.Now()
	s.mu.Unlock()
}

func (c *CLI) runAgentBroker(args []string) int {
	root, err := parseAgentBrokerOptions(args)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitUsage
	}
	socketPath, err := agentBrokerSocketPath(root)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve broker socket path: %v\n", err)
		return exitError
	}

	listener, err := listenAgentBroker(socketPath)
	if err != nil {
		fmt.Fprintf(c.Err, "listen broker socket: %v\n", err)
		return exitError
	}
	defer func() { _ = listener.Close() }()
	defer func() { _ = os.Remove(socketPath) }()

	server := newAgentBrokerServer(root)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				server.refreshSessionStates(now)
				if server.shouldExitForIdle(now) {
					_ = listener.Close()
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return exitOK
			}
			continue
		}
		go server.handleConn(conn)
	}
}

func parseAgentBrokerOptions(args []string) (string, error) {
	rootPath := ""
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case strings.HasPrefix(arg, "--root="):
			rootPath = strings.TrimSpace(strings.TrimPrefix(arg, "--root="))
			rest = rest[1:]
		case arg == "--root":
			if len(rest) < 2 {
				return "", fmt.Errorf("--root requires a value")
			}
			rootPath = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return "", fmt.Errorf("unknown flag for agent broker: %q", arg)
		}
	}
	if len(rest) > 0 {
		return "", fmt.Errorf("unexpected args for agent broker: %q", strings.Join(rest, " "))
	}
	if strings.TrimSpace(rootPath) == "" {
		return "", fmt.Errorf("--root is required")
	}
	return rootPath, nil
}

func ensureAgentBroker(root string) error {
	if err := pingAgentBroker(root); err == nil {
		return nil
	}
	if err := spawnAgentBroker(root); err != nil {
		return err
	}

	deadline := time.Now().Add(agentBrokerStartupTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := pingAgentBroker(root); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(80 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("broker startup timeout")
	}
	return fmt.Errorf("wait broker ready: %w", lastErr)
}

func spawnAgentBroker(root string) error {
	if strings.TrimSpace(os.Getenv(agentBrokerEmbeddedEnvKey)) == "1" {
		return spawnEmbeddedAgentBroker(root)
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	cmd := exec.Command(exePath, "agent", "_broker", "--root", root)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start broker process: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func spawnEmbeddedAgentBroker(root string) error {
	socketPath, err := agentBrokerSocketPath(root)
	if err != nil {
		return err
	}

	embeddedAgentBrokerMu.Lock()
	if embeddedAgentBrokerRunning[socketPath] {
		embeddedAgentBrokerMu.Unlock()
		return nil
	}
	embeddedAgentBrokerRunning[socketPath] = true
	embeddedAgentBrokerMu.Unlock()

	go func() {
		cli := New(io.Discard, io.Discard)
		_ = cli.runAgentBroker([]string{"--root", root})
		embeddedAgentBrokerMu.Lock()
		delete(embeddedAgentBrokerRunning, socketPath)
		embeddedAgentBrokerMu.Unlock()
	}()
	return nil
}

func startSessionWithAgentBroker(root string, req agentBrokerStartRequest) (agentBrokerStartResult, error) {
	resp, err := sendAgentBrokerRequest(root, agentBrokerRequest{
		Action:         agentBrokerActionStart,
		WorkspaceID:    strings.TrimSpace(req.WorkspaceID),
		ExecutionScope: strings.TrimSpace(req.ExecutionScope),
		RepoKey:        strings.TrimSpace(req.RepoKey),
		Kind:           strings.TrimSpace(req.Kind),
		ExecDir:        strings.TrimSpace(req.ExecDir),
		Cols:           req.Cols,
		Rows:           req.Rows,
	})
	if err != nil {
		return agentBrokerStartResult{}, err
	}
	return agentBrokerStartResult{
		SessionID: strings.TrimSpace(resp.SessionID),
		PID:       resp.PID,
	}, nil
}

func stopSessionWithAgentBroker(root string, sessionID string) error {
	_, err := sendAgentBrokerRequest(root, agentBrokerRequest{
		Action:    agentBrokerActionStop,
		SessionID: strings.TrimSpace(sessionID),
	})
	return err
}

func sendInputToAgentBroker(root string, sessionID string, input string) error {
	_, err := sendAgentBrokerRequest(root, agentBrokerRequest{
		Action:    agentBrokerActionInput,
		SessionID: strings.TrimSpace(sessionID),
		Input:     input,
	})
	if err != nil && strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "unknown broker action") {
		return fmt.Errorf("broker does not support input action yet (likely old process); restart target agent session and retry")
	}
	return err
}

func claimControlWithAgentBroker(root string, sessionID string, clientID string) (bool, bool, error) {
	resp, err := sendAgentBrokerRequest(root, agentBrokerRequest{
		Action:    agentBrokerActionClaim,
		SessionID: strings.TrimSpace(sessionID),
		ClientID:  strings.TrimSpace(clientID),
	})
	if err != nil {
		return false, false, err
	}
	return resolveAttachLeaseCompat(resp.InputOK), resolveAttachLeaseCompat(resp.ResizeOK), nil
}

type agentScreenSnapshot struct {
	SessionID string
	Seq       int64
	At        int64
	Screen    string
}

func getAgentScreenSnapshotViaBroker(root string, sessionID string, rows int) (agentScreenSnapshot, error) {
	resp, err := sendAgentBrokerRequest(root, agentBrokerRequest{
		Action:    agentBrokerActionScreen,
		SessionID: strings.TrimSpace(sessionID),
		Rows:      rows,
	})
	if err != nil {
		return agentScreenSnapshot{}, err
	}
	return agentScreenSnapshot{
		SessionID: strings.TrimSpace(resp.SessionID),
		Seq:       resp.ScreenSeq,
		At:        resp.ScreenAt,
		Screen:    resp.Screen,
	}, nil
}

func listAgentRuntimeSessionsViaBroker(root string) ([]agentRuntimeSessionRecord, error) {
	resp, err := sendAgentBrokerRequest(root, agentBrokerRequest{Action: agentBrokerActionList})
	if err != nil {
		return nil, err
	}
	rows := make([]agentRuntimeSessionRecord, 0, len(resp.Sessions))
	for _, row := range resp.Sessions {
		normalizeAgentRuntimeSessionRecord(&row)
		rows = append(rows, row)
	}
	sortAgentRuntimeSessions(rows)
	return rows, nil
}

func pingAgentBroker(root string) error {
	_, err := sendAgentBrokerRequest(root, agentBrokerRequest{Action: agentBrokerActionPing})
	return err
}

func sendAgentBrokerRequest(root string, req agentBrokerRequest) (agentBrokerResponse, error) {
	socketPath, err := agentBrokerSocketPath(root)
	if err != nil {
		return agentBrokerResponse{}, fmt.Errorf("resolve broker socket path: %w", err)
	}
	conn, err := net.DialTimeout("unix", socketPath, agentBrokerDialTimeout)
	if err != nil {
		return agentBrokerResponse{}, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return agentBrokerResponse{}, fmt.Errorf("send broker request: %w", err)
	}
	var resp agentBrokerResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return agentBrokerResponse{}, fmt.Errorf("decode broker response: %w", err)
	}
	if !resp.OK {
		if strings.TrimSpace(resp.Error) == "" {
			return agentBrokerResponse{}, errors.New("broker request failed")
		}
		return agentBrokerResponse{}, errors.New(resp.Error)
	}
	return resp, nil
}

func agentBrokerSocketPath(root string) (string, error) {
	kraHome, err := paths.KraHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve KRA_HOME: %w", err)
	}
	dir := filepath.Join(kraHome, "run", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create broker socket dir: %w", err)
	}
	path := filepath.Join(dir, hashRootPath(root)+".sock")
	// Unix domain sockets have short path limits (often ~104 bytes on BSD/macOS).
	if runtime.GOOS != "windows" && len(path) >= 100 {
		shortDir := filepath.Join(os.TempDir(), "kra-agent-sock")
		if err := os.MkdirAll(shortDir, 0o755); err != nil {
			return "", fmt.Errorf("create short broker socket dir: %w", err)
		}
		homeHash := shortHashText(kraHome)
		rootHash := hashRootPath(root)
		path = filepath.Join(shortDir, homeHash[:8]+"-"+rootHash[:16]+".sock")
	}
	return path, nil
}

func shortHashText(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}

func listenAgentBroker(socketPath string) (*net.UnixListener, error) {
	addr := &net.UnixAddr{Name: socketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err == nil {
		_ = os.Chmod(socketPath, 0o600)
		return listener, nil
	}

	if strings.TrimSpace(socketPath) != "" {
		if _, statErr := os.Stat(socketPath); statErr == nil {
			conn, dialErr := net.DialTimeout("unix", socketPath, agentBrokerDialTimeout)
			if dialErr != nil {
				_ = os.Remove(socketPath)
				listener, err = net.ListenUnix("unix", addr)
				if err == nil {
					_ = os.Chmod(socketPath, 0o600)
					return listener, nil
				}
			} else {
				_ = conn.Close()
			}
		}
	}
	return nil, err
}

func (s *agentBrokerServer) handleConn(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	s.touch(time.Now())

	var req agentBrokerRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(agentBrokerResponse{OK: false, Error: "decode request"})
		return
	}
	if strings.TrimSpace(strings.ToLower(req.Action)) == agentBrokerActionAttach {
		s.handleAttachRequest(conn, req)
		return
	}
	resp := s.handleRequest(req)
	_ = json.NewEncoder(conn).Encode(resp)
}

func (s *agentBrokerServer) handleRequest(req agentBrokerRequest) agentBrokerResponse {
	switch strings.TrimSpace(strings.ToLower(req.Action)) {
	case agentBrokerActionPing:
		return agentBrokerResponse{OK: true}
	case agentBrokerActionStart:
		return s.handleStartRequest(req)
	case agentBrokerActionStop:
		return s.handleStopRequest(req)
	case agentBrokerActionResize:
		return s.handleResizeRequest(req)
	case agentBrokerActionList:
		return s.handleListRequest()
	case agentBrokerActionInput:
		return s.handleInputRequest(req)
	case agentBrokerActionScreen:
		return s.handleScreenSnapshotRequest(req)
	case agentBrokerActionClaim:
		return s.handleClaimControlRequest(req)
	case agentBrokerActionAttach:
		return agentBrokerResponse{OK: false, Error: "attach action requires stream handler"}
	default:
		return agentBrokerResponse{OK: false, Error: "unknown broker action"}
	}
}

func (s *agentBrokerServer) handleListRequest() agentBrokerResponse {
	rows := s.listSessionsSnapshot()
	return agentBrokerResponse{
		OK:       true,
		Sessions: rows,
	}
}

func (s *agentBrokerServer) listSessionsSnapshot() []agentRuntimeSessionRecord {
	s.mu.Lock()
	sessions := make([]*agentBrokerSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()

	rows := make([]agentRuntimeSessionRecord, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		row := session.snapshot()
		normalizeAgentRuntimeSessionRecord(&row)
		rows = append(rows, row)
	}
	sortAgentRuntimeSessions(rows)
	return rows
}

func (s *agentBrokerServer) handleStartRequest(req agentBrokerRequest) agentBrokerResponse {
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	scope := strings.TrimSpace(req.ExecutionScope)
	repoKey := strings.TrimSpace(req.RepoKey)
	kind := strings.TrimSpace(req.Kind)
	execDir := strings.TrimSpace(req.ExecDir)
	cols := req.Cols
	rows := req.Rows
	if workspaceID == "" || kind == "" || execDir == "" {
		return agentBrokerResponse{OK: false, Error: "invalid start request"}
	}
	if scope == "" {
		scope = "workspace"
	}

	cmd := exec.Command(kind)
	cmd.Dir = execDir
	cmd.Env = append(os.Environ(), "KRA_AGENT_WORKSPACE="+workspaceID)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return agentBrokerResponse{OK: false, Error: fmt.Sprintf("start agent process: %v", err)}
	}
	applyPTYSize(ptmx, cols, rows)

	now := time.Now()
	sessionID := newAgentRuntimeSessionID(now, cmd.Process.Pid)
	record := agentRuntimeSessionRecord{
		SessionID:      sessionID,
		RootPath:       s.rootPath,
		WorkspaceID:    workspaceID,
		ExecutionScope: scope,
		RepoKey:        repoKey,
		Kind:           kind,
		PID:            cmd.Process.Pid,
		StartedAt:      now.Unix(),
		UpdatedAt:      now.Unix(),
		Seq:            1,
		RuntimeState:   "running",
	}
	if err := saveAgentRuntimeSession(record); err != nil {
		_ = terminateAgentPID(cmd.Process.Pid)
		_ = ptmx.Close()
		return agentBrokerResponse{OK: false, Error: fmt.Sprintf("save runtime session: %v", err)}
	}

	session := &agentBrokerSession{
		cmd:         cmd,
		ptmx:        ptmx,
		record:      record,
		attachments: map[string]*agentBrokerAttachment{},
		seqParser:   newAgentTerminalSequenceParser(),
	}
	s.addSession(session)

	go s.forwardSessionOutput(session)
	go s.waitSessionExit(session)

	return agentBrokerResponse{
		OK:        true,
		SessionID: sessionID,
		PID:       cmd.Process.Pid,
	}
}

func (s *agentBrokerServer) handleStopRequest(req agentBrokerRequest) agentBrokerResponse {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return agentBrokerResponse{OK: false, Error: "session_id is required"}
	}
	session, ok := s.getSession(sessionID)
	if !ok {
		return agentBrokerResponse{OK: false, Error: "session not found"}
	}
	record := session.snapshot()
	if record.PID <= 0 {
		return agentBrokerResponse{OK: false, Error: "invalid session pid"}
	}
	if err := terminateAgentPID(record.PID); err != nil {
		return agentBrokerResponse{OK: false, Error: fmt.Sprintf("terminate session process: %v", err)}
	}
	return agentBrokerResponse{OK: true}
}

func (s *agentBrokerServer) handleResizeRequest(req agentBrokerRequest) agentBrokerResponse {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return agentBrokerResponse{OK: false, Error: "session_id is required"}
	}
	if req.Cols <= 0 || req.Rows <= 0 {
		return agentBrokerResponse{OK: false, Error: "invalid terminal size"}
	}
	session, ok := s.getSession(sessionID)
	if !ok {
		return agentBrokerResponse{OK: false, Error: "session not found"}
	}
	if !session.canResize(req.ClientID) {
		return agentBrokerResponse{OK: false, Error: "resize lease denied"}
	}
	applyPTYSize(session.ptmx, req.Cols, req.Rows)
	return agentBrokerResponse{OK: true}
}

func (s *agentBrokerServer) handleInputRequest(req agentBrokerRequest) agentBrokerResponse {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return agentBrokerResponse{OK: false, Error: "session_id is required"}
	}
	input := req.Input
	if input == "" {
		return agentBrokerResponse{OK: false, Error: "input is required"}
	}
	session, ok := s.getSession(sessionID)
	if !ok {
		return agentBrokerResponse{OK: false, Error: "session not found"}
	}
	snapshot := session.snapshot()
	if snapshot.RuntimeState == "exited" {
		return agentBrokerResponse{OK: false, Error: "session is exited"}
	}
	if err := writeAllFile(session.ptmx, []byte(input)); err != nil {
		return agentBrokerResponse{OK: false, Error: fmt.Sprintf("write input to pty: %v", err)}
	}
	return agentBrokerResponse{OK: true}
}

func (s *agentBrokerServer) handleScreenSnapshotRequest(req agentBrokerRequest) agentBrokerResponse {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return agentBrokerResponse{OK: false, Error: "session_id is required"}
	}
	session, ok := s.getSession(sessionID)
	if !ok {
		return agentBrokerResponse{OK: false, Error: "session not found"}
	}
	seq, at, screen := session.screenSnapshot(req.Rows)
	return agentBrokerResponse{
		OK:        true,
		SessionID: sessionID,
		ScreenSeq: seq,
		ScreenAt:  at,
		Screen:    screen,
	}
}

func (s *agentBrokerServer) handleClaimControlRequest(req agentBrokerRequest) agentBrokerResponse {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return agentBrokerResponse{OK: false, Error: "session_id is required"}
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		return agentBrokerResponse{OK: false, Error: "client_id is required"}
	}
	session, ok := s.getSession(sessionID)
	if !ok {
		return agentBrokerResponse{OK: false, Error: "session not found"}
	}
	inputOK, resizeOK := session.claimControl(clientID)
	if !inputOK {
		return agentBrokerResponse{OK: false, Error: "claim control denied"}
	}
	return agentBrokerResponse{
		OK:        true,
		SessionID: sessionID,
		InputOK:   leaseBoolPtr(inputOK),
		ResizeOK:  leaseBoolPtr(resizeOK),
	}
}

func (s *agentBrokerServer) handleAttachRequest(conn *net.UnixConn, req agentBrokerRequest) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		_ = json.NewEncoder(conn).Encode(agentBrokerResponse{OK: false, Error: "session_id is required"})
		return
	}
	session, ok := s.getSession(sessionID)
	if !ok {
		_ = json.NewEncoder(conn).Encode(agentBrokerResponse{OK: false, Error: "session not found"})
		return
	}
	if req.ForceRedraw {
		applyPTYSizeForAttach(session.ptmx, req.Cols, req.Rows)
	} else {
		applyPTYSize(session.ptmx, req.Cols, req.Rows)
	}
	clientID := strings.TrimSpace(req.ClientID)
	inputOK, resizeOK := session.acquireControl(clientID, req.AttachMode)
	if err := json.NewEncoder(conn).Encode(agentBrokerResponse{
		OK:        true,
		SessionID: sessionID,
		InputOK:   leaseBoolPtr(inputOK),
		ResizeOK:  leaseBoolPtr(resizeOK),
	}); err != nil {
		return
	}

	attachID := session.addAttachment(conn, true, clientID)
	defer session.removeAttachment(attachID)
	if err := session.replayOutputHistory(attachID); err != nil {
		return
	}

	s.forwardAttachInputToSession(session, conn, inputOK)
}

func (s *agentBrokerServer) forwardAttachInputToSession(session *agentBrokerSession, conn *net.UnixConn, inputOK bool) {
	if session == nil || conn == nil || session.ptmx == nil {
		return
	}
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 && inputOK {
			chunk := sanitizeAttachInputChunk(buf[:n])
			if len(chunk) == 0 {
				if err != nil {
					return
				}
				continue
			}
			if werr := writeAllFile(session.ptmx, chunk); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func sanitizeAttachInputChunk(chunk []byte) []byte {
	if len(chunk) == 0 {
		return nil
	}
	// Ctrl-C is reserved for local detach in attach mode.
	// Even if it slips through client-side filtering, do not forward it to PTY.
	if !containsByte(chunk, 0x03) {
		return append([]byte(nil), chunk...)
	}
	filtered := make([]byte, 0, len(chunk))
	for _, b := range chunk {
		if b == 0x03 {
			continue
		}
		filtered = append(filtered, b)
	}
	return filtered
}

func containsByte(chunk []byte, target byte) bool {
	for _, b := range chunk {
		if b == target {
			return true
		}
	}
	return false
}

func writeAllFile(file *os.File, b []byte) error {
	for len(b) > 0 {
		n, err := file.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func (s *agentBrokerServer) forwardSessionOutput(session *agentBrokerSession) {
	buf := make([]byte, 8192)
	for {
		n, err := session.ptmx.Read(buf)
		if n > 0 {
			payload := append([]byte(nil), buf[:n]...)
			attachments, snapshot, signalEvents := session.appendOutputAndSnapshotWritable(payload, time.Now())
			for _, target := range attachments {
				if target.conn == nil {
					continue
				}
				_ = target.conn.SetWriteDeadline(time.Now().Add(agentBrokerAttachWriteTTL))
				if werr := writeAllUnixConn(target.conn, payload); werr != nil {
					session.removeAttachment(target.attachID)
				}
				_ = target.conn.SetWriteDeadline(time.Time{})
			}
			if snapshot != nil {
				_ = saveAgentRuntimeSession(*snapshot)
				if len(signalEvents) > 0 {
					_ = appendAgentRuntimeSignalEvents(*snapshot, signalEvents)
				}
			} else if len(signalEvents) > 0 {
				record := session.snapshot()
				_ = appendAgentRuntimeSignalEvents(record, signalEvents)
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *agentBrokerServer) waitSessionExit(session *agentBrokerSession) {
	waitErr := session.cmd.Wait()
	_ = session.ptmx.Close()
	session.closeAllAttachments()

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

	session.update(func(record *agentRuntimeSessionRecord) {
		record.Seq++
		record.UpdatedAt = time.Now().Unix()
		record.RuntimeState = finalState
		record.ExitCode = &exitCode
	})

	record := session.snapshot()
	s.deleteSession(record.SessionID)
}

type agentBrokerAttachResult struct {
	Conn     *net.UnixConn
	ClientID string
	InputOK  bool
	ResizeOK bool
}

func attachSessionWithAgentBroker(root string, sessionID string, cols int, rows int, forceRedraw bool, clientID string, attachMode string) (agentBrokerAttachResult, error) {
	socketPath, err := agentBrokerSocketPath(root)
	if err != nil {
		return agentBrokerAttachResult{}, fmt.Errorf("resolve broker socket path: %w", err)
	}
	rawConn, err := net.DialTimeout("unix", socketPath, agentBrokerDialTimeout)
	if err != nil {
		return agentBrokerAttachResult{}, err
	}
	conn, ok := rawConn.(*net.UnixConn)
	if !ok {
		_ = rawConn.Close()
		return agentBrokerAttachResult{}, errors.New("invalid broker connection type")
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = fmt.Sprintf("c-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	if err := json.NewEncoder(conn).Encode(agentBrokerRequest{
		Action:      agentBrokerActionAttach,
		SessionID:   strings.TrimSpace(sessionID),
		Cols:        cols,
		Rows:        rows,
		ForceRedraw: forceRedraw,
		ClientID:    clientID,
		AttachMode:  strings.TrimSpace(attachMode),
	}); err != nil {
		_ = conn.Close()
		return agentBrokerAttachResult{}, fmt.Errorf("send broker attach request: %w", err)
	}
	var resp agentBrokerResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		_ = conn.Close()
		return agentBrokerAttachResult{}, fmt.Errorf("decode broker attach response: %w", err)
	}
	if !resp.OK {
		_ = conn.Close()
		if strings.TrimSpace(resp.Error) == "" {
			return agentBrokerAttachResult{}, errors.New("broker attach request failed")
		}
		return agentBrokerAttachResult{}, errors.New(resp.Error)
	}
	_ = conn.SetDeadline(time.Time{})
	return agentBrokerAttachResult{
		Conn:     conn,
		ClientID: clientID,
		InputOK:  resolveAttachLeaseCompat(resp.InputOK),
		ResizeOK: resolveAttachLeaseCompat(resp.ResizeOK),
	}, nil
}

func resizeSessionWithAgentBroker(root string, sessionID string, clientID string, cols int, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	_, err := sendAgentBrokerRequest(root, agentBrokerRequest{
		Action:    agentBrokerActionResize,
		SessionID: strings.TrimSpace(sessionID),
		ClientID:  strings.TrimSpace(clientID),
		Cols:      cols,
		Rows:      rows,
	})
	return err
}

func applyPTYSize(ptmx *os.File, cols int, rows int) {
	if ptmx == nil || cols <= 0 || rows <= 0 {
		return
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

func applyPTYSizeForAttach(ptmx *os.File, cols int, rows int) {
	if ptmx == nil || cols <= 0 || rows <= 0 {
		return
	}
	// Force a SIGWINCH-style redraw even when dimensions are unchanged.
	// Some interactive CLIs keep inline viewport anchors across reattach.
	if rows > 1 {
		_ = pty.Setsize(ptmx, &pty.Winsize{
			Cols: uint16(cols),
			Rows: uint16(rows - 1),
		})
	} else if cols > 1 {
		_ = pty.Setsize(ptmx, &pty.Winsize{
			Cols: uint16(cols - 1),
			Rows: uint16(rows),
		})
	}
	applyPTYSize(ptmx, cols, rows)
}

func leaseBoolPtr(v bool) *bool {
	return &v
}

func resolveAttachLeaseCompat(v *bool) bool {
	// Legacy broker does not emit lease fields.
	if v == nil {
		return true
	}
	return *v
}

func sanitizeTerminalOutput(payload []byte) string {
	var out strings.Builder
	for i := 0; i < len(payload); i++ {
		c := payload[i]
		if c == 0x1b { // ESC
			if i+1 >= len(payload) {
				continue
			}
			next := payload[i+1]
			if next == '[' {
				i += 2
				for ; i < len(payload); i++ {
					if payload[i] >= 0x40 && payload[i] <= 0x7e {
						break
					}
				}
				continue
			}
			if next == ']' {
				i += 2
				for ; i < len(payload); i++ {
					if payload[i] == 0x07 {
						break
					}
					if payload[i] == 0x1b && i+1 < len(payload) && payload[i+1] == '\\' {
						i++
						break
					}
				}
				continue
			}
			continue
		}
		if c == '\r' {
			continue
		}
		if c < 0x20 && c != '\n' && c != '\t' {
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}
