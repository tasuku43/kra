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

	agentBrokerAcceptDeadline = 1 * time.Second
	agentBrokerDialTimeout    = 300 * time.Millisecond
	agentBrokerStartupTimeout = 4 * time.Second
	agentBrokerIdleTimeout    = 60 * time.Second
	agentBrokerAttachWriteTTL = 2 * time.Second
	agentBrokerEmbeddedEnvKey = "KRA_AGENT_BROKER_EMBEDDED"
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
}

type agentBrokerResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	SessionID string `json:"session_id,omitempty"`
	PID       int    `json:"pid,omitempty"`
}

type agentBrokerAttachment struct {
	conn   *net.UnixConn
	paused bool
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

func (s *agentBrokerSession) addAttachment(conn *net.UnixConn, paused bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attachments == nil {
		s.attachments = map[string]*agentBrokerAttachment{}
	}
	s.nextAttachID++
	attachID := fmt.Sprintf("a-%d", s.nextAttachID)
	s.attachments[attachID] = &agentBrokerAttachment{
		conn:   conn,
		paused: paused,
	}
	return attachID
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

func (s *agentBrokerSession) appendOutputAndSnapshotWritable(payload []byte) []agentBrokerAttachmentTarget {
	s.mu.Lock()
	if len(payload) > 0 {
		s.outputHistory = append(s.outputHistory, payload...)
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
	s.mu.Unlock()
	return out
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
	for {
		_ = listener.SetDeadline(time.Now().Add(agentBrokerAcceptDeadline))
		conn, err := listener.AcceptUnix()
		if err != nil {
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				if server.shouldExitForIdle(time.Now()) {
					return exitOK
				}
				continue
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
	case agentBrokerActionAttach:
		return agentBrokerResponse{OK: false, Error: "attach action requires stream handler"}
	default:
		return agentBrokerResponse{OK: false, Error: "unknown broker action"}
	}
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
	applyPTYSize(session.ptmx, req.Cols, req.Rows)
	return agentBrokerResponse{OK: true}
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
	if err := json.NewEncoder(conn).Encode(agentBrokerResponse{OK: true, SessionID: sessionID}); err != nil {
		return
	}

	attachID := session.addAttachment(conn, true)
	defer session.removeAttachment(attachID)
	if err := session.replayOutputHistory(attachID); err != nil {
		return
	}

	_, _ = io.Copy(session.ptmx, conn)
}

func (s *agentBrokerServer) forwardSessionOutput(session *agentBrokerSession) {
	buf := make([]byte, 8192)
	for {
		n, err := session.ptmx.Read(buf)
		if n > 0 {
			payload := append([]byte(nil), buf[:n]...)
			attachments := session.appendOutputAndSnapshotWritable(payload)
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

func attachSessionWithAgentBroker(root string, sessionID string, cols int, rows int, forceRedraw bool) (*net.UnixConn, error) {
	socketPath, err := agentBrokerSocketPath(root)
	if err != nil {
		return nil, fmt.Errorf("resolve broker socket path: %w", err)
	}
	rawConn, err := net.DialTimeout("unix", socketPath, agentBrokerDialTimeout)
	if err != nil {
		return nil, err
	}
	conn, ok := rawConn.(*net.UnixConn)
	if !ok {
		_ = rawConn.Close()
		return nil, errors.New("invalid broker connection type")
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := json.NewEncoder(conn).Encode(agentBrokerRequest{
		Action:      agentBrokerActionAttach,
		SessionID:   strings.TrimSpace(sessionID),
		Cols:        cols,
		Rows:        rows,
		ForceRedraw: forceRedraw,
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send broker attach request: %w", err)
	}
	var resp agentBrokerResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("decode broker attach response: %w", err)
	}
	if !resp.OK {
		_ = conn.Close()
		if strings.TrimSpace(resp.Error) == "" {
			return nil, errors.New("broker attach request failed")
		}
		return nil, errors.New(resp.Error)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func resizeSessionWithAgentBroker(root string, sessionID string, cols int, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	_, err := sendAgentBrokerRequest(root, agentBrokerRequest{
		Action:    agentBrokerActionResize,
		SessionID: strings.TrimSpace(sessionID),
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
