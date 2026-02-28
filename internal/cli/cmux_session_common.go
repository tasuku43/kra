package cli

import (
	"context"

	appcmuxsession "github.com/tasuku43/kra/internal/app/cmuxsession"
	sessionstore "github.com/tasuku43/kra/internal/cmuxsession"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type cmuxSessionClient interface {
	Identify(ctx context.Context, workspace string, surface string) (map[string]any, error)
	SelectWorkspace(ctx context.Context, workspace string) error
	ListPanes(ctx context.Context, workspace string) ([]cmuxctl.Pane, error)
	ListPaneSurfaces(ctx context.Context, workspace string, pane string) ([]cmuxctl.Surface, error)
	ReadScreen(ctx context.Context, workspace string, surface string, lines int, scrollback bool) (string, error)
	BrowserStateSave(ctx context.Context, workspace string, surface string, path string) error
	BrowserStateLoad(ctx context.Context, workspace string, surface string, path string) error
	FocusPane(ctx context.Context, pane string, workspace string) error
}

var newCMUXSessionClient = func() cmuxSessionClient { return cmuxctl.NewClient() }
var newCMUXSessionStore = func(root string) appcmuxsession.SessionStore { return sessionstore.NewStore(root) }

type cmuxSessionClientAdapter struct {
	inner cmuxSessionClient
}

func (a cmuxSessionClientAdapter) Identify(ctx context.Context, workspace string, surface string) (map[string]any, error) {
	return a.inner.Identify(ctx, workspace, surface)
}

func (a cmuxSessionClientAdapter) SelectWorkspace(ctx context.Context, workspace string) error {
	return a.inner.SelectWorkspace(ctx, workspace)
}

func (a cmuxSessionClientAdapter) ListPanes(ctx context.Context, workspace string) ([]cmuxctl.Pane, error) {
	return a.inner.ListPanes(ctx, workspace)
}

func (a cmuxSessionClientAdapter) ListPaneSurfaces(ctx context.Context, workspace string, pane string) ([]cmuxctl.Surface, error) {
	return a.inner.ListPaneSurfaces(ctx, workspace, pane)
}

func (a cmuxSessionClientAdapter) ReadScreen(ctx context.Context, workspace string, surface string, lines int, scrollback bool) (string, error) {
	return a.inner.ReadScreen(ctx, workspace, surface, lines, scrollback)
}

func (a cmuxSessionClientAdapter) BrowserStateSave(ctx context.Context, workspace string, surface string, path string) error {
	return a.inner.BrowserStateSave(ctx, workspace, surface, path)
}

func (a cmuxSessionClientAdapter) BrowserStateLoad(ctx context.Context, workspace string, surface string, path string) error {
	return a.inner.BrowserStateLoad(ctx, workspace, surface, path)
}

func (a cmuxSessionClientAdapter) FocusPane(ctx context.Context, pane string, workspace string) error {
	return a.inner.FocusPane(ctx, pane, workspace)
}
