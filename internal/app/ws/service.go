package ws

import (
	"context"
	"errors"
)

var ErrWorkspaceNotSelected = errors.New("workspace not selected")
var ErrActionNotSelected = errors.New("action not selected")

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
	if !req.ForceSelect && s.resolver != nil {
		found, ok, err := s.resolver.ResolveFromPath(ctx, req.CurrentPath)
		if err != nil {
			return LauncherResult{}, err
		}
		if ok {
			ref = found
			fromContext = true
		}
	}

	if !fromContext {
		id, err := s.selector.SelectWorkspace(ctx, scope, "select", true)
		if err != nil {
			return LauncherResult{}, err
		}
		if id == "" {
			return LauncherResult{}, ErrWorkspaceNotSelected
		}
		ref = WorkspaceRef{ID: id, Status: scope}
	}

	action, err := s.selector.SelectAction(ctx, ref, fromContext)
	if err != nil {
		return LauncherResult{}, err
	}
	if action == "" {
		return LauncherResult{}, ErrActionNotSelected
	}

	return LauncherResult{
		Workspace: ref,
		Action:    action,
	}, nil
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
