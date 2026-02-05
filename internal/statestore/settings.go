package statestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// EnsureSettings ensures the single-row settings table is initialized.
//
// Behavior:
// - If settings row (id=1) is missing: insert it.
// - If it exists: verify it matches rootPath/repoPoolPath (do not overwrite).
func EnsureSettings(ctx context.Context, db *sql.DB, rootPath string, repoPoolPath string) error {
	if rootPath == "" {
		return fmt.Errorf("root path is required")
	}
	if repoPoolPath == "" {
		return fmt.Errorf("repo pool path is required")
	}

	var existingRoot string
	var existingPool string
	err := db.QueryRowContext(ctx, "SELECT root_path, repo_pool_path FROM settings WHERE id = 1").Scan(&existingRoot, &existingPool)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().Unix()
		_, err := db.ExecContext(ctx, `
INSERT INTO settings (id, root_path, repo_pool_path, created_at, updated_at)
VALUES (1, ?, ?, ?, ?)
`, rootPath, repoPoolPath, now, now)
		if err != nil {
			return fmt.Errorf("insert settings: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("select settings: %w", err)
	}

	if existingRoot != rootPath || existingPool != repoPoolPath {
		return fmt.Errorf("settings already initialized with a different value (root=%q pool=%q)", existingRoot, existingPool)
	}
	return nil
}
