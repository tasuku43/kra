package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListWorkspaceCandidatesByStatus_UsesMetadataTitleOnly(t *testing.T) {
	root := t.TempDir()

	wsNew := filepath.Join(root, "workspaces", "WS-NEW")
	if err := os.MkdirAll(filepath.Join(wsNew, "repos", "repo1"), 0o755); err != nil {
		t.Fatalf("create workspace repo dir: %v", err)
	}
	metaNew := newWorkspaceMetaFileForCreate("WS-NEW", "Implement feature", "", 200)
	metaNew.ReposRestore = []workspaceMetaRepoRestore{{
		RepoUID: "repo-uid-1",
		Alias:   "repo1",
		Branch:  "feature/ws-new",
		BaseRef: "origin/main",
	}}
	if err := writeWorkspaceMetaFile(wsNew, metaNew); err != nil {
		t.Fatalf("write WS-NEW meta: %v", err)
	}

	wsOld := filepath.Join(root, "workspaces", "WS-OLD")
	if err := os.MkdirAll(wsOld, 0o755); err != nil {
		t.Fatalf("create old workspace dir: %v", err)
	}
	metaOld := newWorkspaceMetaFileForCreate("WS-OLD", "Old task", "", 100)
	if err := writeWorkspaceMetaFile(wsOld, metaOld); err != nil {
		t.Fatalf("write WS-OLD meta: %v", err)
	}

	candidates, err := listWorkspaceCandidatesByStatus(context.Background(), root, "active")
	if err != nil {
		t.Fatalf("listWorkspaceCandidatesByStatus() error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}

	if candidates[0].ID != "WS-NEW" || candidates[1].ID != "WS-OLD" {
		t.Fatalf("candidate order = [%s, %s], want [WS-NEW, WS-OLD]", candidates[0].ID, candidates[1].ID)
	}
	if candidates[0].Title != "Implement feature" {
		t.Fatalf("WS-NEW title = %q, want %q", candidates[0].Title, "Implement feature")
	}
	if strings.Contains(candidates[0].Title, "todo |") || strings.Contains(candidates[0].Title, "in-progress |") {
		t.Fatalf("WS-NEW title must not include logical work state prefix: %q", candidates[0].Title)
	}
}
