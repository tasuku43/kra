package cli

import (
	"context"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type cmuxRuntimeClient interface {
	ListWorkspaces(ctx context.Context) ([]cmuxctl.Workspace, error)
}

var newCMUXRuntimeClient = func() cmuxRuntimeClient { return cmuxctl.NewClient() }
