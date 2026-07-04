package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_TaskList_JSON_EmptyWhenRootWorkspaceMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"task", "list", "--format", "json"})
	if code != exitOK {
		t.Fatalf("task list exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "task.list" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["task_state"]; got != "empty" {
		t.Fatalf("task_state = %v, want %q", got, "empty")
	}
	counts, ok := resp.Result["counts"].(map[string]any)
	if !ok {
		t.Fatalf("result.counts missing: %+v", resp.Result)
	}
	if got := counts["total"]; got != float64(0) {
		t.Fatalf("counts.total = %v, want 0", got)
	}
}

func TestCLI_TaskAdd_JSON_CreatesRootWorkspaceDocument(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"task", "add", "--title", "Review release notes", "--description", "Before shipping", "--format", "json"})
	if code != exitOK {
		t.Fatalf("task add exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "task.add" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	content, readErr := os.ReadFile(filepath.Join(env.Root, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read root workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("## Tasks\n\n### TASK-001 Review release notes\nstatus: todo\n\nBefore shipping\n")) {
		t.Fatalf("root workspace.md missing canonical task block: %q", string(content))
	}
}

func TestCLI_TaskStatus_JSON_UpdatesRootWorkspaceDocument(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	if err := os.WriteFile(filepath.Join(env.Root, workspaceDocumentFilename), []byte("## Tasks\n\n### TASK-001 Daily review\nstatus: todo\n"), 0o644); err != nil {
		t.Fatalf("write root workspace.md: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"task", "status", "TASK-001", "done", "--format", "json"})
	if code != exitOK {
		t.Fatalf("task status exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "task.status" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	task, ok := resp.Result["task"].(map[string]any)
	if !ok {
		t.Fatalf("result.task missing: %+v", resp.Result)
	}
	if got := task["status"]; got != "done" {
		t.Fatalf("task.status = %v, want %q", got, "done")
	}

	content, readErr := os.ReadFile(filepath.Join(env.Root, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read root workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: done")) {
		t.Fatalf("root workspace.md should be updated to done: %q", string(content))
	}
}

func TestCLI_TaskStatus_NoArgsOpensRootStatusTUI(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"task", "status"})
	if code != exitError {
		t.Fatalf("task status exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "interactive task TUI requires a TTY") {
		t.Fatalf("stderr should show TUI routing error: %q", err.String())
	}
}

func TestCLI_TaskList_JSON_InvalidRootContractFailsClosed(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	if err := os.WriteFile(filepath.Join(env.Root, workspaceDocumentFilename), []byte("## Tasks\n\n### TASK-001 First\nstatus: todo\n\n### TASK-001 Second\nstatus: doing\n"), 0o644); err != nil {
		t.Fatalf("write root workspace.md: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"task", "list", "--format", "json"})
	if code != exitError {
		t.Fatalf("task list exit code = %d, want %d", code, exitError)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestRootTaskTUI_RendersLikeWorkspaceStatus(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	if err := os.WriteFile(filepath.Join(env.Root, workspaceDocumentFilename), []byte("## Tasks\n\n### TASK-001 Daily review\nstatus: doing\n\n### TASK-002 Pay invoice\nstatus: todo\n"), 0o644); err != nil {
		t.Fatalf("write root workspace.md: %v", err)
	}

	m := newRootTaskTUIModel(env.Root, wsTaskTUIOptions{}, false)
	got := m.View()
	for _, want := range []string{
		"TASKS  root",
		"Current Task",
		"Daily review",
		"Next Task",
		"Pay invoice",
		"Progress  0/2",
		"source: " + filepath.Join(env.Root, workspaceDocumentFilename),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("view missing %q: %q", want, got)
		}
	}
}

func TestRootTaskTUI_ToggleRowUpdatesRootWorkspaceDocument(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	if err := os.WriteFile(filepath.Join(env.Root, workspaceDocumentFilename), []byte("## Tasks\n\n### TASK-001 First\nstatus: todo\n"), 0o644); err != nil {
		t.Fatalf("write root workspace.md: %v", err)
	}

	m := newRootTaskTUIModel(env.Root, wsTaskTUIOptions{}, false)
	editing, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = editing.(wsTaskTUIModel)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok := updated.(wsTaskTUIModel)
	if !ok {
		t.Fatalf("updated model = %T, want wsTaskTUIModel", updated)
	}
	if next.rows[0].Item.Status != wstask.StatusDone {
		t.Fatalf("task status = %s, want done", next.rows[0].Item.Status)
	}
	content, readErr := os.ReadFile(filepath.Join(env.Root, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read root workspace.md: %v", readErr)
	}
	if !bytes.Contains(content, []byte("status: done")) {
		t.Fatalf("root workspace.md should be updated to done: %q", string(content))
	}
}
