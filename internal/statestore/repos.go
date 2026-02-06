package statestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RepoConflictError struct {
	RepoUID   string
	RepoKey   string
	RemoteURL string
}

func (e *RepoConflictError) Error() string {
	return fmt.Sprintf("repo already exists with different metadata (repo_uid=%q repo_key=%q remote_url=%q)", e.RepoUID, e.RepoKey, e.RemoteURL)
}

type WorkspaceRepoAliasConflictError struct {
	WorkspaceID string
	Alias       string
}

func (e *WorkspaceRepoAliasConflictError) Error() string {
	return fmt.Sprintf("repo alias already exists in workspace (workspace_id=%q alias=%q)", e.WorkspaceID, e.Alias)
}

type WorkspaceRepoAlreadyBoundError struct {
	WorkspaceID string
	RepoUID     string
}

func (e *WorkspaceRepoAlreadyBoundError) Error() string {
	return fmt.Sprintf("repo already bound to workspace (workspace_id=%q repo_uid=%q)", e.WorkspaceID, e.RepoUID)
}

func WorkspaceRepoAliasExists(ctx context.Context, db *sql.DB, workspaceID string, alias string) (bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	alias = strings.TrimSpace(alias)
	if workspaceID == "" {
		return false, fmt.Errorf("workspace id is required")
	}
	if alias == "" {
		return false, fmt.Errorf("alias is required")
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM workspace_repos WHERE workspace_id = ? AND alias = ?", workspaceID, alias).Scan(&count); err != nil {
		return false, fmt.Errorf("query workspace repo alias: %w", err)
	}
	return count > 0, nil
}

type EnsureRepoInput struct {
	RepoUID   string
	RepoKey   string
	RemoteURL string
	Now       int64
}

func EnsureRepo(ctx context.Context, db *sql.DB, in EnsureRepoInput) error {
	in.RepoUID = strings.TrimSpace(in.RepoUID)
	in.RepoKey = strings.TrimSpace(in.RepoKey)
	in.RemoteURL = strings.TrimSpace(in.RemoteURL)
	if in.RepoUID == "" {
		return fmt.Errorf("repo uid is required")
	}
	if in.RepoKey == "" {
		return fmt.Errorf("repo key is required")
	}
	if in.RemoteURL == "" {
		return fmt.Errorf("remote url is required")
	}
	if in.Now <= 0 {
		in.Now = time.Now().Unix()
	}

	var existingKey string
	var existingURL string
	err := db.QueryRowContext(ctx, "SELECT repo_key, remote_url FROM repos WHERE repo_uid = ?", in.RepoUID).Scan(&existingKey, &existingURL)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := db.ExecContext(ctx, `
INSERT INTO repos (repo_uid, repo_key, remote_url, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
`, in.RepoUID, in.RepoKey, in.RemoteURL, in.Now, in.Now); err != nil {
			return fmt.Errorf("insert repo: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("select repo: %w", err)
	}

	if existingKey != in.RepoKey || existingURL != in.RemoteURL {
		return &RepoConflictError{RepoUID: in.RepoUID, RepoKey: existingKey, RemoteURL: existingURL}
	}

	if _, err := db.ExecContext(ctx, "UPDATE repos SET updated_at = ? WHERE repo_uid = ?", in.Now, in.RepoUID); err != nil {
		return fmt.Errorf("touch repo updated_at: %w", err)
	}
	return nil
}

type AddWorkspaceRepoInput struct {
	WorkspaceID   string
	RepoUID       string
	RepoKey       string
	Alias         string
	Branch        string
	BaseRef       string
	RepoSpecInput string
	Now           int64
}

func AddWorkspaceRepo(ctx context.Context, db *sql.DB, in AddWorkspaceRepoInput) error {
	in.WorkspaceID = strings.TrimSpace(in.WorkspaceID)
	in.RepoUID = strings.TrimSpace(in.RepoUID)
	in.RepoKey = strings.TrimSpace(in.RepoKey)
	in.Alias = strings.TrimSpace(in.Alias)
	in.Branch = strings.TrimSpace(in.Branch)
	in.BaseRef = strings.TrimSpace(in.BaseRef)
	in.RepoSpecInput = strings.TrimSpace(in.RepoSpecInput)

	if in.WorkspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if in.RepoUID == "" {
		return fmt.Errorf("repo uid is required")
	}
	if in.RepoKey == "" {
		return fmt.Errorf("repo key is required")
	}
	if in.Alias == "" {
		return fmt.Errorf("alias is required")
	}
	if in.Branch == "" {
		return fmt.Errorf("branch is required")
	}
	if in.RepoSpecInput == "" {
		return fmt.Errorf("repo spec input is required")
	}
	if in.Now <= 0 {
		in.Now = time.Now().Unix()
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Ensure not already bound (friendly error instead of relying on PK failure).
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM workspace_repos WHERE workspace_id = ? AND repo_uid = ?", in.WorkspaceID, in.RepoUID).Scan(&exists); err != nil {
		return fmt.Errorf("query existing workspace repo binding: %w", err)
	}
	if exists > 0 {
		return &WorkspaceRepoAlreadyBoundError{WorkspaceID: in.WorkspaceID, RepoUID: in.RepoUID}
	}

	// Ensure alias uniqueness (friendly error).
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM workspace_repos WHERE workspace_id = ? AND alias = ?", in.WorkspaceID, in.Alias).Scan(&exists); err != nil {
		return fmt.Errorf("query existing alias: %w", err)
	}
	if exists > 0 {
		return &WorkspaceRepoAliasConflictError{WorkspaceID: in.WorkspaceID, Alias: in.Alias}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_repos (
  workspace_id, repo_uid, repo_key, alias, branch, base_ref, repo_spec_input,
  missing_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
`, in.WorkspaceID, in.RepoUID, in.RepoKey, in.Alias, in.Branch, in.BaseRef, in.RepoSpecInput, in.Now, in.Now); err != nil {
		return fmt.Errorf("insert workspace_repo: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
