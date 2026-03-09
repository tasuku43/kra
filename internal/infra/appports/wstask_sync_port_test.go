package appports

import (
	"testing"

	"github.com/tasuku43/kra/internal/cmuxmap"
)

func TestWSTaskSyncPortListCMUXWorkspaceIDs_FallsBackToEnvWhenMappingMissing(t *testing.T) {
	t.Setenv("CMUX_WORKSPACE_ID", "cmux-env-1")

	root := t.TempDir()
	port := NewWSTaskSyncPort(nil, cmuxmap.NewStore)

	got, err := port.ListCMUXWorkspaceIDs(root, "WS1")
	if err != nil {
		t.Fatalf("ListCMUXWorkspaceIDs() error = %v", err)
	}
	if len(got) != 1 || got[0] != "cmux-env-1" {
		t.Fatalf("ids = %#v, want [cmux-env-1]", got)
	}
}

func TestWSTaskSyncPortListCMUXWorkspaceIDs_PrefersMappingOverEnvFallback(t *testing.T) {
	t.Setenv("CMUX_WORKSPACE_ID", "cmux-env-1")

	root := t.TempDir()
	store := cmuxmap.NewStore(root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "cmux-map-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	port := NewWSTaskSyncPort(nil, cmuxmap.NewStore)
	got, err := port.ListCMUXWorkspaceIDs(root, "WS1")
	if err != nil {
		t.Fatalf("ListCMUXWorkspaceIDs() error = %v", err)
	}
	if len(got) != 1 || got[0] != "cmux-map-1" {
		t.Fatalf("ids = %#v, want [cmux-map-1]", got)
	}
}
