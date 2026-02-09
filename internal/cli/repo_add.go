package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tasuku43/gionx/internal/app/repocmd"
	"github.com/tasuku43/gionx/internal/infra/appports"
)

func (c *CLI) runRepoAdd(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printRepoAddUsage(c.Out)
			return exitOK
		}
	}
	if len(args) == 0 {
		c.printRepoAddUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	ctx := context.Background()
	repoUC := repocmd.NewService(appports.NewRepoPort(c.ensureDebugLog, c.touchStateRegistry))
	session, err := repoUC.Run(ctx, repocmd.Request{
		CWD:           wd,
		DebugTag:      "repo-add",
		RequireGit:    true,
		TouchRegistry: true,
	})
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if session.DB != nil {
		defer func() { _ = session.DB.Close() }()
	}
	c.debugf("run repo add count=%d", len(args))

	requests := make([]repoPoolAddRequest, 0, len(args))
	for _, arg := range args {
		requests = append(requests, repoPoolAddRequest{RepoSpecInput: strings.TrimSpace(arg)})
	}

	useColorOut := writerSupportsColor(c.Out)
	printRepoPoolSection(c.Out, requests)
	outcomes := applyRepoPoolAddsWithProgress(ctx, session.DB, session.RepoPoolPath, requests, repoPoolAddDefaultWorkers, c.debugf, c.Out, useColorOut)
	printRepoPoolAddResult(c.Out, outcomes, useColorOut)
	if repoPoolAddHadFailure(outcomes) {
		return exitError
	}
	return exitOK
}
