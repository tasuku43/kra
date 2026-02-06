package statestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type WorkspaceAlreadyExistsError struct {
	ID     string
	Status string
}

func (e *WorkspaceAlreadyExistsError) Error() string {
	return fmt.Sprintf("workspace already exists (id=%q status=%q)", e.ID, e.Status)
}

type CreateWorkspaceInput struct {
	ID          string
	Description string
	SourceURL   string
	Now         int64
}

// LookupWorkspaceStatus returns the current snapshot status for the workspace ID.
// ok=false means no snapshot row exists (it may still have past events due to purge).
func LookupWorkspaceStatus(ctx context.Context, db *sql.DB, id string) (status string, ok bool, err error) {
	if id == "" {
		return "", false, fmt.Errorf("workspace id is required")
	}

	var s string
	qErr := db.QueryRowContext(ctx, "SELECT status FROM workspaces WHERE id = ?", id).Scan(&s)
	if errors.Is(qErr, sql.ErrNoRows) {
		return "", false, nil
	}
	if qErr != nil {
		return "", false, fmt.Errorf("select workspace status: %w", qErr)
	}
	return s, true, nil
}

// CreateWorkspace inserts a new workspace snapshot row and appends a `created` event
// in a single transaction, following the generation rules in the specs.
func CreateWorkspace(ctx context.Context, db *sql.DB, in CreateWorkspaceInput) (generation int, err error) {
	if in.ID == "" {
		return 0, fmt.Errorf("workspace id is required")
	}
	if in.Now <= 0 {
		return 0, fmt.Errorf("now is required")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingStatus string
	qErr := tx.QueryRowContext(ctx, "SELECT status FROM workspaces WHERE id = ?", in.ID).Scan(&existingStatus)
	if qErr == nil {
		return 0, &WorkspaceAlreadyExistsError{ID: in.ID, Status: existingStatus}
	}
	if qErr != nil && !errors.Is(qErr, sql.ErrNoRows) {
		return 0, fmt.Errorf("select existing workspace: %w", qErr)
	}

	var maxGen int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(workspace_generation), 0) FROM workspace_events WHERE workspace_id = ?", in.ID).Scan(&maxGen); err != nil {
		return 0, fmt.Errorf("select max generation: %w", err)
	}
	gen := maxGen + 1

	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspaces (
  id, generation, status, description, source_url,
  created_at, updated_at,
  archived_commit_sha, reopened_commit_sha
)
VALUES (?, ?, 'active', ?, ?, ?, ?, NULL, NULL)
`, in.ID, gen, in.Description, in.SourceURL, in.Now, in.Now); err != nil {
		return 0, fmt.Errorf("insert workspace: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_events (
  workspace_id, workspace_generation, event_type, at, meta
)
VALUES (?, ?, 'created', ?, '{}')
`, in.ID, gen, in.Now); err != nil {
		return 0, fmt.Errorf("insert workspace event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return gen, nil
}

type ArchiveWorkspaceInput struct {
	ID                string
	ArchivedCommitSHA string
	Now               int64
}

// ArchiveWorkspace marks an active workspace as archived and appends an `archived` event
// in a single transaction.
func ArchiveWorkspace(ctx context.Context, db *sql.DB, in ArchiveWorkspaceInput) error {
	in.ID = strings.TrimSpace(in.ID)
	in.ArchivedCommitSHA = strings.TrimSpace(in.ArchivedCommitSHA)
	if in.ID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if in.ArchivedCommitSHA == "" {
		return fmt.Errorf("archived commit sha is required")
	}
	if in.Now <= 0 {
		return fmt.Errorf("now is required")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var gen int
	var status string
	qErr := tx.QueryRowContext(ctx, "SELECT generation, status FROM workspaces WHERE id = ?", in.ID).Scan(&gen, &status)
	if errors.Is(qErr, sql.ErrNoRows) {
		return fmt.Errorf("workspace not found: %s", in.ID)
	}
	if qErr != nil {
		return fmt.Errorf("select workspace: %w", qErr)
	}
	if status != "active" {
		return fmt.Errorf("workspace is not active (status=%s): %s", status, in.ID)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE workspaces
SET status = 'archived', updated_at = ?, archived_commit_sha = ?
WHERE id = ?
`, in.Now, in.ArchivedCommitSHA, in.ID); err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_events (workspace_id, workspace_generation, event_type, at, meta)
VALUES (?, ?, 'archived', ?, '{}')
`, in.ID, gen, in.Now); err != nil {
		return fmt.Errorf("insert workspace event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
