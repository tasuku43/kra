package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/statestore"
)

func (c *CLI) runWSList(args []string) int {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printWSListUsage(c.Out)
			return exitOK
		default:
			fmt.Fprintf(c.Err, "unknown flag for ws list: %q\n", args[0])
			c.printWSListUsage(c.Err)
			return exitUsage
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for ws list: %q\n", strings.Join(args, " "))
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

	fmt.Fprintln(c.Out, "id\tstatus\tupdated_at\trepo_count\trisk\tdescription")
	for _, it := range items {
		repos, err := statestore.ListWorkspaceRepos(ctx, db, it.ID)
		if err != nil {
			fmt.Fprintf(c.Err, "list workspace repos: %v\n", err)
			return exitError
		}
		risk := computeWorkspaceRisk(root, it.ID, repos)
		fmt.Fprintf(
			c.Out,
			"%s\t%s\t%s\t%d\t%s\t%s\n",
			it.ID,
			it.Status,
			time.Unix(it.UpdatedAt, 0).UTC().Format(time.RFC3339),
			it.RepoCount,
			risk,
			it.Description,
		)
	}
	return exitOK
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

func computeWorkspaceRisk(root string, workspaceID string, repos []statestore.WorkspaceRepo) string {
	_ = root
	_ = workspaceID
	if len(repos) == 0 {
		return "clean"
	}

	overall := "unknown"
	for _, r := range repos {
		if r.MissingAt.Valid {
			return "unknown"
		}
	}
	return overall
}
