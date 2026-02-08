package ws

import "context"

// Scope represents workspace listing scope.
type Scope string

const (
	ScopeActive   Scope = "active"
	ScopeArchived Scope = "archived"
)

// Action represents launcher-selected workspace operation.
type Action string

const (
	ActionGo      Action = "go"
	ActionAddRepo Action = "add-repo"
	ActionClose   Action = "close"
	ActionReopen  Action = "reopen"
	ActionPurge   Action = "purge"
)

// WorkspaceRef identifies a workspace in app-layer contracts.
type WorkspaceRef struct {
	ID     string
	Status Scope
}

// LauncherRequest defines the human launcher use case input.
type LauncherRequest struct {
	ForceSelect bool
	Scope       Scope
	CurrentPath string
}

// LauncherResult returns selected operation and resolved workspace target.
type LauncherResult struct {
	Workspace WorkspaceRef
	Action    Action
}

// SelectRequest defines shared --select operation contract.
type SelectRequest struct {
	Scope Scope
}

// SelectResult returns selected workspace id from selector flow.
type SelectResult struct {
	WorkspaceID string
}

// SelectorPort abstracts interactive selection UI.
type SelectorPort interface {
	SelectWorkspace(ctx context.Context, scope Scope, action string, single bool) (string, error)
	SelectAction(ctx context.Context, workspace WorkspaceRef, fromContext bool) (Action, error)
}

// WorkspaceResolverPort abstracts cwd-to-workspace resolution.
type WorkspaceResolverPort interface {
	ResolveFromPath(ctx context.Context, path string) (WorkspaceRef, bool, error)
}

// LauncherUseCase is the app-layer orchestration contract for `gionx ws`.
type LauncherUseCase interface {
	Run(ctx context.Context, req LauncherRequest) (LauncherResult, error)
}

// SelectUseCase is the app-layer contract for operation-level --select flow.
type SelectUseCase interface {
	Run(ctx context.Context, req SelectRequest) (SelectResult, error)
}
