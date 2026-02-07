package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
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

	if err := gitutil.EnsureGitInPath(); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "repo-add"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run repo add count=%d", len(args))

	ctx := context.Background()
	dbPath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve state db path: %v\n", err)
		return exitError
	}
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		fmt.Fprintf(c.Err, "resolve repo pool path: %v\n", err)
		return exitError
	}
	db, err := statestore.Open(ctx, dbPath)
	if err != nil {
		fmt.Fprintf(c.Err, "open state store: %v\n", err)
		return exitError
	}
	defer func() { _ = db.Close() }()

	if err := statestore.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		fmt.Fprintf(c.Err, "initialize settings: %v\n", err)
		return exitError
	}
	if err := c.touchStateRegistry(root, dbPath); err != nil {
		fmt.Fprintf(c.Err, "update state registry: %v\n", err)
		return exitError
	}

	requests := make([]repoPoolAddRequest, 0, len(args))
	for _, arg := range args {
		requests = append(requests, repoPoolAddRequest{RepoSpecInput: strings.TrimSpace(arg)})
	}

	useColorOut := writerSupportsColor(c.Out)
	printRepoPoolSection(c.Out, requests)
	outcomes := applyRepoPoolAddsWithProgress(ctx, db, repoPoolPath, requests, repoPoolAddDefaultWorkers, c.debugf, c.Out, useColorOut)
	printRepoPoolAddResult(c.Out, outcomes, useColorOut)
	if repoPoolAddHadFailure(outcomes) {
		return exitError
	}
	return exitOK
}
