package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type fakeCMUXOpenClient struct {
	capabilities cmuxctl.Capabilities
	createID     string
	createIDs    []string
	createIndex  int
	createErr    error
	createCmds   []string
	renameErr    error
	selectErr    error

	renameWorkspace string
	renameTitle     string
	selectWorkspace string
}

func (f *fakeCMUXOpenClient) Capabilities(context.Context) (cmuxctl.Capabilities, error) {
	return f.capabilities, nil
}

func (f *fakeCMUXOpenClient) CreateWorkspaceWithCommand(_ context.Context, command string) (string, error) {
	f.createCmds = append(f.createCmds, command)
	if f.createErr != nil {
		return "", f.createErr
	}
	if len(f.createIDs) > 0 {
		if f.createIndex >= len(f.createIDs) {
			return f.createID, nil
		}
		id := f.createIDs[f.createIndex]
		f.createIndex++
		return id, nil
	}
	return f.createID, nil
}

func (f *fakeCMUXOpenClient) RenameWorkspace(_ context.Context, workspace string, title string) error {
	f.renameWorkspace = workspace
	f.renameTitle = title
	return f.renameErr
}

func (f *fakeCMUXOpenClient) SelectWorkspace(_ context.Context, workspace string) error {
	f.selectWorkspace = workspace
	return f.selectErr
}

func TestCLI_CMUX_Open_JSON_RequiresWorkspaceIDWhenOmitted(t *testing.T) {
	prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "open", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if resp.OK || resp.Action != "cmux.open" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_CMUX_Open_JSON_Success_PersistsMapping(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS1", "hello world", "", now)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createID: "CMUX-WS-1",
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "open", "--format", "json", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty: %q", err.String())
	}

	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if !resp.OK || resp.Action != "cmux.open" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if fake.renameWorkspace != "CMUX-WS-1" {
		t.Fatalf("rename workspace = %q, want %q", fake.renameWorkspace, "CMUX-WS-1")
	}
	if !strings.Contains(fake.renameTitle, "WS1 | hello world [1]") {
		t.Fatalf("rename title = %q, want to contain %q", fake.renameTitle, "WS1 | hello world [1]")
	}
	if fake.selectWorkspace != "CMUX-WS-1" {
		t.Fatalf("select workspace = %q, want %q", fake.selectWorkspace, "CMUX-WS-1")
	}
	if len(fake.createCmds) != 1 || !strings.Contains(fake.createCmds[0], "cd ") || !strings.Contains(fake.createCmds[0], wsPath) {
		t.Fatalf("create command = %+v, want single cd command for workspace path", fake.createCmds)
	}

	mapping, lerr := cmuxmap.NewStore(root).Load()
	if lerr != nil {
		t.Fatalf("load mapping: %v", lerr)
	}
	ws, ok := mapping.Workspaces["WS1"]
	if !ok {
		t.Fatalf("mapping for WS1 not found: %+v", mapping.Workspaces)
	}
	if ws.NextOrdinal != 2 {
		t.Fatalf("next_ordinal = %d, want 2", ws.NextOrdinal)
	}
	if len(ws.Entries) != 1 || ws.Entries[0].CMUXWorkspaceID != "CMUX-WS-1" || ws.Entries[0].Ordinal != 1 {
		t.Fatalf("entries = %+v, want one entry with id=CMUX-WS-1 ordinal=1", ws.Entries)
	}
}

func TestCLI_CMUX_Open_JSON_FailsWhenCapabilityMissing(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
			},
		},
		createID: "CMUX-WS-1",
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "open", "--format", "json", "WS1"})
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
	if resp.OK || resp.Error.Code != "cmux_capability_missing" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_CMUX_Open_JSON_Multi_Success(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath1 := filepath.Join(root, "workspaces", "WS1")
	wsPath2 := filepath.Join(root, "workspaces", "WS2")
	if err := os.MkdirAll(wsPath1, 0o755); err != nil {
		t.Fatalf("mkdir workspace1: %v", err)
	}
	if err := os.MkdirAll(wsPath2, 0o755); err != nil {
		t.Fatalf("mkdir workspace2: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath1, newWorkspaceMetaFileForCreate("WS1", "alpha", "", now)); err != nil {
		t.Fatalf("write workspace1 meta: %v", err)
	}
	if err := writeWorkspaceMetaFile(wsPath2, newWorkspaceMetaFileForCreate("WS2", "beta", "", now)); err != nil {
		t.Fatalf("write workspace2 meta: %v", err)
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createIDs: []string{"CMUX-WS-1", "CMUX-WS-2"},
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "open", "--format", "json", "--multi", "--workspace", "WS1", "--workspace", "WS2"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty: %q", err.String())
	}
	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			Count int `json:"count"`
			Items []struct {
				KraWorkspaceID  string `json:"kra_workspace_id"`
				CMUXWorkspaceID string `json:"cmux_workspace_id"`
			} `json:"items"`
		} `json:"result"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if !resp.OK || resp.Action != "cmux.open" || resp.Result.Count != 2 || len(resp.Result.Items) != 2 {
		t.Fatalf("unexpected json response: %+v", resp)
	}

	mapping, lerr := cmuxmap.NewStore(root).Load()
	if lerr != nil {
		t.Fatalf("load mapping: %v", lerr)
	}
	if len(mapping.Workspaces["WS1"].Entries) != 1 || len(mapping.Workspaces["WS2"].Entries) != 1 {
		t.Fatalf("mapping entries were not created for both targets: %+v", mapping.Workspaces)
	}
}

func TestCLI_CMUX_Open_JSON_MultipleTargetsRequireMulti(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "open", "--format", "json", "--workspace", "WS1", "--workspace", "WS2"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if resp.OK || resp.Error.Code != "invalid_argument" || !strings.Contains(resp.Error.Message, "multiple targets require --multi") {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_CMUX_Open_JSON_ConcurrencyRequiresMulti(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "open", "--format", "json", "--concurrency", "2", "--workspace", "WS1"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if resp.OK || resp.Error.Code != "invalid_argument" || !strings.Contains(resp.Error.Message, "--concurrency requires --multi") {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_CMUX_Open_JSON_MultiConcurrency_PartialFailure(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath1 := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath1, 0o755); err != nil {
		t.Fatalf("mkdir workspace1: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath1, newWorkspaceMetaFileForCreate("WS1", "alpha", "", now)); err != nil {
		t.Fatalf("write workspace1 meta: %v", err)
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createID: "CMUX-WS-1",
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"cmux", "open", "--format", "json", "--multi", "--concurrency", "2", "--workspace", "WS1", "--workspace", "WS9"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitError, err.String(), out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			Count     int `json:"count"`
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
		} `json:"result"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if resp.OK || resp.Action != "cmux.open" || resp.Error.Code != "partial_failure" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if resp.Result.Count != 2 || resp.Result.Succeeded != 1 || resp.Result.Failed != 1 {
		t.Fatalf("unexpected result summary: %+v", resp.Result)
	}

	mapping, lerr := cmuxmap.NewStore(root).Load()
	if lerr != nil {
		t.Fatalf("load mapping: %v", lerr)
	}
	if len(mapping.Workspaces["WS1"].Entries) != 1 {
		t.Fatalf("mapping entries for WS1 = %+v, want 1 entry", mapping.Workspaces["WS1"].Entries)
	}
}
