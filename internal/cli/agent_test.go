package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCLI_RootHelp_HidesAgent(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if strings.Contains(out.String(), "agent") {
		t.Fatalf("root usage should not include agent: %q", out.String())
	}
}

func TestCLI_AgentList_AvailableInExperimentalBuild(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "list"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Agents:") {
		t.Fatalf("stdout should include agent section: %q", out.String())
	}
}

func TestCLI_Agent_Help(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "kra agent <subcommand>") {
		t.Fatalf("stdout missing agent usage: %q", out.String())
	}
}

func TestCLI_AgentList_Empty(t *testing.T) {
	prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "list"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "Agents:") || !strings.Contains(got, "(none)") {
		t.Fatalf("stdout missing empty agent list: %q", got)
	}
}

func TestCLI_AgentList_FilterTSV(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-1",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "repo",
		RepoKey:        "repo/a",
		Kind:           "codex",
		PID:            101,
		StartedAt:      100,
		UpdatedAt:      110,
		Seq:            1,
		RuntimeState:   "running",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-2",
		RootPath:       root,
		WorkspaceID:    "WS-2",
		ExecutionScope: "workspace",
		RepoKey:        "",
		Kind:           "copilot",
		PID:            202,
		StartedAt:      90,
		UpdatedAt:      95,
		Seq:            1,
		RuntimeState:   "exited",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "list", "--workspace", "WS-1", "--format", "tsv"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "session_id\tworkspace_id\texecution_scope\trepo_key\tkind\truntime_state\tstarted_at\tupdated_at\tpid\texit_code") {
		t.Fatalf("tsv header missing: %q", got)
	}
	if !strings.Contains(got, "s-1\tWS-1\trepo\trepo/a\tcodex\trunning\t100\t110\t101\t-") {
		t.Fatalf("filtered row missing: %q", got)
	}
	if strings.Contains(got, "s-2\t") {
		t.Fatalf("unexpected filtered row: %q", got)
	}
}

func TestCLI_AgentList_DefaultHidesExited_AndAllShowsExited(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	now := time.Now().Unix()
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-run",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "workspace",
		Kind:           "codex",
		PID:            101,
		StartedAt:      now - 10,
		UpdatedAt:      now - 1,
		Seq:            1,
		RuntimeState:   "running",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-exit",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "workspace",
		Kind:           "codex",
		PID:            102,
		StartedAt:      now - 20,
		UpdatedAt:      now - 2,
		Seq:            2,
		RuntimeState:   "exited",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	var outDefault bytes.Buffer
	var errDefault bytes.Buffer
	cDefault := New(&outDefault, &errDefault)
	if code := cDefault.Run([]string{"agent", "list", "--workspace", "WS-1"}); code != exitOK {
		t.Fatalf("default list exit code=%d, want %d (stderr=%q)", code, exitOK, errDefault.String())
	}
	if strings.Contains(outDefault.String(), "session:s-exit") {
		t.Fatalf("default list should hide exited sessions: %q", outDefault.String())
	}
	if !strings.Contains(outDefault.String(), "• WS-1  running:1") {
		t.Fatalf("default list should include workspace summary: %q", outDefault.String())
	}
	if !strings.Contains(outDefault.String(), "└─") || !strings.Contains(outDefault.String(), "session:s-run") {
		t.Fatalf("default list should render tree session rows: %q", outDefault.String())
	}

	var outAll bytes.Buffer
	var errAll bytes.Buffer
	cAll := New(&outAll, &errAll)
	if code := cAll.Run([]string{"agent", "list", "--workspace", "WS-1", "--all"}); code != exitOK {
		t.Fatalf("all list exit code=%d, want %d (stderr=%q)", code, exitOK, errAll.String())
	}
	if !strings.Contains(outAll.String(), "• WS-1  running:1  exited:1") {
		t.Fatalf("--all should include exited summary count: %q", outAll.String())
	}
	if !strings.Contains(outAll.String(), "session:s-exit") || !strings.Contains(outAll.String(), "session:s-run") {
		t.Fatalf("--all should include both running and exited sessions: %q", outAll.String())
	}
}

func TestCLI_AgentList_AutoExpandsWorkspaceWithRepoSessions(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	now := time.Now().Unix()
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-ws",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "workspace",
		Kind:           "codex",
		PID:            201,
		StartedAt:      now - 20,
		UpdatedAt:      now - 3,
		Seq:            1,
		RuntimeState:   "running",
	}); err != nil {
		t.Fatalf("save workspace session: %v", err)
	}
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-repo",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "repo",
		RepoKey:        "repo/api",
		Kind:           "codex",
		PID:            202,
		StartedAt:      now - 10,
		UpdatedAt:      now - 1,
		Seq:            1,
		RuntimeState:   "running",
	}); err != nil {
		t.Fatalf("save repo session: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	if code := c.Run([]string{"agent", "list", "--workspace", "WS-1"}); code != exitOK {
		t.Fatalf("list exit code=%d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "• WS-1  running:2") {
		t.Fatalf("workspace summary missing location counts: %q", got)
	}
	if !strings.Contains(got, "session:s-repo") || !strings.Contains(got, "repo:repo/api") {
		t.Fatalf("repo workspace should render session rows: %q", got)
	}
	if !strings.Contains(got, "├─") || !strings.Contains(got, "└─") {
		t.Fatalf("repo workspace should render tree branches: %q", got)
	}
	wsPos := strings.Index(got, "workspace")
	repoPos := strings.Index(got, "repo:repo/api")
	if wsPos < 0 || repoPos < 0 || wsPos > repoPos {
		t.Fatalf("workspace session should be listed before repo session: %q", got)
	}
}

func TestCLI_AgentLS_AliasOfList(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "ls"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Agents:") {
		t.Fatalf("stdout should include agent section: %q", out.String())
	}
}

func TestCLI_AgentBoard_GroupsByWorkspace(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-1",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "workspace",
		Kind:           "codex",
		PID:            101,
		StartedAt:      100,
		UpdatedAt:      110,
		Seq:            1,
		RuntimeState:   "running",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-2",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "repo",
		RepoKey:        "repo/api",
		Kind:           "codex",
		PID:            102,
		StartedAt:      100,
		UpdatedAt:      111,
		Seq:            1,
		RuntimeState:   "idle",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "board"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "Agent Board:") {
		t.Fatalf("board heading missing: %q", got)
	}
	if !strings.Contains(got, "WS-1") || !strings.Contains(got, "repo:repo/api") {
		t.Fatalf("board grouping output missing: %q", got)
	}
}

func TestCLI_AgentRun_RequiresWorkspaceAndKind(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "run"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "--workspace is required in non-interactive mode") {
		t.Fatalf("stderr should include missing workspace error: %q", err.String())
	}
}

func TestCLI_AgentRun_InferWorkspaceFromWorkspaceDir(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	workspaceDir := filepath.Join(root, "workspaces", "WS-1")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(workspaceDir); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	var out bytes.Buffer
	var stderr bytes.Buffer
	c := New(&out, &stderr)

	code := c.Run([]string{"agent", "run", "--kind", "codex"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, stderr.String())
	}
	if !strings.Contains(out.String(), "workspace=WS-1") {
		t.Fatalf("stdout should include inferred workspace id: %q", out.String())
	}
}

func TestCLI_AgentRun_InferWorkspaceFromRepoSubdir(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	repoSubdir := filepath.Join(root, "workspaces", "WS-1", "repos", "repo-a", "pkg")
	if err := os.MkdirAll(repoSubdir, 0o755); err != nil {
		t.Fatalf("mkdir repo subdir: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(repoSubdir); err != nil {
		t.Fatalf("chdir repo subdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	var out bytes.Buffer
	var stderr bytes.Buffer
	c := New(&out, &stderr)

	code := c.Run([]string{"agent", "run", "--kind", "codex"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, stderr.String())
	}
	if !strings.Contains(out.String(), "workspace=WS-1") {
		t.Fatalf("stdout should include inferred workspace id: %q", out.String())
	}
}

func TestCLI_AgentRun_RejectsRemovedTaskFlag(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "run", "--workspace", "WS-1", "--kind", "codex", "--task", "x"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), `unknown flag for agent run: "--task"`) {
		t.Fatalf("stderr should include unsupported flag error: %q", err.String())
	}
}

func TestCLI_AgentRun_WritesRuntimeSessionFile(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WS-1"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "run", "--workspace", "WS-1", "--kind", "codex"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	kraHome, homeErr := os.LookupEnv("KRA_HOME")
	if !homeErr || strings.TrimSpace(kraHome) == "" {
		t.Fatalf("KRA_HOME should be set in tests")
	}
	rootHash := hashRootPath(root)
	stateDir := filepath.Join(kraHome, "state", "agents", rootHash)
	entries, readErr := os.ReadDir(stateDir)
	if readErr != nil {
		t.Fatalf("read runtime state dir: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("runtime session file count=%d, want=1", len(entries))
	}
	sessionPath := filepath.Join(stateDir, entries[0].Name())
	b, fileErr := os.ReadFile(sessionPath)
	if fileErr != nil {
		t.Fatalf("read runtime session file: %v", fileErr)
	}
	var got agentRuntimeSessionRecord
	if unmarshalErr := json.Unmarshal(b, &got); unmarshalErr != nil {
		t.Fatalf("unmarshal runtime session: %v", unmarshalErr)
	}
	if got.RootPath != root || got.WorkspaceID != "WS-1" || got.Kind != "codex" {
		t.Fatalf("unexpected runtime session payload: %+v", got)
	}
	if got.ExecutionScope != "workspace" {
		t.Fatalf("execution_scope=%q, want=workspace", got.ExecutionScope)
	}
	if got.RuntimeState != "exited" {
		t.Fatalf("runtime_state=%q, want=exited", got.RuntimeState)
	}
	if got.Seq != 2 {
		t.Fatalf("seq=%d, want=2", got.Seq)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("exit_code should be 0, got=%v", got.ExitCode)
	}
	if !strings.Contains(out.String(), "session="+got.SessionID) {
		t.Fatalf("stdout should include session id, got=%q", out.String())
	}
	if !strings.HasSuffix(got.SessionID, "-"+strconv.Itoa(got.PID)) {
		t.Fatalf("session_id should contain pid suffix, got=%q pid=%d", got.SessionID, got.PID)
	}
}

func TestCLI_AgentRun_DetachedViaBroker_AndStop(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	t.Setenv("KRA_AGENT_RUN_DRY_RUN", "")
	t.Setenv(agentBrokerEmbeddedEnvKey, "1")
	workspaceDir := filepath.Join(root, "workspaces", "WS-1")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	agentStubPath := filepath.Join(workspaceDir, "agent-stub.sh")
	agentStub := "#!/bin/sh\nwhile true; do sleep 1; done\n"
	if err := os.WriteFile(agentStubPath, []byte(agentStub), 0o755); err != nil {
		t.Fatalf("write agent stub: %v", err)
	}

	var runOut bytes.Buffer
	var runErr bytes.Buffer
	runCLI := New(&runOut, &runErr)
	startCode := runCLI.Run([]string{"agent", "run", "--workspace", "WS-1", "--kind", agentStubPath})
	if startCode != exitOK {
		t.Fatalf("run exit code=%d, want=%d (stderr=%q)", startCode, exitOK, runErr.String())
	}

	sessionID := extractSessionIDFromAgentStarted(runOut.String())
	if sessionID == "" {
		t.Fatalf("session id should be printed, stdout=%q", runOut.String())
	}
	if _, ok := waitForSessionRuntimeState(root, sessionID, "running", 5*time.Second); !ok {
		t.Fatalf("session should transition to running: session=%s", sessionID)
	}

	var stopOut bytes.Buffer
	var stopErr bytes.Buffer
	stopCLI := New(&stopOut, &stopErr)
	stopCode := stopCLI.Run([]string{"agent", "stop", "--session", sessionID})
	if stopCode != exitOK {
		t.Fatalf("stop exit code=%d, want=%d (stderr=%q)", stopCode, exitOK, stopErr.String())
	}
	if !strings.Contains(stopOut.String(), "agent stopped: session="+sessionID) {
		t.Fatalf("stop stdout should include session id, stdout=%q", stopOut.String())
	}

	exited, ok := waitForSessionRuntimeState(root, sessionID, "exited", 5*time.Second)
	if !ok {
		t.Fatalf("session should transition to exited: session=%s", sessionID)
	}
	if exited.ExitCode == nil {
		t.Fatalf("exit_code should be recorded on exited session")
	}
}

func TestCLI_AgentAttach_RequiresSessionInNonInteractiveMode(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("")

	code := c.Run([]string{"agent", "attach"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--session is required in non-interactive mode") {
		t.Fatalf("stderr should include missing session error: %q", err.String())
	}
}

func TestCLI_AgentAttach_BySession_StreamIO(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	t.Setenv("KRA_AGENT_RUN_DRY_RUN", "")
	t.Setenv(agentBrokerEmbeddedEnvKey, "1")
	workspaceDir := filepath.Join(root, "workspaces", "WS-1")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	agentStubPath := filepath.Join(workspaceDir, "agent-echo.sh")
	agentStub := "#!/bin/sh\nwhile IFS= read -r line; do\n  if [ \"$line\" = \"exit\" ]; then\n    echo \"bye\"\n    exit 0\n  fi\n  echo \"ECHO:$line\"\ndone\n"
	if err := os.WriteFile(agentStubPath, []byte(agentStub), 0o755); err != nil {
		t.Fatalf("write agent stub: %v", err)
	}

	var runOut bytes.Buffer
	var runErr bytes.Buffer
	runCLI := New(&runOut, &runErr)
	startCode := runCLI.Run([]string{"agent", "run", "--workspace", "WS-1", "--kind", agentStubPath})
	if startCode != exitOK {
		t.Fatalf("run exit code=%d, want=%d (stderr=%q)", startCode, exitOK, runErr.String())
	}

	sessionID := extractSessionIDFromAgentStarted(runOut.String())
	if sessionID == "" {
		t.Fatalf("session id should be printed, stdout=%q", runOut.String())
	}
	if _, ok := waitForSessionRuntimeState(root, sessionID, "running", 5*time.Second); !ok {
		t.Fatalf("session should transition to running: session=%s", sessionID)
	}

	var attachOut bytes.Buffer
	var attachErr bytes.Buffer
	attachCLI := New(&attachOut, &attachErr)
	attachCLI.In = strings.NewReader("hello\nexit\n")
	attachCode := attachCLI.Run([]string{"agent", "attach", "--session", sessionID})
	if attachCode != exitOK {
		t.Fatalf("attach exit code=%d, want=%d (stderr=%q)", attachCode, exitOK, attachErr.String())
	}
	got := attachOut.String()
	if !strings.Contains(got, "ECHO:hello") {
		t.Fatalf("attach output should include echoed input, stdout=%q", got)
	}
	if !strings.Contains(got, "bye") {
		t.Fatalf("attach output should include process exit marker, stdout=%q", got)
	}
	if _, ok := waitForSessionRuntimeState(root, sessionID, "exited", 5*time.Second); !ok {
		t.Fatalf("session should transition to exited: session=%s", sessionID)
	}
}

func TestFormatAgentAttachSelectorTitle(t *testing.T) {
	if got := formatAgentAttachSelectorTitle(agentContextScope{}); got != "Session to attach:" {
		t.Fatalf("unexpected title without scope: %q", got)
	}
	if got := formatAgentAttachSelectorTitle(agentContextScope{workspaceID: "WS-1"}); got != "Session to attach (workspace: WS-1):" {
		t.Fatalf("unexpected workspace title: %q", got)
	}
	if got := formatAgentAttachSelectorTitle(agentContextScope{workspaceID: "WS-1", repoKey: "repo-a"}); got != "Session to attach (workspace: WS-1 repo:repo-a):" {
		t.Fatalf("unexpected repo title: %q", got)
	}
}

func TestCLI_AgentStop_RequiresSessionOrWorkspace(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "stop"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "either --session or --workspace is required") {
		t.Fatalf("stderr should include selector error: %q", err.String())
	}
}

func TestCLI_AgentStop_FailsWhenSessionMissing(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "stop", "--session", "s-404"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "agent session not found: s-404") {
		t.Fatalf("stderr should include not found error: %q", err.String())
	}
}

func TestCLI_AgentStop_IdempotentWhenAlreadyExited(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	now := time.Now().Unix()
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-1",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "workspace",
		Kind:           "codex",
		PID:            999999,
		StartedAt:      now - 2,
		UpdatedAt:      now - 1,
		Seq:            2,
		RuntimeState:   "exited",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "stop", "--session", "s-1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "agent already stopped: session=s-1") {
		t.Fatalf("stdout should include idempotent message: %q", out.String())
	}
}

func TestCLI_AgentStop_StopsByWorkspaceSelector(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	now := time.Now().Unix()
	if err := saveAgentRuntimeSession(agentRuntimeSessionRecord{
		SessionID:      "s-1",
		RootPath:       root,
		WorkspaceID:    "WS-1",
		ExecutionScope: "workspace",
		Kind:           "codex",
		PID:            999999,
		StartedAt:      now - 2,
		UpdatedAt:      now - 1,
		Seq:            1,
		RuntimeState:   "running",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "stop", "--workspace", "WS-1", "--kind", "codex"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "agent stopped: session=s-1") {
		t.Fatalf("stdout missing stop message: %q", out.String())
	}

	rows, loadErr := loadAgentRuntimeSessions(root)
	if loadErr != nil {
		t.Fatalf("load activities: %v", loadErr)
	}
	if len(rows) != 1 {
		t.Fatalf("record count=%d, want=1", len(rows))
	}
	if rows[0].RuntimeState != "exited" {
		t.Fatalf("runtime_state=%q, want=exited", rows[0].RuntimeState)
	}
	if rows[0].UpdatedAt <= now-1 {
		t.Fatalf("updated_at should increase, got=%d", rows[0].UpdatedAt)
	}
}

func TestCLI_AgentLogs_SubcommandRemoved(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "logs"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), `unknown command: "agent logs"`) {
		t.Fatalf("stderr should include unknown subcommand error: %q", err.String())
	}
}

func extractSessionIDFromAgentStarted(stdout string) string {
	marker := "session="
	start := strings.Index(stdout, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	rest := stdout[start:]
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func waitForSessionRuntimeState(root string, sessionID string, state string, timeout time.Duration) (agentRuntimeSessionRecord, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rows, err := loadAgentRuntimeSessions(root)
		if err != nil {
			return agentRuntimeSessionRecord{}, false
		}
		for _, row := range rows {
			if row.SessionID != sessionID {
				continue
			}
			if row.RuntimeState == state {
				return row, true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return agentRuntimeSessionRecord{}, false
}
