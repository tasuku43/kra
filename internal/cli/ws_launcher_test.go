package cli

import (
	"path/filepath"
	"testing"
)

func TestDetectWorkspaceFromCWD(t *testing.T) {
	root := t.TempDir()

	activePath := filepath.Join(root, "workspaces", "WS-123", "notes")
	got, ok := detectWorkspaceFromCWD(root, activePath)
	if !ok {
		t.Fatalf("detect active workspace: not found")
	}
	if got.ID != "WS-123" || got.Status != "active" {
		t.Fatalf("active detect = %+v, want id=WS-123 status=active", got)
	}

	archivedPath := filepath.Join(root, "archive", "WS-999", "artifacts")
	got, ok = detectWorkspaceFromCWD(root, archivedPath)
	if !ok {
		t.Fatalf("detect archived workspace: not found")
	}
	if got.ID != "WS-999" || got.Status != "archived" {
		t.Fatalf("archived detect = %+v, want id=WS-999 status=archived", got)
	}

	outsidePath := filepath.Join(root, "tmp")
	_, ok = detectWorkspaceFromCWD(root, outsidePath)
	if ok {
		t.Fatalf("outside path unexpectedly detected as workspace")
	}
}
