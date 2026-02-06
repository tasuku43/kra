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
	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

func (c *CLI) runWSClose(args []string) int {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSCloseUsage(c.Out)
			return exitOK
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws close: %q\n", args[0])
			c.printWSCloseUsage(c.Err)
			return exitUsage
		}
	}

	if len(args) == 0 {
		c.printWSCloseUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws close: %q\n", strings.Join(args[1:], " "))
		c.printWSCloseUsage(c.Err)
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

	ctx := context.Background()
	dbPath, err := paths.DefaultStateDBPath()
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

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if fi, err := os.Stat(wsPath); err != nil {
		fmt.Fprintf(c.Err, "stat workspace dir: %v\n", err)
		return exitError
	} else if !fi.IsDir() {
		fmt.Fprintf(c.Err, "workspace path is not a directory: %s\n", wsPath)
		return exitError
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	if _, err := os.Stat(archivePath); err == nil {
		fmt.Fprintf(c.Err, "archive directory already exists: %s\n", archivePath)
		return exitError
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(c.Err, "stat archive dir: %v\n", err)
		return exitError
	}

	repos, err := statestore.ListWorkspaceRepos(ctx, db, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "list workspace repos: %v\n", err)
		return exitError
	}

	risk, perRepo := inspectWorkspaceRepoRisk(ctx, root, workspaceID, repos)
	if risk != workspacerisk.WorkspaceRiskClean {
		fmt.Fprintf(c.Err, "workspace risk=%s: %s\n", risk, workspaceID)
		for _, it := range perRepo {
			fmt.Fprintf(c.Err, "- %s\t%s\n", it.alias, it.state)
		}

		ok, err := c.confirmContinue("continue closing? (y/N): ")
		if err != nil {
			fmt.Fprintf(c.Err, "read confirmation: %v\n", err)
			return exitError
		}
		if !ok {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
	}

	if err := removeWorkspaceWorktrees(ctx, db, root, repoPoolPath, workspaceID, repos); err != nil {
		fmt.Fprintf(c.Err, "remove worktrees: %v\n", err)
		return exitError
	}

	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		fmt.Fprintf(c.Err, "ensure archive dir: %v\n", err)
		return exitError
	}
	if err := os.Rename(wsPath, archivePath); err != nil {
		fmt.Fprintf(c.Err, "archive (rename): %v\n", err)
		return exitError
	}

	if sha, err := commitArchiveChange(ctx, root, workspaceID); err != nil {
		_ = os.Rename(archivePath, wsPath)
		fmt.Fprintf(c.Err, "commit archive change: %v\n", err)
		return exitError
	} else {
		now := time.Now().Unix()
		if err := statestore.ArchiveWorkspace(ctx, db, statestore.ArchiveWorkspaceInput{
			ID:                workspaceID,
			ArchivedCommitSHA: sha,
			Now:               now,
		}); err != nil {
			fmt.Fprintf(c.Err, "update state store: %v\n", err)
			return exitError
		}
	}

	fmt.Fprintf(c.Out, "archived: %s\n", workspaceID)
	return exitOK
}

func (c *CLI) confirmContinue(prompt string) (bool, error) {
	line, err := c.promptLine(prompt)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

type repoRiskItem struct {
	alias string
	state workspacerisk.RepoState
}

func inspectWorkspaceRepoRisk(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo) (workspacerisk.WorkspaceRisk, []repoRiskItem) {
	var states []workspacerisk.RepoState
	var items []repoRiskItem
	for _, r := range repos {
		state := workspacerisk.RepoStateUnknown
		if r.MissingAt.Valid {
			state = workspacerisk.RepoStateUnknown
		} else {
			worktreePath := filepath.Join(root, "workspaces", workspaceID, "repos", r.Alias)
			st := inspectGitRepoStatus(ctx, worktreePath)
			state = workspacerisk.ClassifyRepoStatus(st)
		}
		states = append(states, state)
		items = append(items, repoRiskItem{alias: r.Alias, state: state})
	}
	return workspacerisk.Aggregate(states), items
}

func inspectGitRepoStatus(ctx context.Context, dir string) workspacerisk.RepoStatus {
	if _, err := os.Stat(dir); err != nil {
		return workspacerisk.RepoStatus{Error: err}
	}

	if out, err := gitutil.Run(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		if err == nil {
			err = fmt.Errorf("not a git worktree")
		}
		return workspacerisk.RepoStatus{Error: err}
	}

	dirty := false
	if out, err := gitutil.Run(ctx, dir, "status", "--porcelain=v1"); err != nil {
		return workspacerisk.RepoStatus{Error: err}
	} else if strings.TrimSpace(out) != "" {
		dirty = true
	}

	headMissing := false
	if _, err := gitutil.Run(ctx, dir, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		headMissing = true
	}

	detached := false
	if _, err := gitutil.Run(ctx, dir, "symbolic-ref", "-q", "HEAD"); err != nil {
		detached = true
	}

	upstream := ""
	if out, err := gitutil.Run(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		upstream = strings.TrimSpace(out)
	}

	ahead := 0
	behind := 0
	if !detached && !headMissing && strings.TrimSpace(upstream) != "" {
		out, err := gitutil.Run(ctx, dir, "rev-list", "--left-right", "--count", "HEAD...@{u}")
		if err != nil {
			return workspacerisk.RepoStatus{Error: err}
		}
		parts := strings.Fields(out)
		if len(parts) != 2 {
			return workspacerisk.RepoStatus{Error: fmt.Errorf("unexpected rev-list output: %q", out)}
		}
		var parseErr error
		ahead, behind, parseErr = parseAheadBehind(parts[0], parts[1])
		if parseErr != nil {
			return workspacerisk.RepoStatus{Error: parseErr}
		}
	}

	return workspacerisk.RepoStatus{
		Upstream:    upstream,
		AheadCount:  ahead,
		BehindCount: behind,
		Dirty:       dirty,
		Detached:    detached,
		HeadMissing: headMissing,
	}
}

func parseAheadBehind(left string, right string) (ahead int, behind int, err error) {
	var a, b int
	if _, err := fmt.Sscanf(left, "%d", &a); err != nil {
		return 0, 0, fmt.Errorf("parse ahead count: %w", err)
	}
	if _, err := fmt.Sscanf(right, "%d", &b); err != nil {
		return 0, 0, fmt.Errorf("parse behind count: %w", err)
	}
	// HEAD...@{u} with --left-right yields left=ahead, right=behind.
	return a, b, nil
}

func removeWorkspaceWorktrees(ctx context.Context, db *sql.DB, root string, repoPoolPath string, workspaceID string, repos []statestore.WorkspaceRepo) error {
	reposDir := filepath.Join(root, "workspaces", workspaceID, "repos")

	for _, r := range repos {
		worktreePath := filepath.Join(reposDir, r.Alias)
		if _, err := os.Stat(worktreePath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
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

		if _, err := os.Stat(barePath); err == nil {
			_, err := gitutil.RunBare(ctx, barePath, "worktree", "remove", "--force", worktreePath)
			if err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := os.RemoveAll(worktreePath); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	entries, err := os.ReadDir(reposDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(entries) == 0 {
		_ = os.Remove(reposDir)
	}
	return nil
}

func ensureRootGitWorktree(ctx context.Context, root string) error {
	out, err := gitutil.Run(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("GIONX_ROOT must be a git working tree: %w", err)
	}

	got := filepath.Clean(strings.TrimSpace(out))
	want := filepath.Clean(root)

	if gotEval, err := filepath.EvalSymlinks(got); err == nil {
		got = gotEval
	}
	if wantEval, err := filepath.EvalSymlinks(want); err == nil {
		want = wantEval
	}

	if got != want {
		return fmt.Errorf("GIONX_ROOT must be the git toplevel: got=%s want=%s", strings.TrimSpace(out), root)
	}
	return nil
}

func ensureNoStagedChanges(ctx context.Context, root string) error {
	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("git index has staged changes; commit or unstage them before running ws close")
	}
	return nil
}

func commitArchiveChange(ctx context.Context, root string, workspaceID string) (string, error) {
	archivePrefix := filepath.Join("archive", workspaceID) + string(filepath.Separator)
	workspacesPrefix := filepath.Join("workspaces", workspaceID) + string(filepath.Separator)

	archiveArg := filepath.ToSlash(filepath.Join("archive", workspaceID))
	workspacesArg := filepath.ToSlash(filepath.Join("workspaces", workspaceID))

	if _, err := gitutil.Run(ctx, root, "add", "-A", "--", archiveArg); err != nil {
		return "", err
	}
	if _, err := gitutil.Run(ctx, root, "add", "-u", "--", workspacesArg); err != nil {
		// In an uninitialized git history, `workspaces/<id>` may not be tracked at all yet.
		// Still allow archiving so the archive can be committed.
		if !strings.Contains(err.Error(), "did not match any files") && !strings.Contains(err.Error(), "did not match any file") {
			_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
			return "", err
		}
	}

	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only")
	if err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		return "", err
	}

	staged := strings.Fields(out)
	for _, p := range staged {
		p = filepath.Clean(filepath.FromSlash(p))
		if strings.HasPrefix(p, archivePrefix) || strings.HasPrefix(p, workspacesPrefix) {
			continue
		}
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		return "", fmt.Errorf("unexpected staged path outside allowlist: %s", p)
	}

	if _, err := gitutil.Run(ctx, root, "commit", "-m", fmt.Sprintf("archive: %s", workspaceID)); err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		return "", err
	}

	sha, err := gitutil.Run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}
