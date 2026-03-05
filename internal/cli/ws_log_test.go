package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	krapaths "github.com/tasuku43/kra/internal/paths"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WS_Log_AppendsToWorkspaceLog_AndMirrorsCMUX(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	createWorkspaceForLogTest(t, "WS1")

	logged := make([]string, 0, 1)
	prevRun := runCMUXLogCommand
	runCMUXLogCommand = func(message string) error {
		logged = append(logged, message)
		return nil
	}
	t.Cleanup(func() { runCMUXLogCommand = prevRun })
	t.Setenv("CMUX_WORKSPACE_ID", "cmux-ws-1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "log", "--id", "WS1", "first message"})
	if code != exitOK {
		t.Fatalf("ws log exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	logPath := filepath.Join(env.Root, "workspaces", "WS1", "log.txt")
	got, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log file: %v", readErr)
	}
	if string(got) != "first message\n" {
		t.Fatalf("log content = %q, want %q", string(got), "first message\n")
	}
	if len(logged) != 1 || logged[0] != "first message" {
		t.Fatalf("cmux log messages = %+v, want [first message]", logged)
	}
}

func TestCLI_WS_Log_ResolvesCurrentWorkspaceByCWD(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	createWorkspaceForLogTest(t, "WS1")

	logged := make([]string, 0, 1)
	prevRun := runCMUXLogCommand
	runCMUXLogCommand = func(message string) error {
		logged = append(logged, message)
		return nil
	}
	t.Cleanup(func() { runCMUXLogCommand = prevRun })
	t.Setenv("CMUX_WORKSPACE_ID", "")

	prevWD, werr := os.Getwd()
	if werr != nil {
		t.Fatalf("getwd: %v", werr)
	}
	wsPath := filepath.Join(env.Root, "workspaces", "WS1")
	if err := os.Chdir(wsPath); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })
	actualWD, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatalf("getwd after chdir: %v", getwdErr)
	}
	if writeErr := krapaths.WriteCurrentContext(filepath.Dir(filepath.Dir(actualWD))); writeErr != nil {
		t.Fatalf("rewrite current context: %v", writeErr)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	c := New(&out, &stderr)
	code := c.Run([]string{"ws", "log", "second message"})
	if code != exitOK {
		t.Fatalf("ws log exit code = %d, want %d (stderr=%q)", code, exitOK, stderr.String())
	}

	logPath := filepath.Join(wsPath, "log.txt")
	got, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log file: %v", readErr)
	}
	if string(got) != "second message\n" {
		t.Fatalf("log content = %q, want %q", string(got), "second message\n")
	}
	if len(logged) != 0 {
		t.Fatalf("cmux log should be skipped when runtime is disabled: %+v", logged)
	}
}

func TestCLI_WS_Log_FailsWhenTargetCannotBeResolved(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	prevWD, werr := os.Getwd()
	if werr != nil {
		t.Fatalf("getwd: %v", werr)
	}
	if err := os.Chdir(env.Root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "log", "message"})
	if code != exitUsage {
		t.Fatalf("ws log exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
	}
	if err.Len() == 0 {
		t.Fatalf("stderr should explain missing target")
	}
}

func createWorkspaceForLogTest(t *testing.T, workspaceID string) {
	t.Helper()
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "create", "--no-prompt", workspaceID})
	if code != exitOK {
		t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
}
