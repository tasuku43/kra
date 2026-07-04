package cli

import (
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

var newCMUXTaskSyncClient = func() appports.CMUXTaskSyncClient { return cmuxctl.NewClient() }
var newCMUXTaskMapStore = func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) }

func newWorkspaceTaskService() *wstask.Service {
	return wstask.NewService(
		appports.NewWSTaskPort(),
		appports.NewWSTaskSyncPort(newCMUXTaskSyncClient, newCMUXTaskMapStore),
	)
}

func newRootTaskService() *wstask.Service {
	return wstask.NewService(appports.NewRootTaskPort())
}
