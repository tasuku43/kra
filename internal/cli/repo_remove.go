package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
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
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "repo-remove"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run repo remove args=%d", len(args))

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

	startDay := localDayKey(time.Now().AddDate(0, 0, -29))
	repos, err := statestore.ListRootRepoCandidates(ctx, db, startDay)
	if err != nil {
		fmt.Fprintf(c.Err, "list repos: %v\n", err)
		return exitError
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
	if err := statestore.DeleteReposByUIDs(ctx, db, repoUIDs); err != nil {
		fmt.Fprintf(c.Err, "remove repos: %v\n", err)
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	printRepoRemoveResult(c.Out, selected, useColorOut)
	return exitOK
}

func selectReposForRemove(c *CLI, repos []statestore.RootRepoCandidate, args []string) ([]statestore.RootRepoCandidate, error) {
	byKey := make(map[string]statestore.RootRepoCandidate, len(repos))
	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(repos))
	for _, it := range repos {
		byKey[it.RepoKey] = it
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:          it.RepoKey,
			Description: "",
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
	fmt.Fprintf(out, "%sRemoved %d / %d\n", uiIndent, len(removed), len(removed))
	for _, it := range removed {
		fmt.Fprintf(out, "%s- %s\n", uiIndent, it.RepoKey)
	}
}
