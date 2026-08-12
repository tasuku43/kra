package appports

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/kra/internal/app/cmuxtodo"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type cmuxTodoTestRunner struct {
	outputs [][]byte
	calls   [][]string
}

func (r *cmuxTodoTestRunner) Run(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{}, args...))
	if len(r.outputs) == 0 {
		return nil, nil, errors.New("unexpected cmux call")
	}
	out := r.outputs[0]
	r.outputs = r.outputs[1:]
	return out, nil, nil
}

func TestCMUXTodoTaskDocumentRoutesRootAndWorkspaceAndAppliesBatch(t *testing.T) {
	root := t.TempDir()
	rootPath := filepath.Join(root, "workspace.md")
	workspaceDir := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootPath, []byte("## Tasks\n\n### TASK-001 Root task\nstatus: todo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(workspaceDir, "workspace.yaml")
	if err := os.WriteFile(workspacePath, []byte("schema_version: 1\ntasks:\n  - id: TASK-002\n    title: Workspace task\n    status: doing\n    depends_on: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	port := NewCMUXTodoTaskDocument()
	rootSnapshot, err := port.List(context.Background(), root, cmuxtodo.RootWorkspaceID, "root")
	if err != nil {
		t.Fatalf("root List() error = %v", err)
	}
	workspaceSnapshot, err := port.List(context.Background(), root, "WS1", "active")
	if err != nil {
		t.Fatalf("workspace List() error = %v", err)
	}
	if len(rootSnapshot.Tasks) != 1 || rootSnapshot.Tasks[0].ID != "TASK-001" || rootSnapshot.Tasks[0].Status != cmuxtodo.TaskTodo {
		t.Fatalf("root snapshot = %#v", rootSnapshot)
	}
	if len(workspaceSnapshot.Tasks) != 1 || workspaceSnapshot.Tasks[0].ID != "TASK-002" || workspaceSnapshot.Tasks[0].Status != cmuxtodo.TaskDoing {
		t.Fatalf("workspace snapshot = %#v", workspaceSnapshot)
	}

	updated, err := port.ApplyStatuses(
		context.Background(),
		root,
		"WS1",
		"active",
		workspaceSnapshot.Revision,
		[]cmuxtodo.StatusChange{{TaskID: "TASK-002", From: cmuxtodo.TaskDoing, To: cmuxtodo.TaskDone}},
	)
	if err != nil {
		t.Fatalf("ApplyStatuses() error = %v", err)
	}
	if len(updated.Tasks) != 1 || updated.Tasks[0].Status != cmuxtodo.TaskDone || updated.Revision == workspaceSnapshot.Revision {
		t.Fatalf("updated snapshot = %#v", updated)
	}
	rootContent, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(rootContent) != "## Tasks\n\n### TASK-001 Root task\nstatus: todo\n" {
		t.Fatalf("root document was changed: %q", rootContent)
	}
}

func TestCMUXTodoTaskDocumentCanceledContextDoesNotTouchDocument(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewCMUXTodoTaskDocument().List(ctx, root, cmuxtodo.RootWorkspaceID, "root")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v", err)
	}
}

func TestTodoItemToAppPreservesNativeIdentityAndState(t *testing.T) {
	got := todoItemToApp(cmuxctl.TodoItem{
		ID:     "todo-uuid",
		Text:   "TASK-001 Work",
		State:  "in-progress",
		Origin: "agent",
	})
	if got.ID != "todo-uuid" || got.Text != "TASK-001 Work" || got.State != cmuxtodo.TodoInProgress || got.Origin != "agent" {
		t.Fatalf("todoItemToApp() = %#v", got)
	}
}

func TestCMUXTodoClientCapabilityCheckUsesAdvertiseOrReadOnlyProbe(t *testing.T) {
	t.Run("advertised", func(t *testing.T) {
		runner := &cmuxTodoTestRunner{outputs: [][]byte{[]byte(`{"methods":["workspace.todo.list"]}`)}}
		client := NewCMUXTodoClientWithClient(&cmuxctl.Client{Runner: runner})
		if err := client.CheckCapabilities(context.Background(), "ws-id"); err != nil {
			t.Fatalf("CheckCapabilities() error = %v", err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("calls = %#v, want capabilities only", runner.calls)
		}
	})

	t.Run("under advertised method probes list", func(t *testing.T) {
		runner := &cmuxTodoTestRunner{outputs: [][]byte{
			[]byte(`{"methods":[]}`),
			[]byte(`{"workspace_id":"ws-id","items":[],"progress":{"completed":0,"total":0,"first_unchecked_text":null}}`),
		}}
		client := NewCMUXTodoClientWithClient(&cmuxctl.Client{Runner: runner})
		if err := client.CheckCapabilities(context.Background(), "ws-id"); err != nil {
			t.Fatalf("CheckCapabilities() error = %v", err)
		}
		if len(runner.calls) != 2 || len(runner.calls[1]) < 2 || runner.calls[1][0] != "rpc" || runner.calls[1][1] != "workspace.todo.list" {
			t.Fatalf("calls = %#v", runner.calls)
		}
	})

	for _, tt := range []struct {
		name        string
		workspaceID string
	}{
		{name: "probe omits workspace identity", workspaceID: ""},
		{name: "probe returns another workspace", workspaceID: "other-id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := &cmuxTodoTestRunner{outputs: [][]byte{
				[]byte(`{"methods":[]}`),
				[]byte(`{"workspace_id":"` + tt.workspaceID + `","items":[],"progress":{"completed":0,"total":0,"first_unchecked_text":null}}`),
			}}
			client := NewCMUXTodoClientWithClient(&cmuxctl.Client{Runner: runner})
			if err := client.CheckCapabilities(context.Background(), "ws-id"); err == nil {
				t.Fatal("CheckCapabilities() error = nil")
			}
			if len(runner.calls) != 2 {
				t.Fatalf("calls = %#v", runner.calls)
			}
		})
	}
}
