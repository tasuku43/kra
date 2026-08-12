// Package cmuxtodo reconciles KRA task documents with cmux native Todo lists.
//
// KRA owns task identity and text. cmux owns only the interactive checklist
// state. Reconciliation is three-way, using cmuxmap.File.Todos as the base.
package cmuxtodo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
)

const (
	// RootWorkspaceID is the logical mapping key used by root task sync.
	RootWorkspaceID = "KRA_ROOT"
	MaxTodoItems    = 50
	MaxTodoText     = 500
)

type TaskStatus string

const (
	TaskTodo    TaskStatus = "todo"
	TaskDoing   TaskStatus = "doing"
	TaskBlocked TaskStatus = "blocked"
	TaskDone    TaskStatus = "done"
)

func (s TaskStatus) Valid() bool {
	switch s {
	case TaskTodo, TaskDoing, TaskBlocked, TaskDone:
		return true
	default:
		return false
	}
}

type TodoState string

const (
	TodoPending    TodoState = "pending"
	TodoInProgress TodoState = "in-progress"
	TodoCompleted  TodoState = "completed"
)

func (s TodoState) Valid() bool {
	switch s {
	case TodoPending, TodoInProgress, TodoCompleted:
		return true
	default:
		return false
	}
}

type Preference string

const (
	PreferNone Preference = ""
	PreferKRA  Preference = "kra"
	PreferCMUX Preference = "cmux"
)

func (p Preference) Valid() bool {
	switch p {
	case PreferNone, PreferKRA, PreferCMUX:
		return true
	default:
		return false
	}
}

type TargetKind string

const (
	TargetWorkspace TargetKind = "workspace"
	TargetRoot      TargetKind = "root"
)

func (k TargetKind) Valid() bool {
	switch k {
	case TargetWorkspace, TargetRoot:
		return true
	default:
		return false
	}
}

// Task contains only the fields needed by native Todo projection. Position in
// the slice is the KRA declaration order.
type Task struct {
	ID     string
	Title  string
	Status TaskStatus
}

// TaskSnapshot has an opaque revision used as the compare-and-swap token for
// ApplyStatuses. The document adapter must change Revision whenever the
// persisted task document changes.
type TaskSnapshot struct {
	Tasks    []Task
	Revision string
}

type StatusChange struct {
	TaskID string
	From   TaskStatus
	To     TaskStatus
}

// TaskDocument applies all status changes atomically. ApplyStatuses must fail
// without writing if expectedRevision is stale or if any From value no longer
// matches. The same method is used with inverse changes to roll back a pull.
// It intentionally bypasses interactive workflow transition restrictions:
// reconciliation may need to map completed back to in-progress or pending.
type TaskDocument interface {
	List(ctx context.Context, root string, workspaceID string, scope string) (TaskSnapshot, error)
	ApplyStatuses(ctx context.Context, root string, workspaceID string, scope string, expectedRevision string, changes []StatusChange) (TaskSnapshot, error)
}

type TodoItem struct {
	ID     string
	Text   string
	State  TodoState
	Origin string
}

type TodoList struct {
	WorkspaceID string
	Items       []TodoItem
}

type TodoInput struct {
	Text   string
	State  TodoState
	Origin string
}

// Client is deliberately defined in the application package. The cmuxctl
// adapter converts its transport types at the infrastructure boundary.
type Client interface {
	ListTodos(ctx context.Context, workspaceID string) (TodoList, error)
	AddTodo(ctx context.Context, workspaceID string, input TodoInput) (TodoItem, error)
	SetTodoState(ctx context.Context, workspaceID string, itemID string, state TodoState) (TodoItem, error)
	EditTodo(ctx context.Context, workspaceID string, itemID string, text string) (TodoItem, error)
}

// CapabilityChecker is optional so application tests and alternate adapters
// can remain transport-agnostic. Production adapters use capabilities plus a
// read-only list probe because cmux may under-advertise Todo methods.
type CapabilityChecker interface {
	CheckCapabilities(ctx context.Context, workspaceID string) error
}

type MappingStore interface {
	AcquireLock(ctx context.Context) (func(), error)
	Load() (cmuxmap.File, error)
	Save(cmuxmap.File) error
}

type NewStoreFunc func(root string) MappingStore
type NewClientFunc func() Client

type Service struct {
	NewStore  NewStoreFunc
	NewClient NewClientFunc
	Documents TaskDocument
	Now       func() time.Time
}

func NewService(newStore NewStoreFunc, newClient NewClientFunc, documents TaskDocument) *Service {
	return &Service{
		NewStore:  newStore,
		NewClient: newClient,
		Documents: documents,
	}
}

type Request struct {
	Root        string
	WorkspaceID string
	Scope       string
	Target      TargetKind
	Prefer      Preference
	DryRun      bool
}

func (r Request) normalized() (Request, error) {
	r.Root = strings.TrimSpace(r.Root)
	r.WorkspaceID = strings.TrimSpace(r.WorkspaceID)
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Target == "" {
		r.Target = TargetWorkspace
	}
	if r.Target == TargetRoot && r.WorkspaceID == "" {
		r.WorkspaceID = RootWorkspaceID
	}
	if r.Target == TargetRoot && r.WorkspaceID != RootWorkspaceID {
		return Request{}, fmt.Errorf("%w: root target workspace id must be %q", ErrInvalidRequest, RootWorkspaceID)
	}
	if r.Target == TargetRoot && r.Scope == "" {
		r.Scope = "root"
	}
	if r.Target == TargetWorkspace && r.Scope == "" {
		r.Scope = "active"
	}
	if r.Root == "" {
		return Request{}, fmt.Errorf("%w: KRA root is required", ErrInvalidRequest)
	}
	if r.WorkspaceID == "" {
		return Request{}, fmt.Errorf("%w: workspace id is required", ErrInvalidRequest)
	}
	if !r.Target.Valid() {
		return Request{}, fmt.Errorf("%w: invalid target %q", ErrInvalidRequest, r.Target)
	}
	if !r.Prefer.Valid() {
		return Request{}, fmt.Errorf("%w: invalid preference %q", ErrInvalidRequest, r.Prefer)
	}
	return r, nil
}

type ConflictKind string

const (
	ConflictState          ConflictKind = "state"
	ConflictText           ConflictKind = "text"
	ConflictRemoteDeleted  ConflictKind = "remote_deleted"
	ConflictLocalDeleted   ConflictKind = "local_deleted"
	ConflictCapacity       ConflictKind = "capacity"
	ConflictConcurrentTodo ConflictKind = "concurrent_todo_change"
)

type Conflict struct {
	TaskID string
	Kind   ConflictKind
	Detail string
}

type OperationKind string

const (
	OperationAdd        OperationKind = "add"
	OperationEdit       OperationKind = "edit"
	OperationSetState   OperationKind = "set_state"
	OperationPullStatus OperationKind = "pull_status"
)

type Operation struct {
	Kind       OperationKind
	TaskID     string
	CMUXItemID string
	From       string
	To         string
}

type Result struct {
	WorkspaceID     string
	CMUXWorkspaceID string
	DryRun          bool
	Changed         bool
	ForeignCount    int
	ManagedCount    int
	Truncated       []string
	Conflicts       []Conflict
	Operations      []Operation
}

var (
	ErrInvalidRequest  = errors.New("invalid cmux Todo request")
	ErrInvalidDocument = errors.New("invalid KRA task document")
	ErrInvalidBase     = errors.New("invalid cmux Todo base")
	ErrNotMapped       = errors.New("KRA workspace is not mapped to cmux")
	ErrConflict        = errors.New("cmux Todo reconciliation conflict")
	ErrCapacity        = errors.New("cmux Todo capacity exceeded")
	ErrConcurrentTodo  = errors.New("cmux Todo changed during reconciliation")
	ErrRecovery        = errors.New("cmux Todo reconciliation recovery failed")
)

// RecoveryError reports a primary failure plus any bounded compensation that
// could not be completed safely. Callers should surface Repair verbatim.
type RecoveryError struct {
	Cause  error
	Repair []error
}

func (e *RecoveryError) Error() string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, len(e.Repair))
	for _, err := range e.Repair {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	if len(parts) == 0 {
		return e.Cause.Error()
	}
	return fmt.Sprintf("%v: %v: manual repair required: %s", ErrRecovery, e.Cause, strings.Join(parts, "; "))
}

func (e *RecoveryError) Unwrap() []error {
	if e == nil {
		return nil
	}
	if e.Cause == nil {
		return []error{ErrRecovery}
	}
	return []error{ErrRecovery, e.Cause}
}
