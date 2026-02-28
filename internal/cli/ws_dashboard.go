package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/app/contextcmd"
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
}

type wsDashboardSummary struct {
	Active     int
	Archived   int
	RiskTotals map[string]int
}

type wsDashboardResult struct {
	Root        string
	Context     string
	Scope       string
	GeneratedAt int64
	Summary     wsDashboardSummary
	Workspaces  []wsDashboardRow
	Warnings    []string
	Detail      *workspaceRiskDetail
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

	result, err := buildWSDashboardResult(root, opts)
	if err != nil {
		code := "internal_error"
		if strings.Contains(err.Error(), "workspace not found") {
			code = "not_found"
		}
		return c.writeDashboardError(opts.format, code, err.Error())
	}

	if opts.format == "json" {
		return writeWSDashboardJSON(c.Out, result)
	}
	printWSDashboardHuman(c.Out, result, writerSupportsColor(c.Out))
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

func buildWSDashboardResult(root string, opts wsDashboardOptions) (wsDashboardResult, error) {
	ctx := context.Background()
	now := time.Now().Unix()
	rows, err := listRowsFromFilesystem(ctx, root, opts.scope, true)
	if err != nil {
		return wsDashboardResult{}, fmt.Errorf("list workspaces: %w", err)
	}
	if opts.workspace != "" {
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
	}

	warnings := make([]string, 0, 2)
	contextName, contextErr := resolveDashboardContextName(root)
	if contextErr != nil {
		contextName = root
		warnings = append(warnings, fmt.Sprintf("resolve context: %v", contextErr))
	}

	riskByWorkspace := map[string]workspaceRiskDetail{}
	if opts.scope == "active" {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		riskDetails, riskErr := collectWorkspaceRiskDetails(ctx, root, ids)
		if riskErr != nil {
			warnings = append(warnings, fmt.Sprintf("inspect workspace risk: %v", riskErr))
		} else {
			for _, item := range riskDetails {
				riskByWorkspace[item.id] = item
			}
		}
	}

	items := make([]wsDashboardRow, 0, len(rows))
	riskTotals := map[string]int{
		string(workspacerisk.WorkspaceRiskClean):    0,
		string(workspacerisk.WorkspaceRiskUnpushed): 0,
		string(workspacerisk.WorkspaceRiskDiverged): 0,
		string(workspacerisk.WorkspaceRiskDirty):    0,
		string(workspacerisk.WorkspaceRiskUnknown):  0,
	}
	for _, row := range rows {
		risk := workspacerisk.WorkspaceRiskUnknown
		if detail, ok := riskByWorkspace[row.ID]; ok {
			risk = detail.risk
		}
		riskTotals[string(risk)]++
		items = append(items, wsDashboardRow{
			ID:        row.ID,
			Title:     row.Title,
			Status:    row.Status,
			RepoCount: row.RepoCount,
			Risk:      risk,
		})
	}

	activeRows, _ := listRowsFromFilesystem(ctx, root, "active", false)
	archivedRows, _ := listRowsFromFilesystem(ctx, root, "archived", false)

	var detail *workspaceRiskDetail
	if opts.showDetail {
		if d, ok := riskByWorkspace[opts.workspace]; ok {
			detail = &d
		}
	}

	return wsDashboardResult{
		Root:        root,
		Context:     contextName,
		Scope:       opts.scope,
		GeneratedAt: now,
		Summary: wsDashboardSummary{
			Active:     len(activeRows),
			Archived:   len(archivedRows),
			RiskTotals: riskTotals,
		},
		Workspaces: items,
		Warnings:   warnings,
		Detail:     detail,
	}, nil
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
			"repo_count": row.RepoCount,
		})
	}

	payload := map[string]any{
		"root":         result.Root,
		"context":      result.Context,
		"scope":        result.Scope,
		"generated_at": result.GeneratedAt,
		"summary": map[string]any{
			"active":      result.Summary.Active,
			"archived":    result.Summary.Archived,
			"risk_totals": result.Summary.RiskTotals,
		},
		"workspaces": items,
		"warnings":   result.Warnings,
	}
	if result.Detail != nil {
		repos := make([]map[string]any, 0, len(result.Detail.perRepo))
		for _, r := range result.Detail.perRepo {
			repos = append(repos, map[string]any{
				"alias": r.alias,
				"risk":  string(r.state),
			})
		}
		payload["detail"] = map[string]any{
			"workspace_id": result.Detail.id,
			"risk":         string(result.Detail.risk),
			"repos":        repos,
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
				"%s%s %s: %s  %s:%s  %s:%d",
				uiIndent,
				styleMuted("•", useColor),
				row.ID,
				formatWorkspaceTitle(row.Title),
				styleMuted("risk", useColor),
				renderDashboardWorkspaceRisk(row.Risk, useColor),
				styleMuted("repos", useColor),
				row.RepoCount,
			)
			rows = append(rows, line)
		}
	}
	printSection(out, renderWorkspacesTitle(result.Scope, useColor), rows, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})

	if result.Detail != nil {
		lines := make([]string, 0, len(result.Detail.perRepo)+2)
		lines = append(lines, fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleAccent("workspace", useColor), result.Detail.id))
		lines = append(lines, fmt.Sprintf("%s%s %s: %s", uiIndent, styleMuted("•", useColor), styleMuted("risk", useColor), renderDashboardWorkspaceRisk(result.Detail.risk, useColor)))
		for _, repo := range result.Detail.perRepo {
			lines = append(lines, fmt.Sprintf("%s%s %s (%s)", uiIndent+uiIndent, styleMuted("-", useColor), repo.alias, renderRepoRiskState(repo.state, useColor)))
		}
		printSection(out, styleBold("Detail:", useColor), lines, sectionRenderOptions{
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
