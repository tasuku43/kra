package cli

import (
	"github.com/tasuku43/kra/internal/app/cmuxtodo"
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/appports"
)

var newCMUXTodoReconciler = func() cmuxTodoReconciler {
	return cmuxtodo.NewService(
		func(root string) cmuxtodo.MappingStore { return cmuxmap.NewStore(root) },
		func() cmuxtodo.Client { return appports.NewCMUXTodoClient() },
		appports.NewCMUXTodoTaskDocument(),
	)
}

func newWorkspaceTaskService() *wstask.Service {
	return wstask.NewService(appports.NewWSTaskPort())
}

func newRootTaskService() *wstask.Service {
	return wstask.NewService(appports.NewRootTaskPort())
}
