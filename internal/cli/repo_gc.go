package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/stateregistry"
	"github.com/tasuku43/gionx/internal/statestore"
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
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printRepoGCUsage(c.Out)
			return exitOK
		}
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
	if err := c.ensureDebugLog(root, "repo-gc"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run repo gc args=%d", len(args))

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

	candidates, err := buildRepoGCCandidates(ctx, root, db, dbPath, repoPoolPath, c.debugf)
	if err != nil {
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
		fmt.Fprintln(c.Err, "no gc candidates found in repo pool")
		return exitError
	}

	selected, err := selectRepoGCCandidates(c, selectable, args)
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

	printRepoGCSelection(c.Out, eligibleSelected)
	fmt.Fprintln(c.Out)
	fmt.Fprintln(c.Out, renderRiskTitle(writerSupportsColor(c.Out)))
	fmt.Fprintf(c.Out, "%srepo gc permanently deletes bare repos from pool.\n", uiIndent)
	fmt.Fprintf(c.Out, "%sselected: %d\n", uiIndent, len(eligibleSelected))

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

	removed, failed := applyRepoGC(eligibleSelected)
	useColor := writerSupportsColor(c.Out)
	fmt.Fprintln(c.Out)
	fmt.Fprintln(c.Out, renderResultTitle(useColor))
	summary := fmt.Sprintf("Removed %d / %d", len(removed), len(eligibleSelected))
	if useColor {
		switch {
		case len(removed) == len(eligibleSelected):
			summary = styleSuccess(summary, true)
		case len(removed) == 0:
			summary = styleError(summary, true)
		default:
			summary = styleWarn(summary, true)
		}
	}
	fmt.Fprintf(c.Out, "%s%s\n", uiIndent, summary)
	for _, it := range removed {
		prefix := "✔"
		if useColor {
			prefix = styleSuccess(prefix, true)
		}
		fmt.Fprintf(c.Out, "%s%s %s\n", uiIndent, prefix, it.RepoKey)
	}
	for _, it := range failed {
		prefix := "!"
		if useColor {
			prefix = styleError(prefix, true)
		}
		fmt.Fprintf(c.Out, "%s%s %s (%s)\n", uiIndent, prefix, it.RepoKey, it.SkipReason)
	}
	if len(failed) > 0 {
		return exitError
	}
	return exitOK
}

func buildRepoGCCandidates(ctx context.Context, root string, currentDB *sql.DB, currentDBPath string, repoPoolPath string, debugf func(string, ...any)) ([]repoGCCandidate, error) {
	poolRepos, err := listRepoPoolBareRepos(repoPoolPath)
	if err != nil {
		return nil, err
	}
	if len(poolRepos) == 0 {
		return nil, nil
	}

	currentRepoUIDs, err := statestore.ListRepoUIDs(ctx, currentDB)
	if err != nil {
		return nil, fmt.Errorf("list current root repo_uids: %w", err)
	}
	currentWorkspaceRepoUIDs, err := statestore.ListWorkspaceRepoUIDs(ctx, currentDB)
	if err != nil {
		return nil, fmt.Errorf("list current workspace repo_uids: %w", err)
	}
	currentRepoSet := toSet(currentRepoUIDs)
	currentWorkspaceSet := toSet(currentWorkspaceRepoUIDs)

	globalRefCount, err := collectRegistryRepoRefCounts(ctx, currentDBPath)
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
		cand.CurrentWorkspaceBound = currentWorkspaceSet[repoUID]

		refCount := globalRefCount[repoUID]
		if cand.CurrentRootRegistered && refCount > 0 {
			refCount--
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

func collectRegistryRepoRefCounts(ctx context.Context, currentDBPath string) (map[string]int, error) {
	registryPath, err := stateregistry.Path()
	if err != nil {
		return nil, fmt.Errorf("resolve state registry path: %w", err)
	}
	entries, err := stateregistry.Load(registryPath)
	if err != nil {
		return nil, err
	}
	seenDBPath := map[string]bool{}
	counts := map[string]int{}

	addDB := func(path string) error {
		path = strings.TrimSpace(path)
		if path == "" || seenDBPath[path] {
			return nil
		}
		seenDBPath[path] = true

		if _, statErr := os.Stat(path); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("stat state db %s: %w", path, statErr)
		}
		db, err := statestore.Open(ctx, path)
		if err != nil {
			return fmt.Errorf("open state store %s: %w", path, err)
		}
		defer func() { _ = db.Close() }()
		uids, err := statestore.ListRepoUIDs(ctx, db)
		if err != nil {
			return fmt.Errorf("list repo_uids from %s: %w", path, err)
		}
		for _, uid := range uids {
			counts[uid]++
		}
		return nil
	}

	for _, e := range entries {
		if err := addDB(e.StateDBPath); err != nil {
			return nil, err
		}
	}
	if err := addDB(currentDBPath); err != nil {
		return nil, err
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
			ID:          it.RepoKey,
			Description: "",
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

func printRepoGCSelection(out io.Writer, selected []repoGCCandidate) {
	fmt.Fprintln(out, "Repo pool:")
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
