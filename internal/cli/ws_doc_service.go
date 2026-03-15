package cli

import (
	"context"

	appcmux "github.com/tasuku43/kra/internal/app/cmux"
	appwsdoc "github.com/tasuku43/kra/internal/app/wsdoc"
	"github.com/tasuku43/kra/internal/cmuxdocs"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

var newWSDocService = func() *appwsdoc.Service {
	return appwsdoc.NewService(
		func() appwsdoc.Client { return cmuxctl.NewClient() },
		func(root string) cmuxmap.Store { return cmuxmap.NewStore(root) },
		func(root string) cmuxdocs.Store { return cmuxdocs.NewStore(root) },
		func(ctx context.Context, root string) (string, string, string) {
			target := appcmux.OpenTarget{
				WorkspaceID:   rootCMUXMappingID,
				WorkspacePath: root,
				Title:         "KRA_ROOT",
				StatusText:    "kra:root",
			}
			svc := appcmux.NewService(func() appcmux.Client {
				return wsOpenClientAdapter{inner: newCMUXOpenClient()}
			}, newCMUXMapStore)
			item, code, msg := svc.EnsureWorkspace(ctx, root, target, false)
			if code != "" {
				return "", code, msg
			}
			return item.CMUXWorkspaceID, "", ""
		},
	)
}
