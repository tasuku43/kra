package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

type wsListOptions struct {
	tree   bool
	format string
	scope  string
}

type wsListRow struct {
	ID          string
	Status      string
	UpdatedAt   int64
	RepoCount   int
	Risk        workspacerisk.WorkspaceRisk
	Description string
	Repos       []statestore.WorkspaceRepo
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
	if err := c.touchStateRegistry(root, dbPath); err != nil {
		fmt.Fprintf(c.Err, "update state registry: %v\n", err)
		return exitError
	}

	now := time.Now().Unix()
	if err := importWorkspaceDirs(ctx, db, root, now); err != nil {
		fmt.Fprintf(c.Err, "import workspace dirs: %v\n", err)
		return exitError
	}
	if err := markMissingRepos(ctx, db, root, now); err != nil {
		fmt.Fprintf(c.Err, "mark missing repos: %v\n", err)
		return exitError
	}

	items, err := statestore.ListWorkspaces(ctx, db)
	if err != nil {
		fmt.Fprintf(c.Err, "list workspaces: %v\n", err)
		return exitError
	}

	rows := make([]wsListRow, 0, len(items))
	for _, it := range items {
		if it.Status != opts.scope {
			continue
		}
		repos, err := statestore.ListWorkspaceRepos(ctx, db, it.ID)
		if err != nil {
			fmt.Fprintf(c.Err, "list workspace repos: %v\n", err)
			return exitError
		}
		rows = append(rows, wsListRow{
			ID:          it.ID,
			Status:      it.Status,
			UpdatedAt:   it.UpdatedAt,
			RepoCount:   it.RepoCount,
			Risk:        computeWorkspaceRisk(ctx, root, it.ID, it.Status, repos),
			Description: strings.TrimSpace(it.Description),
			Repos:       repos,
		})
	}

	switch opts.format {
	case "tsv":
		printWSListTSV(c.Out, rows)
	default:
		useColorOut := writerSupportsColor(c.Out)
		printWSListHuman(c.Out, rows, opts.scope, opts.tree, useColorOut)
	}
	c.debugf("ws list completed count=%d", len(items))
	return exitOK
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
	fmt.Fprintln(out, "id\tstatus\tupdated_at\trepo_count\trisk\tdescription")
	for _, row := range rows {
		fmt.Fprintf(
			out,
			"%s\t%s\t%s\t%d\t%s\t%s\n",
			row.ID,
			row.Status,
			time.Unix(row.UpdatedAt, 0).UTC().Format(time.RFC3339),
			row.RepoCount,
			formatWorkspaceRisk(row.Risk),
			row.Description,
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
	desc := row.Description
	if desc == "" {
		desc = "(no description)"
	}

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

func importWorkspaceDirs(ctx context.Context, db *sql.DB, root string, now int64) error {
	entries, err := os.ReadDir(filepath.Join(root, "workspaces"))
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if err := validateWorkspaceID(id); err != nil {
			continue
		}
		if _, ok, err := statestore.LookupWorkspaceStatus(ctx, db, id); err != nil {
			return err
		} else if ok {
			continue
		}

		if _, err := statestore.CreateWorkspace(ctx, db, statestore.CreateWorkspaceInput{
			ID:          id,
			Description: "",
			SourceURL:   "",
			Now:         now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func markMissingRepos(ctx context.Context, db *sql.DB, root string, now int64) error {
	items, err := statestore.ListWorkspaces(ctx, db)
	if err != nil {
		return err
	}

	for _, it := range items {
		if it.Status != "active" {
			continue
		}
		repos, err := statestore.ListWorkspaceRepos(ctx, db, it.ID)
		if err != nil {
			return err
		}

		for _, r := range repos {
			if r.MissingAt.Valid {
				continue
			}
			p := filepath.Join(root, "workspaces", it.ID, "repos", r.Alias)
			if _, err := os.Stat(p); err == nil {
				continue
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}

			if _, err := statestore.MarkWorkspaceRepoMissing(ctx, db, it.ID, r.RepoUID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func computeWorkspaceRisk(ctx context.Context, root string, workspaceID string, status string, repos []statestore.WorkspaceRepo) workspacerisk.WorkspaceRisk {
	if status != "active" {
		return workspacerisk.WorkspaceRiskClean
	}
	risk, _ := inspectWorkspaceRepoRisk(ctx, root, workspaceID, repos)
	return risk
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
