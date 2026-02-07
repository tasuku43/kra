package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

func (c *CLI) runWSPurge(args []string) int {
	var noPrompt bool
	var force bool
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSPurgeUsage(c.Out)
			return exitOK
		case "--no-prompt":
			noPrompt = true
			args = args[1:]
		case "--force":
			force = true
			args = args[1:]
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws purge: %q\n", args[0])
			c.printWSPurgeUsage(c.Err)
			return exitUsage
		}
	}

	if len(args) == 0 {
		c.printWSPurgeUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws purge: %q\n", strings.Join(args[1:], " "))
		c.printWSPurgeUsage(c.Err)
		return exitUsage
	}
	if noPrompt && !force {
		fmt.Fprintln(c.Err, "--no-prompt requires --force for ws purge")
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
	if err := c.ensureDebugLog(root, "ws-purge"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws purge id=%s noPrompt=%t force=%t", workspaceID, noPrompt, force)

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
	if err := ensureNoStagedChanges(ctx, root); err != nil {
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}

	status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "load workspace: %v\n", err)
		return exitError
	}
	if !ok {
		fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		return exitError
	}
	if status != "active" && status != "archived" {
		fmt.Fprintf(c.Err, "workspace cannot be purged (status=%s): %s\n", status, workspaceID)
		return exitError
	}

	if !noPrompt {
		ok, err := c.confirmContinue(fmt.Sprintf("purge workspace %s? this is permanent (y/N): ", workspaceID))
		if err != nil {
			fmt.Fprintf(c.Err, "read confirmation: %v\n", err)
			return exitError
		}
		if !ok {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
	}

	repos, err := statestore.ListWorkspaceRepos(ctx, db, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "list workspace repos: %v\n", err)
		return exitError
	}

	if status == "active" {
		risk, perRepo := inspectWorkspaceRepoRisk(ctx, root, workspaceID, repos)
		if risk != workspacerisk.WorkspaceRiskClean && !noPrompt {
			fmt.Fprintf(c.Err, "workspace risk=%s: %s\n", risk, workspaceID)
			for _, it := range perRepo {
				fmt.Fprintf(c.Err, "- %s\t%s\n", it.alias, it.state)
			}

			ok, err := c.confirmContinue("workspace has risk; continue purging? (y/N): ")
			if err != nil {
				fmt.Fprintf(c.Err, "read confirmation: %v\n", err)
				return exitError
			}
			if !ok {
				fmt.Fprintln(c.Err, "aborted")
				return exitError
			}
		}
	}

	if err := removeWorkspaceWorktrees(ctx, db, root, repoPoolPath, workspaceID, repos); err != nil {
		fmt.Fprintf(c.Err, "remove worktrees: %v\n", err)
		return exitError
	}

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := os.RemoveAll(wsPath); err != nil {
		fmt.Fprintf(c.Err, "delete workspace dir: %v\n", err)
		return exitError
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	if err := os.RemoveAll(archivePath); err != nil {
		fmt.Fprintf(c.Err, "delete archive dir: %v\n", err)
		return exitError
	}

	sha, err := commitPurgeChange(ctx, root, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "commit purge change: %v\n", err)
		return exitError
	}

	now := time.Now().Unix()
	if err := statestore.PurgeWorkspace(ctx, db, statestore.PurgeWorkspaceInput{
		ID:  workspaceID,
		Now: now,
	}); err != nil {
		fmt.Fprintf(c.Err, "update state store: %v\n", err)
		return exitError
	}

	fmt.Fprintf(c.Out, "purged: %s (%s)\n", workspaceID, sha)
	c.debugf("ws purge completed id=%s commit=%s", workspaceID, sha)
	return exitOK
}

func commitPurgeChange(ctx context.Context, root string, workspaceID string) (string, error) {
	archivePrefix := filepath.Join("archive", workspaceID) + string(filepath.Separator)
	workspacesPrefix := filepath.Join("workspaces", workspaceID) + string(filepath.Separator)

	archiveArg := filepath.ToSlash(filepath.Join("archive", workspaceID))
	workspacesArg := filepath.ToSlash(filepath.Join("workspaces", workspaceID))

	if _, err := gitutil.Run(ctx, root, "add", "-A", "--", archiveArg); err != nil {
		if !strings.Contains(err.Error(), "did not match any files") && !strings.Contains(err.Error(), "did not match any file") {
			return "", err
		}
	}
	if _, err := gitutil.Run(ctx, root, "add", "-A", "--", workspacesArg); err != nil {
		if !strings.Contains(err.Error(), "did not match any files") && !strings.Contains(err.Error(), "did not match any file") {
			_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
			return "", err
		}
	}

	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only")
	if err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg)
		return "", err
	}
	staged := strings.Fields(out)
	for _, p := range staged {
		p = filepath.Clean(filepath.FromSlash(p))
		if strings.HasPrefix(p, archivePrefix) || strings.HasPrefix(p, workspacesPrefix) {
			continue
		}
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg)
		return "", fmt.Errorf("unexpected staged path outside allowlist: %s", p)
	}

	if _, err := gitutil.Run(ctx, root, "commit", "--allow-empty", "-m", fmt.Sprintf("purge: %s", workspaceID)); err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg)
		return "", err
	}

	sha, err := gitutil.Run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}
