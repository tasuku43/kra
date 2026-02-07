package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/paths"
	"github.com/tasuku43/gionx/internal/stateregistry"
)

func TestCLI_StateRegistry_InitCreatesEntry(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	if code := c.Run([]string{"init"}); code != exitOK {
		t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	registryPath, pathErr := stateregistry.Path()
	if pathErr != nil {
		t.Fatalf("stateregistry.Path() error: %v", pathErr)
	}
	entries, loadErr := stateregistry.Load(registryPath)
	if loadErr != nil {
		t.Fatalf("stateregistry.Load() error: %v", loadErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entries length = %d, want 1", len(entries))
	}

	statePath, stateErr := paths.StateDBPathForRoot(root)
	if stateErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", stateErr)
	}
	if entries[0].RootPath != root {
		t.Fatalf("root_path = %q, want %q", entries[0].RootPath, root)
	}
	if entries[0].StateDBPath != statePath {
		t.Fatalf("state_db_path = %q, want %q", entries[0].StateDBPath, statePath)
	}
	if entries[0].FirstSeenAt <= 0 || entries[0].LastUsedAt <= 0 {
		t.Fatalf("timestamps must be positive: %+v", entries[0])
	}
}

func TestCLI_StateRegistry_WSCreateUpdatesLastUsed(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"init"}); code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	registryPath, pathErr := stateregistry.Path()
	if pathErr != nil {
		t.Fatalf("stateregistry.Path() error: %v", pathErr)
	}
	statePath, stateErr := paths.StateDBPathForRoot(root)
	if stateErr != nil {
		t.Fatalf("StateDBPathForRoot() error: %v", stateErr)
	}

	seed := fmt.Sprintf("{\n  \"entries\": [\n    {\n      \"root_path\": %q,\n      \"state_db_path\": %q,\n      \"first_seen_at\": 1,\n      \"last_used_at\": 1\n    }\n  ]\n}\n", root, statePath)
	if err := os.WriteFile(registryPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write registry seed: %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"ws", "create", "--no-prompt", "WS-REG-001"}); code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	entries, loadErr := stateregistry.Load(registryPath)
	if loadErr != nil {
		t.Fatalf("stateregistry.Load() error: %v", loadErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entries length = %d, want 1", len(entries))
	}
	if entries[0].LastUsedAt <= 1 {
		t.Fatalf("last_used_at = %d, want > 1", entries[0].LastUsedAt)
	}
	if entries[0].FirstSeenAt != 1 {
		t.Fatalf("first_seen_at = %d, want 1", entries[0].FirstSeenAt)
	}
}

func TestCLI_StateRegistry_WSMalformedRegistryFails(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	cacheHome := filepath.Join(t.TempDir(), "xdg-cache")
	t.Setenv("GIONX_ROOT", root)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		if code := c.Run([]string{"init"}); code != exitOK {
			t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	registryPath, pathErr := stateregistry.Path()
	if pathErr != nil {
		t.Fatalf("stateregistry.Path() error: %v", pathErr)
	}
	if err := os.WriteFile(registryPath, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write malformed registry: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "list"})
	if code != exitError {
		t.Fatalf("ws list exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	if !strings.Contains(err.String(), "update state registry") {
		t.Fatalf("stderr missing update state registry: %q", err.String())
	}
	if !strings.Contains(err.String(), "state registry is malformed") {
		t.Fatalf("stderr missing malformed hint: %q", err.String())
	}
	if !strings.Contains(err.String(), "fix or remove") {
		t.Fatalf("stderr missing recovery hint: %q", err.String())
	}
}
