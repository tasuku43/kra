package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tasuku43/gionx/internal/app/repocmd"
	"github.com/tasuku43/gionx/internal/infra/appports"
	"github.com/tasuku43/gionx/internal/infra/gitutil"
	"github.com/tasuku43/gionx/internal/repodiscovery"
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
	ctx := context.Background()
	repoUC := repocmd.NewService(appports.NewRepoPort(c.ensureDebugLog, c.touchStateRegistry))
	session, err := repoUC.Run(ctx, repocmd.Request{
		CWD:           wd,
		DebugTag:      "repo-discover",
		RequireGit:    true,
		TouchRegistry: true,
	})
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	c.debugf("run repo discover org=%s provider=%s", opts.Org, provider.Name())

	if err := provider.CheckAuth(ctx); err != nil {
		fmt.Fprintf(c.Err, "provider auth: %v\n", err)
		return exitError
	}

	discovered, err := provider.ListOrgRepos(ctx, opts.Org)
	if err != nil {
		fmt.Fprintf(c.Err, "discover repos: %v\n", err)
		return exitError
	}

	existingRepoUIDs, err := listRepoUIDsFromRepoPool(ctx, session.RepoPoolPath)
	if err != nil {
		fmt.Fprintf(c.Err, "list existing repos from pool: %v\n", err)
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
			ID:    cand.RepoKey,
			Title: "",
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
	outcomes := applyRepoPoolAddsWithProgress(ctx, session.RepoPoolPath, requests, repoPoolAddDefaultWorkers, c.debugf, c.Out, useColorOut)
	printRepoPoolAddResult(c.Out, outcomes, useColorOut)
	if repoPoolAddHadFailure(outcomes) {
		return exitError
	}
	return exitOK
}

func listRepoUIDsFromRepoPool(ctx context.Context, repoPoolPath string) ([]string, error) {
	bareRepos, err := listRepoPoolBareRepos(repoPoolPath)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(bareRepos))
	out := make([]string, 0, len(bareRepos))
	for _, barePath := range bareRepos {
		remoteURL, _ := gitutil.RunBare(ctx, barePath, "config", "--get", "remote.origin.url")
		repoUID, _, ok := resolveRepoIdentityForGC(repoPoolPath, barePath, strings.TrimSpace(remoteURL))
		if !ok || seen[repoUID] {
			continue
		}
		seen[repoUID] = true
		out = append(out, repoUID)
	}
	return out, nil
}
