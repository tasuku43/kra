package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/repodiscovery"
	"github.com/tasuku43/gionx/internal/statestore"
)

var newRepoDiscoveryProvider = repodiscovery.NewProvider
var promptRepoDiscoverSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptions("active", "add", "Repo pool:", "repo", candidates)
}

type repoDiscoverOptions struct {
	Org      string
	Provider string
}

func parseRepoDiscoverOptions(args []string) (repoDiscoverOptions, error) {
	opts := repoDiscoverOptions{Provider: "github"}
	rest := append([]string{}, args...)
	for len(rest) > 0 {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return repoDiscoverOptions{}, errHelpRequested
		case strings.HasPrefix(arg, "--org="):
			opts.Org = strings.TrimSpace(strings.TrimPrefix(arg, "--org="))
			rest = rest[1:]
		case arg == "--org":
			if len(rest) < 2 {
				return repoDiscoverOptions{}, fmt.Errorf("--org requires a value")
			}
			opts.Org = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimSpace(strings.TrimPrefix(arg, "--provider="))
			rest = rest[1:]
		case arg == "--provider":
			if len(rest) < 2 {
				return repoDiscoverOptions{}, fmt.Errorf("--provider requires a value")
			}
			opts.Provider = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return repoDiscoverOptions{}, fmt.Errorf("unknown flag for repo discover: %q", arg)
		}
	}
	if opts.Org == "" {
		return repoDiscoverOptions{}, fmt.Errorf("--org is required")
	}
	if opts.Provider == "" {
		opts.Provider = "github"
	}
	return opts, nil
}

func (c *CLI) runRepoDiscover(args []string) int {
	opts, err := parseRepoDiscoverOptions(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			c.printRepoDiscoverUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printRepoDiscoverUsage(c.Err)
		return exitUsage
	}

	if err := gitutil.EnsureGitInPath(); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	provider, err := newRepoDiscoveryProvider(opts.Provider)
	if err != nil {
		fmt.Fprintf(c.Err, "load provider: %v\n", err)
		return exitUsage
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
	if err := c.ensureDebugLog(root, "repo-discover"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run repo discover org=%s provider=%s", opts.Org, provider.Name())

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

	if err := provider.CheckAuth(ctx); err != nil {
		fmt.Fprintf(c.Err, "provider auth: %v\n", err)
		return exitError
	}

	discovered, err := provider.ListOrgRepos(ctx, opts.Org)
	if err != nil {
		fmt.Fprintf(c.Err, "discover repos: %v\n", err)
		return exitError
	}

	existingRepoUIDs, err := statestore.ListRepoUIDs(ctx, db)
	if err != nil {
		fmt.Fprintf(c.Err, "list existing repos: %v\n", err)
		return exitError
	}
	existingSet := map[string]bool{}
	for _, repoUID := range existingRepoUIDs {
		existingSet[repoUID] = true
	}

	candidates := make([]repodiscovery.Repo, 0, len(discovered))
	for _, r := range discovered {
		if existingSet[r.RepoUID] {
			continue
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		fmt.Fprintf(c.Err, "no undiscovered repos found for org: %s\n", opts.Org)
		return exitError
	}

	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(candidates))
	repoByKey := make(map[string]repodiscovery.Repo, len(candidates))
	for _, cand := range candidates {
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:          cand.RepoKey,
			Description: "",
		})
		repoByKey[cand.RepoKey] = cand
	}

	selectedIDs, err := promptRepoDiscoverSelection(c, selectorCandidates)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "select repos: %v\n", err)
		return exitError
	}
	if len(selectedIDs) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	requests := make([]repoPoolAddRequest, 0, len(selectedIDs))
	for _, key := range selectedIDs {
		repo, ok := repoByKey[key]
		if !ok {
			continue
		}
		requests = append(requests, repoPoolAddRequest{RepoSpecInput: repo.RemoteURL, DisplayName: repo.RepoKey})
	}
	if len(requests) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}
	useColorOut := writerSupportsColor(c.Out)
	outcomes := applyRepoPoolAddsWithProgress(ctx, db, repoPoolPath, requests, repoPoolAddDefaultWorkers, c.debugf, c.Out, useColorOut)
	printRepoPoolAddResult(c.Out, outcomes, useColorOut)
	if repoPoolAddHadFailure(outcomes) {
		return exitError
	}
	return exitOK
}
