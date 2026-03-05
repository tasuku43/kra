package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkListRowsFromFilesystem_Active50(b *testing.B) {
	root := benchmarkWorkspaceRoot(b, 50)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := listRowsFromFilesystem(ctx, root, "active", false)
		if err != nil {
			b.Fatalf("listRowsFromFilesystem() error: %v", err)
		}
		if len(rows) != 50 {
			b.Fatalf("row count = %d, want 50", len(rows))
		}
	}
}

func BenchmarkBuildWSDashboardResult_Active50(b *testing.B) {
	root := benchmarkWorkspaceRoot(b, 50)
	opts := wsDashboardOptions{
		scope:  "active",
		format: "json",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := buildWSDashboardResult(root, opts, nil)
		if err != nil {
			b.Fatalf("buildWSDashboardResult() error: %v", err)
		}
		if len(result.Workspaces) != 50 {
			b.Fatalf("workspace count = %d, want 50", len(result.Workspaces))
		}
	}
}

func BenchmarkResolveWorkspaceWorkStateFromBaseline(b *testing.B) {
	root := benchmarkWorkspaceRoot(b, 1)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := resolveWorkspaceWorkState(ctx, root, "active", "WS-001", nil)
		if state != workspaceWorkStateTodo {
			b.Fatalf("work state = %s, want todo", state)
		}
	}
}

func benchmarkWorkspaceRoot(tb testing.TB, count int) string {
	tb.Helper()

	root := tb.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		tb.Fatalf("mkdir workspaces: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		tb.Fatalf("mkdir archive: %v", err)
	}

	for i := 1; i <= count; i++ {
		workspaceID := fmt.Sprintf("WS-%03d", i)
		wsPath := filepath.Join(root, "workspaces", workspaceID)
		if err := os.MkdirAll(filepath.Join(wsPath, "notes"), 0o755); err != nil {
			tb.Fatalf("mkdir workspace notes: %v", err)
		}
		if err := os.WriteFile(filepath.Join(wsPath, "notes", "README.md"), []byte("notes\n"), 0o644); err != nil {
			tb.Fatalf("write workspace notes: %v", err)
		}
		meta := newWorkspaceMetaFileForCreate(workspaceID, "Benchmark workspace", "", 100)
		fsHashes, err := collectWorkspaceNonRepoFileHashes(wsPath)
		if err != nil {
			tb.Fatalf("collect workspace file hashes: %v", err)
		}
		meta.Baseline = &workspaceBaseline{
			Version:   1,
			CreatedAt: 100,
			Repos:     map[string]workspaceBaselineRepo{},
			FS:        fsHashes,
		}
		if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
			tb.Fatalf("write workspace meta: %v", err)
		}
	}

	return root
}
