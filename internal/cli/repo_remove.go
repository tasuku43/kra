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

	"github.com/tasuku43/kra/internal/app/repocmd"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/statestore"
)

var promptRepoRemoveSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptions("active", "remove", "Repo pool:", "repo", candidates)
}

func (c *CLI) runRepoRemove(args []string) int {
	outputFormat := "human"
	repoArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printRepoRemoveUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printRepoRemoveUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(c.Err, "unknown flag for repo remove: %q\n", arg)
				c.printRepoRemoveUsage(c.Err)
				return exitUsage
			}
			repoArgs = append(repoArgs, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printRepoRemoveUsage(c.Err)
		return exitUsage
	}
	if outputFormat == "json" && len(repoArgs) == 0 {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "repo.remove",
			Error: &cliJSONError{
				Code:    "invalid_argument",
				Message: "repo remove --format json requires at least one <repo-key>",
			},
		})
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.remove",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("get working dir: %v", err),
				},
			})
			return exitError
		}
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
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.remove",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: err.Error(),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	c.debugf("run repo remove args=%d", len(repoArgs))

	repos, fallbackErr := listRootRepoCandidatesFromFilesystem(ctx, session.Root, session.RepoPoolPath)
	if fallbackErr != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.remove",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("list repos: %v", fallbackErr),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "list repos: %v\n", fallbackErr)
		return exitError
	}
	if len(repos) == 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.remove",
				Error: &cliJSONError{
					Code:    "not_found",
					Message: "no repos registered in current root",
				},
			})
			return exitError
		}
		fmt.Fprintln(c.Err, "no repos registered in current root")
		return exitError
	}

	selected, err := selectReposForRemove(c, repos, repoArgs)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			if outputFormat == "json" {
				_ = writeCLIJSON(c.Out, cliJSONResponse{
					OK:     false,
					Action: "repo.remove",
					Error: &cliJSONError{
						Code:    "conflict",
						Message: "aborted",
					},
				})
				return exitError
			}
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		if outputFormat == "json" {
			code := "invalid_argument"
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				code = "not_found"
			}
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.remove",
				Error: &cliJSONError{
					Code:    code,
					Message: err.Error(),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if len(selected) == 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.remove",
				Error: &cliJSONError{
					Code:    "conflict",
					Message: "aborted",
				},
			})
			return exitError
		}
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	blocked := make([]string, 0)
	for _, it := range selected {
		if it.WorkspaceRefCount > 0 {
			blocked = append(blocked, fmt.Sprintf("%s (workspace refs: %d)", it.RepoKey, it.WorkspaceRefCount))
		}
	}
	if len(blocked) > 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.remove",
				Result: map[string]any{
					"blocked": blocked,
				},
				Error: &cliJSONError{
					Code:    "conflict",
					Message: "cannot remove repos that are still bound to workspaces",
				},
			})
			return exitError
		}
		fmt.Fprintln(c.Err, "cannot remove repos that are still bound to workspaces:")
		for _, line := range blocked {
			fmt.Fprintf(c.Err, "%s- %s\n", uiIndent, line)
		}
		fmt.Fprintln(c.Err, "hint: remove workspace repo bindings first (for example via ws close/purge or future ws repo detach flow)")
		return exitError
	}
	if outputFormat == "json" {
		reposOut := make([]string, 0, len(selected))
		for _, it := range selected {
			reposOut = append(reposOut, it.RepoKey)
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "repo.remove",
			Result: map[string]any{
				"removed": len(selected),
				"total":   len(selected),
				"repos":   reposOut,
			},
		})
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	printRepoRemoveSelection(c.Out, selected, useColorOut)
	printRepoRemoveResult(c.Out, selected, useColorOut)
	return exitOK
}

func listRootRepoCandidatesFromFilesystem(ctx context.Context, root string, repoPoolPath string) ([]statestore.RootRepoCandidate, error) {
	bareRepos, err := listRepoPoolBareRepos(repoPoolPath)
	if err != nil {
		return nil, err
	}
	workspaceRefs, err := listWorkspaceRepoRefCountFromFilesystem(ctx, root)
	if err != nil {
		return nil, err
	}

	candidates := make([]statestore.RootRepoCandidate, 0, len(bareRepos))
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
	}
	slices.SortFunc(candidates, func(a, b statestore.RootRepoCandidate) int {
		return strings.Compare(a.RepoKey, b.RepoKey)
	})
	return candidates, nil
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
			wsPath := filepath.Join(base, ent.Name())
			meta, err := loadWorkspaceMetaFile(wsPath)
			if err != nil {
				continue
			}
			repos, err := listWorkspaceReposFromFilesystem(ctx, root, scope, ent.Name(), meta)
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

func printRepoRemoveSelection(out io.Writer, selected []statestore.RootRepoCandidate, useColor bool) {
	fmt.Fprintln(out, styleBold("Repo pool:", useColor))
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
		summary = styleSuccess(summary, useColor)
	}
	fmt.Fprintf(out, "%s%s\n", uiIndent, summary)
	for _, it := range removed {
		fmt.Fprintf(out, "%s- %s\n", uiIndent, it.RepoKey)
	}
}
