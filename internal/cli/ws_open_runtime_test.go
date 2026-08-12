package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/testutil"
)

type fakeCMUXOpenClient struct {
	mu           sync.Mutex
	capabilities cmuxctl.Capabilities
	createID     string
	createIDs    []string
	createIndex  int
	createErr    error
	createCmds   []string
	closeErr     error
	closed       []string
	renameErr    error
	selectErr    error
	selectErrBy  map[string]error

	renameWorkspace string
	renameTitle     string
	selectWorkspace string
	notify          cmuxctl.NotifyOptions
	sentWorkspace   string
	sentSurface     string
	sentText        string
	calls           []string
	identifyErr     map[string]error
	listWorkspaces  []cmuxctl.Workspace
	listErr         error
}

func (f *fakeCMUXOpenClient) Capabilities(context.Context) (cmuxctl.Capabilities, error) {
	return f.capabilities, nil
}

func (f *fakeCMUXOpenClient) CreateWorkspaceWithCommand(_ context.Context, command string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

func (f *fakeCMUXOpenClient) CloseWorkspace(_ context.Context, workspace string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, workspace)
	return f.closeErr
}

func (f *fakeCMUXOpenClient) RenameWorkspace(_ context.Context, workspace string, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renameWorkspace = workspace
	f.renameTitle = title
	return f.renameErr
}

func (f *fakeCMUXOpenClient) SelectWorkspace(_ context.Context, workspace string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "select:"+workspace)
	f.selectWorkspace = workspace
	if f.selectErrBy != nil {
		if err, ok := f.selectErrBy[workspace]; ok {
			return err
		}
	}
	return f.selectErr
}

func (f *fakeCMUXOpenClient) Notify(_ context.Context, opts cmuxctl.NotifyOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "notify:"+opts.Workspace)
	f.notify = opts
	return nil
}

func (f *fakeCMUXOpenClient) SendText(_ context.Context, workspace string, surface string, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "send:"+workspace)
	f.sentWorkspace = workspace
	f.sentSurface = surface
	f.sentText = text
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
	f.mu.Lock()
	defer f.mu.Unlock()
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

func TestCLI_WSOpen_InitRoot_KeepsCMUXMappingIgnoredAndCommitsMetadata(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	var createOut bytes.Buffer
	var createErr bytes.Buffer
	createCLI := New(&createOut, &createErr)
	if code := createCLI.Run([]string{"ws", "create", "--no-prompt", "WS1"}); code != exitOK {
		t.Fatalf("ws create exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, createErr.String(), createOut.String())
	}

	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{Methods: map[string]struct{}{
			"workspace.create": {},
			"workspace.rename": {},
		}},
		createID: "CMUX-WS-1",
	}
	previousClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = previousClient })

	var out bytes.Buffer
	var errOut bytes.Buffer
	c := New(&out, &errOut)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws open exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, errOut.String(), out.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.open" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	mappingPathspec := filepath.ToSlash(filepath.Join(".kra", "state", "cmux-workspaces.json"))
	mapping, loadErr := cmuxmap.NewStore(env.Root).Load()
	if loadErr != nil {
		t.Fatalf("load mapping: %v", loadErr)
	}
	if got := mapping.Workspaces["WS1"].Entries; len(got) != 1 || got[0].CMUXWorkspaceID != "CMUX-WS-1" {
		t.Fatalf("mapping = %+v", mapping.Workspaces["WS1"])
	}
	if ignored := strings.TrimSpace(runGit(t, env.Root, "check-ignore", "--", mappingPathspec)); ignored != mappingPathspec {
		t.Fatalf("check-ignore = %q, want %q", ignored, mappingPathspec)
	}
	if tracked := strings.TrimSpace(runGit(t, env.Root, "ls-files", "--", mappingPathspec)); tracked != "" {
		t.Fatalf("runtime mapping is tracked: %q", tracked)
	}
	if status := strings.TrimSpace(runGit(t, env.Root, "status", "--porcelain")); status != "" {
		t.Fatalf("runtime mapping artifacts must stay out of git status: %q", status)
	}
	if subject := strings.TrimSpace(runGit(t, env.Root, "log", "-1", "--pretty=%s")); subject != "open: WS1" {
		t.Fatalf("HEAD subject = %q, want %q", subject, "open: WS1")
	}
	changed := strings.Fields(runGit(t, env.Root, "show", "--pretty=format:", "--name-only", "HEAD"))
	wantChanged := filepath.ToSlash(filepath.Join("workspaces", "WS1", workspaceMetaFilename))
	if len(changed) != 1 || changed[0] != wantChanged {
		t.Fatalf("HEAD changed paths = %v, want [%s]", changed, wantChanged)
	}
	assertWorkspaceWorkState(t, filepath.Join(env.Root, "workspaces", "WS1"), workspaceWorkStateInProgress)
}

func TestCLI_WSOpen_AllInvalidTargetsHaveNoCMUXSideEffects(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	clientCreations := 0
	previousClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient {
		clientCreations++
		return &fakeCMUXOpenClient{}
	}
	t.Cleanup(func() { newCMUXOpenClient = previousClient })

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := New(&out, &errOut).Run([]string{
		"ws", "open", "--multi-select", "--concurrency", "2",
		"--workspace", "MISSING-1", "--workspace", "MISSING-2", "--format", "json",
	})
	if code != exitError || errOut.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q output=%q", code, errOut.String(), out.String())
	}
	if clientCreations != 0 {
		t.Fatalf("cmux client creations = %d, want no runtime access", clientCreations)
	}
	response := decodeJSONResponse(t, out.String())
	if response.OK || response.Error.Code != "partial_failure" {
		t.Fatalf("response = %#v", response)
	}
	if response.Result["succeeded"] != float64(0) || response.Result["failed"] != float64(2) {
		t.Fatalf("result = %#v", response.Result)
	}
	if _, err := os.Stat(cmuxmap.MappingPath(env.Root)); !os.IsNotExist(err) {
		t.Fatalf("cmux mapping should not be created, stat err=%v", err)
	}
}

func TestCLI_CMUX_Open_JSON_Command_CreatesWorkspaceWithCommandAfterCD(t *testing.T) {
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
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1", "--command", "echo hello"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if len(fake.createCmds) != 1 || !strings.Contains(fake.createCmds[0], "cd ") || !strings.Contains(fake.createCmds[0], wsPath) || strings.Contains(fake.createCmds[0], "echo hello") {
		t.Fatalf("create command = %+v, want cd only", fake.createCmds)
	}
	if fake.sentWorkspace != "CMUX-WS-1" || fake.sentSurface != "surface-id-CMUX-WS-1" || fake.sentText != "echo hello\n" {
		t.Fatalf("sent command workspace=%q surface=%q text=%q", fake.sentWorkspace, fake.sentSurface, fake.sentText)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.Result["command"] != "echo hello" || resp.Result["command_executed"] != true {
		t.Fatalf("command result = %+v", resp.Result)
	}
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

func TestCLI_CMUX_Open_JSON_FailsWhenRenameFails(t *testing.T) {
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
		createID:  "CMUX-WS-1",
		renameErr: errors.New("boom"),
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
	if len(resp.Result.Failures) != 1 || resp.Result.Failures[0].Code != "cmux_rename_failed" {
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

func TestCLI_WS_Open_JSON_Command_SendsToExistingWorkspaceSurface(t *testing.T) {
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
			},
		},
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "open", "--format", "json", "--id", "WS1", "--command", "make test"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}
	if fake.sentWorkspace != "CMUX-EXISTING" || fake.sentSurface != "surface-id-CMUX-EXISTING" || fake.sentText != "make test\n" {
		t.Fatalf("sent command workspace=%q surface=%q text=%q", fake.sentWorkspace, fake.sentSurface, fake.sentText)
	}
	if strings.Join(fake.calls, ",") != "send:CMUX-EXISTING,notify:CMUX-EXISTING" {
		t.Fatalf("call order = %v, want send then notify", fake.calls)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.Result["command"] != "make test" || resp.Result["command_executed"] != true {
		t.Fatalf("command result = %+v", resp.Result)
	}
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
