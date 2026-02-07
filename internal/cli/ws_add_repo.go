package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

type addRepoPoolCandidate struct {
	RepoUID   string
	RepoKey   string
	RemoteURL string
	Alias     string
	BarePath  string
}

type addRepoPlanItem struct {
	Candidate addRepoPoolCandidate

	BaseRefInput   string
	DefaultBaseRef string
	BaseRefUsed    string
	Branch         string
	WorktreePath   string

	LocalBranchExists  bool
	RemoteBranchExists bool
}

type addRepoAppliedItem struct {
	Plan addRepoPlanItem

	CreatedLocalBranch bool
	CreatedWorktree    bool
	AddedBinding       bool
}

func (c *CLI) runWSAddRepo(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSAddRepoUsage(c.Out)
			return exitOK
		}
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws add-repo: %q\n", strings.Join(args[1:], " "))
		c.printWSAddRepoUsage(c.Err)
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
	if err := c.ensureDebugLog(root, "ws-add-repo"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}

	workspaceID, err := resolveWorkspaceIDForAddRepo(root, wd, args)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}
	c.debugf("run ws add-repo workspace=%s cwd=%s", workspaceID, wd)

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

	if status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, workspaceID); err != nil {
		fmt.Fprintf(c.Err, "load workspace: %v\n", err)
		return exitError
	} else if !ok {
		fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		return exitError
	} else if status != "active" {
		fmt.Fprintf(c.Err, "workspace is not active (status=%s): %s\n", status, workspaceID)
		return exitError
	}

	releaseLock, err := acquireWorkspaceAddRepoLock(root, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	defer releaseLock()

	candidates, err := listAddRepoPoolCandidates(ctx, db, repoPoolPath, workspaceID, time.Now(), c.debugf)
	if err != nil {
		fmt.Fprintf(c.Err, "list repo pool candidates: %v\n", err)
		return exitError
	}
	if len(candidates) == 0 {
		fmt.Fprintln(c.Err, "no repos available in pool for this workspace")
		return exitError
	}

	selected, err := c.promptAddRepoPoolSelection(candidates)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "select repos from pool: %v\n", err)
		return exitError
	}
	if len(selected) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	plan := make([]addRepoPlanItem, 0, len(selected))
	for _, cand := range selected {
		defaultBaseRef, err := detectDefaultBaseRefFromBare(ctx, cand.BarePath)
		if err != nil {
			fmt.Fprintf(c.Err, "detect default base_ref for %s: %v\n", cand.RepoKey, err)
			return exitError
		}
		baseRefInput, err := c.promptLine(fmt.Sprintf("base_ref for %s (default: %s, empty=use default): ", cand.RepoKey, defaultBaseRef))
		if err != nil {
			fmt.Fprintf(c.Err, "read base_ref: %v\n", err)
			return exitError
		}
		baseRefInput = strings.TrimSpace(baseRefInput)
		if baseRefInput != "" && !strings.HasPrefix(baseRefInput, "origin/") {
			fmt.Fprintf(c.Err, "invalid base_ref (must be origin/<branch>): %q\n", baseRefInput)
			return exitError
		}

		branchInput, err := c.promptLine(fmt.Sprintf("branch for %s (prefill: %s): ", cand.RepoKey, workspaceID+"/"))
		if err != nil {
			fmt.Fprintf(c.Err, "read branch: %v\n", err)
			return exitError
		}
		branch := strings.TrimSpace(branchInput)
		if branch == "" {
			branch = workspaceID + "/"
		}
		if err := gitutil.CheckRefFormat(ctx, "refs/heads/"+branch); err != nil {
			fmt.Fprintf(c.Err, "invalid branch name for %s: %v\n", cand.RepoKey, err)
			return exitError
		}

		plan = append(plan, addRepoPlanItem{
			Candidate:      cand,
			BaseRefInput:   baseRefInput,
			DefaultBaseRef: defaultBaseRef,
			Branch:         branch,
		})
	}

	if err := preflightAddRepoPlan(ctx, db, root, workspaceID, plan); err != nil {
		fmt.Fprintf(c.Err, "preflight add-repo: %v\n", err)
		return exitError
	}

	printAddRepoPlan(c.Out, workspaceID, plan)
	line, err := c.promptLine(fmt.Sprintf("%sadd selected repos to workspace? [Enter=yes / n=no]: ", uiIndent))
	if err != nil {
		fmt.Fprintf(c.Err, "read confirmation: %v\n", err)
		return exitError
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
	default:
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	applied, err := applyAddRepoPlanAllOrNothing(ctx, db, workspaceID, plan, c.debugf)
	if err != nil {
		fmt.Fprintf(c.Err, "apply add-repo: %v\n", err)
		return exitError
	}

	now := time.Now()
	day := localDayKey(now)
	nowUnix := now.Unix()
	for _, it := range applied {
		if err := statestore.TouchRepoUpdatedAt(ctx, db, it.Plan.Candidate.RepoUID, nowUnix); err != nil {
			fmt.Fprintf(c.Err, "touch repo updated_at: %v\n", err)
			return exitError
		}
		if err := statestore.IncrementRepoUsageDaily(ctx, db, it.Plan.Candidate.RepoUID, day, nowUnix); err != nil {
			fmt.Fprintf(c.Err, "update repo usage: %v\n", err)
			return exitError
		}
	}

	printAddRepoResult(c.Out, applied)
	c.debugf("ws add-repo completed workspace=%s added=%d", workspaceID, len(applied))
	return exitOK
}

func resolveWorkspaceIDForAddRepo(root string, cwd string, args []string) (string, error) {
	if len(args) == 1 {
		workspaceID := strings.TrimSpace(args[0])
		if err := validateWorkspaceID(workspaceID); err != nil {
			return "", fmt.Errorf("invalid workspace id: %w", err)
		}
		return workspaceID, nil
	}

	wsRoot := filepath.Join(root, "workspaces")
	rel, err := filepath.Rel(wsRoot, cwd)
	if err != nil {
		return "", fmt.Errorf("workspace id is required")
	}
	rel = filepath.Clean(rel)
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("workspace id is required when not inside GIONX_ROOT/workspaces/<id>")
	}
	first := strings.Split(rel, string(filepath.Separator))[0]
	if err := validateWorkspaceID(first); err != nil {
		return "", fmt.Errorf("workspace id is required when not inside GIONX_ROOT/workspaces/<id>")
	}
	return first, nil
}

func acquireWorkspaceAddRepoLock(root string, workspaceID string) (func(), error) {
	lockDir := filepath.Join(root, ".gionx", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	lockPath := filepath.Join(lockDir, fmt.Sprintf("ws-add-repo-%s.lock", workspaceID))
	f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("workspace is locked by another add-repo operation: %s", workspaceID)
		}
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		_ = f.Close()
		_ = os.Remove(lockPath)
	}, nil
}

func listAddRepoPoolCandidates(ctx context.Context, db *sql.DB, repoPoolPath string, workspaceID string, now time.Time, debugf func(string, ...any)) ([]addRepoPoolCandidate, error) {
	startDay := localDayKey(now.AddDate(0, 0, -29))
	baseCandidates, err := statestore.ListRepoPoolCandidates(ctx, db, startDay)
	if err != nil {
		return nil, err
	}

	bound, err := statestore.ListWorkspaceRepos(ctx, db, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace repos: %w", err)
	}
	boundRepoUID := make(map[string]bool, len(bound))
	for _, r := range bound {
		boundRepoUID[r.RepoUID] = true
	}

	out := make([]addRepoPoolCandidate, 0, len(baseCandidates))
	for _, it := range baseCandidates {
		if boundRepoUID[it.RepoUID] {
			continue
		}
		spec, err := repospec.Normalize(it.RemoteURL)
		if err != nil {
			if debugf != nil {
				debugf("skip repo candidate: normalize failed repo_uid=%s err=%v", it.RepoUID, err)
			}
			continue
		}
		barePath := repostore.StorePath(repoPoolPath, spec)
		if fi, err := os.Stat(barePath); err != nil || !fi.IsDir() {
			if debugf != nil {
				debugf("skip repo candidate: bare not found repo_uid=%s path=%s", it.RepoUID, barePath)
			}
			continue
		}
		out = append(out, addRepoPoolCandidate{
			RepoUID:   it.RepoUID,
			RepoKey:   it.RepoKey,
			RemoteURL: it.RemoteURL,
			Alias:     deriveAliasFromRepoKey(it.RepoKey),
			BarePath:  barePath,
		})
	}
	return out, nil
}

func deriveAliasFromRepoKey(repoKey string) string {
	parts := strings.Split(strings.TrimSpace(repoKey), "/")
	if len(parts) == 0 {
		return strings.TrimSpace(repoKey)
	}
	return parts[len(parts)-1]
}

func (c *CLI) promptAddRepoPoolSelection(candidates []addRepoPoolCandidate) ([]addRepoPoolCandidate, error) {
	if len(candidates) == 0 {
		return nil, errSelectorCanceled
	}

	filter := ""
	for {
		visible := filterAddRepoPoolCandidates(candidates, filter)
		fmt.Fprintln(c.Err, "Repos(pool):")
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
		selected := make([]addRepoPoolCandidate, 0, len(indices))
		for _, idx := range indices {
			selected = append(selected, visible[idx])
		}
		return selected, nil
	}
}

func filterAddRepoPoolCandidates(candidates []addRepoPoolCandidate, filter string) []addRepoPoolCandidate {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return slices.Clone(candidates)
	}
	out := make([]addRepoPoolCandidate, 0, len(candidates))
	for _, it := range candidates {
		if strings.Contains(strings.ToLower(it.RepoKey), filter) {
			out = append(out, it)
		}
	}
	return out
}

func parseMultiSelectIndices(input string, max int) ([]int, error) {
	if max <= 0 {
		return nil, fmt.Errorf("no selectable items")
	}
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty selection")
	}

	seen := map[int]bool{}
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return nil, fmt.Errorf("not a number: %q", f)
		}
		if v < 1 || v > max {
			return nil, fmt.Errorf("out of range: %d (1-%d)", v, max)
		}
		idx := v - 1
		if seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, idx)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty selection")
	}
	return out, nil
}

func detectDefaultBaseRefFromBare(ctx context.Context, barePath string) (string, error) {
	if ref, err := gitutil.RunBare(ctx, barePath, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref = strings.TrimSpace(ref)
		const prefix = "refs/remotes/origin/"
		if strings.HasPrefix(ref, prefix) {
			branch := strings.TrimSpace(strings.TrimPrefix(ref, prefix))
			if branch != "" {
				return "origin/" + branch, nil
			}
		}
	}
	okMain, err := gitutil.ShowRefExistsBare(ctx, barePath, "refs/remotes/origin/main")
	if err != nil {
		return "", err
	}
	if okMain {
		return "origin/main", nil
	}
	okMaster, err := gitutil.ShowRefExistsBare(ctx, barePath, "refs/remotes/origin/master")
	if err != nil {
		return "", err
	}
	if okMaster {
		return "origin/master", nil
	}
	return "", fmt.Errorf("failed to detect default base_ref in bare repo")
}

func preflightAddRepoPlan(ctx context.Context, db *sql.DB, root string, workspaceID string, plan []addRepoPlanItem) error {
	if len(plan) == 0 {
		return fmt.Errorf("no repo selected")
	}

	existingRepos, err := statestore.ListWorkspaceRepos(ctx, db, workspaceID)
	if err != nil {
		return fmt.Errorf("list workspace repos: %w", err)
	}
	aliasTaken := make(map[string]bool, len(existingRepos)+len(plan))
	for _, r := range existingRepos {
		aliasTaken[r.Alias] = true
	}

	for i := range plan {
		p := &plan[i]
		if p.Candidate.Alias == "" {
			return fmt.Errorf("alias is required: %s", p.Candidate.RepoKey)
		}
		if aliasTaken[p.Candidate.Alias] {
			return fmt.Errorf("repo alias conflict: %s", p.Candidate.Alias)
		}
		aliasTaken[p.Candidate.Alias] = true

		p.WorktreePath = filepath.Join(root, "workspaces", workspaceID, "repos", p.Candidate.Alias)
		if _, err := os.Stat(p.WorktreePath); err == nil {
			return fmt.Errorf("worktree path already exists: %s", p.WorktreePath)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat worktree path: %w", err)
		}

		p.BaseRefUsed = strings.TrimSpace(p.BaseRefInput)
		if p.BaseRefUsed == "" {
			p.BaseRefUsed = strings.TrimSpace(p.DefaultBaseRef)
		}
		if p.BaseRefUsed == "" {
			return fmt.Errorf("base_ref is required for %s", p.Candidate.RepoKey)
		}
		if !strings.HasPrefix(p.BaseRefUsed, "origin/") {
			return fmt.Errorf("invalid base_ref (must be origin/<branch>): %q", p.BaseRefUsed)
		}
		baseRef := "refs/remotes/" + p.BaseRefUsed
		ok, err := gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, baseRef)
		if err != nil {
			return fmt.Errorf("check base_ref %s: %w", p.BaseRefUsed, err)
		}
		if !ok {
			return fmt.Errorf("base_ref not found: %s", p.BaseRefUsed)
		}

		localRef := "refs/heads/" + p.Branch
		p.LocalBranchExists, err = gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, localRef)
		if err != nil {
			return fmt.Errorf("check local branch for %s: %w", p.Candidate.RepoKey, err)
		}

		remoteRef := "refs/remotes/origin/" + p.Branch
		p.RemoteBranchExists, err = gitutil.ShowRefExistsBare(ctx, p.Candidate.BarePath, remoteRef)
		if err != nil {
			return fmt.Errorf("check remote branch for %s: %w", p.Candidate.RepoKey, err)
		}

		if p.LocalBranchExists {
			inUse, err := isBranchCheckedOutInBare(ctx, p.Candidate.BarePath, p.Branch)
			if err != nil {
				return fmt.Errorf("check branch checkout status for %s: %w", p.Candidate.RepoKey, err)
			}
			if inUse {
				return fmt.Errorf("branch is already checked out by another worktree: %s", p.Branch)
			}
		}
	}
	return nil
}

func isBranchCheckedOutInBare(ctx context.Context, barePath string, branch string) (bool, error) {
	out, err := gitutil.RunBare(ctx, barePath, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	target := "refs/heads/" + strings.TrimSpace(branch)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "branch ") {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "branch ")) == target {
			return true, nil
		}
	}
	return false, nil
}

func printAddRepoPlan(out io.Writer, workspaceID string, plan []addRepoPlanItem) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Plan:")
	fmt.Fprintln(out)
	for _, p := range plan {
		fmt.Fprintf(out, "%s- %s\n", uiIndent, p.Candidate.RepoKey)
		fmt.Fprintf(out, "%s  alias: %s\n", uiIndent, p.Candidate.Alias)
		fmt.Fprintf(out, "%s  base_ref: %s\n", uiIndent, p.BaseRefUsed)
		fmt.Fprintf(out, "%s  branch: %s\n", uiIndent, p.Branch)
		fmt.Fprintf(out, "%s  target: %s\n", uiIndent, p.WorktreePath)
	}
	fmt.Fprintf(out, "%sworkspace: %s\n", uiIndent, workspaceID)
}

func applyAddRepoPlanAllOrNothing(ctx context.Context, db *sql.DB, workspaceID string, plan []addRepoPlanItem, debugf func(string, ...any)) ([]addRepoAppliedItem, error) {
	applied := make([]addRepoAppliedItem, 0, len(plan))
	now := time.Now().Unix()

	for _, p := range plan {
		current := addRepoAppliedItem{Plan: p}

		if !p.LocalBranchExists {
			if p.RemoteBranchExists {
				if _, err := gitutil.RunBare(ctx, p.Candidate.BarePath, "branch", "--track", p.Branch, "origin/"+p.Branch); err != nil {
					rollbackAddRepoApplied(ctx, db, workspaceID, applied, debugf)
					return nil, fmt.Errorf("create tracking branch for %s: %w", p.Candidate.RepoKey, err)
				}
			} else {
				if _, err := gitutil.RunBare(ctx, p.Candidate.BarePath, "branch", p.Branch, p.BaseRefUsed); err != nil {
					rollbackAddRepoApplied(ctx, db, workspaceID, applied, debugf)
					return nil, fmt.Errorf("create branch from base_ref for %s: %w", p.Candidate.RepoKey, err)
				}
			}
			current.CreatedLocalBranch = true
		}

		if _, err := gitutil.RunBare(ctx, p.Candidate.BarePath, "worktree", "add", p.WorktreePath, p.Branch); err != nil {
			rollbackAddRepoApplied(ctx, db, workspaceID, append(applied, current), debugf)
			return nil, fmt.Errorf("create worktree for %s: %w", p.Candidate.RepoKey, err)
		}
		current.CreatedWorktree = true

		if err := statestore.AddWorkspaceRepo(ctx, db, statestore.AddWorkspaceRepoInput{
			WorkspaceID:   workspaceID,
			RepoUID:       p.Candidate.RepoUID,
			RepoKey:       p.Candidate.RepoKey,
			Alias:         p.Candidate.Alias,
			Branch:        p.Branch,
			BaseRef:       p.BaseRefInput,
			RepoSpecInput: p.Candidate.RemoteURL,
			Now:           now,
		}); err != nil {
			rollbackAddRepoApplied(ctx, db, workspaceID, append(applied, current), debugf)
			return nil, fmt.Errorf("record workspace repo for %s: %w", p.Candidate.RepoKey, err)
		}
		current.AddedBinding = true
		applied = append(applied, current)
	}
	return applied, nil
}

func rollbackAddRepoApplied(ctx context.Context, db *sql.DB, workspaceID string, applied []addRepoAppliedItem, debugf func(string, ...any)) {
	for i := len(applied) - 1; i >= 0; i-- {
		it := applied[i]
		if it.AddedBinding {
			_ = statestore.DeleteWorkspaceRepoBinding(ctx, db, workspaceID, it.Plan.Candidate.RepoUID)
		}
		if it.CreatedWorktree {
			_, _ = gitutil.RunBare(ctx, it.Plan.Candidate.BarePath, "worktree", "remove", "--force", it.Plan.WorktreePath)
			_ = os.RemoveAll(it.Plan.WorktreePath)
		}
		if it.CreatedLocalBranch {
			_, _ = gitutil.RunBare(ctx, it.Plan.Candidate.BarePath, "branch", "-D", it.Plan.Branch)
		}
	}
	if debugf != nil {
		debugf("ws add-repo rolled back applied_count=%d", len(applied))
	}
}

func printAddRepoResult(out io.Writer, applied []addRepoAppliedItem) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, renderResultTitle(writerSupportsColor(out)))
	fmt.Fprintf(out, "%sAdded %d / %d\n", uiIndent, len(applied), len(applied))
	for _, it := range applied {
		fmt.Fprintf(out, "%s✔ %s\n", uiIndent, it.Plan.Candidate.RepoKey)
	}
}

func localDayKey(t time.Time) int {
	year, month, day := t.Date()
	return year*10000 + int(month)*100 + day
}
