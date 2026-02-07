package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveContextUseRoot_NotDirectory(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := resolveContextUseRoot(filePath)
	if err == nil {
		t.Fatal("expected error for file path")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveContextUseRoot_MissingParent(t *testing.T) {
	base := filepath.Join(t.TempDir(), "missing-parent")
	target := filepath.Join(base, "root")

	_, err := resolveContextUseRoot(target)
	if err == nil {
		t.Fatal("expected error when parent directory is missing")
	}
	if !strings.Contains(err.Error(), "parent directory missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveContextUseRoot_MissingButCreatablePath(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "new-root")

	got, err := resolveContextUseRoot(target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Clean(target) {
		t.Fatalf("resolved root=%q, want=%q", got, filepath.Clean(target))
	}
}
