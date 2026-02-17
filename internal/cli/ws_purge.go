package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/kra/internal/core/workspacerisk"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type purgeWorkspaceMeta struct {
	status           string
	risk             workspacerisk.WorkspaceRisk
	perRepo          []repoRiskItem
	purgeGuardActive bool
}

type purgeCommitTrace struct {
	CommitEnabled bool
	PreCommitSHA  string
	PostCommitSHA string
}

func (c *CLI) runWSPurge(args []string) int {
	var noPrompt bool
	var force bool
	doCommit := true
	var outputFormat = "human"
	var dryRun bool
	commitModeExplicit := ""
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
		case "--commit":
			if commitModeExplicit == "no-commit" {
				fmt.Fprintln(c.Err, "--commit and --no-commit cannot be used together")
				c.printWSPurgeUsage(c.Err)
				return exitUsage
			}
			doCommit = true
			commitModeExplicit = "commit"
			args = args[1:]
		case "--no-commit":
			if commitModeExplicit == "commit" {
				fmt.Fprintln(c.Err, "--commit and --no-commit cannot be used together")
				c.printWSPurgeUsage(c.Err)
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
				c.printWSPurgeUsage(c.Err)
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
			fmt.Fprintf(c.Err, "unknown flag for ws purge: %q\n", args[0])
			c.printWSPurgeUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSPurgeUsage(c.Err)
		return exitUsage
	}
	if dryRun && outputFormat != "json" {
		fmt.Fprintln(c.Err, "--dry-run requires --format json")
		c.printWSPurgeUsage(c.Err)
		return exitUsage
	}

	if len(args) != 1 {
		if len(args) > 1 {
			fmt.Fprintf(c.Err, "unexpected args for ws purge: %q\n", strings.Join(args[1:], " "))
		}
		fmt.Fprintln(c.Err, "ws purge requires <id>; use `kra ws select --archived` for interactive selection")
		c.printWSPurgeUsage(c.Err)
		return exitUsage
	}
	if noPrompt && !force {
		fmt.Fprintln(c.Err, "--no-prompt requires --force for ws purge")
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
	if err := c.ensureDebugLog(root, "ws-purge"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws purge args=%q noPrompt=%t force=%t", args, noPrompt, force)
	if outputFormat == "json" {
		return c.runWSPurgeJSON(context.Background(), root, directWorkspaceID, force, doCommit, dryRun)
	}

	ctx := context.Background()

	if doCommit {
		if err := ensureRootGitWorktree(ctx, root); err != nil {
			fmt.Fprintf(c.Err, "%v\n", err)
			return exitError
		}
	}
	useColorOut := writerSupportsColor(c.Out)
	purgeTraces := make(map[string]purgeCommitTrace, 1)

	selectedIDs := make([]string, 0, 4)
	riskMeta := make(map[string]purgeWorkspaceMeta, 4)
	hasActiveRisk := false

	flow := workspaceSelectRiskResultFlowConfig{
		FlowName: "ws purge",
		PrintResult: func(done []string, total int, useColor bool) {
			c.printWSPurgeFlowResult(done, total, useColor, purgeTraces)
		},
		SelectItems: func() ([]workspaceFlowSelection, error) {
			selected := []workspaceFlowSelection{{ID: directWorkspaceID}}
			c.debugf("ws purge direct mode selected=%v", workspaceFlowSelectionIDs(selected))
			return selected, nil
		},
		CollectRiskStage: func(items []workspaceFlowSelection) (workspaceFlowRiskStage, error) {
			selectedIDs = workspaceFlowSelectionIDs(items)
			riskMeta = make(map[string]purgeWorkspaceMeta, len(items))
			hasActiveRisk = false
			for _, id := range selectedIDs {
				meta, err := collectPurgeWorkspaceMeta(ctx, root, id)
				if err != nil {
					return workspaceFlowRiskStage{}, err
				}
				if meta.purgeGuardActive {
					return workspaceFlowRiskStage{}, fmt.Errorf("purge guard is enabled for workspace %s (hint: run 'kra ws unlock %s' before purge)", id, id)
				}
				if meta.status != "archived" {
					return workspaceFlowRiskStage{}, fmt.Errorf("workspace cannot be purged unless archived (run: kra ws --act close %s)", id)
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
			trace, err := c.purgeWorkspace(ctx, root, item.ID, doCommit)
			if err != nil {
				return err
			}
			purgeTraces[item.ID] = trace
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

func (c *CLI) runWSPurgeJSON(ctx context.Context, root string, workspaceID string, force bool, doCommit bool, dryRun bool) int {
	if !dryRun {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "purge",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "invalid_argument",
				Message: "--format json currently supports --dry-run only for ws purge",
			},
		})
		return exitUsage
	}
	meta, err := collectPurgeWorkspaceMeta(ctx, root, workspaceID)
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "ws.purge.dry-run",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "not_found",
				Message: err.Error(),
			},
		})
		return exitError
	}
	checks := make([]map[string]any, 0, 3)
	executable := true
	if meta.status != "archived" {
		executable = false
		checks = append(checks, map[string]any{"name": "archived_only", "status": "fail", "message": "ws purge supports archived workspaces only"})
	} else {
		checks = append(checks, map[string]any{"name": "archived_only", "status": "pass", "message": "workspace is archived"})
	}
	requiresForce := meta.status == "active" && meta.risk != workspacerisk.WorkspaceRiskClean && !force
	if meta.purgeGuardActive {
		checks = append(checks, map[string]any{"name": "purge_guard", "status": "fail", "message": "purge guard is enabled"})
		executable = false
	} else {
		checks = append(checks, map[string]any{"name": "purge_guard", "status": "pass", "message": "purge guard is disabled"})
	}
	if requiresForce {
		checks = append(checks, map[string]any{"name": "risk_gate", "status": "fail", "message": "active non-clean risk requires --force"})
		executable = false
	} else {
		checks = append(checks, map[string]any{"name": "risk_gate", "status": "pass", "message": "purge can proceed"})
	}
	_ = writeCLIJSON(c.Out, cliJSONResponse{
		OK:          executable,
		Action:      "ws.purge.dry-run",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"executable": executable,
			"checks":     checks,
			"risk": map[string]any{
				"workspace": string(meta.risk),
				"repos":     renderRiskItemsJSON(meta.perRepo),
			},
			"planned_effects": []map[string]any{
				{"path": filepath.Join(root, "workspaces", workspaceID), "effect": "delete_if_exists"},
				{"path": filepath.Join(root, "archive", workspaceID), "effect": "delete_if_exists"},
			},
			"requires_confirmation": true,
			"requires_force":        requiresForce,
			"commit_enabled":        doCommit,
			"purge_guard_enabled":   meta.purgeGuardActive,
		},
	})
	if !executable {
		return exitError
	}
	return exitOK
}

func collectPurgeWorkspaceMeta(ctx context.Context, root string, workspaceID string) (purgeWorkspaceMeta, error) {
	status := ""
	activePath := filepath.Join(root, "workspaces", workspaceID)
	archivePath := filepath.Join(root, "archive", workspaceID)
	if fi, err := os.Stat(activePath); err == nil && fi.IsDir() {
		status = "active"
	} else if err != nil && !os.IsNotExist(err) {
		return purgeWorkspaceMeta{}, fmt.Errorf("stat workspace dir: %w", err)
	}
	if status == "" {
		if fi, err := os.Stat(archivePath); err == nil && fi.IsDir() {
			status = "archived"
		} else if err != nil && !os.IsNotExist(err) {
			return purgeWorkspaceMeta{}, fmt.Errorf("stat archive dir: %w", err)
		}
	}
	if status == "" {
		return purgeWorkspaceMeta{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	if status != "active" && status != "archived" {
		return purgeWorkspaceMeta{}, fmt.Errorf("workspace cannot be purged (status=%s): %s", status, workspaceID)
	}

	var wsPath string
	if status == "active" {
		wsPath = activePath
	} else {
		wsPath = archivePath
	}
	metaFile, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return purgeWorkspaceMeta{}, fmt.Errorf("load %s: %w", workspaceMetaFilename, err)
	}
	meta := purgeWorkspaceMeta{
		status:           status,
		risk:             workspacerisk.WorkspaceRiskClean,
		purgeGuardActive: workspaceMetaPurgeGuardEnabled(metaFile),
	}
	if status != "active" {
		return meta, nil
	}

	repos, err := listWorkspaceReposForClose(ctx, root, workspaceID)
	if err != nil {
		return purgeWorkspaceMeta{}, fmt.Errorf("list workspace repos: %w", err)
	}
	risk, perRepo := inspectWorkspaceRepoRisk(ctx, root, workspaceID, repos)
	meta.risk = risk
	meta.perRepo = perRepo
	return meta, nil
}

func printPurgeRiskSection(out io.Writer, selectedIDs []string, riskMeta map[string]purgeWorkspaceMeta, useColor bool) {
	body := []string{
		fmt.Sprintf("%spurge is permanent and cannot be undone.", uiIndent),
		fmt.Sprintf("%s%s %d", uiIndent, styleAccent("selected:", useColor), len(selectedIDs)),
	}

	hasRepoRisk := false
	for _, id := range selectedIDs {
		meta := riskMeta[id]
		if meta.status == "active" && meta.risk != workspacerisk.WorkspaceRiskClean {
			hasRepoRisk = true
			break
		}
	}
	if hasRepoRisk {
		body = append(body, fmt.Sprintf("%s%s", uiIndent, styleAccent("active workspace risk detected:", useColor)))
		for _, id := range selectedIDs {
			meta := riskMeta[id]
			if meta.status != "active" || meta.risk == workspacerisk.WorkspaceRiskClean {
				continue
			}
			body = append(body, fmt.Sprintf("%s- %s %s", uiIndent, id, renderWorkspaceRiskBadge(meta.risk, useColor)))
			for _, repo := range meta.perRepo {
				body = append(body, fmt.Sprintf("%s  - %s %s", uiIndent, repo.alias, renderRepoRiskState(repo.state, useColor)))
			}
		}
	}
	printSection(out, renderRiskTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func (c *CLI) purgeWorkspace(ctx context.Context, root string, workspaceID string, doCommit bool) (purgeCommitTrace, error) {
	meta, err := collectPurgeWorkspaceMeta(ctx, root, workspaceID)
	if err != nil {
		return purgeCommitTrace{}, err
	}
	if meta.purgeGuardActive {
		return purgeCommitTrace{}, fmt.Errorf("purge guard is enabled for workspace %s (hint: run 'kra ws unlock %s' before purge)", workspaceID, workspaceID)
	}
	if meta.status != "archived" {
		return purgeCommitTrace{}, fmt.Errorf("workspace cannot be purged unless archived (run: kra ws --act close %s)", workspaceID)
	}
	trace := purgeCommitTrace{CommitEnabled: doCommit}
	if doCommit {
		preSHA, err := commitPurgePreSnapshot(ctx, root, workspaceID)
		if err != nil {
			return purgeCommitTrace{}, fmt.Errorf("commit purge pre-snapshot: %w", err)
		}
		trace.PreCommitSHA = preSHA
	}

	repos, err := listWorkspaceReposForClose(ctx, root, workspaceID)
	if err != nil {
		return purgeCommitTrace{}, fmt.Errorf("list workspace repos: %w", err)
	}
	if err := removeWorkspaceWorktrees(ctx, root, workspaceID, repos); err != nil {
		return purgeCommitTrace{}, fmt.Errorf("remove worktrees: %w", err)
	}

	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := os.RemoveAll(wsPath); err != nil {
		return purgeCommitTrace{}, fmt.Errorf("delete workspace dir: %w", err)
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	if err := os.RemoveAll(archivePath); err != nil {
		return purgeCommitTrace{}, fmt.Errorf("delete archive dir: %w", err)
	}

	if doCommit {
		postSHA, err := commitPurgeChange(ctx, root, workspaceID)
		if err != nil {
			return purgeCommitTrace{}, fmt.Errorf("commit purge change: %w", err)
		}
		trace.PostCommitSHA = postSHA
	}

	return trace, nil
}

func (c *CLI) printWSPurgeFlowResult(done []string, total int, useColor bool, traces map[string]purgeCommitTrace) {
	body := make([]string, 0, len(done)*5+1)
	body = append(body, fmt.Sprintf("%sPurged %d / %d", uiIndent, len(done), total))
	successMark := styleSuccess("✔", useColor)
	for _, id := range done {
		body = append(body, fmt.Sprintf("%s%s %s", uiIndent, successMark, id))
		trace, ok := traces[id]
		if !ok {
			continue
		}
		if trace.CommitEnabled {
			body = append(body, fmt.Sprintf("%s%s %s purge-pre: %s %s",
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
		body = append(body, fmt.Sprintf("%s%s %s delete workspaces/%s, archive/%s",
			uiIndent+uiIndent,
			styleMuted("2/3", useColor),
			styleAccent("purge:", useColor),
			id,
			id,
		))
		if trace.CommitEnabled {
			body = append(body, fmt.Sprintf("%s%s %s purge: %s %s",
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

func commitPurgeChange(ctx context.Context, root string, workspaceID string) (string, error) {
	archivePrefix, err := toGitTopLevelPath(ctx, root, filepath.Join("archive", workspaceID))
	if err != nil {
		return "", err
	}
	workspacesPrefix, err := toGitTopLevelPath(ctx, root, filepath.Join("workspaces", workspaceID))
	if err != nil {
		return "", err
	}
	archivePrefix += string(filepath.Separator)
	workspacesPrefix += string(filepath.Separator)

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

	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only", "--", archiveArg, workspacesArg)
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

	if _, err := gitutil.Run(ctx, root, "commit", "--allow-empty", "--only", "-m", fmt.Sprintf("purge: %s", workspaceID), "--", archiveArg, workspacesArg); err != nil {
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
		_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg)
		return "", err
	}
	_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg)
	_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", workspacesArg)

	sha, err := gitutil.Run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func commitPurgePreSnapshot(ctx context.Context, root string, workspaceID string) (string, error) {
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
	if _, err := gitutil.Run(ctx, root, "commit", "--allow-empty", "--only", "-m", fmt.Sprintf("purge-pre: %s", workspaceID), "--", archiveArg); err != nil {
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
