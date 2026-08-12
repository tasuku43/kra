package appports

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/kra/internal/app/cmuxtodo"
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

// CMUXTodoTaskDocument adapts the existing root/workspace task services to the
// batch CAS contract required by native cmux Todo reconciliation.
type CMUXTodoTaskDocument struct {
	workspace *wstask.Service
	root      *wstask.Service
}

func NewCMUXTodoTaskDocument() *CMUXTodoTaskDocument {
	return &CMUXTodoTaskDocument{
		workspace: wstask.NewService(NewWSTaskPort()),
		root:      wstask.NewService(NewRootTaskPort()),
	}
}

func (p *CMUXTodoTaskDocument) List(
	ctx context.Context,
	root string,
	workspaceID string,
	scope string,
) (cmuxtodo.TaskSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return cmuxtodo.TaskSnapshot{}, err
	}
	snapshot, err := p.service(workspaceID, scope).Snapshot(root, workspaceID, scope)
	if err != nil {
		return cmuxtodo.TaskSnapshot{}, err
	}
	return taskSnapshotToCMUXTodo(snapshot), nil
}

func (p *CMUXTodoTaskDocument) ApplyStatuses(
	ctx context.Context,
	root string,
	workspaceID string,
	scope string,
	expectedRevision string,
	changes []cmuxtodo.StatusChange,
) (cmuxtodo.TaskSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return cmuxtodo.TaskSnapshot{}, err
	}
	batch := make([]wstask.BatchStatusChange, len(changes))
	for i, change := range changes {
		batch[i] = wstask.BatchStatusChange{
			TaskID: strings.TrimSpace(change.TaskID),
			From:   wstask.Status(change.From),
			To:     wstask.Status(change.To),
		}
	}
	snapshot, err := p.service(workspaceID, scope).ApplyStatuses(
		root,
		workspaceID,
		scope,
		expectedRevision,
		batch,
	)
	if err != nil {
		return cmuxtodo.TaskSnapshot{}, err
	}
	return taskSnapshotToCMUXTodo(snapshot), nil
}

func (p *CMUXTodoTaskDocument) service(workspaceID string, scope string) *wstask.Service {
	if strings.TrimSpace(workspaceID) == cmuxtodo.RootWorkspaceID || strings.TrimSpace(scope) == "root" {
		return p.root
	}
	return p.workspace
}

func taskSnapshotToCMUXTodo(snapshot wstask.TaskSnapshot) cmuxtodo.TaskSnapshot {
	tasks := make([]cmuxtodo.Task, len(snapshot.Items))
	for i, item := range snapshot.Items {
		tasks[i] = cmuxtodo.Task{
			ID:     item.ID,
			Title:  item.Title,
			Status: cmuxtodo.TaskStatus(item.Status),
		}
	}
	return cmuxtodo.TaskSnapshot{Tasks: tasks, Revision: snapshot.Revision}
}

// CMUXTodoClient keeps transport JSON and command spelling outside the
// application reconciliation package.
type CMUXTodoClient struct {
	inner *cmuxctl.Client
}

func NewCMUXTodoClient() *CMUXTodoClient {
	return &CMUXTodoClient{inner: cmuxctl.NewClient()}
}

func NewCMUXTodoClientWithClient(inner *cmuxctl.Client) *CMUXTodoClient {
	return &CMUXTodoClient{inner: inner}
}

// CheckCapabilities performs a read-only capability check. cmux v0.64.x can
// omit native Todo methods from capabilities even when the dispatcher accepts
// them, so ListTodos is the authoritative fallback probe.
func (c *CMUXTodoClient) CheckCapabilities(ctx context.Context, workspaceID string) error {
	inner, err := c.client()
	if err != nil {
		return err
	}
	caps, err := inner.Capabilities(ctx)
	if err != nil {
		return fmt.Errorf("read cmux Todo capabilities: %w", err)
	}
	if caps.Supports("workspace.todo.list") {
		return nil
	}
	list, err := inner.ListTodos(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("cmux native Todo is unavailable (workspace.todo.list probe failed): %w", err)
	}
	requested := strings.TrimSpace(workspaceID)
	got := strings.TrimSpace(list.WorkspaceID)
	if got == "" || got != requested {
		return fmt.Errorf(
			"cmux native Todo is unavailable (workspace.todo.list probe returned workspace %q, expected %q)",
			got,
			requested,
		)
	}
	return nil
}

func (c *CMUXTodoClient) ListTodos(ctx context.Context, workspaceID string) (cmuxtodo.TodoList, error) {
	inner, err := c.client()
	if err != nil {
		return cmuxtodo.TodoList{}, err
	}
	list, err := inner.ListTodos(ctx, workspaceID)
	if err != nil {
		return cmuxtodo.TodoList{}, err
	}
	items := make([]cmuxtodo.TodoItem, len(list.Items))
	for i, item := range list.Items {
		items[i] = todoItemToApp(item)
	}
	return cmuxtodo.TodoList{WorkspaceID: list.WorkspaceID, Items: items}, nil
}

func (c *CMUXTodoClient) AddTodo(
	ctx context.Context,
	workspaceID string,
	input cmuxtodo.TodoInput,
) (cmuxtodo.TodoItem, error) {
	inner, err := c.client()
	if err != nil {
		return cmuxtodo.TodoItem{}, err
	}
	item, err := inner.AddTodo(ctx, workspaceID, cmuxctl.TodoItemInput{
		Text:   input.Text,
		State:  string(input.State),
		Origin: input.Origin,
	})
	if err != nil {
		return cmuxtodo.TodoItem{}, err
	}
	return todoItemToApp(item), nil
}

func (c *CMUXTodoClient) SetTodoState(
	ctx context.Context,
	workspaceID string,
	itemID string,
	state cmuxtodo.TodoState,
) (cmuxtodo.TodoItem, error) {
	inner, err := c.client()
	if err != nil {
		return cmuxtodo.TodoItem{}, err
	}
	item, err := inner.SetTodoState(ctx, workspaceID, itemID, string(state))
	if err != nil {
		return cmuxtodo.TodoItem{}, err
	}
	return todoItemToApp(item), nil
}

func (c *CMUXTodoClient) EditTodo(
	ctx context.Context,
	workspaceID string,
	itemID string,
	text string,
) (cmuxtodo.TodoItem, error) {
	inner, err := c.client()
	if err != nil {
		return cmuxtodo.TodoItem{}, err
	}
	item, err := inner.EditTodo(ctx, workspaceID, itemID, text)
	if err != nil {
		return cmuxtodo.TodoItem{}, err
	}
	return todoItemToApp(item), nil
}

func (c *CMUXTodoClient) client() (*cmuxctl.Client, error) {
	if c == nil || c.inner == nil {
		return nil, fmt.Errorf("cmux Todo client is not initialized")
	}
	return c.inner, nil
}

func todoItemToApp(item cmuxctl.TodoItem) cmuxtodo.TodoItem {
	return cmuxtodo.TodoItem{
		ID:     item.ID,
		Text:   item.Text,
		State:  cmuxtodo.TodoState(item.State),
		Origin: item.Origin,
	}
}
