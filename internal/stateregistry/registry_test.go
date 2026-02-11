package stateregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTouch_CreateOnFirstUse(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	root := t.TempDir()
	now := time.Unix(1710000000, 0)
	if err := Touch(root, now); err != nil {
		t.Fatalf("Touch() error: %v", err)
	}

	registryPath, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	entries, err := Load(registryPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries length = %d, want 1", len(entries))
	}
	if entries[0].RootPath != root {
		t.Fatalf("root_path = %q, want %q", entries[0].RootPath, root)
	}
	if entries[0].FirstSeenAt != now.Unix() {
		t.Fatalf("first_seen_at = %d, want %d", entries[0].FirstSeenAt, now.Unix())
	}
	if entries[0].LastUsedAt != now.Unix() {
		t.Fatalf("last_used_at = %d, want %d", entries[0].LastUsedAt, now.Unix())
	}
}

func TestTouch_UpdatesLastUsedAtMonotonic(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	root := t.TempDir()
	if err := Touch(root, time.Unix(200, 0)); err != nil {
		t.Fatalf("Touch(first) error: %v", err)
	}
	if err := Touch(root, time.Unix(150, 0)); err != nil {
		t.Fatalf("Touch(second) error: %v", err)
	}
	if err := Touch(root, time.Unix(350, 0)); err != nil {
		t.Fatalf("Touch(third) error: %v", err)
	}

	registryPath, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	entries, err := Load(registryPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries length = %d, want 1", len(entries))
	}
	if entries[0].FirstSeenAt != 200 {
		t.Fatalf("first_seen_at = %d, want 200", entries[0].FirstSeenAt)
	}
	if entries[0].LastUsedAt != 350 {
		t.Fatalf("last_used_at = %d, want 350", entries[0].LastUsedAt)
	}
}

func TestTouch_MalformedRegistry_ReturnsRecoveryHint(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	registryPath, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(registryPath, []byte("{broken-json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	root := t.TempDir()
	err = Touch(root, time.Unix(10, 0))
	if err == nil {
		t.Fatalf("Touch() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "root registry is malformed") {
		t.Fatalf("error missing malformed hint: %v", err)
	}
	if !strings.Contains(err.Error(), registryPath) {
		t.Fatalf("error missing registry path: %v", err)
	}
	if !strings.Contains(err.Error(), "fix or remove") {
		t.Fatalf("error missing recovery hint: %v", err)
	}
}

func TestRenameContextName_UpdatesNameAndRejectsConflict(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	rootA := t.TempDir()
	rootB := t.TempDir()
	if err := SetContextName(rootA, "old", time.Unix(100, 0)); err != nil {
		t.Fatalf("SetContextName(rootA): %v", err)
	}
	if err := SetContextName(rootB, "existing", time.Unix(100, 0)); err != nil {
		t.Fatalf("SetContextName(rootB): %v", err)
	}

	if _, err := RenameContextName("old", "existing", time.Unix(200, 0)); err == nil {
		t.Fatalf("RenameContextName conflict should fail")
	}

	gotRoot, err := RenameContextName("old", "new", time.Unix(200, 0))
	if err != nil {
		t.Fatalf("RenameContextName() error: %v", err)
	}
	if gotRoot != rootA {
		t.Fatalf("renamed root = %q, want %q", gotRoot, rootA)
	}
	if _, ok, err := ResolveRootByContextName("new"); err != nil || !ok {
		t.Fatalf("ResolveRootByContextName(new) failed, ok=%t err=%v", ok, err)
	}
	if _, ok, err := ResolveRootByContextName("old"); err != nil || ok {
		t.Fatalf("ResolveRootByContextName(old) should be missing, ok=%t err=%v", ok, err)
	}
}

func TestRemoveContextName_RemovesEntry(t *testing.T) {
	t.Setenv("GIONX_HOME", filepath.Join(t.TempDir(), ".gionx"))

	root := t.TempDir()
	if err := SetContextName(root, "remove-me", time.Unix(100, 0)); err != nil {
		t.Fatalf("SetContextName(): %v", err)
	}

	gotRoot, err := RemoveContextName("remove-me")
	if err != nil {
		t.Fatalf("RemoveContextName() error: %v", err)
	}
	if gotRoot != root {
		t.Fatalf("removed root = %q, want %q", gotRoot, root)
	}
	if _, ok, err := ResolveRootByContextName("remove-me"); err != nil || ok {
		t.Fatalf("context should be removed, ok=%t err=%v", ok, err)
	}
}
