package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/workspacerisk"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/paths"
	"github.com/tasuku43/kra/internal/infra/statestore"
)

var errNoActiveWorkspaces = errors.New("no active workspaces available")

func (c *CLI) runWSClose(args []string) int {
	directWorkspaceID := ""
	outputFormat := "human"
	force := false
	doCommit := false
	dryRun := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSCloseUsage(c.Out)
			return exitOK
		case "--force":
			force = true
			args = args[1:]
		case "--commit":
			doCommit = true
			args = args[1:]
		case "--dry-run":
			dryRun = true
			args = args[1:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSCloseUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--id":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSCloseUsage(c.Err)
				return exitUsage
			}
			directWorkspaceID = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--id=") {
				directWorkspaceID = strings.TrimSpace(strings.TrimPrefix(args[0], "--id="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws close: %q\n", args[0])
			c.printWSCloseUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSCloseUsage(c.Err)
		return exitUsage
	}
	if dryRun && outputFormat != "json" {
		fmt.Fprintln(c.Err, "--dry-run requires --format json")
		c.printWSCloseUsage(c.Err)
		return exitUsage
	}

	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws close: %q\n", strings.Join(args[1:], " "))
		c.printWSCloseUsage(c.Err)
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
	if err := c.ensureDebugLog(root, "ws-close"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws close args=%q", args)

	ctx := context.Background()

	if doCommit {
		if err := ensureRootGitWorktree(ctx, root); err != nil {
			fmt.Fprintf(c.Err, "%v\n", err)
			return exitError
		}
	}
	useColorOut := writerSupportsColor(c.Out)

	if len(args) == 1 {
		if directWorkspaceID != "" {
			fmt.Fprintln(c.Err, "--id and positional <id> cannot be used together")
			c.printWSCloseUsage(c.Err)
			return exitUsage
		}
		directWorkspaceID = strings.TrimSpace(args[0])
	}
	if directWorkspaceID != "" {
		if err := validateWorkspaceID(directWorkspaceID); err != nil {
			fmt.Fprintf(c.Err, "invalid workspace id: %v\n", err)
			return exitUsage
		}
	} else {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "close",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: "ws close requires --id <id> or positional <id> in --format json mode",
				},
			})
			return exitUsage
		}
		fromCWD, ok := detectWorkspaceFromCWD(root, wd)
		if !ok || fromCWD.Status != "active" {
			fmt.Fprintln(c.Err, "ws close requires --id <id> or active workspace context")
			c.printWSCloseUsage(c.Err)
			return exitUsage
		}
		directWorkspaceID = fromCWD.ID
	}
	if outputFormat == "json" {
		return c.runWSCloseJSON(directWorkspaceID, force, wd, root, doCommit, dryRun)
	}
	shouldShiftCWD := isPathInside(filepath.Join(root, "workspaces", directWorkspaceID), wd)
	if shouldShiftCWD {
		if err := os.Chdir(root); err != nil {
			fmt.Fprintf(c.Err, "shift process cwd to KRA_ROOT: %v\n", err)
			return exitError
		}
	}

	flow := workspaceSelectRiskResultFlowConfig{
		FlowName: "ws close",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			selected := []workspaceFlowSelection{{ID: directWorkspaceID}}
			c.debugf("ws close direct mode selected=%v", workspaceFlowSelectionIDs(selected))
			return selected, nil
		},
		CollectRiskStage: func(items []workspaceFlowSelection) (workspaceFlowRiskStage, error) {
			riskItems, err := collectWorkspaceRiskDetails(ctx, root, workspaceFlowSelectionIDs(items))
			if err != nil {
				return workspaceFlowRiskStage{}, fmt.Errorf("inspect workspace risk: %w", err)
			}
			hasRisk := hasNonCleanRisk(riskItems)
			if hasRisk {
				c.debugf("ws close risk detected count=%d", len(riskItems))
			}
			return workspaceFlowRiskStage{
				HasRisk: hasRisk,
				Print: func(useColor bool) {
					printRiskSection(c.Out, riskItems, useColor)
				},
			}, nil
		},
		ConfirmRisk: c.confirmRiskProceed,
		ApplyOne: func(item workspaceFlowSelection) error {
			c.debugf("ws close archive start workspace=%s", item.ID)
			if err := c.closeWorkspace(ctx, root, item.ID, doCommit); err != nil {
				return err
			}
			c.debugf("ws close archive completed workspace=%s", item.ID)
			return nil
		},
		ResultVerb: "Closed",
		ResultMark: "✔",
	}

	archived, err := c.runWorkspaceSelectRiskResultFlow(flow, useColorOut)
	if err != nil {
		switch {
		case errors.Is(err, errNoActiveWorkspaces):
			fmt.Fprintln(c.Err, "no active workspaces available")
			return exitError
		case errors.Is(err, errSelectorCanceled):
			c.debugf("ws close selector canceled")
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		case errors.Is(err, errWorkspaceFlowCanceled):
			c.debugf("ws close canceled at risk confirmation")
			return exitError
		default:
			fmt.Fprintf(c.Err, "run ws close flow: %v\n", err)
			return exitError
		}
	}

	if shouldShiftCWD {
		if err := emitShellActionCD(root); err != nil {
			fmt.Fprintf(c.Err, "write shell action: %v\n", err)
			return exitError
		}
	}
	c.debugf("ws close completed archived=%v", archived)
	return exitOK
}

func (c *CLI) runWSCloseJSON(workspaceID string, force bool, wd string, root string, doCommit bool, dryRun bool) int {
	ctx := context.Background()
	shouldShiftCWD := isPathInside(filepath.Join(root, "workspaces", workspaceID), wd)
	if shouldShiftCWD {
		if err := os.Chdir(root); err != nil {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "close",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("shift process cwd to KRA_ROOT: %v", err),
				},
			})
			return exitError
		}
	}

	riskItems, err := collectWorkspaceRiskDetails(ctx, root, []string{workspaceID})
	if err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "close",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("inspect workspace risk: %v", err),
			},
		})
		return exitError
	}
	if hasNonCleanRisk(riskItems) && !force {
		if dryRun {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "ws.close.dry-run",
				WorkspaceID: workspaceID,
				Result: map[string]any{
					"executable": false,
					"checks": []map[string]any{
						{"name": "workspace_exists_active", "status": "pass", "message": "workspace exists and is active"},
						{"name": "risk_gate", "status": "fail", "message": "non-clean risk requires --force"},
					},
					"risk": map[string]any{
						"workspace": string(workspaceRiskFromDetails(riskItems)),
						"repos":     renderRiskDetailItemsJSON(riskItems),
					},
					"planned_effects": []map[string]any{
						{"path": filepath.Join(root, "workspaces", workspaceID), "effect": "move_to_archive"},
						{"path": filepath.Join(root, "archive", workspaceID), "effect": "create"},
					},
					"requires_confirmation": true,
					"requires_force":        true,
				},
			})
			return exitError
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "close",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "conflict",
				Message: "risk confirmation required (pass --force to proceed in --format json mode)",
			},
		})
		return exitError
	}
	if dryRun {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "ws.close.dry-run",
			WorkspaceID: workspaceID,
			Result: map[string]any{
				"executable": true,
				"checks": []map[string]any{
					{"name": "workspace_exists_active", "status": "pass", "message": "workspace exists and is active"},
					{"name": "risk_gate", "status": "pass", "message": "close can proceed"},
				},
				"risk": map[string]any{
					"workspace": string(workspaceRiskFromDetails(riskItems)),
					"repos":     renderRiskDetailItemsJSON(riskItems),
				},
				"planned_effects": []map[string]any{
					{"path": filepath.Join(root, "workspaces", workspaceID), "effect": "move_to_archive"},
					{"path": filepath.Join(root, "archive", workspaceID), "effect": "create"},
				},
				"requires_confirmation": hasNonCleanRisk(riskItems),
				"requires_force":        false,
			},
		})
		return exitOK
	}

	if err := c.closeWorkspace(ctx, root, workspaceID, doCommit); err != nil {
		code := "internal_error"
		msg := err.Error()
		switch {
		case strings.Contains(msg, "workspace not found"):
			code = "workspace_not_found"
		case strings.Contains(msg, "workspace is not active"):
			code = "conflict"
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "close",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: msg,
			},
		})
		return exitError
	}
	if shouldShiftCWD {
		if err := emitShellActionCD(root); err != nil {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "close",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("write shell action: %v", err),
				},
			})
			return exitError
		}
	}
	_ = writeCLIJSON(c.Out, cliJSONResponse{
		OK:          true,
		Action:      "close",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"archived_path": filepath.Join(root, "archive", workspaceID),
		},
	})
	return exitOK
}

func renderRiskItemsJSON(items []repoRiskItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"alias": it.alias,
			"risk":  string(it.state),
		})
	}
	return out
}

func renderRiskDetailItemsJSON(items []workspaceRiskDetail) []map[string]any {
	if len(items) == 0 {
		return []map[string]any{}
	}
	return renderRiskItemsJSON(items[0].perRepo)
}

func workspaceRiskFromDetails(items []workspaceRiskDetail) workspacerisk.WorkspaceRisk {
	if len(items) == 0 {
		return workspacerisk.WorkspaceRiskClean
	}
	return items[0].risk
}

func isPathInside(base string, target string) bool {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	basePath := filepath.Clean(base)
	targetPath := filepath.Clean(target)
	if resolved, err := filepath.EvalSymlinks(basePath); err == nil {
		basePath = filepath.Clean(resolved)
	}
	if resolved, err := filepath.EvalSymlinks(targetPath); err == nil {
		targetPath = filepath.Clean(resolved)
	}
	rel, err := filepath.Rel(basePath, targetPath)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func (c *CLI) closeWorkspace(ctx context.Context, root string, workspaceID string, doCommit bool) error {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if fi, err := os.Stat(wsPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			archivePath := filepath.Join(root, "archive", workspaceID)
			if afi, aerr := os.Stat(archivePath); aerr == nil && afi.IsDir() {
				return fmt.Errorf("workspace is not active (status=archived): %s", workspaceID)
			}
			return fmt.Errorf("workspace not found: %s", workspaceID)
		}
		return fmt.Errorf("stat workspace dir: %w", err)
	} else if !fi.IsDir() {
		return fmt.Errorf("workspace path is not a directory: %s", wsPath)
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	if _, err := os.Stat(archivePath); err == nil {
		return fmt.Errorf("archive directory already exists: %s", archivePath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat archive dir: %w", err)
	}

	repos, err := listWorkspaceReposForClose(ctx, root, workspaceID)
	if err != nil {
		return fmt.Errorf("list workspace repos: %w", err)
	}
	originalMeta, updatedMeta, err := buildWorkspaceMetaForClose(ctx, root, workspaceID, repos)
	if err != nil {
		return fmt.Errorf("prepare %s for close: %w", workspaceMetaFilename, err)
	}
	if err := writeWorkspaceMetaFile(wsPath, updatedMeta); err != nil {
		return fmt.Errorf("write %s: %w", workspaceMetaFilename, err)
	}

	if err := removeWorkspaceWorktrees(ctx, root, workspaceID, repos); err != nil {
		_ = writeWorkspaceMetaFile(wsPath, originalMeta)
		return fmt.Errorf("remove worktrees: %w", err)
	}

	expectedFiles, err := listWorkspaceNonRepoFiles(wsPath)
	if err != nil {
		return fmt.Errorf("list workspace files for archive commit: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		return fmt.Errorf("ensure archive dir: %w", err)
	}
	if err := os.Rename(wsPath, archivePath); err != nil {
		_ = writeWorkspaceMetaFile(wsPath, originalMeta)
		return fmt.Errorf("archive (rename): %w", err)
	}

	if doCommit {
		_, err = commitArchiveChange(ctx, root, workspaceID, expectedFiles)
		if err != nil {
			_ = os.Rename(archivePath, wsPath)
			_ = writeWorkspaceMetaFile(wsPath, originalMeta)
			return fmt.Errorf("commit archive change: %w", err)
		}
	}

	return nil
}

func buildWorkspaceMetaForClose(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo) (workspaceMetaFile, workspaceMetaFile, error) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	original, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return workspaceMetaFile{}, workspaceMetaFile{}, err
	}
	existingByAlias := make(map[string]workspaceMetaRepoRestore, len(original.ReposRestore))
	for _, r := range original.ReposRestore {
		existingByAlias[r.Alias] = r
	}
	entries := make([]workspaceMetaRepoRestore, 0, len(repos))
	for _, r := range repos {
		remoteURL := ""
		worktreePath := filepath.Join(root, "workspaces", workspaceID, "repos", r.Alias)
		if out, err := gitutil.Run(ctx, worktreePath, "remote", "get-url", "origin"); err == nil {
			remoteURL = strings.TrimSpace(out)
		}
		if remoteURL == "" {
			if prev, ok := existingByAlias[r.Alias]; ok {
				remoteURL = strings.TrimSpace(prev.RemoteURL)
			}
		}
		if remoteURL == "" {
			return workspaceMetaFile{}, workspaceMetaFile{}, fmt.Errorf("resolve remote url: alias=%s", r.Alias)
		}
		spec, err := repospec.Normalize(remoteURL)
		if err != nil {
			return workspaceMetaFile{}, workspaceMetaFile{}, fmt.Errorf("normalize repo remote url: %w", err)
		}
		branch := detectBranchForClose(ctx, worktreePath, r.Branch)
		baseRef := strings.TrimSpace(r.BaseRef)
		if baseRef == "" {
			if prev, ok := existingByAlias[r.Alias]; ok && strings.TrimSpace(prev.BaseRef) != "" {
				baseRef = strings.TrimSpace(prev.BaseRef)
			}
		}
		if baseRef == "" {
			baseRef = detectOriginHeadBaseRef(ctx, worktreePath)
			if baseRef == "" {
				return workspaceMetaFile{}, workspaceMetaFile{}, fmt.Errorf("detect default base_ref for %s", spec.RepoKey)
			}
		}

		entries = append(entries, workspaceMetaRepoRestore{
			RepoUID:   r.RepoUID,
			RepoKey:   spec.RepoKey,
			RemoteURL: remoteURL,
			Alias:     r.Alias,
			Branch:    branch,
			BaseRef:   baseRef,
		})
	}
	slices.SortFunc(entries, func(a, b workspaceMetaRepoRestore) int {
		return strings.Compare(a.Alias, b.Alias)
	})
	updated := original
	updated.ReposRestore = entries
	updated.Workspace.Status = "archived"
	updated.Workspace.UpdatedAt = time.Now().Unix()
	return original, updated, nil
}

func detectBranchForClose(ctx context.Context, worktreePath string, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fi, err := os.Stat(worktreePath); err == nil && fi.IsDir() {
		if out, err := gitutil.Run(ctx, worktreePath, "branch", "--show-current"); err == nil {
			branch := strings.TrimSpace(out)
			if branch != "" {
				return branch
			}
		}
	}
	return fallback
}

func (c *CLI) confirmRiskProceed() (bool, error) {
	line, err := c.promptLine(renderCloseRiskApplyPrompt(writerSupportsColor(c.Out)))
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "yes"), nil
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

type workspaceRiskDetail struct {
	id        string
	risk      workspacerisk.WorkspaceRisk
	perRepo   []repoRiskItem
	repoPlans []closeRepoPlanDetail
}

type closeRepoPlanDetail struct {
	repoKey    string
	alias      string
	branch     string
	state      workspacerisk.RepoState
	upstream   string
	ahead      int
	behind     int
	staged     int
	unstaged   int
	untracked  int
	files      []string
	filesANSI  []string
	worktreeOK bool
}

func collectWorkspaceRiskDetails(ctx context.Context, root string, workspaceIDs []string) ([]workspaceRiskDetail, error) {
	out := make([]workspaceRiskDetail, 0, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		repos, err := listWorkspaceReposForClose(ctx, root, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("list workspace repos for %s: %w", workspaceID, err)
		}
		plans := collectCloseRepoPlanDetails(ctx, root, workspaceID, repos)
		risk, perRepo := buildWorkspaceRiskFromPlans(plans)
		out = append(out, workspaceRiskDetail{
			id:        workspaceID,
			risk:      risk,
			perRepo:   perRepo,
			repoPlans: plans,
		})
	}
	return out, nil
}

func buildWorkspaceRiskFromPlans(plans []closeRepoPlanDetail) (workspacerisk.WorkspaceRisk, []repoRiskItem) {
	states := make([]workspacerisk.RepoState, 0, len(plans))
	items := make([]repoRiskItem, 0, len(plans))
	for _, p := range plans {
		states = append(states, p.state)
		items = append(items, repoRiskItem{
			alias: p.alias,
			state: p.state,
		})
	}
	return workspacerisk.Aggregate(states), items
}

func hasNonCleanRisk(items []workspaceRiskDetail) bool {
	for _, it := range items {
		if it.risk != workspacerisk.WorkspaceRiskClean {
			return true
		}
	}
	return false
}

func printRiskSection(w io.Writer, items []workspaceRiskDetail, useColor bool) {
	body := make([]string, 0, len(items)*8+4)
	if len(items) == 1 {
		body = append(body, fmt.Sprintf("%s%s close workspace %s", uiIndent, styleMuted("•", useColor), items[0].id))
		body = append(body, fmt.Sprintf("%s%s %s:", uiIndent, styleMuted("•", useColor), styleAccent("repos", useColor)))
		appendCloseRepoPlanBody(&body, items[0], useColor)
	} else {
		body = append(body, fmt.Sprintf("%s%s close %d workspaces", uiIndent, styleMuted("•", useColor), len(items)))
		for _, it := range items {
			body = append(body, fmt.Sprintf("%s%s workspace %s", uiIndent, styleMuted("•", useColor), it.id))
			body = append(body, fmt.Sprintf("%s%s %s:", uiIndent, styleMuted("•", useColor), styleAccent("repos", useColor)))
			appendCloseRepoPlanBody(&body, it, useColor)
		}
	}

	printSection(w, styleBold("Plan:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func appendCloseRepoPlanBody(body *[]string, item workspaceRiskDetail, useColor bool) {
	plans := item.repoPlans
	if len(plans) == 0 {
		plans = make([]closeRepoPlanDetail, 0, len(item.perRepo))
		for _, r := range item.perRepo {
			plans = append(plans, closeRepoPlanDetail{
				repoKey: r.alias,
				alias:   r.alias,
				state:   r.state,
			})
		}
	}
	for i, p := range plans {
		connector := "├─ "
		prefix := styleMuted("│  ", useColor)
		if i == len(plans)-1 {
			connector = "└─ "
			prefix = "   "
		}
		branchSuffix := ""
		if strings.TrimSpace(p.branch) != "" {
			branchSuffix = fmt.Sprintf(" (%s%s)", styleMuted("branch: ", useColor), styleGitRefLocal(p.branch, useColor))
		}
		*body = append(*body, fmt.Sprintf("%s%s%s%s", uiIndent+uiIndent, styleMuted(connector, useColor), p.repoKey, branchSuffix))
		*body = append(*body, fmt.Sprintf("%s%s%s %s", uiIndent+uiIndent, prefix, styleMuted("risk:", useColor), renderClosePlanRiskLabel(p, useColor)))
		*body = append(*body, fmt.Sprintf("%s%s%s %s%s %s%s %s%s",
			uiIndent+uiIndent,
			prefix,
			styleMuted("sync:", useColor),
			styleMuted("upstream=", useColor),
			renderPlanUpstreamLabel(p.upstream, useColor),
			styleMuted("ahead=", useColor),
			renderPlanAheadBehindValue(p.ahead, useColor),
			styleMuted("behind=", useColor),
			renderPlanAheadBehindValue(p.behind, useColor),
		))
		if len(p.files) > 0 {
			*body = append(*body, fmt.Sprintf("%s%s%s", uiIndent+uiIndent, prefix, styleMuted("files:", useColor)))
			for _, f := range p.files {
				*body = append(*body, fmt.Sprintf("%s%s  %s", uiIndent+uiIndent, prefix, styleGitStatusShortLine(f, useColor)))
			}
		}
	}
}

func renderClosePlanRiskLabel(detail closeRepoPlanDetail, useColor bool) string {
	riskText := string(detail.state)
	switch detail.state {
	case workspacerisk.RepoStateClean:
		return styleMuted(riskText, useColor)
	case workspacerisk.RepoStateUnpushed, workspacerisk.RepoStateDiverged:
		return styleWarn(riskText, useColor)
	default:
		base := styleError(riskText, useColor)
		parts := make([]string, 0, 3)
		if detail.staged > 0 {
			parts = append(parts, renderPlanDirtyCounter("staged", detail.staged, useColor))
		}
		if detail.unstaged > 0 {
			parts = append(parts, renderPlanDirtyCounter("unstaged", detail.unstaged, useColor))
		}
		if detail.untracked > 0 {
			parts = append(parts, renderPlanDirtyCounter("untracked", detail.untracked, useColor))
		}
		if len(parts) == 0 {
			return base
		}
		return fmt.Sprintf("%s (%s)", base, strings.Join(parts, " "))
	}
}

func renderCloseRiskApplyPrompt(useColor bool) string {
	bullet := styleMuted("•", useColor)
	return fmt.Sprintf("%s%s type %s to apply close on non-clean workspaces: ", uiIndent, bullet, styleAccent("yes", useColor))
}

func collectCloseRepoPlanDetails(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo) []closeRepoPlanDetail {
	details := make([]closeRepoPlanDetail, 0, len(repos))
	for _, r := range repos {
		repoKey := strings.TrimSpace(r.RepoKey)
		if repoKey == "" {
			repoKey = strings.TrimSpace(r.Alias)
		}
		worktreePath := filepath.Join(root, "workspaces", workspaceID, "repos", r.Alias)
		d := closeRepoPlanDetail{
			repoKey: repoKey,
			alias:   strings.TrimSpace(r.Alias),
			branch:  strings.TrimSpace(r.Branch),
			state:   workspacerisk.RepoStateUnknown,
		}
		if !r.MissingAt.Valid {
			snapshot := inspectGitRepoSnapshot(ctx, worktreePath)
			status := snapshot.Status
			d.state = workspacerisk.ClassifyRepoStatus(status)
			d.upstream = strings.TrimSpace(status.Upstream)
			d.ahead = status.AheadCount
			d.behind = status.BehindCount
			if b := strings.TrimSpace(snapshot.Branch); b != "" {
				d.branch = b
			}
			d.staged = snapshot.Staged
			d.unstaged = snapshot.Unstaged
			d.untracked = snapshot.Untracked
			d.files = append([]string{}, snapshot.Files...)
		}
		details = append(details, d)
	}
	slices.SortFunc(details, func(a, b closeRepoPlanDetail) int {
		if a.repoKey != b.repoKey {
			return strings.Compare(a.repoKey, b.repoKey)
		}
		return strings.Compare(a.alias, b.alias)
	})
	return details
}

func renderWorkspaceRiskBadge(risk workspacerisk.WorkspaceRisk, useColor bool) string {
	tag := fmt.Sprintf("[%s]", risk)
	switch risk {
	case workspacerisk.WorkspaceRiskDirty, workspacerisk.WorkspaceRiskUnknown:
		return styleError(tag, useColor)
	case workspacerisk.WorkspaceRiskDiverged, workspacerisk.WorkspaceRiskUnpushed:
		return styleWarn(tag, useColor)
	default:
		return tag
	}
}

func renderRepoRiskState(state workspacerisk.RepoState, useColor bool) string {
	tag := fmt.Sprintf("[%s]", state)
	switch state {
	case workspacerisk.RepoStateDirty, workspacerisk.RepoStateUnknown:
		return styleError(tag, useColor)
	case workspacerisk.RepoStateDiverged, workspacerisk.RepoStateUnpushed:
		return styleWarn(tag, useColor)
	default:
		return tag
	}
}

func removeWorkspaceWorktrees(ctx context.Context, root string, workspaceID string, repos []statestore.WorkspaceRepo) error {
	reposDir := filepath.Join(root, "workspaces", workspaceID, "repos")

	for _, r := range repos {
		worktreePath := filepath.Join(reposDir, r.Alias)
		if _, err := os.Stat(worktreePath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}

		barePath, err := resolveBarePathFromWorktreeGitdir(worktreePath)
		if err != nil {
			return err
		}

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

func listWorkspaceReposForClose(ctx context.Context, root string, workspaceID string) ([]statestore.WorkspaceRepo, error) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, _ := loadWorkspaceMetaFile(wsPath)
	return listWorkspaceReposFromFilesystem(ctx, root, "active", workspaceID, meta)
}

func resolveBarePathFromSpec(repoPoolPath string, spec repospec.Spec) string {
	return filepath.Join(repoPoolPath, spec.Host, spec.Owner, spec.Repo+".git")
}

func resolveBarePathFromWorktreeGitdir(worktreePath string) (string, error) {
	gitFile := filepath.Join(worktreePath, ".git")
	b, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", gitFile, err)
	}
	line := strings.TrimSpace(string(b))
	const pfx = "gitdir:"
	if !strings.HasPrefix(strings.ToLower(line), pfx) {
		return "", fmt.Errorf("unexpected .git format: %s", gitFile)
	}
	dir := strings.TrimSpace(line[len(pfx):])
	if !filepath.IsAbs(dir) {
		dir = filepath.Clean(filepath.Join(worktreePath, dir))
	}
	// <bare>/worktrees/<name>
	bare := filepath.Dir(filepath.Dir(dir))
	return bare, nil
}

func ensureRootGitWorktree(ctx context.Context, root string) error {
	out, err := gitutil.Run(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("KRA_ROOT must be a git working tree: %w", err)
	}

	got := filepath.Clean(strings.TrimSpace(out))
	want := filepath.Clean(root)

	if gotEval, err := filepath.EvalSymlinks(got); err == nil {
		got = gotEval
	}
	if wantEval, err := filepath.EvalSymlinks(want); err == nil {
		want = wantEval
	}

	rel, relErr := filepath.Rel(got, want)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("KRA_ROOT must be inside the git worktree: toplevel=%s root=%s", strings.TrimSpace(out), root)
	}
	return nil
}

func ensureNoStagedChanges(ctx context.Context, root string) error {
	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("git index has staged changes; commit or unstage them before running ws purge")
	}
	return nil
}

func listWorkspaceNonRepoFiles(wsPath string) ([]string, error) {
	files := make([]string, 0, 8)
	reposDir := filepath.Join(wsPath, "repos")

	err := filepath.WalkDir(wsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == wsPath {
			return nil
		}
		if path == reposDir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(path, reposDir+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(wsPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func resetArchiveStaging(ctx context.Context, root, archiveArg, workspacesArg string) {
	_, _ = gitutil.Run(ctx, root, "reset", "-q", "--", archiveArg, workspacesArg)
}

func commitArchiveChange(ctx context.Context, root string, workspaceID string, expectedArchiveFiles []string) (string, error) {
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
		return "", err
	}
	if _, err := gitutil.Run(ctx, root, "add", "-u", "--", workspacesArg); err != nil {
		// In an uninitialized git history, `workspaces/<id>` may not be tracked at all yet.
		// Still allow archiving so the archive can be committed.
		if !strings.Contains(err.Error(), "did not match any files") && !strings.Contains(err.Error(), "did not match any file") {
			resetArchiveStaging(ctx, root, archiveArg, workspacesArg)
			return "", err
		}
	}

	out, err := gitutil.Run(ctx, root, "diff", "--cached", "--name-only", "--", archiveArg, workspacesArg)
	if err != nil {
		resetArchiveStaging(ctx, root, archiveArg, workspacesArg)
		return "", err
	}

	staged := strings.Fields(out)
	stagedSet := make(map[string]struct{}, len(staged))
	hasWorkspacesStage := false
	for _, p := range staged {
		p = filepath.Clean(filepath.FromSlash(p))
		stagedSet[p] = struct{}{}
		if strings.HasPrefix(p, workspacesPrefix) {
			hasWorkspacesStage = true
		}
	}

	for _, rel := range expectedArchiveFiles {
		candidate, err := toGitTopLevelPath(ctx, root, filepath.Join("archive", workspaceID, filepath.FromSlash(rel)))
		if err != nil {
			resetArchiveStaging(ctx, root, archiveArg, workspacesArg)
			return "", err
		}
		if _, ok := stagedSet[candidate]; ok {
			continue
		}
		ignored, ignoreErr := isGitIgnoredRelativeToRoot(root, filepath.Join("archive", workspaceID, filepath.FromSlash(rel)))
		if ignoreErr != nil {
			resetArchiveStaging(ctx, root, archiveArg, workspacesArg)
			return "", ignoreErr
		}
		if ignored {
			continue
		}
		resetArchiveStaging(ctx, root, archiveArg, workspacesArg)
		return "", fmt.Errorf("workspace contains files ignored by git; cannot archive commit: %s", rel)
	}

	commitArgs := []string{"commit", "--only", "-m", fmt.Sprintf("archive: %s", workspaceID), "--", archiveArg}
	if hasWorkspacesStage {
		commitArgs = append(commitArgs, workspacesArg)
	}
	if _, err := gitutil.Run(ctx, root, commitArgs...); err != nil {
		resetArchiveStaging(ctx, root, archiveArg, workspacesArg)
		return "", err
	}
	// commit --only does not clear index entries staged by earlier add commands.
	// Unstage only the command scope so unrelated user staged changes stay intact.
	resetArchiveStaging(ctx, root, archiveArg, workspacesArg)

	sha, err := gitutil.Run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func isGitIgnoredRelativeToRoot(root string, relPath string) (bool, error) {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	ignored, err := gitutil.IsIgnored(context.Background(), root, relPath)
	if err != nil {
		return false, err
	}
	return ignored, nil
}
