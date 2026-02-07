package stateregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/gionx/internal/paths"
)

func TestTouch_CreateOnFirstUse(t *testing.T) {
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)

	root := t.TempDir()
	statePath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", err)
	}

	now := time.Unix(1710000000, 0)
	if err := Touch(root, statePath, now); err != nil {
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
	if entries[0].StateDBPath != statePath {
		t.Fatalf("state_db_path = %q, want %q", entries[0].StateDBPath, statePath)
	}
	if entries[0].FirstSeenAt != now.Unix() {
		t.Fatalf("first_seen_at = %d, want %d", entries[0].FirstSeenAt, now.Unix())
	}
	if entries[0].LastUsedAt != now.Unix() {
		t.Fatalf("last_used_at = %d, want %d", entries[0].LastUsedAt, now.Unix())
	}
}

func TestTouch_UpdatesLastUsedAtMonotonic(t *testing.T) {
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)

	root := t.TempDir()
	statePath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", err)
	}

	if err := Touch(root, statePath, time.Unix(200, 0)); err != nil {
		t.Fatalf("Touch(first) error: %v", err)
	}
	if err := Touch(root, statePath, time.Unix(150, 0)); err != nil {
		t.Fatalf("Touch(second) error: %v", err)
	}
	if err := Touch(root, statePath, time.Unix(350, 0)); err != nil {
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
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)

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
	statePath, err := paths.StateDBPathForRoot(root)
	if err != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", err)
	}

	err = Touch(root, statePath, time.Unix(10, 0))
	if err == nil {
		t.Fatalf("Touch() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state registry is malformed") {
		t.Fatalf("error missing malformed hint: %v", err)
	}
	if !strings.Contains(err.Error(), registryPath) {
		t.Fatalf("error missing registry path: %v", err)
	}
	if !strings.Contains(err.Error(), "fix or remove") {
		t.Fatalf("error missing recovery hint: %v", err)
	}
}
