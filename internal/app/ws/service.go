package ws

import (
	"context"
	"errors"
	"fmt"
)

var ErrWorkspaceNotSelected = errors.New("workspace not selected")
var ErrWorkspaceNotFound = errors.New("workspace not found")
var ErrActionNotSelected = errors.New("action not selected")
var ErrActionNotAllowed = errors.New("action not allowed for workspace scope")

type Service struct {
	selector SelectorPort
	resolver WorkspaceResolverPort
}

func NewService(selector SelectorPort, resolver WorkspaceResolverPort) *Service {
	return &Service{selector: selector, resolver: resolver}
}

func (s *Service) Run(ctx context.Context, req LauncherRequest) (LauncherResult, error) {
	scope := req.Scope
	if scope == "" {
		scope = ScopeActive
	}

	ref := WorkspaceRef{}
	fromContext := false
	if req.WorkspaceID != "" && s.resolver != nil {
		found, ok, err := s.resolver.ResolveByID(ctx, req.WorkspaceID)
		if err != nil {
			return LauncherResult{}, err
		}
		if !ok {
			return LauncherResult{}, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, req.WorkspaceID)
		}
		ref = found
	} else if !req.ForceSelect && s.resolver != nil {
		found, ok, err := s.resolver.ResolveFromPath(ctx, req.CurrentPath)
		if err != nil {
			return LauncherResult{}, err
		}
		if ok {
			ref = found
			fromContext = true
		}
	}

	if req.WorkspaceID == "" && !fromContext {
		if !req.ForceSelect {
			return LauncherResult{}, ErrWorkspaceNotSelected
		}
		id, err := s.selector.SelectWorkspace(ctx, scope, "select", true)
		if err != nil {
			return LauncherResult{}, err
		}
		if id == "" {
			return LauncherResult{}, ErrWorkspaceNotSelected
		}
		ref = WorkspaceRef{ID: id, Status: scope}
	}

	action := req.FixedAction
	if action == "" {
		var err error
		action, err = s.selector.SelectAction(ctx, ref, fromContext)
		if err != nil {
			return LauncherResult{}, err
		}
		if action == "" {
			return LauncherResult{}, ErrActionNotSelected
		}
	} else if !isActionAllowedForScope(action, ref.Status) {
		return LauncherResult{}, ErrActionNotAllowed
	}

	return LauncherResult{
		Workspace: ref,
		Action:    action,
	}, nil
}

func isActionAllowedForScope(action Action, scope Scope) bool {
	switch scope {
	case ScopeActive:
		return action == ActionGo || action == ActionAddRepo || action == ActionClose
	case ScopeArchived:
		return action == ActionReopen || action == ActionPurge
	default:
		return false
	}
}

func (s *Service) RunSelect(ctx context.Context, req SelectRequest) (SelectResult, error) {
	scope := req.Scope
	if scope == "" {
		scope = ScopeActive
	}
	action := req.Action
	if action == "" {
		action = "select"
	}
	id, err := s.selector.SelectWorkspace(ctx, scope, action, true)
	if err != nil {
		return SelectResult{}, err
	}
	if id == "" {
		return SelectResult{}, ErrWorkspaceNotSelected
	}
	return SelectResult{WorkspaceID: id}, nil
}
