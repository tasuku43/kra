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

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/infra/gitutil"
	"github.com/tasuku43/gionx/internal/infra/paths"
	"github.com/tasuku43/gionx/internal/infra/statestore"
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
	Risk      workspacerisk.WorkspaceRisk
	WorkState string
	Title     string
	Repos     []statestore.WorkspaceRepo
}

func (c *CLI) runWSList(args []string) int {
	opts, err := parseWSListOptions(args)
	if err != nil {
		if err == errHelpRequested {
			c.printWSListUsage(c.Out)
			return exitOK
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSListUsage(c.Err)
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
	if err := c.ensureDebugLog(root, "ws-list"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run ws list tree=%t format=%s scope=%s", opts.tree, opts.format, opts.scope)

	ctx := context.Background()
	if err := c.touchStateRegistry(root); err != nil {
		fmt.Fprintf(c.Err, "update root registry: %v\n", err)
		return exitError
	}

	now := time.Now().Unix()
	rows, usedFSFallback, err := buildWSListRows(ctx, root, opts.scope, now)
	if err != nil {
		fmt.Fprintf(c.Err, "list workspaces: %v\n", err)
		return exitError
	}
	if usedFSFallback {
		c.debugf("ws list fallback to filesystem-only rows (state db unavailable)")
	}

	switch opts.format {
	case "tsv":
		printWSListTSV(c.Out, rows)
	default:
		useColorOut := writerSupportsColor(c.Out)
		printWSListHuman(c.Out, rows, opts.scope, opts.tree, useColorOut)
	}
	c.debugf("ws list completed count=%d", len(rows))
	return exitOK
}

func buildWSListRows(ctx context.Context, root string, scope string, now int64) ([]wsListRow, bool, error) {
	_ = now
	rows, err := listRowsFromFilesystem(ctx, root, scope)
	if err != nil {
		return nil, false, err
	}
	return rows, false, nil
}

func listRowsFromFilesystem(ctx context.Context, root string, scope string) ([]wsListRow, error) {
	baseDir := filepath.Join(root, "workspaces")
	if scope == "archived" {
		baseDir = filepath.Join(root, "archive")
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	rows := make([]wsListRow, 0, len(entries))
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
		repos, err := listWorkspaceReposFromFilesystem(ctx, root, scope, id, meta)
		if err != nil {
			return nil, err
		}
		rows = append(rows, wsListRow{
			ID:        id,
			Status:    scope,
			UpdatedAt: updatedAt,
			RepoCount: len(repos),
			Risk:      computeWorkspaceRisk(ctx, root, id, scope, repos),
			WorkState: deriveLogicalWorkState(ctx, root, id, scope, repos),
			Title:     title,
			Repos:     repos,
		})
	}

	slices.SortFunc(rows, func(a, b wsListRow) int {
		if a.UpdatedAt != b.UpdatedAt {
			if a.UpdatedAt > b.UpdatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return rows, nil
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
	case "human", "tsv":
	default:
		return wsListOptions{}, fmt.Errorf("unsupported --format: %q (supported: human, tsv)", opts.format)
	}
	return opts, nil
}

func printWSListTSV(out io.Writer, rows []wsListRow) {
	fmt.Fprintln(out, "id\tstatus\tupdated_at\trepo_count\trisk\twork_state\ttitle")
	for _, row := range rows {
		fmt.Fprintf(
			out,
			"%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			row.ID,
			row.Status,
			time.Unix(row.UpdatedAt, 0).UTC().Format(time.RFC3339),
			row.RepoCount,
			formatWorkspaceRisk(row.Risk),
			row.WorkState,
			row.Title,
		)
	}
}

func printWSListHuman(out io.Writer, rows []wsListRow, scope string, tree bool, useColor bool) {
	fmt.Fprintln(out, renderWorkspacesTitle(scope, useColor))
	fmt.Fprintln(out)

	if len(rows) == 0 {
		fmt.Fprintf(out, "%s(none)\n", uiIndent)
		return
	}

	idWidth := len("workspace")
	for _, row := range rows {
		if n := displayWidth(row.ID); n > idWidth {
			idWidth = n
		}
	}
	if idWidth < 10 {
		idWidth = 10
	}
	if idWidth > 24 {
		idWidth = 24
	}

	repoWidth := len("repos:99")
	for _, row := range rows {
		repoToken := fmt.Sprintf("repos:%d", row.RepoCount)
		if n := displayWidth(repoToken); n > repoWidth {
			repoWidth = n
		}
	}

	riskWidth := 1

	maxCols := listTerminalWidth()
	for _, row := range rows {
		fmt.Fprintln(out, renderWSListSummaryRow(row, idWidth, riskWidth, repoWidth, maxCols, useColor))

		if !tree {
			continue
		}
		printWSListTreeLines(out, row.Repos, maxCols, useColor)
	}
}

func renderWSListSummaryRow(row wsListRow, idWidth int, riskWidth int, repoWidth int, maxCols int, useColor bool) string {
	idPlain := fmt.Sprintf("%-*s", idWidth, truncateDisplay(row.ID, idWidth))
	riskPlain := fmt.Sprintf("%-*s", riskWidth, renderWorkspaceRiskIndicator(row.Risk, false))
	repoPlain := fmt.Sprintf("%-*s", repoWidth, fmt.Sprintf("repos:%d", row.RepoCount))
	desc := formatWorkspaceTitleWithLogicalState(row.WorkState, row.Title)

	prefixPlain := fmt.Sprintf("%s%s  %s  %s  ", uiIndent, idPlain, riskPlain, repoPlain)
	availableDescCols := maxCols - displayWidth(prefixPlain)
	if availableDescCols < 8 {
		availableDescCols = 8
	}
	desc = truncateDisplay(desc, availableDescCols)

	idText := colorizeRiskID(idPlain, row.Risk, useColor)
	riskText := fmt.Sprintf("%-*s", riskWidth, renderWorkspaceRiskIndicator(row.Risk, useColor))

	line := fmt.Sprintf("%s%s  %s  %s  %s", uiIndent, idText, riskText, repoPlain, desc)
	return truncateDisplay(line, maxCols)
}

func printWSListTreeLines(out io.Writer, repos []statestore.WorkspaceRepo, maxCols int, useColor bool) {
	repoIndent := uiIndent + uiIndent
	if len(repos) == 0 {
		line := repoIndent + "(no repos)"
		if useColor {
			line = styleMuted(line, true)
		}
		fmt.Fprintln(out, line)
		return
	}
	for _, repo := range repos {
		state := "tracked"
		if repo.MissingAt.Valid {
			state = "missing"
		}
		line := fmt.Sprintf("%s- %s  branch:%s  state:%s", repoIndent, repo.Alias, repo.Branch, state)
		line = truncateDisplay(line, maxCols)
		if useColor {
			line = styleMuted(line, true)
		}
		fmt.Fprintln(out, line)
	}
}

func computeWorkspaceRisk(ctx context.Context, root string, workspaceID string, status string, repos []statestore.WorkspaceRepo) workspacerisk.WorkspaceRisk {
	if status != "active" {
		return workspacerisk.WorkspaceRiskClean
	}
	risk, _ := inspectWorkspaceRepoRisk(ctx, root, workspaceID, repos)
	return risk
}

func deriveLogicalWorkState(ctx context.Context, root string, workspaceID string, status string, repos []statestore.WorkspaceRepo) string {
	if status != "active" {
		return ""
	}
	if len(repos) == 0 {
		return "todo"
	}
	for _, repo := range repos {
		if repo.MissingAt.Valid {
			return "in-progress"
		}
		worktreePath := filepath.Join(root, "workspaces", workspaceID, "repos", repo.Alias)
		repoStatus := inspectGitRepoStatus(ctx, worktreePath)
		if workspacerisk.ClassifyRepoStatus(repoStatus) != workspacerisk.RepoStateClean {
			return "in-progress"
		}
		started, known := hasCommitsSinceBaseRef(ctx, worktreePath, repo.BaseRef)
		if !known || started {
			return "in-progress"
		}
	}
	return "todo"
}

func hasCommitsSinceBaseRef(ctx context.Context, worktreePath string, baseRef string) (started bool, known bool) {
	baseRef = strings.TrimSpace(baseRef)
	if baseRef == "" {
		baseRef = detectOriginHeadBaseRef(ctx, worktreePath)
	}
	if baseRef == "" {
		return false, false
	}
	out, err := gitutil.Run(ctx, worktreePath, "rev-list", "--count", baseRef+"..HEAD")
	if err != nil {
		return false, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return false, false
	}
	return n > 0, true
}

func detectOriginHeadBaseRef(ctx context.Context, worktreePath string) string {
	out, err := gitutil.Run(ctx, worktreePath, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(out)
	const pfx = "refs/remotes/origin/"
	if !strings.HasPrefix(ref, pfx) {
		return ""
	}
	branch := strings.TrimSpace(strings.TrimPrefix(ref, pfx))
	if branch == "" {
		return ""
	}
	return "origin/" + branch
}

func formatWorkspaceTitleWithLogicalState(workState string, title string) string {
	desc := strings.TrimSpace(title)
	if workState == "" {
		if desc == "" {
			return "(no title)"
		}
		return desc
	}
	if desc == "" {
		return workState
	}
	return workState + " | " + desc
}

func renderWorkspaceRiskIndicator(risk workspacerisk.WorkspaceRisk, useColor bool) string {
	text := "*"
	if !useColor {
		return text
	}
	switch risk {
	case workspacerisk.WorkspaceRiskDirty, workspacerisk.WorkspaceRiskUnknown:
		return styleError(text, true)
	case workspacerisk.WorkspaceRiskDiverged, workspacerisk.WorkspaceRiskUnpushed:
		return styleWarn(text, true)
	default:
		return styleMuted(text, true)
	}
}

func formatWorkspaceRisk(risk workspacerisk.WorkspaceRisk) string {
	switch risk {
	case workspacerisk.WorkspaceRiskDirty:
		return "dirty"
	case workspacerisk.WorkspaceRiskDiverged:
		return "diverged"
	case workspacerisk.WorkspaceRiskUnpushed:
		return "unpushed"
	case workspacerisk.WorkspaceRiskUnknown:
		return "unknown"
	default:
		return "clean"
	}
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
