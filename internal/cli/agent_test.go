//go:build experimental

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	stateDir := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	content := `[
  {"workspace_id":"WS-1","agent_kind":"codex","started_at":100,"last_heartbeat_at":110,"status":"running","log_path":"logs/ws1.log"},
  {"workspace_id":"WS-2","agent_kind":"copilot","started_at":90,"last_heartbeat_at":95,"status":"failed","log_path":"logs/ws2.log"}
]`
	if err := os.WriteFile(filepath.Join(stateDir, agentActivitiesFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write activity file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "list", "--workspace", "WS-1", "--format", "tsv"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "workspace_id\tagent_kind\tstarted_at\tlast_heartbeat_at\tstatus\tlog_path") {
		t.Fatalf("tsv header missing: %q", got)
	}
	if !strings.Contains(got, "WS-1\tcodex\t100\t110\trunning\tlogs/ws1.log") {
		t.Fatalf("filtered row missing: %q", got)
	}
	if strings.Contains(got, "WS-2\t") {
		t.Fatalf("unexpected filtered row: %q", got)
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

func TestCLI_AgentRun_RequiresWorkspaceAndKind(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "run", "--workspace", "WS-1"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--kind is required") {
		t.Fatalf("stderr should include missing kind error: %q", err.String())
	}
}

func TestCLI_AgentRun_WritesRunningRecord(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "run", "--workspace", "WS-1", "--kind", "codex", "--log-path", "logs/ws1.log"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "agent started: workspace=WS-1 kind=codex") {
		t.Fatalf("stdout missing start message: %q", out.String())
	}

	b, readErr := os.ReadFile(filepath.Join(root, ".kra", "state", agentActivitiesFilename))
	if readErr != nil {
		t.Fatalf("read activity file: %v", readErr)
	}
	var rows []agentActivityRecord
	if unmarshalErr := json.Unmarshal(b, &rows); unmarshalErr != nil {
		t.Fatalf("unmarshal activity file: %v", unmarshalErr)
	}
	if len(rows) != 1 {
		t.Fatalf("record count=%d, want=1", len(rows))
	}
	if rows[0].WorkspaceID != "WS-1" || rows[0].AgentKind != "codex" || rows[0].Status != "running" {
		t.Fatalf("unexpected record: %+v", rows[0])
	}
	if rows[0].StartedAt <= 0 || rows[0].LastHeartbeatAt <= 0 {
		t.Fatalf("timestamps should be positive: %+v", rows[0])
	}
}

func TestCLI_AgentRun_UpsertsByWorkspaceID(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	stateDir := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	content := `[
  {"workspace_id":"WS-1","agent_kind":"legacy","started_at":10,"last_heartbeat_at":10,"status":"failed","log_path":"logs/legacy.log"},
  {"workspace_id":"WS-2","agent_kind":"copilot","started_at":20,"last_heartbeat_at":21,"status":"running","log_path":"logs/ws2.log"}
]`
	if err := os.WriteFile(filepath.Join(stateDir, agentActivitiesFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write activity file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "run", "--workspace", "WS-1", "--kind", "codex"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	rows, loadErr := loadAgentActivities(root)
	if loadErr != nil {
		t.Fatalf("load activity file: %v", loadErr)
	}
	if len(rows) != 2 {
		t.Fatalf("record count=%d, want=2", len(rows))
	}
	var ws1 agentActivityRecord
	for _, r := range rows {
		if r.WorkspaceID == "WS-1" {
			ws1 = r
		}
	}
	if ws1.AgentKind != "codex" || ws1.Status != "running" {
		t.Fatalf("WS-1 should be replaced to running codex, got %+v", ws1)
	}
}

func TestCLI_AgentStop_RequiresWorkspace(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "stop"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--workspace is required") {
		t.Fatalf("stderr should include missing workspace error: %q", err.String())
	}
}

func TestCLI_AgentStop_FailsWhenWorkspaceRecordMissing(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "stop", "--workspace", "WS-1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "agent activity not found for workspace: WS-1") {
		t.Fatalf("stderr should include not found error: %q", err.String())
	}
}

func TestCLI_AgentStop_FailsWhenNotRunning(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	stateDir := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	content := `[
  {"workspace_id":"WS-1","agent_kind":"codex","started_at":10,"last_heartbeat_at":11,"status":"failed","log_path":"logs/ws1.log"}
]`
	if err := os.WriteFile(filepath.Join(stateDir, agentActivitiesFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write activity file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "stop", "--workspace", "WS-1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "agent activity is not running") {
		t.Fatalf("stderr should include not running error: %q", err.String())
	}
}

func TestCLI_AgentStop_UpdatesStatusAndHeartbeat(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	stateDir := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	content := `[
  {"workspace_id":"WS-1","agent_kind":"codex","started_at":100,"last_heartbeat_at":100,"status":"running","log_path":"logs/ws1.log"}
]`
	if err := os.WriteFile(filepath.Join(stateDir, agentActivitiesFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write activity file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "stop", "--workspace", "WS-1", "--status", "succeeded"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "agent stopped: workspace=WS-1 status=succeeded") {
		t.Fatalf("stdout missing stop message: %q", out.String())
	}

	rows, loadErr := loadAgentActivities(root)
	if loadErr != nil {
		t.Fatalf("load activities: %v", loadErr)
	}
	if len(rows) != 1 {
		t.Fatalf("record count=%d, want=1", len(rows))
	}
	if rows[0].Status != "succeeded" {
		t.Fatalf("status=%q, want=succeeded", rows[0].Status)
	}
	if rows[0].LastHeartbeatAt <= 100 {
		t.Fatalf("last_heartbeat_at should increase, got=%d", rows[0].LastHeartbeatAt)
	}
}

func TestCLI_AgentLogs_RequiresWorkspace(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "logs"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--workspace is required") {
		t.Fatalf("stderr should include missing workspace error: %q", err.String())
	}
}

func TestCLI_AgentLogs_FailsWhenWorkspaceRecordMissing(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "logs", "--workspace", "WS-1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "agent activity not found for workspace: WS-1") {
		t.Fatalf("stderr should include not found error: %q", err.String())
	}
}

func TestCLI_AgentLogs_FailsWhenLogPathEmpty(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	stateDir := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	content := `[
  {"workspace_id":"WS-1","agent_kind":"codex","started_at":100,"last_heartbeat_at":100,"status":"running","log_path":""}
]`
	if err := os.WriteFile(filepath.Join(stateDir, agentActivitiesFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write activity file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "logs", "--workspace", "WS-1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "log_path is empty for workspace: WS-1") {
		t.Fatalf("stderr should include empty log path error: %q", err.String())
	}
}

func TestCLI_AgentLogs_ShowsTailLines(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	stateDir := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	logRelPath := filepath.Join("logs", "ws1.log")
	logAbsPath := filepath.Join(root, logRelPath)
	if err := os.MkdirAll(filepath.Dir(logAbsPath), 0o755); err != nil {
		t.Fatalf("mkdir logs dir: %v", err)
	}
	logContent := "line1\nline2\nline3\nline4\n"
	if err := os.WriteFile(logAbsPath, []byte(logContent), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	content := `[
  {"workspace_id":"WS-1","agent_kind":"codex","started_at":100,"last_heartbeat_at":100,"status":"running","log_path":"` + filepath.ToSlash(logRelPath) + `"}
]`
	if err := os.WriteFile(filepath.Join(stateDir, agentActivitiesFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write activity file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"agent", "logs", "--workspace", "WS-1", "--tail", "2"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	if strings.Contains(got, "line1") || strings.Contains(got, "line2") {
		t.Fatalf("unexpected old lines in output: %q", got)
	}
	if !strings.Contains(got, "line3\nline4\n") {
		t.Fatalf("tail output mismatch: %q", got)
	}
}
