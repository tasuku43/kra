package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeCMUXStatusClient struct {
	workspaces []cmuxctl.Workspace
	err        error
}

func (f *fakeCMUXStatusClient) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	return f.workspaces, f.err
}

func TestCLI_CMUX_List_JSON_Success(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	store := cmuxmap.NewStore(root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 3,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-1", Ordinal: 1, TitleSnapshot: "WS1 | one [1]"},
					{CMUXWorkspaceID: "CMUX-2", Ordinal: 2, TitleSnapshot: "WS1 | one [2]"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "list", "--format", "json", "--workspace", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty: %q", err.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Items []map[string]any `json:"items"`
		} `json:"result"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if !resp.OK || len(resp.Result.Items) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCLI_CMUX_Status_JSON_ReportsExists(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	store := cmuxmap.NewStore(root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 3,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-1", Ordinal: 1, TitleSnapshot: "WS1 | one [1]"},
					{CMUXWorkspaceID: "CMUX-2", Ordinal: 2, TitleSnapshot: "WS1 | one [2]"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	prev := newCMUXStatusClient
	newCMUXStatusClient = func() cmuxStatusClient {
		return &fakeCMUXStatusClient{
			workspaces: []cmuxctl.Workspace{
				{ID: "CMUX-1"},
			},
		}
	}
	t.Cleanup(func() { newCMUXStatusClient = prev })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "status", "--format", "json", "--workspace", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty: %q", err.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Items []struct {
				CMUXID string `json:"cmux_workspace_id"`
				Exists bool   `json:"exists"`
			} `json:"items"`
		} `json:"result"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if !resp.OK || len(resp.Result.Items) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	gotExists := map[string]bool{}
	for _, it := range resp.Result.Items {
		gotExists[it.CMUXID] = it.Exists
	}
	if !gotExists["CMUX-1"] || gotExists["CMUX-2"] {
		t.Fatalf("exists flags = %+v, want CMUX-1=true CMUX-2=false", gotExists)
	}
}
