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
	"strings"
	"time"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/infra/gitutil"
	"github.com/tasuku43/gionx/internal/infra/paths"
	"github.com/tasuku43/gionx/internal/infra/statestore"
)

type removeRepoCandidate struct {
	RepoUID      string
	RepoKey      string
	Alias        string
	SelectorID   string
	WorktreePath string
}

type removeRepoPlanDetail struct {
	candidate removeRepoCandidate
	branch    string
	state     workspacerisk.RepoState
	upstream  string
	ahead     int
	behind    int
	staged    int
	unstaged  int
	untracked int
	files     []string
	filesANSI []string
}

func (c *CLI) runWSRemoveRepo(args []string) int {
	idFromFlag := ""
	outputFormat := "human"
	force := false
	yes := false
	repoKeysFromFlag := make([]string, 0, 4)
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSRemoveRepoUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printWSRemoveRepoUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--repo":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--repo requires a value")
				c.printWSRemoveRepoUsage(c.Err)
				return exitUsage
			}
			repoKeysFromFlag = append(repoKeysFromFlag, strings.TrimSpace(args[1]))
			args = args[2:]
		case "--yes":
			yes = true
			args = args[1:]
		case "--force":
			force = true
			args = args[1:]
		case "--id":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--id requires a value")
				c.printWSRemoveRepoUsage(c.Err)
				return exitUsage
			}
			idFromFlag = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--id=") {
				idFromFlag = strings.TrimSpace(strings.TrimPrefix(args[0], "--id="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--repo=") {
				repoKeysFromFlag = append(repoKeysFromFlag, strings.TrimSpace(strings.TrimPrefix(args[0], "--repo=")))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for ws remove-repo: %q\n", args[0])
			c.printWSRemoveRepoUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printWSRemoveRepoUsage(c.Err)
		return exitUsage
	}
	if outputFormat == "human" && (len(repoKeysFromFlag) > 0 || yes) {
		fmt.Fprintln(c.Err, "--repo/--yes are only supported with --format json")
		c.printWSRemoveRepoUsage(c.Err)
		return exitUsage
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for ws remove-repo: %q\n", strings.Join(args[1:], " "))
		c.printWSRemoveRepoUsage(c.Err)
		return exitUsage
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
	if err := c.ensureDebugLog(root, "ws-remove-repo"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}

	workspaceID := ""
	resolveArgs := append([]string{}, args...)
	if idFromFlag != "" {
		if len(resolveArgs) > 0 {
			fmt.Fprintln(c.Err, "--id and positional <workspace-id> cannot be used together")
			c.printWSRemoveRepoUsage(c.Err)
			return exitUsage
		}
		resolveArgs = []string{idFromFlag}
	}
	workspaceID, err = resolveWorkspaceIDForAddRepo(root, wd, resolveArgs)
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "remove-repo",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: err.Error(),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSRemoveRepoUsage(c.Err)
		return exitUsage
	}

	existsFS, activeFS, fsErr := workspaceActiveOnFilesystem(root, workspaceID)
	if fsErr != nil {
		fmt.Fprintf(c.Err, "load workspace: %v\n", fsErr)
		return exitError
	}
	if !existsFS {
		fmt.Fprintf(c.Err, "workspace not found: %s\n", workspaceID)
		return exitError
	}
	if !activeFS {
		fmt.Fprintf(c.Err, "workspace is not active (status=archived): %s\n", workspaceID)
		return exitError
	}

	ctx := context.Background()
	var db *sql.DB
	if outputFormat == "json" {
		if len(repoKeysFromFlag) == 0 {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "remove-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: "--repo is required in --format json mode",
				},
			})
			return exitUsage
		}
		if !yes {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "remove-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: "--yes is required in --format json mode",
				},
			})
			return exitUsage
		}
	}

	candidates, err := listRemoveRepoCandidates(ctx, db, root, workspaceID)
	if err != nil {
		fmt.Fprintf(c.Err, "list workspace repos: %v\n", err)
		return exitError
	}
	if len(candidates) == 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "remove-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: "no repos are bound to this workspace",
				},
			})
			return exitUsage
		}
		fmt.Fprintln(c.Err, "no repos are bound to this workspace")
		return exitError
	}

	if outputFormat == "json" {
		return c.runWSRemoveRepoJSON(root, workspaceID, db, candidates, repoKeysFromFlag, yes, force)
	}

	selected, err := c.promptRemoveRepoSelection(candidates)
	if err != nil {
		if errors.Is(err, errSelectorCanceled) {
			fmt.Fprintln(c.Err, "aborted")
			return exitError
		}
		fmt.Fprintf(c.Err, "select workspace repos: %v\n", err)
		return exitError
	}
	if len(selected) == 0 {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	planDetails := collectRemoveRepoPlanDetails(ctx, selected)
	risky := extractRiskyRepoItems(planDetails)

	printRemoveRepoPlan(c.Out, workspaceID, selected, planDetails, useColorOut)
	prompt := renderAddRepoApplyPrompt(useColorOut)
	if len(risky) > 0 {
		prompt = fmt.Sprintf("%stype %s to apply remove on non-clean repos: ", uiIndent, styleAccent("yes", useColorOut))
	}
	line, err := c.promptLine(prompt)
	if err != nil {
		fmt.Fprintf(c.Err, "read confirmation: %v\n", err)
		return exitError
	}
	confirmed := strings.TrimSpace(line) == ""
	if len(risky) > 0 {
		confirmed = strings.EqualFold(strings.TrimSpace(line), "yes")
	}
	if !confirmed {
		fmt.Fprintln(c.Err, "aborted")
		return exitError
	}

	if err := applyRemoveRepoPlan(ctx, db, root, workspaceID, selected); err != nil {
		fmt.Fprintf(c.Err, "apply remove-repo: %v\n", err)
		return exitError
	}
	nowUnix := time.Now().Unix()
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := removeWorkspaceMetaReposRestoreByAlias(wsPath, selectedAliases(selected), nowUnix); err != nil {
		fmt.Fprintf(c.Err, "update %s: %v\n", workspaceMetaFilename, err)
		return exitError
	}

	printRemoveRepoResult(c.Out, selected, useColorOut)
	return exitOK
}

func (c *CLI) runWSRemoveRepoJSON(root string, workspaceID string, db *sql.DB, candidates []removeRepoCandidate, repoKeys []string, yes bool, force bool) int {
	_ = yes
	byRepoKey := make(map[string]removeRepoCandidate, len(candidates))
	for _, cand := range candidates {
		byRepoKey[cand.RepoKey] = cand
	}
	selected := make([]removeRepoCandidate, 0, len(repoKeys))
	seen := map[string]bool{}
	for _, raw := range repoKeys {
		repoKey := strings.TrimSpace(raw)
		if repoKey == "" || seen[repoKey] {
			continue
		}
		seen[repoKey] = true
		cand, ok := byRepoKey[repoKey]
		if !ok {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "remove-repo",
				WorkspaceID: workspaceID,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("repo not bound to workspace: %s", repoKey),
				},
			})
			return exitUsage
		}
		selected = append(selected, cand)
	}

	risky := evaluateRemoveRepoRisk(context.Background(), selected)
	if len(risky) > 0 && !force {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "remove-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "conflict",
				Message: "preflight remove-repo: selected repos include non-clean worktrees (use --force to proceed)",
			},
		})
		return exitError
	}

	if err := applyRemoveRepoPlan(context.Background(), db, root, workspaceID, selected); err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "remove-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("apply remove-repo: %v", err),
			},
		})
		return exitError
	}
	nowUnix := time.Now().Unix()
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if err := removeWorkspaceMetaReposRestoreByAlias(wsPath, selectedAliases(selected), nowUnix); err != nil {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "remove-repo",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    "internal_error",
				Message: fmt.Sprintf("update %s: %v", workspaceMetaFilename, err),
			},
		})
		return exitError
	}
	repos := make([]string, 0, len(selected))
	for _, it := range selected {
		repos = append(repos, it.RepoKey)
	}
	_ = writeCLIJSON(c.Out, cliJSONResponse{
		OK:          true,
		Action:      "remove-repo",
		WorkspaceID: workspaceID,
		Result: map[string]any{
			"removed": len(selected),
			"repos":   repos,
		},
	})
	return exitOK
}

func listRemoveRepoCandidates(ctx context.Context, db *sql.DB, root string, workspaceID string) ([]removeRepoCandidate, error) {
	repos, err := listWorkspaceReposForClose(ctx, db, root, workspaceID)
	if err != nil {
		return nil, err
	}
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, _ := loadWorkspaceMetaFile(wsPath)
	restoreByAlias := make(map[string]workspaceMetaRepoRestore, len(meta.ReposRestore))
	for _, r := range meta.ReposRestore {
		restoreByAlias[strings.TrimSpace(r.Alias)] = r
	}
	out := make([]removeRepoCandidate, 0, len(repos))
	for _, r := range repos {
		alias := strings.TrimSpace(r.Alias)
		if alias == "" {
			continue
		}
		restore := restoreByAlias[alias]
		repoKey := strings.TrimSpace(r.RepoKey)
		if repoKey == "" {
			repoKey = strings.TrimSpace(restore.RepoKey)
		}
		if repoKey == "" {
			repoKey = alias
		}
		repoUID := strings.TrimSpace(r.RepoUID)
		if repoUID == "" {
			repoUID = strings.TrimSpace(restore.RepoUID)
		}
		out = append(out, removeRepoCandidate{
			RepoUID:      repoUID,
			RepoKey:      repoKey,
			Alias:        alias,
			WorktreePath: filepath.Join(root, "workspaces", workspaceID, "repos", alias),
		})
	}
	slices.SortFunc(out, func(a, b removeRepoCandidate) int {
		if a.RepoKey != b.RepoKey {
			return strings.Compare(a.RepoKey, b.RepoKey)
		}
		return strings.Compare(a.Alias, b.Alias)
	})

	repoKeyCount := map[string]int{}
	for _, it := range out {
		repoKeyCount[it.RepoKey]++
	}
	for i := range out {
		if repoKeyCount[out[i].RepoKey] > 1 {
			out[i].SelectorID = fmt.Sprintf("%s (%s)", out[i].RepoKey, out[i].Alias)
		} else {
			out[i].SelectorID = out[i].RepoKey
		}
	}
	return out, nil
}

func (c *CLI) promptRemoveRepoSelection(candidates []removeRepoCandidate) ([]removeRepoCandidate, error) {
	selectorCandidates := make([]workspaceSelectorCandidate, 0, len(candidates))
	candidateByID := make(map[string]removeRepoCandidate, len(candidates))
	for _, it := range candidates {
		selectorCandidates = append(selectorCandidates, workspaceSelectorCandidate{
			ID:    it.SelectorID,
			Title: "",
		})
		candidateByID[it.SelectorID] = it
	}
	selectedIDs, err := c.promptWorkspaceSelectorWithOptions("active", "remove", "Repos(workspace):", "repo", selectorCandidates)
	if err != nil {
		return nil, err
	}
	selected := make([]removeRepoCandidate, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		cand, ok := candidateByID[id]
		if !ok {
			continue
		}
		selected = append(selected, cand)
	}
	return selected, nil
}

func evaluateRemoveRepoRisk(ctx context.Context, selected []removeRepoCandidate) []repoRiskItem {
	details := collectRemoveRepoPlanDetails(ctx, selected)
	return extractRiskyRepoItems(details)
}

func extractRiskyRepoItems(details []removeRepoPlanDetail) []repoRiskItem {
	risky := make([]repoRiskItem, 0, len(details))
	for _, it := range details {
		if it.state != workspacerisk.RepoStateClean {
			risky = append(risky, repoRiskItem{alias: it.candidate.Alias, state: it.state})
		}
	}
	return risky
}

func collectRemoveRepoPlanDetails(ctx context.Context, selected []removeRepoCandidate) []removeRepoPlanDetail {
	details := make([]removeRepoPlanDetail, 0, len(selected))
	for _, it := range selected {
		d := removeRepoPlanDetail{
			candidate: it,
			state:     workspacerisk.RepoStateUnknown,
		}
		if _, err := os.Stat(it.WorktreePath); errors.Is(err, os.ErrNotExist) {
			details = append(details, d)
			continue
		}

		status := inspectGitRepoStatus(ctx, it.WorktreePath)
		d.state = workspacerisk.ClassifyRepoStatus(status)
		d.upstream = strings.TrimSpace(status.Upstream)
		d.ahead = status.AheadCount
		d.behind = status.BehindCount
		d.branch = detectBranchForClose(ctx, it.WorktreePath, "")
		d.staged, d.unstaged, d.untracked, d.files, d.filesANSI = collectGitShortStatusSummary(ctx, it.WorktreePath)
		details = append(details, d)
	}
	return details
}

func collectGitShortStatusSummary(ctx context.Context, worktreePath string) (staged int, unstaged int, untracked int, files []string, filesANSI []string) {
	out, err := gitutil.Run(ctx, worktreePath, "status", "--short")
	if err != nil {
		return 0, 0, 0, nil, nil
	}
	outANSI, err := gitutil.Run(ctx, worktreePath, "-c", "color.status=always", "status", "--short")
	if err != nil {
		outANSI = ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	linesANSI := strings.Split(strings.TrimSpace(outANSI), "\n")
	if strings.TrimSpace(outANSI) == "" {
		linesANSI = nil
	}
	useANSI := len(linesANSI) == len(lines)
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		files = append(files, line)
		if len(line) >= 2 {
			x := line[0]
			y := line[1]
			if x == '?' && y == '?' {
				untracked++
				continue
			}
			if x != ' ' {
				staged++
			}
			if y != ' ' {
				unstaged++
			}
		} else {
			unstaged++
		}
	}
	if useANSI {
		for _, line := range linesANSI {
			line = strings.TrimRight(line, " \t")
			if strings.TrimSpace(line) == "" {
				continue
			}
			filesANSI = append(filesANSI, line)
		}
	}
	return staged, unstaged, untracked, files, filesANSI
}

func selectedAliases(selected []removeRepoCandidate) []string {
	aliases := make([]string, 0, len(selected))
	seen := map[string]bool{}
	for _, it := range selected {
		alias := strings.TrimSpace(it.Alias)
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	return aliases
}

func applyRemoveRepoPlan(ctx context.Context, db *sql.DB, root string, workspaceID string, selected []removeRepoCandidate) error {
	for _, it := range selected {
		if _, err := os.Stat(it.WorktreePath); err == nil {
			barePath, bareErr := resolveBarePathFromWorktreeGitdir(it.WorktreePath)
			if bareErr == nil {
				if _, statErr := os.Stat(barePath); statErr == nil {
					if _, err := gitutil.RunBare(ctx, barePath, "worktree", "remove", "--force", it.WorktreePath); err != nil {
						return err
					}
				}
			}
			if err := os.RemoveAll(it.WorktreePath); err != nil {
				return err
			}
		}
		if db != nil && strings.TrimSpace(it.RepoUID) != "" {
			if err := statestore.DeleteWorkspaceRepoBinding(ctx, db, workspaceID, it.RepoUID); err != nil {
				return err
			}
		}
	}
	reposDir := filepath.Join(root, "workspaces", workspaceID, "repos")
	entries, err := os.ReadDir(reposDir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(reposDir)
	}
	return nil
}

func printRemoveRepoPlan(out io.Writer, workspaceID string, selected []removeRepoCandidate, details []removeRepoPlanDetail, useColor bool) {
	bullet := styleMuted("•", useColor)
	reposLabel := styleAccent("repos", useColor)
	body := []string{
		fmt.Sprintf("%s%s remove %d repos from workspace %s", uiIndent, bullet, len(selected), workspaceID),
	}
	if len(selected) > 0 {
		body = append(body, fmt.Sprintf("%s%s %s:", uiIndent, bullet, reposLabel))
	}
	for i, p := range selected {
		connector := "├─ "
		if i == len(selected)-1 {
			connector = "└─ "
		}
		d, ok := lookupRemoveRepoPlanDetail(details, p)
		branchSuffix := ""
		if ok && strings.TrimSpace(d.branch) != "" {
			branchSuffix = fmt.Sprintf(" (branch: %s)", d.branch)
		}
		body = append(body, fmt.Sprintf("%s%s%s%s", uiIndent+uiIndent, styleMuted(connector, useColor), p.RepoKey, branchSuffix))
		if !ok {
			continue
		}
		riskLine := fmt.Sprintf("%s%srisk: %s", uiIndent+uiIndent, styleMuted("│  ", useColor), renderPlanRiskLabel(d, useColor))
		if i == len(selected)-1 {
			riskLine = fmt.Sprintf("%s%srisk: %s", uiIndent+uiIndent, "   ", renderPlanRiskLabel(d, useColor))
		}
		body = append(body, riskLine)
		syncLine := fmt.Sprintf("%s%ssync: upstream=%s ahead=%s behind=%s",
			uiIndent+uiIndent,
			styleMuted("│  ", useColor),
			renderPlanUpstreamLabel(d.upstream, useColor),
			renderPlanAheadBehindValue(d.ahead, useColor),
			renderPlanAheadBehindValue(d.behind, useColor),
		)
		if i == len(selected)-1 {
			syncLine = fmt.Sprintf("%s%ssync: upstream=%s ahead=%s behind=%s",
				uiIndent+uiIndent,
				"   ",
				renderPlanUpstreamLabel(d.upstream, useColor),
				renderPlanAheadBehindValue(d.ahead, useColor),
				renderPlanAheadBehindValue(d.behind, useColor),
			)
		}
		body = append(body, syncLine)
		if len(d.files) > 0 {
			prefix := styleMuted("│  ", useColor)
			if i == len(selected)-1 {
				prefix = "   "
			}
			body = append(body, fmt.Sprintf("%s%sfiles:", uiIndent+uiIndent, prefix))
			renderLines := d.files
			if useColor && len(d.filesANSI) == len(d.files) {
				renderLines = d.filesANSI
			}
			for _, f := range renderLines {
				body = append(body, fmt.Sprintf("%s%s  %s", uiIndent+uiIndent, prefix, f))
			}
		}
	}
	fmt.Fprintln(out)
	printSection(out, styleBold("Plan:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func lookupRemoveRepoPlanDetail(details []removeRepoPlanDetail, candidate removeRepoCandidate) (removeRepoPlanDetail, bool) {
	for _, d := range details {
		if d.candidate.RepoKey == candidate.RepoKey && d.candidate.Alias == candidate.Alias {
			return d, true
		}
	}
	return removeRepoPlanDetail{}, false
}

func renderPlanRiskLabel(detail removeRepoPlanDetail, useColor bool) string {
	riskText := string(detail.state)
	switch detail.state {
	case workspacerisk.RepoStateClean:
		return riskText
	case workspacerisk.RepoStateUnpushed, workspacerisk.RepoStateDiverged:
		return styleWarn(riskText, useColor)
	default:
		base := styleError(riskText, useColor)
		parts := make([]string, 0, 3)
		if detail.staged > 0 {
			parts = append(parts, fmt.Sprintf("staged=%d", detail.staged))
		}
		if detail.unstaged > 0 {
			parts = append(parts, fmt.Sprintf("unstaged=%d", detail.unstaged))
		}
		if detail.untracked > 0 {
			parts = append(parts, fmt.Sprintf("untracked=%d", detail.untracked))
		}
		if len(parts) == 0 {
			return base
		}
		return fmt.Sprintf("%s (%s)", base, strings.Join(parts, " "))
	}
}

func renderPlanUpstreamLabel(upstream string, useColor bool) string {
	if strings.TrimSpace(upstream) == "" {
		return styleMuted("(none)", useColor)
	}
	return upstream
}

func renderPlanAheadBehindValue(v int, useColor bool) string {
	if v > 0 {
		return styleWarn(fmt.Sprintf("%d", v), useColor)
	}
	return fmt.Sprintf("%d", v)
}

func printRemoveRepoResult(out io.Writer, removed []removeRepoCandidate, useColor bool) {
	bullet := styleMuted("•", useColor)
	body := make([]string, 0, len(removed)+1)
	body = append(body, fmt.Sprintf("%s%s Removed %d / %d", uiIndent, bullet, len(removed), len(removed)))
	for _, it := range removed {
		check := styleSuccess("✔", useColor)
		body = append(body, fmt.Sprintf("%s%s %s %s", uiIndent, bullet, check, it.RepoKey))
	}
	printSection(out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}
