package statestore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpen_appliesMigrationsAndRecordsSchemaMigrations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var got []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM schema_migrations ORDER BY id")
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	_ = rows.Close()

	want := []string{"0001_init.sql"}
	if len(got) != len(want) {
		t.Fatalf("schema_migrations length mismatch: want=%d got=%d (got=%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schema_migrations mismatch at %d: want=%q got=%q (got=%v)", i, want[i], got[i], got)
		}
	}
}

func TestOpen_enablesForeignKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// workspace_repos has a foreign key to workspaces(id). If foreign_keys is OFF,
	// this insert would succeed (which we do not want).
	_, err = db.ExecContext(ctx, `
INSERT INTO workspace_repos (
  workspace_id, repo_uid, repo_key, alias, branch, base_ref, repo_spec_input, missing_at, created_at, updated_at
) VALUES (
  'ws1', 'github.com/o/r', 'o/r', 'r', 'main', '', 'github.com/o/r', NULL, 0, 0
);
`)
	if err == nil {
		t.Fatalf("expected foreign key error, got nil")
	}
}

func TestEnsureSettings_InsertsThenNoopsWhenSame(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := EnsureSettings(ctx, db, "/tmp/root1", "/tmp/pool1"); err != nil {
		t.Fatalf("EnsureSettings(insert) error: %v", err)
	}
	if err := EnsureSettings(ctx, db, "/tmp/root1", "/tmp/pool1"); err != nil {
		t.Fatalf("EnsureSettings(noop) error: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM settings WHERE id = 1").Scan(&count); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 1 {
		t.Fatalf("settings row count = %d, want 1", count)
	}
}

func TestEnsureSettings_ErrorsWhenDifferent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := EnsureSettings(ctx, db, "/tmp/root1", "/tmp/pool1"); err != nil {
		t.Fatalf("EnsureSettings(insert) error: %v", err)
	}

	err = EnsureSettings(ctx, db, "/tmp/root2", "/tmp/pool1")
	if err == nil {
		t.Fatalf("EnsureSettings(different) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "different value") {
		t.Fatalf("unexpected error: %v", err)
	}
}
