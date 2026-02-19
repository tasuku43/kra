package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCLI_AgentRun_BrokerActivityStateTransitionsToIdle(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	t.Setenv("KRA_AGENT_RUN_DRY_RUN", "")
	t.Setenv(agentBrokerEmbeddedEnvKey, "1")
	workspaceDir := filepath.Join(root, "workspaces", "WS-1")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	agentStubPath := filepath.Join(workspaceDir, "agent-activity.sh")
	agentStub := "#!/bin/sh\necho \"boot\"\nwhile true; do sleep 1; done\n"
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
		t.Fatalf("session should transition to running by output activity: session=%s", sessionID)
	}
	if _, ok := waitForSessionRuntimeState(root, sessionID, "idle", 5*time.Second); !ok {
		t.Fatalf("session should transition to idle by output silence: session=%s", sessionID)
	}

	var stopOut bytes.Buffer
	var stopErr bytes.Buffer
	stopCLI := New(&stopOut, &stopErr)
	stopCode := stopCLI.Run([]string{"agent", "stop", "--session", sessionID})
	if stopCode != exitOK {
		t.Fatalf("stop exit code=%d, want=%d (stderr=%q)", stopCode, exitOK, stopErr.String())
	}
	if _, ok := waitForSessionRuntimeState(root, sessionID, "exited", 5*time.Second); !ok {
		t.Fatalf("session should transition to exited after stop: session=%s", sessionID)
	}
}
