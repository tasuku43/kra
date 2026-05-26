package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/core/cmuxstyle"
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
	selectErrBy  map[string]error
	setStatusErr error

	renameWorkspace string
	renameTitle     string
	selectWorkspace string
	statusWorkspace string
	statusLabel     string
	statusText      string
	statusIcon      string
	statusColor     string
	notify          cmuxctl.NotifyOptions
	calls           []string
	identifyErr     map[string]error
	listWorkspaces  []cmuxctl.Workspace
	listErr         error
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
	f.calls = append(f.calls, "select:"+workspace)
	f.selectWorkspace = workspace
	if f.selectErrBy != nil {
		if err, ok := f.selectErrBy[workspace]; ok {
			return err
		}
	}
	return f.selectErr
}

func (f *fakeCMUXOpenClient) SetStatus(_ context.Context, workspace string, label string, text string, icon string, color string) error {
	f.statusWorkspace = workspace
	f.statusLabel = label
	f.statusText = text
	f.statusIcon = icon
	f.statusColor = color
	return f.setStatusErr
}

func (f *fakeCMUXOpenClient) Notify(_ context.Context, opts cmuxctl.NotifyOptions) error {
	f.calls = append(f.calls, "notify:"+opts.Workspace)
	f.notify = opts
	return nil
}

func (f *fakeCMUXOpenClient) ListPanes(_ context.Context, workspace string) ([]cmuxctl.Pane, error) {
	return []cmuxctl.Pane{{
		ID:                "pane-" + workspace,
		Ref:               "pane:" + workspace,
		Focused:           true,
		SelectedSurfaceID: "surface-id-" + workspace,
	}}, nil
}

func (f *fakeCMUXOpenClient) Identify(_ context.Context, workspace string, _ string) (map[string]any, error) {
	if f.identifyErr != nil {
		if err, ok := f.identifyErr[workspace]; ok {
			return nil, err
		}
	}
	return map[string]any{
		"workspace_id": workspace,
		"focused": map[string]any{
			"surface_ref": "surface:" + workspace,
		},
	}, nil
}

func (f *fakeCMUXOpenClient) ListWorkspaces(context.Context) ([]cmuxctl.Workspace, error) {
	return f.listWorkspaces, f.listErr
}

func assertWorkspaceWorkState(t *testing.T, wsPath string, want workspaceWorkState) {
	t.Helper()
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		t.Fatalf("load workspace meta: %v", err)
	}
	if got := normalizeWorkspaceWorkState(workspaceWorkState(meta.Workspace.WorkState)); got != want {
		t.Fatalf("workspace.work_state = %q, want %q", meta.Workspace.WorkState, want)
	}
}

func TestCLI_CMUX_Open_JSON_RequiresWorkspaceIDWhenOmitted(t *testing.T) {
	prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "ws open requires one of --id <id>, --current, --select, --multi-select, or --all") {
		t.Fatalf("stderr should explain missing target mode: %q", err.String())
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
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1"})
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
	if !resp.OK || resp.Action != "ws.open" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if fake.renameWorkspace != "CMUX-WS-1" {
		t.Fatalf("rename workspace = %q, want %q", fake.renameWorkspace, "CMUX-WS-1")
	}
	if !strings.Contains(fake.renameTitle, "WS1 | hello world") {
		t.Fatalf("rename title = %q, want to contain %q", fake.renameTitle, "WS1 | hello world")
	}
	if fake.selectWorkspace != "" {
		t.Fatalf("select should be skipped for ws open, got %q", fake.selectWorkspace)
	}
	if fake.statusWorkspace != "CMUX-WS-1" || fake.statusLabel != "kra" || fake.statusText != "kra:workspace" || fake.statusIcon != "tag" || fake.statusColor != cmuxstyle.WorkspaceLabelColor {
		t.Fatalf("status args = workspace=%q label=%q text=%q icon=%q color=%q", fake.statusWorkspace, fake.statusLabel, fake.statusText, fake.statusIcon, fake.statusColor)
	}
	if fake.notify.Workspace != "CMUX-WS-1" || fake.notify.Surface != "surface-id-CMUX-WS-1" || fake.notify.Title != "kra workspace opened" || fake.notify.Subtitle != "WS1" {
		t.Fatalf("notify args = %+v", fake.notify)
	}
	if strings.Join(fake.calls, ",") != "notify:CMUX-WS-1" {
		t.Fatalf("call order = %v, want notify only", fake.calls)
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
	assertWorkspaceWorkState(t, wsPath, workspaceWorkStateInProgress)
}

func TestCLI_CMUX_Open_JSON_FallbacksToDirectoryWhenCapabilityMissing(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS1", "hello world", "", now)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	actionFile := filepath.Join(t.TempDir(), "action.sh")
	t.Setenv(shellActionFileEnv, actionFile)

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
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if !resp.OK || resp.Action != "ws.open" || resp.WorkspaceID != "WS1" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	mode, _ := resp.Result["mode"].(string)
	if mode != "fallback-cd" {
		t.Fatalf("mode = %q, want %q (result=%+v)", mode, "fallback-cd", resp.Result)
	}
	runtimeAvailable, ok := resp.Result["runtime_available"].(bool)
	if !ok || runtimeAvailable {
		t.Fatalf("runtime_available = %v, want false", resp.Result["runtime_available"])
	}

	action, readErr := os.ReadFile(actionFile)
	if readErr != nil {
		t.Fatalf("read action file: %v", readErr)
	}
	if !strings.HasPrefix(string(action), "cd ") || !strings.Contains(string(action), wsPath) {
		t.Fatalf("unexpected shell action: %q", string(action))
	}
	assertWorkspaceWorkState(t, wsPath, workspaceWorkStateInProgress)
}

func TestCLI_CMUX_Open_JSON_CapabilityMissing_MultiTargetRemainsError(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath1 := filepath.Join(root, "workspaces", "WS1")
	wsPath2 := filepath.Join(root, "workspaces", "WS2")
	if err := os.MkdirAll(wsPath1, 0o755); err != nil {
		t.Fatalf("mkdir workspace1: %v", err)
	}
	if err := os.MkdirAll(wsPath2, 0o755); err != nil {
		t.Fatalf("mkdir workspace2: %v", err)
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
	code := c.Run([]string{"ws", "open", "--format", "json", "--multi-select", "--workspace", "WS1", "--workspace", "WS2"})
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
	if !strings.Contains(resp.Error.Message, "single target only") {
		t.Fatalf("error message should mention fallback limit: %q", resp.Error.Message)
	}
}

func TestCLI_CMUX_Open_Human_FallbacksToDirectoryWhenCapabilityMissing(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	actionFile := filepath.Join(t.TempDir(), "action.sh")
	t.Setenv(shellActionFileEnv, actionFile)

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
			},
		},
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in human fallback mode: %q", err.String())
	}
	if !strings.Contains(out.String(), "fallback-cd") {
		t.Fatalf("stdout should mention fallback mode: %q", out.String())
	}
	action, readErr := os.ReadFile(actionFile)
	if readErr != nil {
		t.Fatalf("read action file: %v", readErr)
	}
	if !strings.HasPrefix(string(action), "cd ") || !strings.Contains(string(action), wsPath) {
		t.Fatalf("unexpected shell action: %q", string(action))
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
	code := c.Run([]string{"ws", "open", "--format", "json", "--multi-select", "--workspace", "WS1", "--workspace", "WS2"})
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
	if !resp.OK || resp.Action != "ws.open" || resp.Result.Count != 2 || len(resp.Result.Items) != 2 {
		t.Fatalf("unexpected json response: %+v", resp)
	}

	mapping, lerr := cmuxmap.NewStore(root).Load()
	if lerr != nil {
		t.Fatalf("load mapping: %v", lerr)
	}
	if len(mapping.Workspaces["WS1"].Entries) != 1 || len(mapping.Workspaces["WS2"].Entries) != 1 {
		t.Fatalf("mapping entries were not created for both targets: %+v", mapping.Workspaces)
	}
	assertWorkspaceWorkState(t, wsPath1, workspaceWorkStateInProgress)
	assertWorkspaceWorkState(t, wsPath2, workspaceWorkStateInProgress)
}

func TestCLI_CMUX_Open_JSON_All_Success(t *testing.T) {
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
	code := c.Run([]string{"ws", "open", "--format", "json", "--all"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.open" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["count"]; got != float64(2) {
		t.Fatalf("result.count = %v, want 2", got)
	}
	assertWorkspaceWorkState(t, wsPath1, workspaceWorkStateInProgress)
	assertWorkspaceWorkState(t, wsPath2, workspaceWorkStateInProgress)
}

func TestCLI_CMUX_Open_JSON_All_IgnoresConcurrencyAndRunsSequentially(t *testing.T) {
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
	clientCount := 0
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient {
		clientCount++
		return fake
	}
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--all", "--concurrency", "4"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if clientCount != 1 {
		t.Fatalf("cmux client count = %d, want 1 for sequential --all open", clientCount)
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.open" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["count"]; got != float64(2) {
		t.Fatalf("result.count = %v, want 2", got)
	}
}

func TestCLI_CMUX_Open_JSON_All_RejectsExplicitTargets(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--all", "--workspace", "WS1"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Error.Code != "invalid_argument" || !strings.Contains(resp.Error.Message, "--all cannot be used with explicit targets") {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCLI_CMUX_Open_JSON_MultipleTargetsRequireMulti(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1", "--workspace", "WS1", "--workspace", "WS2"})
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
	if resp.OK || resp.Error.Code != "invalid_argument" || !strings.Contains(resp.Error.Message, "multiple targets require --multi-select") {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_CMUX_Open_JSON_ConcurrencyRequiresMulti(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1", "--concurrency", "2", "--workspace", "WS1"})
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
	if resp.OK || resp.Error.Code != "invalid_argument" || !strings.Contains(resp.Error.Message, "--concurrency requires --multi-select") {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}

func TestCLI_CMUX_Open_JSON_LegacyMultiFlagRejected(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1", "--multi"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "unknown flag for ws open: \"--multi\"") {
		t.Fatalf("stderr should mention unknown legacy flag: %q", err.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty for usage error: %q", out.String())
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
	code := c.Run([]string{"ws", "open", "--format", "json", "--multi-select", "--concurrency", "2", "--workspace", "WS1", "--workspace", "WS9"})
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
	if resp.OK || resp.Action != "ws.open" || resp.Error.Code != "partial_failure" {
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
	assertWorkspaceWorkState(t, wsPath1, workspaceWorkStateInProgress)
}

func TestCLI_CMUX_Open_JSON_FailsWhenSetStatusFails(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS1", "alpha", "", now)); err != nil {
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
		createID:     "CMUX-WS-1",
		setStatusErr: errors.New("boom"),
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1"})
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
			Failures []struct {
				Code string `json:"code"`
			} `json:"failures"`
		} `json:"result"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if resp.OK || resp.Error.Code != "partial_failure" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Result.Failures) != 1 || resp.Result.Failures[0].Code != "cmux_set_status_failed" {
		t.Fatalf("unexpected failure detail: %+v", resp.Result.Failures)
	}
}

func TestCLI_WS_Open_JSON_ReusesExistingMapping_AsSwitchFallback(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS1", "alpha", "", now)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	store := cmuxmap.NewStore(root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{
						CMUXWorkspaceID: "CMUX-EXISTING",
						Ordinal:         1,
						TitleSnapshot:   "WS1 | alpha",
						CreatedAt:       time.Now().UTC().Format(time.RFC3339),
						LastUsedAt:      time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createID: "CMUX-NEW",
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if len(fake.createCmds) != 0 {
		t.Fatalf("create should not be called when mapping exists: %+v", fake.createCmds)
	}
	if fake.selectWorkspace != "" {
		t.Fatalf("select should be skipped for ws open, got %q", fake.selectWorkspace)
	}
	if fake.statusWorkspace != "" {
		t.Fatalf("status should not be called when mapping exists: workspace=%q", fake.statusWorkspace)
	}
	if fake.notify.Workspace != "CMUX-EXISTING" || fake.notify.Surface != "surface-id-CMUX-EXISTING" || fake.notify.Subtitle != "WS1" {
		t.Fatalf("notify args = %+v", fake.notify)
	}
	if strings.Join(fake.calls, ",") != "notify:CMUX-EXISTING" {
		t.Fatalf("call order = %v, want notify only", fake.calls)
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			CMUXWorkspaceID string `json:"cmux_workspace_id"`
			ReusedExisting  bool   `json:"reused_existing"`
		} `json:"result"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if !resp.OK || resp.Action != "ws.open" || resp.Result.CMUXWorkspaceID != "CMUX-EXISTING" || !resp.Result.ReusedExisting {
		t.Fatalf("unexpected response: %+v", resp)
	}
	assertWorkspaceWorkState(t, wsPath, workspaceWorkStateInProgress)
}

func TestCLI_WS_Open_JSON_RecreatesWhenMappedWorkspaceBecomesNotFoundOnSelect(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS1", "alpha", "", now)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	store := cmuxmap.NewStore(root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{
						CMUXWorkspaceID: "CMUX-EXISTING",
						Ordinal:         1,
						TitleSnapshot:   "WS1 | alpha",
						CreatedAt:       time.Now().UTC().Format(time.RFC3339),
						LastUsedAt:      time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createID: "CMUX-NEW",
		identifyErr: map[string]error{
			"CMUX-EXISTING": errors.New("cmux identify: Error: not_found: Workspace not found"),
		},
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if len(fake.createCmds) != 1 {
		t.Fatalf("create should be called once to recreate stale mapping: %+v", fake.createCmds)
	}
	if fake.selectWorkspace != "" {
		t.Fatalf("select should be skipped for ws open, got %q", fake.selectWorkspace)
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			CMUXWorkspaceID string `json:"cmux_workspace_id"`
			ReusedExisting  bool   `json:"reused_existing"`
		} `json:"result"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if !resp.OK || resp.Action != "ws.open" || resp.Result.CMUXWorkspaceID != "CMUX-NEW" || resp.Result.ReusedExisting {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCLI_WS_Open_JSON_RelinksByTitleWhenMappedWorkspaceIDIsStale(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	wsPath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	now := time.Now().Unix()
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS1", "alpha", "", now)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	store := cmuxmap.NewStore(root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{
						CMUXWorkspaceID: "CMUX-STALE",
						Ordinal:         1,
						TitleSnapshot:   "WS1 | alpha",
						CreatedAt:       time.Now().UTC().Format(time.RFC3339),
						LastUsedAt:      time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save mapping: %v", err)
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createID: "CMUX-NEW",
		identifyErr: map[string]error{
			"CMUX-STALE": errors.New("cmux identify: Error: not_found: Workspace not found"),
		},
		listWorkspaces: []cmuxctl.Workspace{
			{ID: "CMUX-RELINKED", Ref: "workspace:5", Title: "WS1 | alpha"},
		},
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if len(fake.createCmds) != 0 {
		t.Fatalf("create should not be called when title relink succeeds: %+v", fake.createCmds)
	}
	if fake.selectWorkspace != "" {
		t.Fatalf("select should be skipped for ws open, got %q", fake.selectWorkspace)
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			CMUXWorkspaceID string `json:"cmux_workspace_id"`
			ReusedExisting  bool   `json:"reused_existing"`
		} `json:"result"`
	}
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v (out=%q)", uerr, out.String())
	}
	if !resp.OK || resp.Action != "ws.open" || resp.Result.CMUXWorkspaceID != "CMUX-RELINKED" || !resp.Result.ReusedExisting {
		t.Fatalf("unexpected response: %+v", resp)
	}

	mapping, lerr := cmuxmap.NewStore(root).Load()
	if lerr != nil {
		t.Fatalf("load mapping: %v", lerr)
	}
	if got := mapping.Workspaces["WS1"].Entries[0].CMUXWorkspaceID; got != "CMUX-RELINKED" {
		t.Fatalf("mapping cmux_workspace_id = %q, want %q", got, "CMUX-RELINKED")
	}
}
