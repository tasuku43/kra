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
	Repos     []statestore.WorkspaceRepo
}

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

	now := time.Now().Unix()
	rows, usedFSFallback, err := buildWSListRows(ctx, root, opts.scope, now, opts.tree)
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
	if usedFSFallback {
		c.debugf("ws list fallback to filesystem-only rows (state db unavailable)")
	}

	switch opts.format {
	case "tsv":
		printWSListTSV(c.Out, rows)
	case "json":
		printWSListJSON(c.Out, rows, opts.scope, opts.tree)
	default:
		useColorOut := writerSupportsColor(c.Out)
		printWSListHuman(c.Out, rows, opts.scope, opts.tree, useColorOut)
	}
	c.debugf("ws list completed count=%d", len(rows))
	return exitOK
}

func buildWSListRows(ctx context.Context, root string, scope string, now int64, includeRepos bool) ([]wsListRow, bool, error) {
	_ = now
	rows, err := listRowsFromFilesystem(ctx, root, scope, includeRepos)
	if err != nil {
		return nil, false, err
	}
	return rows, false, nil
}

func listRowsFromFilesystem(ctx context.Context, root string, scope string, includeRepos bool) ([]wsListRow, error) {
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
		repoCount := 0
		var repos []statestore.WorkspaceRepo
		if includeRepos {
			var listErr error
			repos, listErr = listWorkspaceReposFromFilesystem(ctx, root, scope, id, meta)
			if listErr != nil {
				return nil, listErr
			}
			repoCount = len(repos)
		} else {
			var countErr error
			repoCount, countErr = countWorkspaceReposFromFilesystem(root, scope, id, meta)
			if countErr != nil {
				return nil, countErr
			}
		}
		rows = append(rows, wsListRow{
			ID:        id,
			Status:    scope,
			UpdatedAt: updatedAt,
			RepoCount: repoCount,
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
	fmt.Fprintln(out, "id\tstatus\tupdated_at\trepo_count\ttitle")
	for _, row := range rows {
		fmt.Fprintf(
			out,
			"%s\t%s\t%s\t%d\t%s\n",
			row.ID,
			row.Status,
			time.Unix(row.UpdatedAt, 0).UTC().Format(time.RFC3339),
			row.RepoCount,
			row.Title,
		)
	}
}

func printWSListJSON(out io.Writer, rows []wsListRow, scope string, tree bool) {
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
		items = append(items, item)
	}
	_ = writeCLIJSON(out, cliJSONResponse{
		OK:     true,
		Action: "ws.list",
		Result: map[string]any{
			"scope": scope,
			"tree":  tree,
			"items": items,
		},
	})
}

func printWSListHuman(out io.Writer, rows []wsListRow, scope string, tree bool, useColor bool) {
	body := make([]string, 0, len(rows)*2+1)
	if len(rows) == 0 {
		body = append(body, fmt.Sprintf("%s(none)", uiIndent))
		printSection(out, renderWorkspacesTitle(scope, useColor), body, sectionRenderOptions{
			blankAfterHeading: true,
			trailingBlank:     true,
		})
		return
	}

	maxCols := listTerminalWidth()
	for _, row := range rows {
		body = append(body, renderWSListSummaryRow(row, maxCols, useColor))

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

func renderWSListSummaryRow(row wsListRow, maxCols int, useColor bool) string {
	idPlain := strings.TrimSpace(row.ID)
	if idPlain == "" {
		idPlain = "(unknown)"
	}
	desc := formatWorkspaceTitle(row.Title)

	separatorPlain := ": "
	prefixPlain := fmt.Sprintf("%s• %s%s", uiIndent, idPlain, separatorPlain)
	availableDescCols := maxCols - displayWidth(prefixPlain)
	if availableDescCols < 8 {
		availableDescCols = 8
	}
	desc = truncateDisplay(desc, availableDescCols)

	bullet := "•"
	separator := separatorPlain
	if useColor {
		bullet = styleMuted(bullet, useColor)
		separator = styleMuted(separatorPlain, useColor)
	}
	line := fmt.Sprintf("%s%s %s%s", uiIndent, bullet, idPlain, separator) + desc
	return truncateDisplay(line, maxCols)
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
