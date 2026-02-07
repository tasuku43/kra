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

func LookupRepoRemoteURL(ctx context.Context, db *sql.DB, repoUID string) (remoteURL string, ok bool, err error) {
	repoUID = strings.TrimSpace(repoUID)
	if repoUID == "" {
		return "", false, fmt.Errorf("repo uid is required")
	}

	var url string
	qErr := db.QueryRowContext(ctx, "SELECT remote_url FROM repos WHERE repo_uid = ?", repoUID).Scan(&url)
	if errors.Is(qErr, sql.ErrNoRows) {
		return "", false, nil
	}
	if qErr != nil {
		return "", false, fmt.Errorf("select repo remote_url: %w", qErr)
	}
	return url, true, nil
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

func DeleteWorkspaceRepoBinding(ctx context.Context, db *sql.DB, workspaceID string, repoUID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	repoUID = strings.TrimSpace(repoUID)
	if workspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if repoUID == "" {
		return fmt.Errorf("repo uid is required")
	}
	if _, err := db.ExecContext(ctx, `
DELETE FROM workspace_repos
WHERE workspace_id = ? AND repo_uid = ?
`, workspaceID, repoUID); err != nil {
		return fmt.Errorf("delete workspace repo binding: %w", err)
	}
	return nil
}

func TouchRepoUpdatedAt(ctx context.Context, db *sql.DB, repoUID string, now int64) error {
	repoUID = strings.TrimSpace(repoUID)
	if repoUID == "" {
		return fmt.Errorf("repo uid is required")
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	if _, err := db.ExecContext(ctx, "UPDATE repos SET updated_at = ? WHERE repo_uid = ?", now, repoUID); err != nil {
		return fmt.Errorf("touch repo updated_at: %w", err)
	}
	return nil
}

type RepoPoolCandidate struct {
	RepoUID   string
	RepoKey   string
	RemoteURL string
	UpdatedAt int64
	Score30d  int
}

type RootRepoCandidate struct {
	RepoUID           string
	RepoKey           string
	RemoteURL         string
	UpdatedAt         int64
	Score30d          int
	WorkspaceRefCount int
}

func ListRepoPoolCandidates(ctx context.Context, db *sql.DB, startDay int) ([]RepoPoolCandidate, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
  r.repo_uid,
  r.repo_key,
  r.remote_url,
  r.updated_at,
  COALESCE(SUM(u.add_count), 0) AS score_30d
FROM repos r
LEFT JOIN repo_usage_daily u
  ON u.repo_uid = r.repo_uid
 AND u.day >= ?
GROUP BY r.repo_uid, r.repo_key, r.remote_url, r.updated_at
ORDER BY score_30d DESC, r.updated_at DESC, r.repo_key ASC
`, startDay)
	if err != nil {
		return nil, fmt.Errorf("query repo pool candidates: %w", err)
	}
	defer rows.Close()

	out := make([]RepoPoolCandidate, 0, 32)
	for rows.Next() {
		var it RepoPoolCandidate
		if err := rows.Scan(&it.RepoUID, &it.RepoKey, &it.RemoteURL, &it.UpdatedAt, &it.Score30d); err != nil {
			return nil, fmt.Errorf("scan repo pool candidate: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repo pool candidates: %w", err)
	}
	return out, nil
}

func IncrementRepoUsageDaily(ctx context.Context, db *sql.DB, repoUID string, day int, now int64) error {
	repoUID = strings.TrimSpace(repoUID)
	if repoUID == "" {
		return fmt.Errorf("repo uid is required")
	}
	if day <= 0 {
		return fmt.Errorf("day is required")
	}
	if now <= 0 {
		now = time.Now().Unix()
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO repo_usage_daily (repo_uid, day, add_count, last_added_at)
VALUES (?, ?, 1, ?)
ON CONFLICT(repo_uid, day)
DO UPDATE SET
  add_count = add_count + 1,
  last_added_at = excluded.last_added_at
`, repoUID, day, now); err != nil {
		return fmt.Errorf("increment repo_usage_daily: %w", err)
	}
	return nil
}

func ListRepoUIDs(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT repo_uid
FROM repos
ORDER BY repo_uid
`)
	if err != nil {
		return nil, fmt.Errorf("query repo_uids: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, 32)
	for rows.Next() {
		var repoUID string
		if err := rows.Scan(&repoUID); err != nil {
			return nil, fmt.Errorf("scan repo_uid: %w", err)
		}
		out = append(out, repoUID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repo_uids: %w", err)
	}
	return out, nil
}

func ListRootRepoCandidates(ctx context.Context, db *sql.DB, startDay int) ([]RootRepoCandidate, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
  r.repo_uid,
  r.repo_key,
  r.remote_url,
  r.updated_at,
  COALESCE(SUM(u.add_count), 0) AS score_30d,
  COUNT(DISTINCT wr.workspace_id) AS workspace_ref_count
FROM repos r
LEFT JOIN repo_usage_daily u
  ON u.repo_uid = r.repo_uid
 AND u.day >= ?
LEFT JOIN workspace_repos wr
  ON wr.repo_uid = r.repo_uid
GROUP BY r.repo_uid, r.repo_key, r.remote_url, r.updated_at
ORDER BY score_30d DESC, r.updated_at DESC, r.repo_key ASC
`, startDay)
	if err != nil {
		return nil, fmt.Errorf("query root repo candidates: %w", err)
	}
	defer rows.Close()

	out := make([]RootRepoCandidate, 0, 32)
	for rows.Next() {
		var it RootRepoCandidate
		if err := rows.Scan(&it.RepoUID, &it.RepoKey, &it.RemoteURL, &it.UpdatedAt, &it.Score30d, &it.WorkspaceRefCount); err != nil {
			return nil, fmt.Errorf("scan root repo candidate: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate root repo candidates: %w", err)
	}
	return out, nil
}

func DeleteReposByUIDs(ctx context.Context, db *sql.DB, repoUIDs []string) error {
	if len(repoUIDs) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(repoUIDs))
	args := make([]any, 0, len(repoUIDs))
	for _, repoUID := range repoUIDs {
		repoUID = strings.TrimSpace(repoUID)
		if repoUID == "" {
			return fmt.Errorf("repo uid is required")
		}
		placeholders = append(placeholders, "?")
		args = append(args, repoUID)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := fmt.Sprintf("DELETE FROM repos WHERE repo_uid IN (%s)", strings.Join(placeholders, ","))
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete repos: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
