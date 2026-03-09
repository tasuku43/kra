package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/app/contextcmd"
	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/core/workspacerisk"
	"github.com/tasuku43/kra/internal/infra/appports"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type wsDashboardOptions struct {
	scope      string
	format     string
	workspace  string
	showDetail bool
}

type wsDashboardRow struct {
	ID        string
	Title     string
	Status    string
	RepoCount int
	Risk      workspacerisk.WorkspaceRisk
	Coverage  workspaceOutputCoverage
	Tasks     wstask.Overview
}

type wsDashboardSummary struct {
	Active         int
	Archived       int
	RiskTotals     map[string]int
	CoverageTotals map[string]int
}

type wsDashboardResult struct {
	Root        string
	Context     string
	Scope       string
	GeneratedAt int64
	Summary     wsDashboardSummary
	Workspaces  []wsDashboardRow
	Warnings    []string
	Detail      *wsDashboardDetail
	DetailCover *workspaceOutputCoverageSummary
}

type wsDashboardDetail struct {
	WorkspaceID string
	Risk        workspacerisk.WorkspaceRisk
	Repos       []repoRiskItem
	Tasks       wstask.Overview
}

func (c *CLI) runWSDashboard(args []string) int {
	opts, err := parseWSDashboardOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSDashboardUsage(c.Out)
			return exitOK
		}
		if opts.format == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.dashboard",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: err.Error(),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSDashboardUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeDashboardError(opts.format, "internal_error", fmt.Sprintf("get working dir: %v", err))
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeDashboardError(opts.format, "not_found", fmt.Sprintf("resolve KRA_ROOT: %v", err))
	}
	if err := c.ensureDebugLog(root, "ws-dashboard"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws dashboard scope=%s format=%s workspace=%s", opts.scope, opts.format, opts.workspace)

	buildStart := time.Now()
	result, err := buildWSDashboardResult(root, opts, c.debugf)
	if err != nil {
		code := "internal_error"
		if strings.Contains(err.Error(), "workspace not found") {
			code = "not_found"
		}
		return c.writeDashboardError(opts.format, code, err.Error())
	}
	debugPhasef(c.debugf, "ws-dashboard", "build_result", buildStart, "scope=%s count=%d", opts.scope, len(result.Workspaces))

	renderStart := time.Now()
	if opts.format == "json" {
		debugPhasef(c.debugf, "ws-dashboard", "render", renderStart, "format=json count=%d", len(result.Workspaces))
		return writeWSDashboardJSON(c.Out, result)
	}
	printWSDashboardHuman(c.Out, result, writerSupportsColor(c.Out))
	debugPhasef(c.debugf, "ws-dashboard", "render", renderStart, "format=human count=%d", len(result.Workspaces))
	return exitOK
}

func (c *CLI) writeDashboardError(format string, code string, message string) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "ws.dashboard",
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitError
	}
	fmt.Fprintf(c.Err, "%s\n", message)
	return exitError
}

func parseWSDashboardOptions(args []string) (wsDashboardOptions, error) {
	opts := wsDashboardOptions{
		scope:  "active",
		format: "human",
	}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsDashboardOptions{}, errHelpRequested
		case arg == "--archived":
			opts.scope = "archived"
			rest = rest[1:]
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case arg == "--format":
			if len(rest) < 2 {
				return wsDashboardOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--workspace="):
			opts.workspace = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
			rest = rest[1:]
		case arg == "--workspace":
			if len(rest) < 2 {
				return wsDashboardOptions{}, fmt.Errorf("--workspace requires a value")
			}
			opts.workspace = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return wsDashboardOptions{}, fmt.Errorf("unknown flag for ws dashboard: %q", arg)
		}
	}
	if len(rest) > 0 {
		return wsDashboardOptions{}, fmt.Errorf("unexpected args for ws dashboard: %q", strings.Join(rest, " "))
	}
	if opts.workspace != "" {
		if err := validateWorkspaceID(opts.workspace); err != nil {
			return wsDashboardOptions{}, fmt.Errorf("invalid --workspace: %w", err)
		}
		opts.showDetail = true
	}
	switch opts.format {
	case "human", "json":
	default:
		return wsDashboardOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	return opts, nil
}

func buildWSDashboardResult(root string, opts wsDashboardOptions, debugf func(string, ...any)) (wsDashboardResult, error) {
	ctx := context.Background()
	now := time.Now().Unix()
	rowsStart := time.Now()
	observer := func(phase string, scope string, workspaceID string, elapsed time.Duration) {
		if debugf == nil {
			return
		}
		if strings.TrimSpace(workspaceID) == "" {
			debugf("ws-dashboard phase=%s scope=%s elapsed_ms=%d", phase, scope, elapsed.Milliseconds())
			return
		}
		debugf("ws-dashboard phase=%s scope=%s workspace=%s elapsed_ms=%d", phase, scope, workspaceID, elapsed.Milliseconds())
	}
	rowResult, err := listRowsFromFilesystemObservedResult(ctx, root, opts.scope, true, observer)
	if err != nil {
		return wsDashboardResult{}, fmt.Errorf("list workspaces: %w", err)
	}
	rows := rowResult.Rows
	debugPhasef(debugf, "ws-dashboard", "list_scope", rowsStart, "scope=%s count=%d", opts.scope, len(rows))
	scopeCount := len(rows)
	if opts.workspace != "" {
		filterStart := time.Now()
		filtered := make([]wsListRow, 0, 1)
		for _, row := range rows {
			if row.ID == opts.workspace {
				filtered = append(filtered, row)
				break
			}
		}
		if len(filtered) == 0 {
			return wsDashboardResult{}, fmt.Errorf("workspace not found in %s scope: %s", opts.scope, opts.workspace)
		}
		rows = filtered
		debugPhasef(debugf, "ws-dashboard", "filter_workspace", filterStart, "workspace=%s", opts.workspace)
	}

	warnings := append([]string{}, rowResult.Warnings...)
	contextStart := time.Now()
	contextName, contextErr := resolveDashboardContextName(root)
	if contextErr != nil {
		contextName = root
		warnings = append(warnings, "context resolution is degraded; using root path as fallback context")
	}
	debugPhasef(debugf, "ws-dashboard", "resolve_context", contextStart, "scope=%s", opts.scope)

	riskByWorkspace := map[string]workspaceRiskDetail{}
	if opts.scope == "active" {
		riskStart := time.Now()
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		riskDetails, riskErr := collectWorkspaceRiskDetails(ctx, root, ids)
		if riskErr != nil {
			warnings = append(warnings, "workspace risk inspection is degraded; rows may show unknown risk")
		} else {
			for _, item := range riskDetails {
				riskByWorkspace[item.id] = item
			}
		}
		debugPhasef(debugf, "ws-dashboard", "collect_risk", riskStart, "scope=%s count=%d", opts.scope, len(ids))
	}

	items := make([]wsDashboardRow, 0, len(rows))
	coverageByWorkspace := make(map[string]workspaceOutputCoverageSummary, len(rows))
	taskByWorkspace := map[string]wstask.Overview{}
	taskSvc := newWorkspaceTaskService()
	for _, row := range rows {
		taskResult, taskErr := taskSvc.Overview(root, row.ID, opts.scope)
		if taskErr != nil {
			warnings = append(warnings, fmt.Sprintf("load workspace tasks %s: %v", row.ID, taskErr))
			taskByWorkspace[row.ID] = wstask.Overview{
				Summary: wstask.SummaryInvalid,
				Warning: taskErr.Error(),
			}
			continue
		}
		taskByWorkspace[row.ID] = taskResult.Overview
	}
	riskTotals := map[string]int{
		string(workspacerisk.WorkspaceRiskClean):    0,
		string(workspacerisk.WorkspaceRiskUnpushed): 0,
		string(workspacerisk.WorkspaceRiskDiverged): 0,
		string(workspacerisk.WorkspaceRiskDirty):    0,
		string(workspacerisk.WorkspaceRiskUnknown):  0,
	}
	coverageTotals := map[string]int{
		string(workspaceOutputCoverageEmpty):         0,
		string(workspaceOutputCoverageNotesOnly):     0,
		string(workspaceOutputCoverageArtifactsOnly): 0,
		string(workspaceOutputCoverageDocumented):    0,
	}
	for _, row := range rows {
		risk := workspacerisk.WorkspaceRiskUnknown
		if detail, ok := riskByWorkspace[row.ID]; ok {
			risk = detail.risk
		}
		coverage, coverageErr := deriveWorkspaceOutputCoverage(root, opts.scope, row.ID)
		if coverageErr != nil {
			warnings = append(warnings, fmt.Sprintf("output coverage scan is degraded for %s: %v", row.ID, coverageErr))
			coverage = workspaceOutputCoverageSummary{Coverage: workspaceOutputCoverageEmpty}
		}
		riskTotals[string(risk)]++
		coverageTotals[string(coverage.Coverage)]++
		coverageByWorkspace[row.ID] = coverage
		items = append(items, wsDashboardRow{
			ID:        row.ID,
			Title:     row.Title,
			Status:    row.Status,
			RepoCount: row.RepoCount,
			Risk:      risk,
			Coverage:  coverage.Coverage,
			Tasks:     taskByWorkspace[row.ID],
		})
	}

	summaryStart := time.Now()
	activeCount := 0
	archivedCount := 0
	switch opts.scope {
	case "active":
		activeCount = scopeCount
		archivedCount, _ = countWorkspaceDirectories(root, "archived")
	default:
		archivedCount = scopeCount
		activeCount, _ = countWorkspaceDirectories(root, "active")
	}
	debugPhasef(debugf, "ws-dashboard", "summary_counts", summaryStart, "active=%d archived=%d", activeCount, archivedCount)

	var detail *wsDashboardDetail
	var detailCoverage *workspaceOutputCoverageSummary
	if opts.showDetail {
		risk := workspacerisk.WorkspaceRiskUnknown
		repos := []repoRiskItem(nil)
		if d, ok := riskByWorkspace[opts.workspace]; ok {
			risk = d.risk
			repos = d.perRepo
		}
		detail = &wsDashboardDetail{
			WorkspaceID: opts.workspace,
			Risk:        risk,
			Repos:       repos,
			Tasks:       taskByWorkspace[opts.workspace],
		}
		if coverage, ok := coverageByWorkspace[opts.workspace]; ok {
			coverageCopy := coverage
			detailCoverage = &coverageCopy
		}
	}

	return wsDashboardResult{
		Root:        root,
		Context:     contextName,
		Scope:       opts.scope,
		GeneratedAt: now,
		Summary: wsDashboardSummary{
			Active:         activeCount,
			Archived:       archivedCount,
			RiskTotals:     riskTotals,
			CoverageTotals: coverageTotals,
		},
		Workspaces:  items,
		Warnings:    warnings,
		Detail:      detail,
		DetailCover: detailCoverage,
	}, nil
}

func countWorkspaceDirectories(root string, scope string) (int, error) {
	baseDir := filepath.Join(root, "workspaces")
	if scope == "archived" {
		baseDir = filepath.Join(root, "archive")
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateWorkspaceID(strings.TrimSpace(entry.Name())); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

func resolveDashboardContextName(root string) (string, error) {
	svc := contextcmd.NewService(appports.NewContextPort(resolveContextUseRoot))
	current, err := svc.Current(root)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(current), nil
}

func writeWSDashboardJSON(out io.Writer, result wsDashboardResult) int {
	items := make([]map[string]any, 0, len(result.Workspaces))
	for _, row := range result.Workspaces {
		items = append(items, map[string]any{
			"id":         row.ID,
			"title":      row.Title,
			"status":     row.Status,
			"risk":       string(row.Risk),
			"coverage":   string(row.Coverage),
			"repo_count": row.RepoCount,
			"tasks":      wsListJSONTaskSummary(row.Tasks),
		})
	}

	payload := map[string]any{
		"root":         result.Root,
		"context":      result.Context,
		"scope":        result.Scope,
		"generated_at": result.GeneratedAt,
		"summary": map[string]any{
			"active":          result.Summary.Active,
			"archived":        result.Summary.Archived,
			"risk_totals":     result.Summary.RiskTotals,
			"coverage_totals": result.Summary.CoverageTotals,
		},
		"workspaces": items,
		"warnings":   result.Warnings,
	}
	if result.Detail != nil {
		repos := make([]map[string]any, 0, len(result.Detail.Repos))
		for _, r := range result.Detail.Repos {
			repos = append(repos, map[string]any{
				"alias": r.alias,
				"risk":  string(r.state),
			})
		}
		activeItems := map[string]any{
			"doing":   wsDashboardJSONTaskItems(result.Detail.Tasks.ItemsByStatus(wstask.StatusDoing)),
			"blocked": wsDashboardJSONTaskItems(result.Detail.Tasks.ItemsByStatus(wstask.StatusBlocked)),
		}
		payload["detail"] = map[string]any{
			"workspace_id": result.Detail.WorkspaceID,
			"risk":         string(result.Detail.Risk),
			"repos":        repos,
			"tasks": map[string]any{
				"summary":      string(result.Detail.Tasks.Summary),
				"warning":      result.Detail.Tasks.Warning,
				"task_state":   string(result.Detail.Tasks.TaskState),
				"counts":       wsDashboardJSONTaskCounts(result.Detail.Tasks),
				"active_items": activeItems,
			},
		}
		if result.DetailCover != nil {
			payload["detail"].(map[string]any)["coverage"] = map[string]any{
				"state":           string(result.DetailCover.Coverage),
				"notes_count":     result.DetailCover.NotesCount,
				"artifacts_count": result.DetailCover.ArtifactsCount,
			}
		}
	}
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:     true,
		Action: "ws.dashboard",
		Result: payload,
	})
	return exitOK
}

func printWSDashboardHuman(out io.Writer, result wsDashboardResult, useColor bool) {
	header := []string{
		fmt.Sprintf("%s%s %s", uiIndent, styleMuted("•", useColor), result.Root),
		fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleAccent("context", useColor), result.Context),
		fmt.Sprintf("%s%s %s: %d", uiIndent, styleMuted("•", useColor), styleMuted("generated_at", useColor), result.GeneratedAt),
	}
	printSection(out, styleBold("Dashboard:", useColor), header, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})

	summary := []string{
		fmt.Sprintf("%s%s %s: %d", uiIndent, styleMuted("•", useColor), styleAccent("active", useColor), result.Summary.Active),
		fmt.Sprintf("%s%s %s: %d", uiIndent, styleMuted("•", useColor), styleMuted("archived", useColor), result.Summary.Archived),
		fmt.Sprintf("%s%s risk: clean=%d warning=%d danger=%d unknown=%d",
			uiIndent,
			styleMuted("•", useColor),
			result.Summary.RiskTotals[string(workspacerisk.WorkspaceRiskClean)],
			result.Summary.RiskTotals[string(workspacerisk.WorkspaceRiskUnpushed)]+result.Summary.RiskTotals[string(workspacerisk.WorkspaceRiskDiverged)],
			result.Summary.RiskTotals[string(workspacerisk.WorkspaceRiskDirty)],
			result.Summary.RiskTotals[string(workspacerisk.WorkspaceRiskUnknown)],
		),
		fmt.Sprintf("%s%s coverage: empty=%d notes-only=%d artifacts-only=%d documented=%d",
			uiIndent,
			styleMuted("•", useColor),
			result.Summary.CoverageTotals[string(workspaceOutputCoverageEmpty)],
			result.Summary.CoverageTotals[string(workspaceOutputCoverageNotesOnly)],
			result.Summary.CoverageTotals[string(workspaceOutputCoverageArtifactsOnly)],
			result.Summary.CoverageTotals[string(workspaceOutputCoverageDocumented)],
		),
	}
	printSection(out, styleBold("Summary:", useColor), summary, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})

	rows := make([]string, 0, len(result.Workspaces)+1)
	if len(result.Workspaces) == 0 {
		rows = append(rows, fmt.Sprintf("%s(none)", uiIndent))
	} else {
		for _, row := range result.Workspaces {
			line := fmt.Sprintf(
				"%s%s %s: %s  %s:%s  %s:%d  %s",
				uiIndent,
				styleMuted("•", useColor),
				row.ID,
				formatWorkspaceTitle(row.Title),
				styleMuted("risk", useColor),
				renderDashboardWorkspaceRisk(row.Risk, useColor),
				styleMuted("repos", useColor),
				row.RepoCount,
				styleMuted(row.Tasks.SummaryText(), useColor),
			)
			line += fmt.Sprintf("  %s:%s", styleMuted("coverage", useColor), renderDashboardCoverage(row.Coverage, useColor))
			rows = append(rows, line)
		}
	}
	printSection(out, renderWorkspacesTitle(result.Scope, useColor), rows, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})

	if result.Detail != nil {
		lines := make([]string, 0, len(result.Detail.Repos)+2)
		lines = append(lines, fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleAccent("workspace", useColor), result.Detail.WorkspaceID))
		lines = append(lines, fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("risk", useColor), renderDashboardWorkspaceRisk(result.Detail.Risk, useColor)))
		if result.DetailCover != nil {
			lines = append(lines, fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("coverage", useColor), renderDashboardCoverage(result.DetailCover.Coverage, useColor)))
			lines = append(lines, fmt.Sprintf("%s%s %s: notes=%d artifacts=%d", uiIndent, styleMuted("•", useColor), styleMuted("coverage_counts", useColor), result.DetailCover.NotesCount, result.DetailCover.ArtifactsCount))
		}
		for _, repo := range result.Detail.Repos {
			lines = append(lines, fmt.Sprintf("%s%s %s (%s)", uiIndent+uiIndent, styleMuted("-", useColor), repo.alias, renderRepoRiskState(repo.state, useColor)))
		}
		printSection(out, styleBold("Detail:", useColor), lines, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})

		taskLines := []string{fmt.Sprintf("%s%s", uiIndent, result.Detail.Tasks.SummaryText())}
		doing := result.Detail.Tasks.ItemsByStatus(wstask.StatusDoing)
		if len(doing) > 0 {
			taskLines = append(taskLines, fmt.Sprintf("%sDoing:", uiIndent))
			for _, item := range doing {
				taskLines = append(taskLines, fmt.Sprintf("%s%s%s: %s", uiIndent+uiIndent, styleMuted("-", useColor), item.ID, item.Title))
			}
		}
		blocked := result.Detail.Tasks.ItemsByStatus(wstask.StatusBlocked)
		if len(blocked) > 0 {
			taskLines = append(taskLines, fmt.Sprintf("%sBlocked:", uiIndent))
			for _, item := range blocked {
				taskLines = append(taskLines, fmt.Sprintf("%s%s%s: %s", uiIndent+uiIndent, styleMuted("-", useColor), item.ID, item.Title))
			}
		}
		for i := range taskLines {
			taskLines[i] = styleMuted(taskLines[i], useColor)
		}
		printSection(out, styleBold("Tasks:", useColor), taskLines, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})
	}

	if len(result.Warnings) > 0 {
		lines := make([]string, 0, len(result.Warnings))
		for _, w := range result.Warnings {
			lines = append(lines, fmt.Sprintf("%s%s %s", uiIndent, styleWarn("•", useColor), w))
		}
		printSection(out, styleBold(styleWarn("Warnings:", useColor), useColor), lines, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})
	}
}

func wsDashboardJSONTaskCounts(tasks wstask.Overview) map[string]any {
	if tasks.Summary != wstask.SummaryCounts {
		return nil
	}
	return map[string]any{
		"total":   tasks.Counts.Total,
		"doing":   tasks.Counts.Doing,
		"blocked": tasks.Counts.Blocked,
		"todo":    tasks.Counts.Todo,
		"done":    tasks.Counts.Done,
	}
}

func wsDashboardJSONTaskItems(items []wstask.Item) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id":     item.ID,
			"title":  item.Title,
			"status": string(item.Status),
		})
	}
	return out
}

func renderDashboardWorkspaceRisk(risk workspacerisk.WorkspaceRisk, useColor bool) string {
	switch risk {
	case workspacerisk.WorkspaceRiskDirty, workspacerisk.WorkspaceRiskUnknown:
		return styleError(string(risk), useColor)
	case workspacerisk.WorkspaceRiskDiverged, workspacerisk.WorkspaceRiskUnpushed:
		return styleWarn(string(risk), useColor)
	default:
		return styleMuted(string(risk), useColor)
	}
}

func renderDashboardCoverage(coverage workspaceOutputCoverage, useColor bool) string {
	switch coverage {
	case workspaceOutputCoverageDocumented:
		return styleSuccess(string(coverage), useColor)
	case workspaceOutputCoverageNotesOnly, workspaceOutputCoverageArtifactsOnly:
		return styleWarn(string(coverage), useColor)
	default:
		return styleError(string(coverage), useColor)
	}
}
