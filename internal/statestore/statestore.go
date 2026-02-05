package statestore

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tasuku43/gionx/migrations"
)

func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("db path is required")
	}

	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn, err := sqliteDSN(dbPath)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := enableForeignKeys(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := applyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func sqliteDSN(dbPath string) (string, error) {
	if dbPath == ":memory:" {
		return "file::memory:?cache=shared&_pragma=foreign_keys(ON)", nil
	}

	u := url.URL{Scheme: "file", Path: dbPath}
	q := u.Query()
	q.Set("_pragma", "foreign_keys(ON)")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func enableForeignKeys(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	return nil
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	ms, err := migrations.All()
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("enable foreign keys (tx): %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  id TEXT PRIMARY KEY,
  applied_at INTEGER NOT NULL
);`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied, err := loadApplied(ctx, tx)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	for _, m := range ms {
		if applied[m.ID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			return fmt.Errorf("apply migration %q: %w", m.ID, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (id, applied_at) VALUES (?, ?)", m.ID, now); err != nil {
			return fmt.Errorf("record migration %q: %w", m.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func loadApplied(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, "SELECT id FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("load applied migrations: %w", err)
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return out, nil
}

