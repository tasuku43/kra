package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/infra/gitutil"
	"github.com/tasuku43/gionx/internal/infra/paths"
	"github.com/tasuku43/gionx/internal/infra/statestore"
)

var errNoArchivedWorkspaces = errors.New("no archived workspaces available")

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

	if len(args) != 1 {
		if len(args) > 1 {
			fmt.Fprintf(c.Err, "unexpected args for ws reopen: %q\n", strings.Join(args[1:], " "))
		}
		fmt.Fprintln(c.Err, "ws reopen requires <id>; use `gionx ws select --archived` for interactive selection")
		c.printWSReopenUsage(c.Err)
		return exitUsage
	}

	directWorkspaceID := args[0]
	if err := validateWorkspaceID(directWorkspaceID); err != nil {
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
	c.debugf("run ws reopen args=%q", args)

	ctx := context.Background()
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		fmt.Fprintf(c.Err, "resolve repo pool path: %v\n", err)
		return exitError
	}

	var db *sql.DB
	if dbPath, err := paths.StateDBPathForRoot(root); err == nil {
		if opened, err := statestore.Open(ctx, dbPath); err == nil {
			if err := statestore.EnsureSettings(ctx, opened, root, repoPoolPath); err == nil {
				db = opened
				defer func() { _ = db.Close() }()
			} else {
				_ = opened.Close()
				c.debugf("ws reopen: state store unavailable (initialize settings): %v", err)
			}
		} else {
			c.debugf("ws reopen: state store unavailable (open): %v", err)
		}
	} else {
		c.debugf("ws reopen: state store unavailable (resolve path): %v", err)
	}

	if err := ensureRootGitWorktree(ctx, root); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	if err := ensureNoStagedChangesForReopen(ctx, root); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	useColorOut := writerSupportsColor(c.Out)

	flow := workspaceSelectRiskResultFlowConfig{
		FlowName: "ws reopen",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			selected := []workspaceFlowSelection{{ID: directWorkspaceID}}
			c.debugf("ws reopen direct mode selected=%v", workspaceFlowSelectionIDs(selected))
			return selected, nil
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			c.debugf("ws reopen start workspace=%s", item.ID)
			if err := c.reopenWorkspace(ctx, db, root, repoPoolPath, item.ID); err != nil {
				return err
			}
			c.debugf("ws reopen completed workspace=%s", item.ID)
			return nil
		},
		ResultVerb: "Reopened",
		ResultMark: "✔",
	}

	reopened, err := c.runWorkspaceSelectRiskResultFlow(flow, useColorOut)
	if err != nil {
		switch {
		case errors.Is(err, errNoArchivedWorkspaces):
			fmt.Fprintln(c.Err, "no archived workspaces available")
			return exitError
		case errors.Is(err, errSelectorCanceled):
			c.debugf("ws reopen selector canceled")
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		case errors.Is(err, errWorkspaceFlowCanceled):
			c.debugf("ws reopen canceled in flow")
			return exitError
		default:
			fmt.Fprintf(c.Err, "run ws reopen flow: %v\n", err)
			return exitError
		}
	}

	c.debugf("ws reopen completed reopened=%v", reopened)
	return exitOK
}

func (c *CLI) reopenWorkspace(ctx context.Context, db *sql.DB, root string, repoPoolPath string, workspaceID string) error {
	if db != nil {
		if status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, workspaceID); err != nil {
			return fmt.Errorf("load workspace: %w", err)
		} else if !ok {
			return fmt.Errorf("workspace not found: %s", workspaceID)
		} else if status != "archived" {
			return fmt.Errorf("workspace is not archived (status=%s): %s", status, workspaceID)
		}
	}

	archivePath := filepath.Join(root, "archive", workspaceID)
	if fi, err := os.Stat(archivePath); err != nil {
		return fmt.Errorf("stat archive dir: %w", err)
	} else if !fi.IsDir() {
		return fmt.Errorf("archive path is not a directory: %s", archivePath)
	}

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if _, err := os.Stat(wsPath); err == nil {
		return fmt.Errorf("workspace directory already exists: %s", wsPath)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat workspace dir: %w", err)
	}

	if err := os.Rename(archivePath, wsPath); err != nil {
		return fmt.Errorf("restore workspace (rename): %w", err)
	}
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		_ = os.Rename(wsPath, archivePath)
		return fmt.Errorf("load %s: %w", workspaceMetaFilename, err)
	}

	if err := recreateWorkspaceWorktreesFromMeta(ctx, root, repoPoolPath, workspaceID, meta.ReposRestore); err != nil {
		_ = os.Rename(wsPath, archivePath)
		return fmt.Errorf("recreate worktrees: %w", err)
	}
	originalMeta := meta
	meta.Workspace.Status = "active"
	meta.Workspace.UpdatedAt = time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		_ = os.Rename(wsPath, archivePath)
		return fmt.Errorf("update %s: %w", workspaceMetaFilename, err)
	}

	sha, err := commitReopenChange(ctx, root, workspaceID)
	if err != nil {
		_ = writeWorkspaceMetaFile(wsPath, originalMeta)
		_ = os.Rename(wsPath, archivePath)
		return fmt.Errorf("commit reopen change: %w", err)
	}

	if db != nil {
		now := time.Now().Unix()
		if err := statestore.ReopenWorkspace(ctx, db, statestore.ReopenWorkspaceInput{
			ID:                workspaceID,
			ReopenedCommitSHA: sha,
			Now:               now,
		}); err != nil {
			return fmt.Errorf("update state store: %w", err)
		}
	}

	return nil
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

func recreateWorkspaceWorktreesFromMeta(ctx context.Context, root string, repoPoolPath string, workspaceID string, repos []workspaceMetaRepoRestore) error {
	reposDir := filepath.Join(root, "workspaces", workspaceID, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		return err
	}

	aliasSeen := make(map[string]bool, len(repos))
	for _, r := range repos {
		if strings.TrimSpace(r.Alias) == "" {
			return fmt.Errorf("invalid repos_restore entry: alias is required")
		}
		if aliasSeen[r.Alias] {
			return fmt.Errorf("invalid repos_restore entry: duplicate alias %q", r.Alias)
		}
		aliasSeen[r.Alias] = true
		if strings.TrimSpace(r.RepoUID) == "" || strings.TrimSpace(r.RemoteURL) == "" || strings.TrimSpace(r.Branch) == "" {
			return fmt.Errorf("invalid repos_restore entry for alias %q", r.Alias)
		}
		worktreePath := filepath.Join(reposDir, r.Alias)
		if _, err := os.Stat(worktreePath); err == nil {
			return fmt.Errorf("worktree path already exists: %s", worktreePath)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}

		spec, err := repospec.Normalize(r.RemoteURL)
		if err != nil {
			return fmt.Errorf("normalize repo remote url: %w", err)
		}
		barePath := repostore.StorePath(repoPoolPath, spec)

		defaultBaseRef, err := gitutil.EnsureBareRepoFetched(ctx, r.RemoteURL, barePath, baseBranchFromBaseRef(r.BaseRef))
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
