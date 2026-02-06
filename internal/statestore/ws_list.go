package statestore

import (
	"context"
	"database/sql"
	"fmt"
)

type WorkspaceListItem struct {
	ID          string
	Status      string
	UpdatedAt   int64
	RepoCount   int
	Description string
}

func ListWorkspaces(ctx context.Context, db *sql.DB) ([]WorkspaceListItem, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
  w.id,
  w.status,
  w.updated_at,
  w.description,
  COALESCE(rc.cnt, 0) AS repo_count
FROM workspaces w
LEFT JOIN (
  SELECT workspace_id, COUNT(*) AS cnt
  FROM workspace_repos
  GROUP BY workspace_id
) rc ON rc.workspace_id = w.id
ORDER BY w.id ASC
`)
	if err != nil {
		return nil, fmt.Errorf("query workspaces: %w", err)
	}
	defer rows.Close()

	var out []WorkspaceListItem
	for rows.Next() {
		var it WorkspaceListItem
		if err := rows.Scan(&it.ID, &it.Status, &it.UpdatedAt, &it.Description, &it.RepoCount); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspaces: %w", err)
	}
	return out, nil
}

type WorkspaceRepo struct {
	RepoUID   string
	Alias     string
	MissingAt sql.NullInt64
}

func ListWorkspaceRepos(ctx context.Context, db *sql.DB, workspaceID string) ([]WorkspaceRepo, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required")
	}

	rows, err := db.QueryContext(ctx, `
SELECT repo_uid, alias, missing_at
FROM workspace_repos
WHERE workspace_id = ?
ORDER BY alias ASC
`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query workspace repos: %w", err)
	}
	defer rows.Close()

	var out []WorkspaceRepo
	for rows.Next() {
		var r WorkspaceRepo
		if err := rows.Scan(&r.RepoUID, &r.Alias, &r.MissingAt); err != nil {
			return nil, fmt.Errorf("scan workspace repo: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace repos: %w", err)
	}
	return out, nil
}

// MarkWorkspaceRepoMissing sets missing_at (and updated_at) when the repo's worktree is detected missing.
// It is idempotent: calling it again when missing_at is already set is a no-op.
func MarkWorkspaceRepoMissing(ctx context.Context, db *sql.DB, workspaceID string, repoUID string, now int64) (changed bool, err error) {
	if workspaceID == "" {
		return false, fmt.Errorf("workspace id is required")
	}
	if repoUID == "" {
		return false, fmt.Errorf("repo uid is required")
	}
	if now <= 0 {
		return false, fmt.Errorf("now is required")
	}

	res, err := db.ExecContext(ctx, `
UPDATE workspace_repos
SET
  missing_at = COALESCE(missing_at, ?),
  updated_at = CASE WHEN missing_at IS NULL THEN ? ELSE updated_at END
WHERE workspace_id = ? AND repo_uid = ?
`, now, now, workspaceID, repoUID)
	if err != nil {
		return false, fmt.Errorf("update workspace repo missing_at: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
}
