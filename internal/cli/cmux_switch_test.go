package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
)

type fakeCMUXSwitchClient struct {
	selected string
	err      error
}

func (f *fakeCMUXSwitchClient) SelectWorkspace(_ context.Context, workspace string) error {
	f.selected = workspace
	return f.err
}

func TestCLI_CMUX_Switch_JSON_Success_WithExplicitWorkspaceAndCMUX(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	store := cmuxmap.NewStore(root)
	mapping := cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{
						CMUXWorkspaceID: "CMUX-WS-1",
						Ordinal:         1,
						TitleSnapshot:   "WS1 | hello [1]",
						CreatedAt:       time.Now().UTC().Format(time.RFC3339),
						LastUsedAt:      "2026-01-01T00:00:00Z",
					},
				},
			},
		},
	}
	if err := store.Save(mapping); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeCMUXSwitchClient{}
	prev := newCMUXSwitchClient
	newCMUXSwitchClient = func() cmuxSwitchClient { return fake }
	t.Cleanup(func() { newCMUXSwitchClient = prev })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "switch", "--format", "json", "--workspace", "WS1", "--cmux", "CMUX-WS-1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty: %q", err.String())
	}
	if fake.selected != "CMUX-WS-1" {
		t.Fatalf("selected workspace = %q, want %q", fake.selected, "CMUX-WS-1")
	}

	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if !resp.OK || resp.Action != "cmux.switch" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}

	after, lerr := store.Load()
	if lerr != nil {
		t.Fatalf("reload mapping: %v", lerr)
	}
	lastUsed := after.Workspaces["WS1"].Entries[0].LastUsedAt
	if lastUsed == "" || lastUsed == "2026-01-01T00:00:00Z" {
		t.Fatalf("last_used_at should be updated, got %q", lastUsed)
	}
}

func TestCLI_CMUX_Switch_JSON_AmbiguousWithoutWorkspace_Fails(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	store := cmuxmap.NewStore(root)
	mapping := cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-1", Ordinal: 1, TitleSnapshot: "WS1 | one [1]"},
				},
			},
			"WS2": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-2", Ordinal: 1, TitleSnapshot: "WS2 | two [1]"},
				},
			},
		},
	}
	if err := store.Save(mapping); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "switch", "--format", "json", "--cmux", "workspace:1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if resp.OK || resp.Error.Code != "cmux_ambiguous_target" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_CMUX_Switch_JSON_RequiresTargetIfNoFlags(t *testing.T) {
	prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "switch", "--format", "json"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty: %q", err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if resp.OK || resp.Error.Code != "non_interactive_selection_required" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestFilterCMUXEntries_MatchesIDAndOrdinalHandle(t *testing.T) {
	entries := []cmuxmap.Entry{
		{CMUXWorkspaceID: "CMUX-1", Ordinal: 1},
		{CMUXWorkspaceID: "CMUX-2", Ordinal: 2},
	}
	got := filterCMUXEntries(entries, "CMUX-2")
	if len(got) != 1 || got[0].CMUXWorkspaceID != "CMUX-2" {
		t.Fatalf("id match failed: %+v", got)
	}
	got = filterCMUXEntries(entries, "workspace:1")
	if len(got) != 1 || got[0].CMUXWorkspaceID != "CMUX-1" {
		t.Fatalf("ordinal match failed: %+v", got)
	}
}

func TestCLI_CMUX_Switch_JSON_NonMappedWorkspace(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	path := filepath.Join(root, ".kra", "state")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "switch", "--format", "json", "--workspace", "WS9", "--cmux", "CMUX-X"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if resp.OK || resp.Error.Code != "cmux_not_mapped" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}
