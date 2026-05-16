package cli

import (
	"os"
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

func TestDetectWorkspaceFromCWD_ResolvesSymlinkedRootAndCWD(t *testing.T) {
	realBase := t.TempDir()
	realRoot := filepath.Join(realBase, "root")
	if err := os.MkdirAll(filepath.Join(realRoot, "workspaces", "WS-123", "notes"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	linkBase := t.TempDir()
	linkRoot := filepath.Join(linkBase, "root-link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("symlink root: %v", err)
	}

	got, ok := detectWorkspaceFromCWD(linkRoot, filepath.Join(realRoot, "workspaces", "WS-123", "notes"))
	if !ok {
		t.Fatalf("detect active workspace through symlinked root: not found")
	}
	if got.ID != "WS-123" || got.Status != "active" {
		t.Fatalf("active detect = %+v, want id=WS-123 status=active", got)
	}
}
