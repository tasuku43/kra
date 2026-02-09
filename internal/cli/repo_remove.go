package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/gionx/internal/app/repocmd"
	"github.com/tasuku43/gionx/internal/infra/appports"
	"github.com/tasuku43/gionx/internal/infra/gitutil"
	"github.com/tasuku43/gionx/internal/infra/statestore"
)

var promptRepoRemoveSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptions("active", "remove", "Repo pool:", "repo", candidates)
}

func (c *CLI) runRepoRemove(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printRepoRemoveUsage(c.Out)
			return exitOK
		}
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
		DebugTag:      "repo-remove",
		TouchRegistry: true,
	})
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if session.DB != nil {
		defer func() { _ = session.DB.Close() }()
	}
	c.debugf("run repo remove args=%d", len(args))

	startDay := localDayKey(time.Now().AddDate(0, 0, -29))
	var (
		repos       []statestore.RootRepoCandidate
		bareByRepo  map[string]string
		fallbackErr error
	)
	if session.DB != nil {
		repos, err = statestore.ListRootRepoCandidates(ctx, session.DB, startDay)
		if err != nil {
			fmt.Fprintf(c.Err, "list repos: %v\n", err)
			return exitError
		}
	} else {
		repos, bareByRepo, fallbackErr = listRootRepoCandidatesFromFilesystem(ctx, session.Root, session.RepoPoolPath)
		if fallbackErr != nil {
			fmt.Fprintf(c.Err, "list repos: %v\n", fallbackErr)
			return exitError
		}
	}
	if len(repos) == 0 {
		fmt.Fprintln(c.Err, "no repos registered in current root")
		return exitError
	}

	selected, err := selectReposForRemove(c, repos, args)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if len(selected) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	blocked := make([]string, 0)
	repoUIDs := make([]string, 0, len(selected))
	for _, it := range selected {
		if it.WorkspaceRefCount > 0 {
			blocked = append(blocked, fmt.Sprintf("%s (workspace refs: %d)", it.RepoKey, it.WorkspaceRefCount))
		}
		repoUIDs = append(repoUIDs, it.RepoUID)
	}
	if len(blocked) > 0 {
		fmt.Fprintln(c.Err, "cannot remove repos that are still bound to workspaces:")
		for _, line := range blocked {
			fmt.Fprintf(c.Err, "%s- %s\n", uiIndent, line)
		}
		fmt.Fprintln(c.Err, "hint: remove workspace repo bindings first (for example via ws close/purge or future ws repo detach flow)")
		return exitError
	}

	printRepoRemoveSelection(c.Out, selected)
	if session.DB != nil {
		if err := statestore.DeleteReposByUIDs(ctx, session.DB, repoUIDs); err != nil {
			fmt.Fprintf(c.Err, "remove repos: %v\n", err)
			return exitError
		}
	} else {
		for _, repoUID := range repoUIDs {
			barePath := strings.TrimSpace(bareByRepo[repoUID])
			if barePath == "" {
				continue
			}
			if err := os.RemoveAll(barePath); err != nil {
				fmt.Fprintf(c.Err, "remove repos: %v\n", err)
				return exitError
			}
		}
	}

	useColorOut := writerSupportsColor(c.Out)
	printRepoRemoveResult(c.Out, selected, useColorOut)
	return exitOK
}

func listRootRepoCandidatesFromFilesystem(ctx context.Context, root string, repoPoolPath string) ([]statestore.RootRepoCandidate, map[string]string, error) {
	bareRepos, err := listRepoPoolBareRepos(repoPoolPath)
	if err != nil {
		return nil, nil, err
	}
	workspaceRefs, err := listWorkspaceRepoRefCountFromFilesystem(ctx, root)
	if err != nil {
		return nil, nil, err
	}

	candidates := make([]statestore.RootRepoCandidate, 0, len(bareRepos))
	bareByRepo := make(map[string]string, len(bareRepos))
	seen := make(map[string]bool, len(bareRepos))
	for _, barePath := range bareRepos {
		remoteURL, _ := gitutil.RunBare(ctx, barePath, "config", "--get", "remote.origin.url")
		repoUID, repoKey, ok := resolveRepoIdentityForGC(repoPoolPath, barePath, strings.TrimSpace(remoteURL))
		if !ok || seen[repoUID] {
			continue
		}
		seen[repoUID] = true
		candidates = append(candidates, statestore.RootRepoCandidate{
			RepoUID:           repoUID,
			RepoKey:           repoKey,
			RemoteURL:         strings.TrimSpace(remoteURL),
			WorkspaceRefCount: workspaceRefs[repoUID],
		})
		bareByRepo[repoUID] = barePath
	}
	slices.SortFunc(candidates, func(a, b statestore.RootRepoCandidate) int {
		return strings.Compare(a.RepoKey, b.RepoKey)
	})
	return candidates, bareByRepo, nil
}

func listWorkspaceRepoRefCountFromFilesystem(ctx context.Context, root string) (map[string]int, error) {
	counts := map[string]int{}
	for _, scope := range []string{"workspaces", "archive"} {
		base := filepath.Join(root, scope)
		entries, err := os.ReadDir(base)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			repos, err := listWorkspaceReposForClose(ctx, nil, root, ent.Name())
			if err != nil {
				continue
			}
			for _, r := range repos {
				counts[r.RepoUID]++
			}
		}
	}
	return counts, nil
}

func selectReposForRemove(c *CLI, repos []statestore.RootRepoCandidate, args []string) ([]statestore.RootRepoCandidate, error) {
	byKey := make(map[string]statestore.RootRepoCandidate, len(repos))
	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(repos))
	for _, it := range repos {
		byKey[it.RepoKey] = it
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:    it.RepoKey,
			Title: "",
		})
	}

	if len(args) > 0 {
		selected := make([]statestore.RootRepoCandidate, 0, len(args))
		seen := map[string]bool{}
		for _, raw := range args {
			key := strings.TrimSpace(raw)
			if key == "" {
				continue
			}
			it, ok := byKey[key]
			if !ok {
				return nil, fmt.Errorf("repo not found in current root: %s", key)
			}
			if seen[it.RepoUID] {
				continue
			}
			seen[it.RepoUID] = true
			selected = append(selected, it)
		}
		return selected, nil
	}

	selectedIDs, err := promptRepoRemoveSelection(c, selectorCandidates)
	if err != nil {
		return nil, err
	}
	selected := make([]statestore.RootRepoCandidate, 0, len(selectedIDs))
	for _, key := range selectedIDs {
		it, ok := byKey[key]
		if !ok {
			continue
		}
		selected = append(selected, it)
	}
	slices.SortFunc(selected, func(a, b statestore.RootRepoCandidate) int {
		return strings.Compare(a.RepoKey, b.RepoKey)
	})
	return selected, nil
}

func printRepoRemoveSelection(out io.Writer, selected []statestore.RootRepoCandidate) {
	fmt.Fprintln(out, "Repo pool:")
	fmt.Fprintln(out)
	for _, it := range selected {
		fmt.Fprintf(out, "%s- %s\n", uiIndent, it.RepoKey)
	}
}

func printRepoRemoveResult(out io.Writer, removed []statestore.RootRepoCandidate, useColor bool) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, renderResultTitle(useColor))
	summary := fmt.Sprintf("Removed %d / %d", len(removed), len(removed))
	if useColor {
		summary = styleSuccess(summary, true)
	}
	fmt.Fprintf(out, "%s%s\n", uiIndent, summary)
	for _, it := range removed {
		fmt.Fprintf(out, "%s- %s\n", uiIndent, it.RepoKey)
	}
}
