package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/repodiscovery"
	"github.com/tasuku43/gionx/internal/statestore"
)

var newRepoDiscoveryProvider = repodiscovery.NewProvider

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

	selected, err := c.promptRepoDiscoverSelection(candidates)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "select repos: %v\n", err)
		return exitError
	}
	if len(selected) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	requests := make([]repoPoolAddRequest, 0, len(selected))
	for _, s := range selected {
		requests = append(requests, repoPoolAddRequest{RepoSpecInput: s.RemoteURL, DisplayName: s.RepoKey})
	}
	printRepoPoolSection(c.Out, requests)
	outcomes := applyRepoPoolAdds(ctx, db, repoPoolPath, requests, c.debugf)
	printRepoPoolAddResult(c.Out, outcomes, writerSupportsColor(c.Out))
	if repoPoolAddHadFailure(outcomes) {
		return exitError
	}
	return exitOK
}

func (c *CLI) promptRepoDiscoverSelection(candidates []repodiscovery.Repo) ([]repodiscovery.Repo, error) {
	if len(candidates) == 0 {
		return nil, errSelectorCanceled
	}
	filter := ""
	for {
		visible := filterRepoDiscoverCandidates(candidates, filter)
		fmt.Fprintln(c.Err, "Repo pool:")
		fmt.Fprintln(c.Err)
		if len(visible) == 0 {
			fmt.Fprintf(c.Err, "%s(none)\n", uiIndent)
		} else {
			for i, it := range visible {
				fmt.Fprintf(c.Err, "%s[%d] %s\n", uiIndent, i+1, it.RepoKey)
			}
		}
		fmt.Fprintln(c.Err)
		fmt.Fprintf(c.Err, "%sfilter: %s\n", uiIndent, filter)

		line, err := c.promptLine(fmt.Sprintf("%sselect repos (comma numbers, /filter, empty=cancel): ", uiIndent))
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return nil, errSelectorCanceled
		}
		if strings.HasPrefix(line, "/") {
			filter = strings.TrimSpace(strings.TrimPrefix(line, "/"))
			continue
		}

		indices, err := parseMultiSelectIndices(line, len(visible))
		if err != nil {
			fmt.Fprintf(c.Err, "%sinvalid selection: %v\n", uiIndent, err)
			continue
		}
		selected := make([]repodiscovery.Repo, 0, len(indices))
		for _, idx := range indices {
			selected = append(selected, visible[idx])
		}
		return selected, nil
	}
}

func filterRepoDiscoverCandidates(candidates []repodiscovery.Repo, filter string) []repodiscovery.Repo {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return slices.Clone(candidates)
	}
	out := make([]repodiscovery.Repo, 0, len(candidates))
	for _, it := range candidates {
		if strings.Contains(strings.ToLower(it.RepoKey), filter) {
			out = append(out, it)
		}
	}
	return out
}
