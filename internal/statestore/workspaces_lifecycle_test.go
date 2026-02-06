package statestore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestArchiveAndReopenWorkspace_UpdatesSnapshotAndEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := CreateWorkspace(ctx, db, CreateWorkspaceInput{
		ID:          "WS1",
		Description: "",
		SourceURL:   "",
		Now:         100,
	}); err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}

	if err := ArchiveWorkspace(ctx, db, ArchiveWorkspaceInput{
		ID:                "WS1",
		ArchivedCommitSHA: "abc123",
		Now:               200,
	}); err != nil {
		t.Fatalf("ArchiveWorkspace() error: %v", err)
	}

	if err := ReopenWorkspace(ctx, db, ReopenWorkspaceInput{
		ID:                "WS1",
		ReopenedCommitSHA: "def456",
		Now:               300,
	}); err != nil {
		t.Fatalf("ReopenWorkspace() error: %v", err)
	}

	var status string
	var reopenedSHA string
	if err := db.QueryRowContext(ctx, "SELECT status, reopened_commit_sha FROM workspaces WHERE id = ?", "WS1").Scan(&status, &reopenedSHA); err != nil {
		t.Fatalf("query workspaces: %v", err)
	}
	if status != "active" {
		t.Fatalf("status = %q, want %q", status, "active")
	}
	if reopenedSHA != "def456" {
		t.Fatalf("reopened_commit_sha = %q, want %q", reopenedSHA, "def456")
	}

	var createdCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM workspace_events
WHERE workspace_id = ? AND event_type = 'reopened'
`, "WS1").Scan(&createdCount); err != nil {
		t.Fatalf("query workspace_events: %v", err)
	}
	if createdCount != 1 {
		t.Fatalf("reopened events count = %d, want 1", createdCount)
	}
}

func TestReopenWorkspace_FailsWhenWorkspaceNotArchived(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := CreateWorkspace(ctx, db, CreateWorkspaceInput{
		ID:          "WS1",
		Description: "",
		SourceURL:   "",
		Now:         100,
	}); err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}

	err = ReopenWorkspace(ctx, db, ReopenWorkspaceInput{
		ID:                "WS1",
		ReopenedCommitSHA: "def456",
		Now:               300,
	})
	if err == nil {
		t.Fatalf("ReopenWorkspace() error = nil, want error")
	}
}
