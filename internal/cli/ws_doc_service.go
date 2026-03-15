package cli

import (
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
	)
}
