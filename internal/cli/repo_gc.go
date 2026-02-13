package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tasuku43/kra/internal/app/repocmd"
	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/stateregistry"
)

type repoGCCandidate struct {
	RepoUID   string
	RepoKey   string
	RemoteURL string
	BarePath  string

	CurrentRootRegistered bool
	CurrentWorkspaceBound bool
	OtherRootRefCount     int
	HasWorktrees          bool

	Inspectable bool
	SkipReason  string
}

func (c repoGCCandidate) eligible() bool {
	if !c.Inspectable {
		return false
	}
	return !c.CurrentRootRegistered && !c.CurrentWorkspaceBound && c.OtherRootRefCount == 0 && !c.HasWorktrees
}

var promptRepoGCSelection = func(c *CLI, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptions("active", "gc", "Repo pool:", "repo", candidates)
}

func (c *CLI) runRepoGC(args []string) int {
	outputFormat := "human"
	forceYes := false
	selectArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printRepoGCUsage(c.Out)
			return exitOK
		case "--yes":
			forceYes = true
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printRepoGCUsage(c.Err)
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
				fmt.Fprintf(c.Err, "unknown flag for repo gc: %q\n", arg)
				c.printRepoGCUsage(c.Err)
				return exitUsage
			}
			selectArgs = append(selectArgs, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printRepoGCUsage(c.Err)
		return exitUsage
	}
	if outputFormat == "json" {
		if len(selectArgs) == 0 {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.gc",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: "repo gc --format json requires at least one <repo-key|repo-uid>",
				},
			})
			return exitUsage
		}
		if !forceYes {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.gc",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: "--yes is required in --format json mode",
				},
			})
			return exitUsage
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.gc",
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
		DebugTag:      "repo-gc",
		RequireGit:    true,
		TouchRegistry: true,
	})
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.gc",
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
	c.debugf("run repo gc args=%d", len(selectArgs))

	candidates, err := buildRepoGCCandidates(ctx, session.Root, session.RepoPoolPath, c.debugf)
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.gc",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("build repo gc candidates: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "build repo gc candidates: %v\n", err)
		return exitError
	}
	selectable := make([]repoGCCandidate, 0, len(candidates))
	for _, it := range candidates {
		if it.Inspectable {
			selectable = append(selectable, it)
		}
	}
	if len(selectable) == 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.gc",
				Error: &cliJSONError{
					Code:    "not_found",
					Message: "no gc candidates found in repo pool",
				},
			})
			return exitError
		}
		fmt.Fprintln(c.Err, "no gc candidates found in repo pool")
		return exitError
	}

	selected, err := selectRepoGCCandidates(c, selectable, selectArgs)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			if outputFormat == "json" {
				_ = writeCLIJSON(c.Out, cliJSONResponse{
					OK:     false,
					Action: "repo.gc",
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
				Action: "repo.gc",
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
				Action: "repo.gc",
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
	blocked := make([]repoGCCandidate, 0)
	eligibleSelected := make([]repoGCCandidate, 0, len(selected))
	for _, it := range selected {
		if it.eligible() {
			eligibleSelected = append(eligibleSelected, it)
			continue
		}
		blocked = append(blocked, it)
	}
	if len(blocked) > 0 {
		if outputFormat == "json" {
			items := make([]map[string]any, 0, len(blocked))
			for _, it := range blocked {
				items = append(items, map[string]any{
					"repo_key": it.RepoKey,
					"reason":   describeRepoGCSkipReason(it),
				})
			}
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.gc",
				Result: map[string]any{
					"blocked": items,
				},
				Error: &cliJSONError{
					Code:    "conflict",
					Message: "selected repos are not eligible for gc",
				},
			})
			return exitError
		}
		fmt.Fprintln(c.Err, "selected repos are not eligible for gc:")
		for _, it := range blocked {
			reason := describeRepoGCSkipReason(it)
			if strings.TrimSpace(reason) == "" {
				reason = "not eligible"
			}
			fmt.Fprintf(c.Err, "%s- %s (%s)\n", uiIndent, it.RepoKey, reason)
		}
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	if outputFormat == "human" {
		printRepoGCSelection(c.Out, eligibleSelected, useColorOut)
		fmt.Fprintln(c.Out)
		fmt.Fprintln(c.Out, renderRiskTitle(useColorOut))
		fmt.Fprintf(c.Out, "%srepo gc permanently deletes bare repos from pool.\n", uiIndent)
		fmt.Fprintf(c.Out, "%s%s %d\n", uiIndent, styleAccent("selected:", useColorOut), len(eligibleSelected))

		line, err := c.promptLine(fmt.Sprintf("%sremove selected bare repos from pool? this is permanent (y/N): ", uiIndent))
		if err != nil {
			fmt.Fprintf(c.Err, "read confirmation: %v\n", err)
			return exitError
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
		default:
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
	}

	removed, failed := applyRepoGC(eligibleSelected)
	if outputFormat == "json" {
		removedItems := make([]map[string]any, 0, len(removed))
		for _, it := range removed {
			removedItems = append(removedItems, map[string]any{
				"repo_key": it.RepoKey,
			})
		}
		failedItems := make([]map[string]any, 0, len(failed))
		for _, it := range failed {
			failedItems = append(failedItems, map[string]any{
				"repo_key": it.RepoKey,
				"reason":   it.SkipReason,
			})
		}
		if len(failed) == 0 {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     true,
				Action: "repo.gc",
				Result: map[string]any{
					"removed": len(removed),
					"total":   len(eligibleSelected),
					"items":   removedItems,
				},
			})
			return exitOK
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "repo.gc",
			Result: map[string]any{
				"removed": len(removed),
				"total":   len(eligibleSelected),
				"items":   removedItems,
				"failed":  failedItems,
			},
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("failed to remove %d repo(s)", len(failed)),
			},
		})
		return exitError
	}
	useColor := useColorOut
	fmt.Fprintln(c.Out)
	fmt.Fprintln(c.Out, renderResultTitle(useColor))
	summary := fmt.Sprintf("Removed %d / %d", len(removed), len(eligibleSelected))
	if useColor {
		switch {
		case len(removed) == len(eligibleSelected):
			summary = styleSuccess(summary, useColor)
		case len(removed) == 0:
			summary = styleError(summary, useColor)
		default:
			summary = styleWarn(summary, useColor)
		}
	}
	fmt.Fprintf(c.Out, "%s%s\n", uiIndent, summary)
	for _, it := range removed {
		prefix := "✔"
		if useColor {
			prefix = styleSuccess(prefix, useColor)
		}
		fmt.Fprintf(c.Out, "%s%s %s\n", uiIndent, prefix, it.RepoKey)
	}
	for _, it := range failed {
		prefix := "!"
		if useColor {
			prefix = styleError(prefix, useColor)
		}
		fmt.Fprintf(c.Out, "%s%s %s (%s)\n", uiIndent, prefix, it.RepoKey, it.SkipReason)
	}
	if len(failed) > 0 {
		return exitError
	}
	return exitOK
}

func buildRepoGCCandidates(ctx context.Context, root string, repoPoolPath string, debugf func(string, ...any)) ([]repoGCCandidate, error) {
	poolRepos, err := listRepoPoolBareRepos(repoPoolPath)
	if err != nil {
		return nil, err
	}
	if len(poolRepos) == 0 {
		return nil, nil
	}

	currentRepoSet := map[string]bool{}

	globalRefCount, currentRootRefCount, err := collectRegistryRepoRefCountsFromMetadata(root)
	if err != nil {
		return nil, err
	}

	out := make([]repoGCCandidate, 0, len(poolRepos))
	for _, barePath := range poolRepos {
		cand := repoGCCandidate{
			BarePath:    barePath,
			Inspectable: false,
		}

		remoteURL, remoteErr := gitutil.RunBare(ctx, barePath, "config", "--get", "remote.origin.url")
		repoUID, repoKey, resolved := resolveRepoIdentityForGC(repoPoolPath, barePath, strings.TrimSpace(remoteURL))
		if !resolved {
			if remoteErr != nil {
				cand.SkipReason = "cannot resolve repo identity (remote/path)"
			} else {
				cand.SkipReason = "cannot normalize remote.origin.url"
			}
			out = append(out, cand)
			continue
		}
		cand.RepoUID = repoUID
		cand.RepoKey = repoKey
		cand.RemoteURL = strings.TrimSpace(remoteURL)
		cand.Inspectable = true
		cand.CurrentRootRegistered = currentRepoSet[repoUID]
		cand.CurrentWorkspaceBound = currentRootRefCount[repoUID] > 0

		refCount := globalRefCount[repoUID]
		if currentRootRefCount[repoUID] > 0 {
			refCount -= currentRootRefCount[repoUID]
			if refCount < 0 {
				refCount = 0
			}
		}
		cand.OtherRootRefCount = refCount

		hasWorktrees, err := bareRepoHasLinkedWorktrees(ctx, barePath)
		if err != nil {
			cand.SkipReason = "cannot inspect worktrees"
			cand.Inspectable = false
			out = append(out, cand)
			continue
		}
		cand.HasWorktrees = hasWorktrees

		if !cand.eligible() && strings.TrimSpace(cand.SkipReason) == "" {
			cand.SkipReason = describeRepoGCSkipReason(cand)
		}
		out = append(out, cand)
	}

	slices.SortFunc(out, func(a, b repoGCCandidate) int {
		ak := strings.TrimSpace(a.RepoKey)
		bk := strings.TrimSpace(b.RepoKey)
		if ak == "" && bk == "" {
			return strings.Compare(a.BarePath, b.BarePath)
		}
		if ak == "" {
			return 1
		}
		if bk == "" {
			return -1
		}
		return strings.Compare(ak, bk)
	})
	if debugf != nil {
		debugf("repo gc candidates total=%d eligible=%d root=%s", len(out), countEligibleRepoGCCandidates(out), root)
	}
	return out, nil
}

func resolveRepoIdentityForGC(repoPoolPath string, barePath string, remoteURL string) (repoUID string, repoKey string, ok bool) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL != "" {
		if spec, err := repospec.Normalize(remoteURL); err == nil {
			return fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo), fmt.Sprintf("%s/%s", spec.Owner, spec.Repo), true
		}
	}

	rel, err := filepath.Rel(repoPoolPath, barePath)
	if err != nil {
		return "", "", false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	parts := strings.Split(rel, "/")
	if len(parts) != 3 {
		return "", "", false
	}
	host := strings.TrimSpace(parts[0])
	owner := strings.TrimSpace(parts[1])
	repoGit := strings.TrimSpace(parts[2])
	if host == "" || owner == "" || !strings.HasSuffix(repoGit, ".git") {
		return "", "", false
	}
	repo := strings.TrimSuffix(repoGit, ".git")
	if strings.TrimSpace(repo) == "" {
		return "", "", false
	}
	return fmt.Sprintf("%s/%s/%s", host, owner, repo), fmt.Sprintf("%s/%s", owner, repo), true
}

func countEligibleRepoGCCandidates(items []repoGCCandidate) int {
	count := 0
	for _, it := range items {
		if it.eligible() {
			count++
		}
	}
	return count
}

func listRepoPoolBareRepos(repoPoolPath string) ([]string, error) {
	roots := make([]string, 0, 32)
	err := filepath.WalkDir(repoPoolPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".git") {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
			return nil
		}
		roots = append(roots, path)
		return filepath.SkipDir
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk repo pool: %w", err)
	}
	slices.Sort(roots)
	return roots, nil
}

func collectRegistryRepoRefCountsFromMetadata(currentRoot string) (map[string]int, map[string]int, error) {
	registryPath, err := stateregistry.Path()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve root registry path: %w", err)
	}
	entries, err := stateregistry.Load(registryPath)
	if err != nil {
		return nil, nil, err
	}
	seenRoot := map[string]bool{}
	currentMarked := map[string]bool{}
	counts := map[string]int{}
	currentCounts := map[string]int{}

	addRoot := func(rootPath string, collectCurrent bool) error {
		rootPath = strings.TrimSpace(rootPath)
		if rootPath == "" {
			return nil
		}
		if seenRoot[rootPath] {
			if collectCurrent && !currentMarked[rootPath] {
				refs, err := scanRootRepoRefsFromMetadata(rootPath)
				if err != nil {
					return fmt.Errorf("scan root repo refs from metadata %s: %w", rootPath, err)
				}
				for uid, n := range refs {
					currentCounts[uid] += n
				}
				currentMarked[rootPath] = true
			}
			return nil
		}
		seenRoot[rootPath] = true
		if _, statErr := os.Stat(rootPath); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("stat root %s: %w", rootPath, statErr)
		}
		refs, err := scanRootRepoRefsFromMetadata(rootPath)
		if err != nil {
			return fmt.Errorf("scan root repo refs from metadata %s: %w", rootPath, err)
		}
		for uid, n := range refs {
			counts[uid] += n
			if collectCurrent {
				currentCounts[uid] += n
			}
		}
		if collectCurrent {
			currentMarked[rootPath] = true
		}
		return nil
	}

	for _, e := range entries {
		if err := addRoot(e.RootPath, false); err != nil {
			return nil, nil, err
		}
	}
	if err := addRoot(currentRoot, true); err != nil {
		return nil, nil, err
	}
	return counts, currentCounts, nil
}

func scanRootRepoRefsFromMetadata(root string) (map[string]int, error) {
	counts := map[string]int{}
	for _, scope := range []string{"workspaces", "archive"} {
		scopeDir := filepath.Join(root, scope)
		entries, err := os.ReadDir(scopeDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			wsPath := filepath.Join(scopeDir, e.Name())
			meta, err := loadWorkspaceMetaFile(wsPath)
			if err != nil {
				return nil, err
			}
			for _, r := range meta.ReposRestore {
				uid := strings.TrimSpace(r.RepoUID)
				if uid == "" {
					continue
				}
				counts[uid]++
			}
		}
	}
	return counts, nil
}

func toSet(values []string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		m[v] = true
	}
	return m
}

func describeRepoGCSkipReason(c repoGCCandidate) string {
	switch {
	case c.CurrentWorkspaceBound:
		return "workspace bindings exist in current root"
	case c.CurrentRootRegistered:
		return "still registered in current root"
	case c.OtherRootRefCount > 0:
		return fmt.Sprintf("referenced by %d other root(s)", c.OtherRootRefCount)
	case c.HasWorktrees:
		return "linked worktrees still exist"
	default:
		return "not eligible"
	}
}

func bareRepoHasLinkedWorktrees(ctx context.Context, barePath string) (bool, error) {
	out, err := gitutil.RunBare(ctx, barePath, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		if path == "" {
			continue
		}
		if filepath.Clean(path) == filepath.Clean(barePath) {
			continue
		}
		gitMetaPath := filepath.Join(path, ".git")
		info, statErr := os.Stat(gitMetaPath)
		if statErr != nil {
			continue
		}
		if !info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}

func selectRepoGCCandidates(c *CLI, candidates []repoGCCandidate, args []string) ([]repoGCCandidate, error) {
	byUID := make(map[string]repoGCCandidate, len(candidates))
	byKey := make(map[string]repoGCCandidate, len(candidates))
	ambiguousKeys := map[string]bool{}
	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(candidates))

	for _, it := range candidates {
		byUID[it.RepoUID] = it
		if prev, exists := byKey[it.RepoKey]; exists && prev.RepoUID != it.RepoUID {
			ambiguousKeys[it.RepoKey] = true
		} else {
			byKey[it.RepoKey] = it
		}
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:    it.RepoKey,
			Title: "",
		})
	}

	if len(args) > 0 {
		selected := make([]repoGCCandidate, 0, len(args))
		seen := map[string]bool{}
		for _, raw := range args {
			token := strings.TrimSpace(raw)
			if token == "" {
				continue
			}
			var cand repoGCCandidate
			var ok bool
			if cand, ok = byUID[token]; !ok {
				if ambiguousKeys[token] {
					return nil, fmt.Errorf("ambiguous repo key (use repo_uid): %s", token)
				}
				cand, ok = byKey[token]
			}
			if !ok {
				return nil, fmt.Errorf("repo not found in gc candidates: %s", token)
			}
			if seen[cand.RepoUID] {
				continue
			}
			seen[cand.RepoUID] = true
			selected = append(selected, cand)
		}
		return selected, nil
	}

	selectedIDs, err := promptRepoGCSelection(c, selectorCandidates)
	if err != nil {
		return nil, err
	}
	selected := make([]repoGCCandidate, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		cand, ok := byKey[id]
		if !ok {
			continue
		}
		selected = append(selected, cand)
	}
	slices.SortFunc(selected, func(a, b repoGCCandidate) int {
		return strings.Compare(a.RepoKey, b.RepoKey)
	})
	return selected, nil
}

func printRepoGCSelection(out io.Writer, selected []repoGCCandidate, useColor bool) {
	fmt.Fprintln(out, styleBold("Repo pool:", useColor))
	fmt.Fprintln(out)
	for _, it := range selected {
		fmt.Fprintf(out, "%s- %s\n", uiIndent, it.RepoKey)
	}
}

func applyRepoGC(selected []repoGCCandidate) (removed []repoGCCandidate, failed []repoGCCandidate) {
	removed = make([]repoGCCandidate, 0, len(selected))
	failed = make([]repoGCCandidate, 0)
	for _, it := range selected {
		if err := os.RemoveAll(it.BarePath); err != nil {
			it.SkipReason = err.Error()
			failed = append(failed, it)
			continue
		}
		removed = append(removed, it)
	}
	return removed, failed
}
