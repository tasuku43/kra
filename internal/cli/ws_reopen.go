package cli

import (
	"context"
	"database/sql"
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

func (c *CLI) runWSReopen(args []string) int {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSReopenUsage(c.Out)
			return exitOK
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws reopen: %q\n", args[0])
			c.printWSReopenUsage(c.Err)
			return exitUsage
		}
	}

	if len(args) == 0 {
		c.printWSReopenUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws reopen: %q\n", strings.Join(args[1:], " "))
		c.printWSReopenUsage(c.Err)
		return exitUsage
	}

	workspaceID := args[0]
	if err := validateWorkspaceID(workspaceID); err != nil {
		fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
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
	if err := c.ensureDebugLog(root, "ws-reopen"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws reopen id=%s", workspaceID)

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

	if err := ensureRootGitWorktree(ctx, root); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if err := ensureNoStagedChangesForReopen(ctx, root); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	if status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, workspaceID); err != nil {
		fmt.Fprintf(c.Err, "load workspace: %v\n", err)
		return exitError
	} else if !ok {
		fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		return exitError
	} else if status != "archived" {
		fmt.Fprintf(c.Err, "workspace is not archived (status=%s): %s\n", status, workspaceID)
		return exitError
	}

	archivePath := filepath.Join(root, "archive", workspaceID)
	if fi, err := os.Stat(archivePath); err != nil {
		fmt.Fprintf(c.Err, "stat archive dir: %v\n", err)
		return exitError
	} else if !fi.IsDir() {
		fmt.Fprintf(c.Err, "archive path is not a directory: %s\n", archivePath)
		return exitError
	}

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if _, err := os.Stat(wsPath); err == nil {
		fmt.Fprintf(c.Err, "workspace directory already exists: %s\n", wsPath)
		return exitError
	} else if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(c.Err, "stat workspace dir: %v\n", err)
		return exitError
	}

	repos, err := statestore.ListWorkspaceRepos(ctx, db, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "list workspace repos: %v\n", err)
		return exitError
	}

	if err := os.Rename(archivePath, wsPath); err != nil {
		fmt.Fprintf(c.Err, "restore workspace (rename): %v\n", err)
		return exitError
	}

	if err := recreateWorkspaceWorktrees(ctx, db, root, repoPoolPath, workspaceID, repos); err != nil {
		fmt.Fprintf(c.Err, "recreate worktrees: %v\n", err)
		return exitError
	}

	if sha, err := commitReopenChange(ctx, root, workspaceID); err != nil {
		_ = os.Rename(wsPath, archivePath)
		fmt.Fprintf(c.Err, "commit reopen change: %v\n", err)
		return exitError
	} else {
		now := time.Now().Unix()
		if err := statestore.ReopenWorkspace(ctx, db, statestore.ReopenWorkspaceInput{
			ID:                workspaceID,
			ReopenedCommitSHA: sha,
			Now:               now,
		}); err != nil {
			fmt.Fprintf(c.Err, "update state store: %v\n", err)
			return exitError
		}
	}

	fmt.Fprintf(c.Out, "reopened: %s\n", workspaceID)
	c.debugf("ws reopen completed id=%s", workspaceID)
	return exitOK
}

func ensureNoStagedChangesForReopen(ctx context.Context, root string) error {
	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("git index has staged changes; commit or unstage them before running ws reopen")
	}
	return nil
}

func recreateWorkspaceWorktrees(ctx context.Context, db *sql.DB, root string, repoPoolPath string, workspaceID string, repos []statestore.WorkspaceRepo) error {
	reposDir := filepath.Join(root, "workspaces", workspaceID, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		return err
	}

	for _, r := range repos {
		worktreePath := filepath.Join(reposDir, r.Alias)
		if _, err := os.Stat(worktreePath); err == nil {
			return fmt.Errorf("worktree path already exists: %s", worktreePath)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}

		remoteURL, ok, err := statestore.LookupRepoRemoteURL(ctx, db, r.RepoUID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("repo not found in repos table: %s", r.RepoUID)
		}

		spec, err := repospec.Normalize(remoteURL)
		if err != nil {
			return fmt.Errorf("normalize repo remote url: %w", err)
		}
		barePath := repostore.StorePath(repoPoolPath, spec)

		defaultBaseRef, err := gitutil.EnsureBareRepoFetched(ctx, remoteURL, barePath, baseBranchFromBaseRef(r.BaseRef))
		if err != nil {
			return err
		}

		baseRefUsed := strings.TrimSpace(r.BaseRef)
		if baseRefUsed == "" {
			baseRefUsed = strings.TrimSpace(defaultBaseRef)
		}
		if !strings.HasPrefix(baseRefUsed, "origin/") {
			return fmt.Errorf("invalid base_ref (must be origin/<branch>): %q", baseRefUsed)
		}

		remoteBranchRef := "refs/remotes/origin/" + r.Branch
		remoteExists, err := gitutil.ShowRefExistsBare(ctx, barePath, remoteBranchRef)
		if err != nil {
			return err
		}

		localBranchRef := "refs/heads/" + r.Branch
		localExists, err := gitutil.ShowRefExistsBare(ctx, barePath, localBranchRef)
		if err != nil {
			return err
		}

		if !localExists {
			if remoteExists {
				if _, err := gitutil.RunBare(ctx, barePath, "branch", "--track", r.Branch, "origin/"+r.Branch); err != nil {
					return err
				}
			} else {
				if _, err := gitutil.RunBare(ctx, barePath, "branch", r.Branch, baseRefUsed); err != nil {
					return err
				}
			}
		}

		if _, err := gitutil.RunBare(ctx, barePath, "worktree", "add", worktreePath, r.Branch); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "already checked out") || strings.Contains(msg, "already used by worktree") {
				return fmt.Errorf("branch is already checked out by another worktree: %s", r.Branch)
			}
			return err
		}
	}

	return nil
}

func baseBranchFromBaseRef(baseRef string) string {
	baseRef = strings.TrimSpace(baseRef)
	if !strings.HasPrefix(baseRef, "origin/") {
		return ""
	}
	b := strings.TrimPrefix(baseRef, "origin/")
	if strings.TrimSpace(b) == "" {
		return ""
	}
	return b
}

func commitReopenChange(ctx context.Context, root string, workspaceID string) (string, error) {
	workspacesPrefix := filepath.Join("workspaces", workspaceID) + string(filepath.Separator)
	archivePrefix := filepath.Join("archive", workspaceID) + string(filepath.Separator)

	workspacesArg := filepath.ToSlash(filepath.Join("workspaces", workspaceID))
	archiveArg := filepath.ToSlash(filepath.Join("archive", workspaceID))

	if _, err := gitutil.Run(ctx, root, "add", "-A", "--", workspacesArg); err != nil {
		return "", err
	}
	if _, err := gitutil.Run(ctx, root, "add", "-u", "--", archiveArg); err != nil {
		if !strings.Contains(err.Error(), "did not match any files") && !strings.Contains(err.Error(), "did not match any file") {
			_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg, archiveArg)
			return "", err
		}
	}

	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only")
	if err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg, archiveArg)
		return "", err
	}

	staged := strings.Fields(out)
	for _, p := range staged {
		p = filepath.Clean(filepath.FromSlash(p))
		if strings.HasPrefix(p, workspacesPrefix) || strings.HasPrefix(p, archivePrefix) {
			continue
		}
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg, archiveArg)
		return "", fmt.Errorf("unexpected staged path outside allowlist: %s", p)
	}

	if _, err := gitutil.Run(ctx, root, "commit", "-m", fmt.Sprintf("reopen: %s", workspaceID)); err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg, archiveArg)
		return "", err
	}

	sha, err := gitutil.Run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}
