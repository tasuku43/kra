package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

func (c *CLI) runWSAddRepo(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSAddRepoUsage(c.Out)
			return exitOK
		}
	}
	if len(args) < 2 {
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}
	if len(args) > 2 {
		fmt.Fprintf(c.Err, "unexpected args for ws add-repo: %q\n", strings.Join(args[2:], " "))
		c.printWSAddRepoUsage(c.Err)
		return exitUsage
	}

	workspaceID := args[0]
	if err := validateWorkspaceID(workspaceID); err != nil {
		fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
		return exitUsage
	}

	repoSpecInput := strings.TrimSpace(args[1])
	if repoSpecInput == "" {
		fmt.Fprintf(c.Err, "repo is required\n")
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

	spec, err := repospec.Normalize(repoSpecInput)
	if err != nil {
		fmt.Fprintf(c.Err, "normalize repo spec: %v\n", err)
		return exitError
	}

	repoKey := fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
	repoUID := fmt.Sprintf("%s/%s", spec.Host, repoKey)
	alias := spec.Repo

	defaultBranch, err := gitutil.DefaultBranchFromRemote(ctx, repoSpecInput)
	if err != nil {
		fmt.Fprintf(c.Err, "detect default branch: %v\n", err)
		return exitError
	}
	suggestedDefaultBaseRef := "origin/" + defaultBranch

	barePath := repostore.StorePath(repoPoolPath, spec)

	type ensureResult struct {
		defaultBaseRef string
		err            error
	}
	ensureCh := make(chan ensureResult, 1)
	go func() {
		baseRef, err := gitutil.EnsureBareRepoFetched(ctx, repoSpecInput, barePath, defaultBranch)
		ensureCh <- ensureResult{defaultBaseRef: baseRef, err: err}
	}()

	baseRefInput, err := c.promptLine(fmt.Sprintf("base_ref (default: %s, empty=use default): ", suggestedDefaultBaseRef))
	if err != nil {
		fmt.Fprintf(c.Err, "read base_ref: %v\n", err)
		return exitError
	}
	baseRefInput = strings.TrimSpace(baseRefInput)
	if baseRefInput != "" {
		if !strings.HasPrefix(baseRefInput, "origin/") {
			fmt.Fprintf(c.Err, "invalid base_ref (must be origin/<branch>): %q\n", baseRefInput)
			return exitError
		}
		if err := gitutil.CheckRefFormat(ctx, "refs/remotes/"+baseRefInput); err != nil {
			fmt.Fprintf(c.Err, "invalid base_ref: %v\n", err)
			return exitError
		}
	}

	branchInput, err := c.promptLine(fmt.Sprintf("branch (prefill: %s): ", workspaceID+"/"))
	if err != nil {
		fmt.Fprintf(c.Err, "read branch: %v\n", err)
		return exitError
	}
	branch := strings.TrimSpace(branchInput)
	if branch == "" {
		branch = workspaceID + "/"
	}
	if err := gitutil.CheckRefFormat(ctx, "refs/heads/"+branch); err != nil {
		fmt.Fprintf(c.Err, "invalid branch name: %v\n", err)
		return exitError
	}

	ensureRes := <-ensureCh
	if ensureRes.err != nil {
		fmt.Fprintf(c.Err, "ensure bare repo: %v\n", ensureRes.err)
		return exitError
	}
	defaultBaseRef := strings.TrimSpace(ensureRes.defaultBaseRef)
	if defaultBaseRef == "" {
		defaultBaseRef = suggestedDefaultBaseRef
	}

	baseRefUsed := baseRefInput
	if baseRefUsed == "" {
		baseRefUsed = defaultBaseRef
	}
	baseRefRef := "refs/remotes/" + baseRefUsed
	if ok, err := gitutil.ShowRefExistsBare(ctx, barePath, baseRefRef); err != nil {
		fmt.Fprintf(c.Err, "check base_ref: %v\n", err)
		return exitError
	} else if !ok {
		fmt.Fprintf(c.Err, "base_ref not found: %s\n", baseRefUsed)
		return exitError
	}

	remoteBranchRef := "refs/remotes/origin/" + branch
	remoteExists, err := gitutil.ShowRefExistsBare(ctx, barePath, remoteBranchRef)
	if err != nil {
		fmt.Fprintf(c.Err, "check remote branch: %v\n", err)
		return exitError
	}

	localBranchRef := "refs/heads/" + branch
	localExists, err := gitutil.ShowRefExistsBare(ctx, barePath, localBranchRef)
	if err != nil {
		fmt.Fprintf(c.Err, "check local branch: %v\n", err)
		return exitError
	}

	if !localExists {
		if remoteExists {
			if _, err := gitutil.RunBare(ctx, barePath, "branch", "--track", branch, "origin/"+branch); err != nil {
				fmt.Fprintf(c.Err, "create tracking branch: %v\n", err)
				return exitError
			}
		} else {
			if _, err := gitutil.RunBare(ctx, barePath, "branch", branch, baseRefUsed); err != nil {
				fmt.Fprintf(c.Err, "create branch from base_ref: %v\n", err)
				return exitError
			}
		}
	}

	reposDir := filepath.Join(root, "workspaces", workspaceID, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		fmt.Fprintf(c.Err, "ensure repos dir: %v\n", err)
		return exitError
	}
	worktreePath := filepath.Join(reposDir, alias)
	if _, err := os.Stat(worktreePath); err == nil {
		fmt.Fprintf(c.Err, "worktree path already exists: %s\n", worktreePath)
		return exitError
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(c.Err, "stat worktree path: %v\n", err)
		return exitError
	}

	if _, err := gitutil.RunBare(ctx, barePath, "worktree", "add", worktreePath, branch); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "already checked out") {
			fmt.Fprintf(c.Err, "branch is already checked out by another worktree: %s\n", branch)
			return exitError
		}
		fmt.Fprintf(c.Err, "create worktree: %v\n", err)
		return exitError
	}

	now := time.Now().Unix()
	if err := statestore.EnsureRepo(ctx, db, statestore.EnsureRepoInput{
		RepoUID:   repoUID,
		RepoKey:   repoKey,
		RemoteURL: repoSpecInput,
		Now:       now,
	}); err != nil {
		fmt.Fprintf(c.Err, "record repo: %v\n", err)
		return exitError
	}

	if err := statestore.AddWorkspaceRepo(ctx, db, statestore.AddWorkspaceRepoInput{
		WorkspaceID:   workspaceID,
		RepoUID:       repoUID,
		RepoKey:       repoKey,
		Alias:         alias,
		Branch:        branch,
		BaseRef:       baseRefInput, // empty means "use detected default branch"
		RepoSpecInput: repoSpecInput,
		Now:           now,
	}); err != nil {
		var aliasErr *statestore.WorkspaceRepoAliasConflictError
		if errors.As(err, &aliasErr) {
			fmt.Fprintf(c.Err, "repo alias conflict: %s\n", aliasErr.Alias)
			return exitError
		}
		fmt.Fprintf(c.Err, "record workspace repo: %v\n", err)
		return exitError
	}

	fmt.Fprintf(c.Out, "added: %s\n", worktreePath)
	return exitOK
}
