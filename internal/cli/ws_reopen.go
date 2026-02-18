package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/paths"
)

var errNoArchivedWorkspaces = errors.New("no archived workspaces available")

type reopenCommitTrace struct {
	CommitEnabled bool
	PreCommitSHA  string
	PostCommitSHA string
}

func (c *CLI) runWSReopen(args []string) int {
	doCommit := true
	outputFormat := "human"
	dryRun := false
	commitModeExplicit := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSReopenUsage(c.Out)
			return exitOK
		case "--commit":
			if commitModeExplicit == "no-commit" {
				fmt.Fprintln(c.Err, "--commit and --no-commit cannot be used together")
				c.printWSReopenUsage(c.Err)
				return exitUsage
			}
			doCommit = true
			commitModeExplicit = "commit"
			args = args[1:]
		case "--no-commit":
			if commitModeExplicit == "commit" {
				fmt.Fprintln(c.Err, "--commit and --no-commit cannot be used together")
				c.printWSReopenUsage(c.Err)
				return exitUsage
			}
			doCommit = false
			commitModeExplicit = "no-commit"
			args = args[1:]
		case "--dry-run":
			dryRun = true
			args = args[1:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSReopenUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws reopen: %q\n", args[0])
			c.printWSReopenUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSReopenUsage(c.Err)
		return exitUsage
	}
	if dryRun && outputFormat != "json" {
		fmt.Fprintln(c.Err, "--dry-run requires --format json")
		c.printWSReopenUsage(c.Err)
		return exitUsage
	}

	if len(args) != 1 {
		if len(args) > 1 {
			fmt.Fprintf(c.Err, "unexpected args for ws reopen: %q\n", strings.Join(args[1:], " "))
		}
		fmt.Fprintln(c.Err, "ws reopen requires <id>; use `kra ws select --archived` for interactive selection")
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
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-reopen"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws reopen args=%q", args)
	if outputFormat == "json" {
		return c.runWSReopenJSON(root, directWorkspaceID, doCommit, dryRun)
	}

	ctx := context.Background()
	repoPoolPath, err := paths.DefaultRepoPoolPath()
	if err != nil {
		fmt.Fprintf(c.Err, "resolve repo pool path: %v\n", err)
		return exitError
	}

	if doCommit {
		if err := ensureRootGitWorktree(ctx, root); err != nil {
			fmt.Fprintf(c.Err, "%v\n", err)
			return exitError
		}
	}
	useColorOut := writerSupportsColor(c.Out)
	reopenTraces := make(map[string]reopenCommitTrace, 1)

	flow := workspaceSelectRiskResultFlowConfig{
		FlowName: "ws reopen",
		PrintResult: func(done []string, total int, useColor bool) {
			c.printWSReopenFlowResult(done, total, useColor, reopenTraces)
		},
		SelectItems: func() ([]workspaceFlowSelection, error) {
			selected := []workspaceFlowSelection{{ID: directWorkspaceID}}
			c.debugf("ws reopen direct mode selected=%v", workspaceFlowSelectionIDs(selected))
			return selected, nil
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			c.debugf("ws reopen start workspace=%s", item.ID)
			trace, err := c.reopenWorkspace(ctx, root, repoPoolPath, item.ID, doCommit)
			if err != nil {
				return err
			}
			reopenTraces[item.ID] = trace
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

func (c *CLI) runWSReopenJSON(root string, workspaceID string, doCommit bool, dryRun bool) int {
	if !dryRun {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "reopen",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "invalid_argument",
				Message: "--format json currently supports --dry-run only for ws reopen",
			},
		})
		return exitUsage
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	workspacePath := filepath.Join(root, "workspaces", workspaceID)
	checks := make([]map[string]any, 0, 3)
	executable := true

	if fi, err := os.Stat(archivePath); err != nil || !fi.IsDir() {
		checks = append(checks, map[string]any{"name": "archive_exists", "status": "fail", "message": "archive workspace not found"})
		executable = false
	} else {
		checks = append(checks, map[string]any{"name": "archive_exists", "status": "pass", "message": "archive workspace exists"})
	}
	if _, err := os.Stat(workspacePath); err == nil {
		checks = append(checks, map[string]any{"name": "workspace_absent", "status": "fail", "message": "active workspace already exists"})
		executable = false
	} else if !os.IsNotExist(err) {
		checks = append(checks, map[string]any{"name": "workspace_absent", "status": "fail", "message": err.Error()})
		executable = false
	} else {
		checks = append(checks, map[string]any{"name": "workspace_absent", "status": "pass", "message": "active workspace path is free"})
	}
	result := map[string]any{
		"executable": executable,
		"checks":     checks,
		"risk": map[string]any{
			"workspace": "clean",
			"repos":     []map[string]any{},
		},
		"planned_effects": []map[string]any{
			{"path": archivePath, "effect": "move_to_workspaces"},
			{"path": workspacePath, "effect": "create"},
		},
		"requires_confirmation": false,
		"requires_force":        false,
		"commit_enabled":        doCommit,
	}
	_ = writeCLIJSON(c.Out, cliJSONResponse{
		OK:          executable,
		Action:      "ws.reopen.dry-run",
		WorkspaceID: workspaceID,
		Result:      result,
	})
	if !executable {
		return exitError
	}
	return exitOK
}

func (c *CLI) reopenWorkspace(ctx context.Context, root string, repoPoolPath string, workspaceID string, doCommit bool) (reopenCommitTrace, error) {
	archivePath := filepath.Join(root, "archive", workspaceID)
	if fi, err := os.Stat(archivePath); err != nil {
		return reopenCommitTrace{}, fmt.Errorf("stat archive dir: %w", err)
	} else if !fi.IsDir() {
		return reopenCommitTrace{}, fmt.Errorf("archive path is not a directory: %s", archivePath)
	}

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if _, err := os.Stat(wsPath); err == nil {
		return reopenCommitTrace{}, fmt.Errorf("workspace directory already exists: %s", wsPath)
	} else if err != nil && !os.IsNotExist(err) {
		return reopenCommitTrace{}, fmt.Errorf("stat workspace dir: %w", err)
	}

	trace := reopenCommitTrace{CommitEnabled: doCommit}
	if doCommit {
		preSHA, err := commitReopenPreSnapshot(ctx, root, workspaceID)
		if err != nil {
			return reopenCommitTrace{}, fmt.Errorf("commit reopen pre-snapshot: %w", err)
		}
		trace.PreCommitSHA = preSHA
	}

	if err := os.Rename(archivePath, wsPath); err != nil {
		return reopenCommitTrace{}, fmt.Errorf("restore workspace (rename): %w", err)
	}
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		_ = os.Rename(wsPath, archivePath)
		return reopenCommitTrace{}, fmt.Errorf("load %s: %w", workspaceMetaFilename, err)
	}

	if err := recreateWorkspaceWorktreesFromMeta(ctx, root, repoPoolPath, workspaceID, meta.ReposRestore); err != nil {
		_ = os.Rename(wsPath, archivePath)
		return reopenCommitTrace{}, fmt.Errorf("recreate worktrees: %w", err)
	}
	meta.Workspace.Status = "active"
	meta.Workspace.UpdatedAt = time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		_ = os.Rename(wsPath, archivePath)
		return reopenCommitTrace{}, fmt.Errorf("update %s: %w", workspaceMetaFilename, err)
	}
	if err := createOrRefreshWorkspaceBaseline(ctx, root, workspaceID, time.Now().Unix()); err != nil {
		c.debugf("reopen workspace baseline refresh failed workspace=%s err=%v", workspaceID, err)
	}

	if doCommit {
		postSHA, err := commitReopenChange(ctx, root, workspaceID)
		if err != nil {
			return reopenCommitTrace{}, fmt.Errorf("commit reopen change: %w", err)
		}
		trace.PostCommitSHA = postSHA
	}

	return trace, nil
}

func (c *CLI) printWSReopenFlowResult(done []string, total int, useColor bool, traces map[string]reopenCommitTrace) {
	body := make([]string, 0, len(done)*5+1)
	body = append(body, fmt.Sprintf("%sReopened %d / %d", uiIndent, len(done), total))
	successMark := styleSuccess("✔", useColor)
	for _, id := range done {
		body = append(body, fmt.Sprintf("%s%s %s", uiIndent, successMark, id))
		trace, ok := traces[id]
		if !ok {
			continue
		}
		if trace.CommitEnabled {
			body = append(body, fmt.Sprintf("%s%s %s reopen-pre: %s %s",
				uiIndent+uiIndent,
				styleMuted("1/3", useColor),
				styleAccent("commit(pre):", useColor),
				id,
				styleMuted(shortCommitSHA(trace.PreCommitSHA), useColor),
			))
		} else {
			body = append(body, fmt.Sprintf("%s%s %s %s",
				uiIndent+uiIndent,
				styleMuted("1/3", useColor),
				styleAccent("commit(pre):", useColor),
				styleMuted("skipped (--no-commit)", useColor),
			))
		}
		body = append(body, fmt.Sprintf("%s%s %s archive/%s -> workspaces/%s",
			uiIndent+uiIndent,
			styleMuted("2/3", useColor),
			styleAccent("reopen:", useColor),
			id,
			id,
		))
		if trace.CommitEnabled {
			body = append(body, fmt.Sprintf("%s%s %s reopen: %s %s",
				uiIndent+uiIndent,
				styleMuted("3/3", useColor),
				styleAccent("commit(post):", useColor),
				id,
				styleMuted(shortCommitSHA(trace.PostCommitSHA), useColor),
			))
		} else {
			body = append(body, fmt.Sprintf("%s%s %s %s",
				uiIndent+uiIndent,
				styleMuted("3/3", useColor),
				styleAccent("commit(post):", useColor),
				styleMuted("skipped (--no-commit)", useColor),
			))
		}
	}
	printSection(c.Out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
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
	workspacesPrefix, err := toGitTopLevelPath(ctx, root, filepath.Join("workspaces", workspaceID))
	if err != nil {
		return "", err
	}
	archivePrefix, err := toGitTopLevelPath(ctx, root, filepath.Join("archive", workspaceID))
	if err != nil {
		return "", err
	}
	workspacesPrefix += string(filepath.Separator)
	archivePrefix += string(filepath.Separator)

	workspacesArg := filepath.ToSlash(filepath.Join("workspaces", workspaceID))
	archiveArg := filepath.ToSlash(filepath.Join("archive", workspaceID))
	baselineArg := filepath.ToSlash(filepath.Join(".kra", "state", workspaceBaselineDirName, workspaceID+".json"))
	workStateArg := filepath.ToSlash(filepath.Join(".kra", "state", workspaceWorkStateCacheFilename))
	baselinePath, err := toGitTopLevelPath(ctx, root, filepath.Join(".kra", "state", workspaceBaselineDirName, workspaceID+".json"))
	if err != nil {
		return "", err
	}
	workStatePath, err := toGitTopLevelPath(ctx, root, filepath.Join(".kra", "state", workspaceWorkStateCacheFilename))
	if err != nil {
		return "", err
	}
	resetArgs := []string{workspacesArg, archiveArg, baselineArg, workStateArg}
	resetStaging := func() {
		cmd := append([]string{"reset", "-q", "--"}, resetArgs...)
		_, _ = gitutil.Run(ctx, root, cmd...)
	}

	if _, err := gitutil.Run(ctx, root, "add", "-A", "--", workspacesArg); err != nil {
		return "", err
	}
	// Use `-A` (not `-u`) so deletions are staged reliably after the source
	// directory was moved away by os.Rename.
	if _, err := gitutil.Run(ctx, root, "add", "-A", "--", archiveArg); err != nil {
		if !strings.Contains(err.Error(), "did not match any files") && !strings.Contains(err.Error(), "did not match any file") {
			resetStaging()
			return "", err
		}
	}
	for _, arg := range []string{baselineArg, workStateArg} {
		if _, err := gitutil.Run(ctx, root, "add", "-A", "--", arg); err != nil {
			if strings.Contains(err.Error(), "did not match any files") || strings.Contains(err.Error(), "did not match any file") {
				continue
			}
			resetStaging()
			return "", err
		}
	}

	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only", "--", workspacesArg, archiveArg, baselineArg, workStateArg)
	if err != nil {
		resetStaging()
		return "", err
	}

	staged := strings.Fields(out)
	hasBaselineStage := false
	hasWorkStateStage := false
	for _, p := range staged {
		p = filepath.Clean(filepath.FromSlash(p))
		if strings.HasPrefix(p, workspacesPrefix) || strings.HasPrefix(p, archivePrefix) {
			continue
		}
		if p == baselinePath {
			hasBaselineStage = true
			continue
		}
		if p == workStatePath {
			hasWorkStateStage = true
			continue
		}
		resetStaging()
		return "", fmt.Errorf("unexpected staged path outside allowlist: %s", p)
	}

	commitArgs := []string{"commit", "--only", "-m", fmt.Sprintf("reopen: %s", workspaceID), "--", workspacesArg, archiveArg}
	if hasBaselineStage {
		commitArgs = append(commitArgs, baselineArg)
	}
	if hasWorkStateStage {
		commitArgs = append(commitArgs, workStateArg)
	}
	if _, err := gitutil.Run(ctx, root, commitArgs...); err != nil {
		resetStaging()
		return "", err
	}
	resetStaging()

	sha, err := gitutil.Run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func commitReopenPreSnapshot(ctx context.Context, root string, workspaceID string) (string, error) {
	archivePrefix, err := toGitTopLevelPath(ctx, root, filepath.Join("archive", workspaceID))
	if err != nil {
		return "", err
	}
	archivePrefix += string(filepath.Separator)
	archiveArg := filepath.ToSlash(filepath.Join("archive", workspaceID))

	if _, err := gitutil.Run(ctx, root, "add", "-A", "--", archiveArg); err != nil {
		return "", err
	}
	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only", "--", archiveArg)
	if err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		return "", err
	}
	for _, p := range strings.Fields(out) {
		p = filepath.Clean(filepath.FromSlash(p))
		if strings.HasPrefix(p, archivePrefix) {
			continue
		}
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		return "", fmt.Errorf("unexpected staged path outside allowlist: %s", p)
	}
	if _, err := gitutil.Run(ctx, root, "commit", "--allow-empty", "--only", "-m", fmt.Sprintf("reopen-pre: %s", workspaceID), "--", archiveArg); err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		return "", err
	}
	_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)

	sha, err := gitutil.Run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}
