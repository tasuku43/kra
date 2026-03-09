package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/app/wstask"
	"github.com/tasuku43/kra/internal/infra/gitutil"
	"github.com/tasuku43/kra/internal/infra/paths"
	"github.com/tasuku43/kra/internal/infra/statestore"
)

type wsListOptions struct {
	tree   bool
	format string
	scope  string
}

type wsListRow struct {
	ID        string
	Status    string
	UpdatedAt int64
	RepoCount int
	Title     string
	WorkState workspaceWorkState
	Repos     []statestore.WorkspaceRepo
	Tasks     wstask.Overview
}

type wsListRowsResult struct {
	Rows     []wsListRow
	Warnings []string
}

type wsListPhaseObserver func(phase string, scope string, workspaceID string, elapsed time.Duration)

func (c *CLI) runWSList(args []string) int {
	requestedJSON := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--format" {
			if i+1 < len(args) && strings.TrimSpace(args[i+1]) == "json" {
				requestedJSON = true
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "--format=") && strings.TrimSpace(strings.TrimPrefix(arg, "--format=")) == "json" {
			requestedJSON = true
		}
	}

	opts, err := parseWSListOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSListUsage(c.Out)
			return exitOK
		}
		if requestedJSON {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.list",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: err.Error(),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSListUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		if opts.format == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.list",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("get working dir: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		if opts.format == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.list",
				Error: &cliJSONError{
					Code:    "not_found",
					Message: fmt.Sprintf("resolve KRA_ROOT: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "ws-list"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws list tree=%t format=%s scope=%s", opts.tree, opts.format, opts.scope)

	ctx := context.Background()
	registryStart := time.Now()
	if err := c.touchStateRegistry(root); err != nil {
		if opts.format == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.list",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("update root registry: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "update root registry: %v\n", err)
		return exitError
	}
	debugPhasef(c.debugf, "ws-list", "registry_touch", registryStart, "scope=%s", opts.scope)

	now := time.Now().Unix()
	rowsStart := time.Now()
	observer := func(phase string, scope string, workspaceID string, elapsed time.Duration) {
		if strings.TrimSpace(workspaceID) == "" {
			c.debugf("ws-list phase=%s scope=%s elapsed_ms=%d", phase, scope, elapsed.Milliseconds())
			return
		}
		c.debugf("ws-list phase=%s scope=%s workspace=%s elapsed_ms=%d", phase, scope, workspaceID, elapsed.Milliseconds())
	}
	listResult, usedFSFallback, err := buildWSListRowsResultObserved(ctx, root, opts.scope, now, opts.tree, observer)
	if err != nil {
		if opts.format == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.list",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("list workspaces: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "list workspaces: %v\n", err)
		return exitError
	}
	debugPhasef(c.debugf, "ws-list", "build_rows", rowsStart, "scope=%s count=%d", opts.scope, len(listResult.Rows))
	taskStart := time.Now()
	if err := hydrateWSListTaskOverviews(root, opts.scope, listResult.Rows); err != nil {
		if opts.format == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "ws.list",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("load workspace tasks: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "load workspace tasks: %v\n", err)
		return exitError
	}
	debugPhasef(c.debugf, "ws-list", "tasks", taskStart, "scope=%s count=%d", opts.scope, len(listResult.Rows))
	if usedFSFallback {
		c.debugf("ws list fallback to filesystem-only rows (state db unavailable)")
	}

	renderStart := time.Now()
	switch opts.format {
	case "tsv":
		printWSListTSV(c.Out, listResult.Rows)
	case "json":
		printWSListJSON(c.Out, listResult.Rows, listResult.Warnings, opts.scope, opts.tree)
	default:
		useColorOut := writerSupportsColor(c.Out)
		printWSListHuman(c.Out, listResult.Rows, listResult.Warnings, opts.scope, opts.tree, useColorOut)
	}
	debugPhasef(c.debugf, "ws-list", "render", renderStart, "scope=%s format=%s count=%d", opts.scope, opts.format, len(listResult.Rows))
	c.debugf("ws list completed count=%d", len(listResult.Rows))
	return exitOK
}

func buildWSListRows(ctx context.Context, root string, scope string, now int64, includeRepos bool) ([]wsListRow, bool, error) {
	result, usedFSFallback, err := buildWSListRowsResult(ctx, root, scope, now, includeRepos)
	return result.Rows, usedFSFallback, err
}

func buildWSListRowsResult(ctx context.Context, root string, scope string, now int64, includeRepos bool) (wsListRowsResult, bool, error) {
	return buildWSListRowsResultObserved(ctx, root, scope, now, includeRepos, nil)
}

func buildWSListRowsResultObserved(ctx context.Context, root string, scope string, now int64, includeRepos bool, observer wsListPhaseObserver) (wsListRowsResult, bool, error) {
	_ = now
	result, err := listRowsFromFilesystemObservedResult(ctx, root, scope, includeRepos, observer)
	if err != nil {
		return wsListRowsResult{}, false, err
	}
	return result, false, nil
}

func listRowsFromFilesystem(ctx context.Context, root string, scope string, includeRepos bool) ([]wsListRow, error) {
	result, err := listRowsFromFilesystemResult(ctx, root, scope, includeRepos)
	return result.Rows, err
}

func listRowsFromFilesystemResult(ctx context.Context, root string, scope string, includeRepos bool) (wsListRowsResult, error) {
	return listRowsFromFilesystemObservedResult(ctx, root, scope, includeRepos, nil)
}

func listRowsFromFilesystemObserved(ctx context.Context, root string, scope string, includeRepos bool, observer wsListPhaseObserver) ([]wsListRow, error) {
	result, err := listRowsFromFilesystemObservedResult(ctx, root, scope, includeRepos, observer)
	return result.Rows, err
}

func listRowsFromFilesystemObservedResult(ctx context.Context, root string, scope string, includeRepos bool, observer wsListPhaseObserver) (wsListRowsResult, error) {
	baseDir := filepath.Join(root, "workspaces")
	if scope == "archived" {
		baseDir = filepath.Join(root, "archive")
	}
	readDirStart := time.Now()
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return wsListRowsResult{}, err
	}
	if observer != nil {
		observer("scan_dir", scope, "", time.Since(readDirStart))
	}

	rows := make([]wsListRow, 0, len(entries))
	warnings := make([]string, 0)
	warningSeen := map[string]bool{}
	appendWarning := func(warning string) {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" || warningSeen[trimmed] {
			return
		}
		warningSeen[trimmed] = true
		warnings = append(warnings, trimmed)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := strings.TrimSpace(e.Name())
		if err := validateWorkspaceID(id); err != nil {
			continue
		}
		wsPath := filepath.Join(baseDir, id)
		meta, metaErr := loadWorkspaceMetaFile(wsPath)
		title := ""
		updatedAt := int64(0)
		if metaErr == nil {
			title = strings.TrimSpace(meta.Workspace.Title)
			updatedAt = meta.Workspace.UpdatedAt
		}
		if updatedAt <= 0 {
			fi, statErr := os.Stat(wsPath)
			if statErr == nil {
				updatedAt = fi.ModTime().Unix()
			}
		}
		repoCount := 0
		var repos []statestore.WorkspaceRepo
		if includeRepos {
			repoPhaseStart := time.Now()
			var listErr error
			repos, listErr = listWorkspaceReposFromFilesystem(ctx, root, scope, id, meta)
			if listErr != nil {
				return wsListRowsResult{}, listErr
			}
			repoCount = len(repos)
			if observer != nil {
				observer("repos", scope, id, time.Since(repoPhaseStart))
			}
		} else {
			repoPhaseStart := time.Now()
			var countErr error
			repoCount, countErr = countWorkspaceReposFromFilesystem(root, scope, id, meta)
			if countErr != nil {
				return wsListRowsResult{}, countErr
			}
			if observer != nil {
				observer("repo_count", scope, id, time.Since(repoPhaseStart))
			}
		}

		workState := workspaceWorkStateTodo
		if scope == "active" {
			workStateStart := time.Now()
			reposForState := repos
			if reposForState == nil {
				reposForState, err = listWorkspaceReposFromFilesystem(ctx, root, scope, id, meta)
				if err != nil {
					return wsListRowsResult{}, err
				}
			}
			var workStateWarnings []string
			workState, workStateWarnings = resolveWorkspaceListWorkState(ctx, root, id, reposForState, meta, metaErr)
			for _, warning := range workStateWarnings {
				appendWarning(warning)
			}
			if observer != nil {
				observer("work_state", scope, id, time.Since(workStateStart))
			}
		}
		rows = append(rows, wsListRow{
			ID:        id,
			Status:    scope,
			UpdatedAt: updatedAt,
			RepoCount: repoCount,
			Title:     title,
			WorkState: workState,
			Repos:     repos,
		})
	}

	slices.SortFunc(rows, func(a, b wsListRow) int {
		if scope == "active" {
			wa := workStateSortPriority(a.WorkState)
			wb := workStateSortPriority(b.WorkState)
			if wa != wb {
				if wa < wb {
					return -1
				}
				return 1
			}
		}
		if a.UpdatedAt != b.UpdatedAt {
			if a.UpdatedAt > b.UpdatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return wsListRowsResult{
		Rows:     rows,
		Warnings: warnings,
	}, nil
}

func hydrateWSListTaskOverviews(root string, scope string, rows []wsListRow) error {
	svc := newWorkspaceTaskService()
	for i := range rows {
		result, err := svc.Overview(root, rows[i].ID, scope)
		if err != nil {
			return err
		}
		rows[i].Tasks = result.Overview
	}
	return nil
}

func countWorkspaceReposFromFilesystem(root string, scope string, workspaceID string, meta workspaceMetaFile) (int, error) {
	wsBase := filepath.Join(root, "workspaces", workspaceID)
	if scope == "archived" {
		wsBase = filepath.Join(root, "archive", workspaceID)
	}
	reposDir := filepath.Join(wsBase, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}

	aliases := make(map[string]struct{}, len(entries)+len(meta.ReposRestore))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		alias := strings.TrimSpace(e.Name())
		if alias == "" {
			continue
		}
		aliases[alias] = struct{}{}
	}
	for _, r := range meta.ReposRestore {
		alias := strings.TrimSpace(r.Alias)
		if alias == "" {
			continue
		}
		aliases[alias] = struct{}{}
	}
	return len(aliases), nil
}

func listWorkspaceReposFromFilesystem(ctx context.Context, root string, scope string, workspaceID string, meta workspaceMetaFile) ([]statestore.WorkspaceRepo, error) {
	wsBase := filepath.Join(root, "workspaces", workspaceID)
	if scope == "archived" {
		wsBase = filepath.Join(root, "archive", workspaceID)
	}
	reposDir := filepath.Join(wsBase, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	restoreByAlias := map[string]workspaceMetaRepoRestore{}
	for _, r := range meta.ReposRestore {
		alias := strings.TrimSpace(r.Alias)
		if alias == "" {
			continue
		}
		restoreByAlias[alias] = r
	}

	repos := make([]statestore.WorkspaceRepo, 0, len(entries)+len(restoreByAlias))
	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		alias := strings.TrimSpace(e.Name())
		if alias == "" {
			continue
		}
		repoPath := filepath.Join(reposDir, alias)
		branch := ""
		if out, runErr := gitutil.Run(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD"); runErr == nil {
			branch = strings.TrimSpace(out)
		}
		restore := restoreByAlias[alias]
		repos = append(repos, statestore.WorkspaceRepo{
			RepoUID: strings.TrimSpace(restore.RepoUID),
			Alias:   alias,
			Branch:  firstNonEmpty(branch, strings.TrimSpace(restore.Branch)),
			BaseRef: strings.TrimSpace(restore.BaseRef),
		})
		seen[alias] = true
	}
	for alias, restore := range restoreByAlias {
		if seen[alias] {
			continue
		}
		repos = append(repos, statestore.WorkspaceRepo{
			RepoUID: strings.TrimSpace(restore.RepoUID),
			Alias:   alias,
			Branch:  strings.TrimSpace(restore.Branch),
			BaseRef: strings.TrimSpace(restore.BaseRef),
			MissingAt: sql.NullInt64{
				Int64: 1,
				Valid: scope == "active",
			},
		})
	}

	slices.SortFunc(repos, func(a, b statestore.WorkspaceRepo) int {
		return strings.Compare(a.Alias, b.Alias)
	})
	return repos, nil
}

var errHelpRequested = fmt.Errorf("help requested")

func parseWSListOptions(args []string) (wsListOptions, error) {
	opts := wsListOptions{
		tree:   false,
		format: "human",
		scope:  "active",
	}

	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsListOptions{}, errHelpRequested
		case arg == "--archived":
			opts.scope = "archived"
			rest = rest[1:]
		case arg == "--tree":
			opts.tree = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case arg == "--format":
			if len(rest) < 2 {
				return wsListOptions{}, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return wsListOptions{}, fmt.Errorf("unknown flag for ws list: %q", arg)
		}
	}

	if len(rest) > 0 {
		return wsListOptions{}, fmt.Errorf("unexpected args for ws list: %q", strings.Join(rest, " "))
	}
	switch opts.format {
	case "human", "tsv", "json":
	default:
		return wsListOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, tsv, json)", opts.format)
	}
	return opts, nil
}

func printWSListTSV(out io.Writer, rows []wsListRow) {
	fmt.Fprintln(out, "id\tstatus\tupdated_at\trepo_count\ttitle\ttask_summary\ttask_total\ttask_doing\ttask_blocked\ttask_todo\ttask_done")
	for _, row := range rows {
		taskTotal := ""
		taskDoing := ""
		taskBlocked := ""
		taskTodo := ""
		taskDone := ""
		taskSummary := string(row.Tasks.Summary)
		if taskSummary == "" {
			taskSummary = string(wstask.SummaryEmpty)
		}
		if row.Tasks.Summary == wstask.SummaryCounts {
			taskTotal = strconv.Itoa(row.Tasks.Counts.Total)
			taskDoing = strconv.Itoa(row.Tasks.Counts.Doing)
			taskBlocked = strconv.Itoa(row.Tasks.Counts.Blocked)
			taskTodo = strconv.Itoa(row.Tasks.Counts.Todo)
			taskDone = strconv.Itoa(row.Tasks.Counts.Done)
		}
		fmt.Fprintf(
			out,
			"%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.ID,
			row.Status,
			time.Unix(row.UpdatedAt, 0).UTC().Format(time.RFC3339),
			row.RepoCount,
			row.Title,
			taskSummary,
			taskTotal,
			taskDoing,
			taskBlocked,
			taskTodo,
			taskDone,
		)
	}
}

func printWSListJSON(out io.Writer, rows []wsListRow, warnings []string, scope string, tree bool) {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":         row.ID,
			"status":     row.Status,
			"updated_at": row.UpdatedAt,
			"repo_count": row.RepoCount,
			"title":      row.Title,
		}
		if tree {
			repos := make([]map[string]any, 0, len(row.Repos))
			for _, r := range row.Repos {
				repos = append(repos, map[string]any{
					"repo_uid": r.RepoUID,
					"alias":    r.Alias,
					"branch":   r.Branch,
					"base_ref": r.BaseRef,
					"missing":  r.MissingAt.Valid,
				})
			}
			item["repos"] = repos
		}
		item["tasks"] = wsListJSONTaskSummary(row.Tasks)
		items = append(items, item)
	}
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:     true,
		Action: "ws.list",
		Result: map[string]any{
			"scope":    scope,
			"tree":     tree,
			"items":    items,
			"warnings": warnings,
		},
	})
}

func printWSListHuman(out io.Writer, rows []wsListRow, warnings []string, scope string, tree bool, useColor bool) {
	body := make([]string, 0, len(rows)*2+1)
	if len(rows) == 0 {
		body = append(body, fmt.Sprintf("%s(none)", uiIndent))
		printSection(out, renderWorkspacesTitle(scope, useColor), body, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})
	} else {
		maxCols := listTerminalWidth()
		for _, row := range rows {
			body = append(body, renderWSListSummaryRow(row, maxCols, useColor))
			body = append(body, renderWSListTaskSummaryLine(row.Tasks, maxCols, useColor))

			if !tree {
				continue
			}
			body = append(body, renderWSListTreeLines(row.Repos, maxCols, useColor)...)
		}
		printSection(out, renderWorkspacesTitle(scope, useColor), body, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})
	}
	if len(warnings) == 0 {
		return
	}
	lines := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		lines = append(lines, fmt.Sprintf("%s%s %s", uiIndent, styleWarn("•", useColor), warning))
	}
	printSection(out, styleBold(styleWarn("Warnings:", useColor), useColor), lines, sectionRenderOptions{
		blankAfterHeading: true,
		trailingBlank:     true,
	})
}

func resolveWorkspaceListWorkState(
	ctx context.Context,
	root string,
	workspaceID string,
	repos []statestore.WorkspaceRepo,
	meta workspaceMetaFile,
	metaErr error,
) (workspaceWorkState, []string) {
	if metaErr != nil {
		return workspaceWorkStateTodo, nil
	}
	storedRaw := strings.TrimSpace(meta.Workspace.WorkState)
	if storedRaw != "" {
		return normalizeWorkspaceWorkState(workspaceWorkState(storedRaw)), nil
	}

	warnings := []string{formatWorkspaceListMissingWorkStateWarning(workspaceID)}
	if meta.Baseline == nil {
		warnings = append(warnings, formatWorkspaceListMissingBaselineWarning(workspaceID))
		return deriveWorkspaceWorkStateFromRepoRisk(ctx, root, workspaceID, repos), warnings
	}

	state, err := deriveWorkspaceWorkStateFromBaseline(ctx, root, workspaceID, repos)
	if err != nil {
		warnings = append(warnings, formatWorkspaceListDegradedBaselineWarning(workspaceID))
		return deriveWorkspaceWorkStateFromRepoRisk(ctx, root, workspaceID, repos), warnings
	}
	return state, warnings
}

func formatWorkspaceListMissingWorkStateWarning(workspaceID string) string {
	return fmt.Sprintf("workspace %s is missing .kra.meta.json.workspace.work_state; run kra doctor --fix --plan", workspaceID)
}

func formatWorkspaceListMissingBaselineWarning(workspaceID string) string {
	return fmt.Sprintf("workspace %s is missing .kra.meta.json.baseline; run kra doctor --fix --plan", workspaceID)
}

func formatWorkspaceListDegradedBaselineWarning(workspaceID string) string {
	return fmt.Sprintf("workspace %s uses fallback work-state ordering because baseline inspection is degraded; run kra doctor", workspaceID)
}

func renderWSListSummaryRow(row wsListRow, maxCols int, useColor bool) string {
	idPlain := strings.TrimSpace(row.ID)
	if idPlain == "" {
		idPlain = "(unknown)"
	}
	desc := formatWorkspaceTitle(row.Title)

	separatorPlain := ": "
	mark := wsListMarkerForWorkState(normalizeWorkspaceWorkState(row.WorkState))
	prefixPlain := fmt.Sprintf("%s%s %s%s", uiIndent, mark, idPlain, separatorPlain)
	availableDescCols := maxCols - displayWidth(prefixPlain)
	if availableDescCols < 8 {
		availableDescCols = 8
	}
	desc = truncateDisplay(desc, availableDescCols)

	markerText := mark
	separator := separatorPlain
	if useColor {
		markerText = styleMuted(markerText, useColor)
		separator = styleMuted(separatorPlain, useColor)
	}
	line := fmt.Sprintf("%s%s %s%s", uiIndent, markerText, idPlain, separator) + desc
	return truncateDisplay(line, maxCols)
}

func renderWSListTaskSummaryLine(tasks wstask.Overview, maxCols int, useColor bool) string {
	line := uiIndent + uiIndent + tasks.SummaryText()
	line = truncateDisplay(line, maxCols)
	if useColor {
		line = styleMuted(line, useColor)
	}
	return line
}

func renderWSListTreeLines(repos []statestore.WorkspaceRepo, maxCols int, useColor bool) []string {
	repoIndent := uiIndent + uiIndent
	if len(repos) == 0 {
		line := repoIndent + "(no repos)"
		if useColor {
			line = styleMuted(line, useColor)
		}
		return []string{line}
	}
	lines := make([]string, 0, len(repos))
	for _, repo := range repos {
		state := "tracked"
		if repo.MissingAt.Valid {
			state = "missing"
		}
		line := fmt.Sprintf("%s- %s  branch:%s  state:%s", repoIndent, repo.Alias, repo.Branch, state)
		line = truncateDisplay(line, maxCols)
		if useColor {
			line = styleMuted(line, useColor)
		}
		lines = append(lines, line)
	}
	return lines
}

func formatWorkspaceTitle(title string) string {
	desc := strings.TrimSpace(title)
	if desc == "" {
		return "(no title)"
	}
	return desc
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func listTerminalWidth() int {
	const fallback = 120
	raw := strings.TrimSpace(os.Getenv("COLUMNS"))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 60 {
		return fallback
	}
	return v
}

func wsListJSONTaskSummary(tasks wstask.Overview) map[string]any {
	summary := tasks.Summary
	if summary == "" {
		summary = wstask.SummaryEmpty
	}
	out := map[string]any{
		"summary": string(summary),
	}
	if summary != wstask.SummaryInvalid {
		out["task_state"] = string(tasks.TaskState)
	}
	if summary == wstask.SummaryCounts {
		out["counts"] = map[string]any{
			"total":   tasks.Counts.Total,
			"doing":   tasks.Counts.Doing,
			"blocked": tasks.Counts.Blocked,
			"todo":    tasks.Counts.Todo,
			"done":    tasks.Counts.Done,
		}
	}
	if strings.TrimSpace(tasks.Warning) != "" {
		out["warning"] = tasks.Warning
	}
	return out
}
