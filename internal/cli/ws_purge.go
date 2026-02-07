package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

type purgeWorkspaceMeta struct {
	status  string
	risk    workspacerisk.WorkspaceRisk
	perRepo []repoRiskItem
}

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

	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws purge: %q\n", strings.Join(args[1:], " "))
		c.printWSPurgeUsage(c.Err)
		return exitUsage
	}
	if noPrompt && !force {
		fmt.Fprintln(c.Err, "--no-prompt requires --force for ws purge")
		return exitUsage
	}
	if noPrompt && len(args) == 0 {
		fmt.Fprintln(c.Err, "--no-prompt selector mode is not supported; pass <id> with --force")
		return exitUsage
	}

	directWorkspaceID := ""
	if len(args) == 1 {
		directWorkspaceID = args[0]
		if err := validateWorkspaceID(directWorkspaceID); err != nil {
			fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
			return exitUsage
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
	if err := c.ensureDebugLog(root, "ws-purge"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws purge args=%q noPrompt=%t force=%t", args, noPrompt, force)

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
	useColorOut := writerSupportsColor(c.Out)

	selectedIDs := make([]string, 0, 4)
	riskMeta := make(map[string]purgeWorkspaceMeta, 4)
	hasActiveRisk := false

	flow := workspaceSelectRiskResultFlowConfig{
		FlowName: "ws purge",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			if directWorkspaceID != "" {
				selected := []workspaceFlowSelection{{ID: directWorkspaceID}}
				c.debugf("ws purge direct mode selected=%v", workspaceFlowSelectionIDs(selected))
				return selected, nil
			}

			candidates, err := listWorkspaceCandidatesByStatus(ctx, db, "archived")
			if err != nil {
				return nil, fmt.Errorf("list archived workspaces: %w", err)
			}
			if len(candidates) == 0 {
				return nil, errNoArchivedWorkspaces
			}

			ids, err := c.promptWorkspaceSelector("archived", "purge", candidates)
			if err != nil {
				return nil, err
			}
			c.debugf("ws purge selector mode selected=%v", ids)
			selected := make([]workspaceFlowSelection, 0, len(ids))
			for _, id := range ids {
				selected = append(selected, workspaceFlowSelection{ID: id})
			}
			return selected, nil
		},
		CollectRiskStage: func(items []workspaceFlowSelection) (workspaceFlowRiskStage, error) {
			selectedIDs = workspaceFlowSelectionIDs(items)
			riskMeta = make(map[string]purgeWorkspaceMeta, len(items))
			hasActiveRisk = false
			for _, id := range selectedIDs {
				meta, err := collectPurgeWorkspaceMeta(ctx, db, root, id)
				if err != nil {
					return workspaceFlowRiskStage{}, err
				}
				riskMeta[id] = meta
				if meta.status == "active" && meta.risk != workspacerisk.WorkspaceRiskClean {
					hasActiveRisk = true
				}
			}

			if noPrompt {
				return workspaceFlowRiskStage{}, nil
			}

			return workspaceFlowRiskStage{
				HasRisk: true,
				Print: func(useColor bool) {
					printPurgeRiskSection(c.Out, selectedIDs, riskMeta, useColor)
				},
			}, nil
		},
		ConfirmRisk: func() (bool, error) {
			if noPrompt {
				return true, nil
			}
			prompt := fmt.Sprintf("%spurge selected workspaces? this is permanent (y/N): ", uiIndent)
			if len(selectedIDs) == 1 {
				prompt = fmt.Sprintf("%spurge workspace %s? this is permanent (y/N): ", uiIndent, selectedIDs[0])
			}
			ok, err := c.confirmContinue(prompt)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
			if !hasActiveRisk {
				return true, nil
			}
			return c.confirmContinue(fmt.Sprintf("%sworkspace has risk; continue purging? (y/N): ", uiIndent))
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			c.debugf("ws purge start workspace=%s", item.ID)
			if err := c.purgeWorkspace(ctx, db, root, repoPoolPath, item.ID); err != nil {
				return err
			}
			c.debugf("ws purge completed workspace=%s", item.ID)
			return nil
		},
		ResultVerb: "Purged",
		ResultMark: "✔",
	}

	purged, err := c.runWorkspaceSelectRiskResultFlow(flow, useColorOut)
	if err != nil {
		switch {
		case errors.Is(err, errNoArchivedWorkspaces):
			fmt.Fprintln(c.Err, "no archived workspaces available")
			return exitError
		case errors.Is(err, errSelectorCanceled):
			c.debugf("ws purge selector canceled")
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		case errors.Is(err, errWorkspaceFlowCanceled):
			c.debugf("ws purge canceled in flow")
			return exitError
		default:
			fmt.Fprintf(c.Err, "run ws purge flow: %v\n", err)
			return exitError
		}
	}

	c.debugf("ws purge done=%v", purged)
	return exitOK
}

func collectPurgeWorkspaceMeta(ctx context.Context, db *sql.DB, root string, workspaceID string) (purgeWorkspaceMeta, error) {
	status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, workspaceID)
	if err != nil {
		return purgeWorkspaceMeta{}, fmt.Errorf("load workspace: %w", err)
	}
	if !ok {
		return purgeWorkspaceMeta{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	if status != "active" && status != "archived" {
		return purgeWorkspaceMeta{}, fmt.Errorf("workspace cannot be purged (status=%s): %s", status, workspaceID)
	}

	meta := purgeWorkspaceMeta{status: status, risk: workspacerisk.WorkspaceRiskClean}
	if status != "active" {
		return meta, nil
	}

	repos, err := statestore.ListWorkspaceRepos(ctx, db, workspaceID)
	if err != nil {
		return purgeWorkspaceMeta{}, fmt.Errorf("list workspace repos: %w", err)
	}
	risk, perRepo := inspectWorkspaceRepoRisk(ctx, root, workspaceID, repos)
	meta.risk = risk
	meta.perRepo = perRepo
	return meta, nil
}

func printPurgeRiskSection(out io.Writer, selectedIDs []string, riskMeta map[string]purgeWorkspaceMeta, useColor bool) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, renderRiskTitle(useColor))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%spurge is permanent and cannot be undone.\n", uiIndent)
	fmt.Fprintf(out, "%sselected: %d\n", uiIndent, len(selectedIDs))

	hasRepoRisk := false
	for _, id := range selectedIDs {
		meta := riskMeta[id]
		if meta.status == "active" && meta.risk != workspacerisk.WorkspaceRiskClean {
			hasRepoRisk = true
			break
		}
	}
	if !hasRepoRisk {
		return
	}

	fmt.Fprintf(out, "%sactive workspace risk detected:\n", uiIndent)
	for _, id := range selectedIDs {
		meta := riskMeta[id]
		if meta.status != "active" || meta.risk == workspacerisk.WorkspaceRiskClean {
			continue
		}
		fmt.Fprintf(out, "%s- %s [%s]\n", uiIndent, id, meta.risk)
		for _, repo := range meta.perRepo {
			fmt.Fprintf(out, "%s  - %s\t%s\n", uiIndent, repo.alias, repo.state)
		}
	}
}

func (c *CLI) purgeWorkspace(ctx context.Context, db *sql.DB, root string, repoPoolPath string, workspaceID string) error {
	status, ok, err := statestore.LookupWorkspaceStatus(ctx, db, workspaceID)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	if !ok {
		return fmt.Errorf("workspace not found: %s", workspaceID)
	}
	if status != "active" && status != "archived" {
		return fmt.Errorf("workspace cannot be purged (status=%s): %s", status, workspaceID)
	}

	repos, err := statestore.ListWorkspaceRepos(ctx, db, workspaceID)
	if err != nil {
		return fmt.Errorf("list workspace repos: %w", err)
	}
	if err := removeWorkspaceWorktrees(ctx, db, root, repoPoolPath, workspaceID, repos); err != nil {
		return fmt.Errorf("remove worktrees: %w", err)
	}

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := os.RemoveAll(wsPath); err != nil {
		return fmt.Errorf("delete workspace dir: %w", err)
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	if err := os.RemoveAll(archivePath); err != nil {
		return fmt.Errorf("delete archive dir: %w", err)
	}

	if _, err := commitPurgeChange(ctx, root, workspaceID); err != nil {
		return fmt.Errorf("commit purge change: %w", err)
	}

	now := time.Now().Unix()
	if err := statestore.PurgeWorkspace(ctx, db, statestore.PurgeWorkspaceInput{
		ID:  workspaceID,
		Now: now,
	}); err != nil {
		return fmt.Errorf("update state store: %w", err)
	}
	return nil
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
