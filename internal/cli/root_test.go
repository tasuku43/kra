package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

func TestCLI_Root_Current_Human(t *testing.T) {
	root := prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "current"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	if strings.TrimSpace(out.String()) != root {
		t.Fatalf("stdout=%q, want=%q", strings.TrimSpace(out.String()), root)
	}
}

func TestCLI_Root_Open_JSON_Success(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	fake := &fakeCMUXOpenClient{
		capabilities: cmuxctl.Capabilities{
			Methods: map[string]struct{}{
				"workspace.create": {},
				"workspace.rename": {},
				"workspace.select": {},
			},
		},
		createID: "CMUX-ROOT-1",
	}
	prevClient := newCMUXOpenClient
	newCMUXOpenClient = func() cmuxOpenClient { return fake }
	t.Cleanup(func() { newCMUXOpenClient = prevClient })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "open", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q out=%q)", code, exitOK, err.String(), out.String())
	}

	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if !resp.OK || resp.Action != "root.open" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if fake.statusLabel != "kra" || fake.statusText != "kra:root" {
		t.Fatalf("status label/text = %q/%q, want %q/%q", fake.statusLabel, fake.statusText, "kra", "kra:root")
	}

	mapping, lerr := cmuxmap.NewStore(root).Load()
	if lerr != nil {
		t.Fatalf("load mapping: %v", lerr)
	}
	ws, ok := mapping.Workspaces[rootCMUXMappingID]
	if !ok || len(ws.Entries) != 1 {
		t.Fatalf("root mapping missing: %+v", mapping.Workspaces)
	}
}

func TestCLI_Root_Open_JSON_FallbackToCD(t *testing.T) {
	root := prepareCurrentRootForTest(t)
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
	code := c.Run([]string{"root", "open", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d", code, exitOK)
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if mode, _ := resp.Result["mode"].(string); mode != "fallback-cd" {
		t.Fatalf("mode=%q, want fallback-cd", mode)
	}
	action, readErr := os.ReadFile(actionFile)
	if readErr != nil {
		t.Fatalf("read action file: %v", readErr)
	}
	if !strings.HasPrefix(string(action), "cd ") || !strings.Contains(string(action), root) {
		t.Fatalf("unexpected action: %q", string(action))
	}
}

func TestCLI_RootMigrate_PlanDoesNotWrite(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	if err := os.MkdirAll(filepath.Join(root, "templates", "default"), 0o755); err != nil {
		t.Fatalf("mkdir default template: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WS1"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "plan: 2 action(s)") {
		t.Fatalf("stdout missing plan count: %q", out.String())
	}
	for _, path := range []string{
		filepath.Join(root, ".cmux", "dock.json"),
		filepath.Join(root, "templates", "default", ".cmux", "dock.json"),
		filepath.Join(root, "templates", "default", workspaceDocumentFilename),
		filepath.Join(root, "workspaces", "WS1", workspaceDocumentFilename),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("plan should not write %s, stat err=%v", path, statErr)
		}
	}
}

func TestCLI_RootMigrate_ApplyWritesDefaultTemplateAndActiveWorkspaces(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	if err := os.MkdirAll(filepath.Join(root, "templates", "default"), 0o755); err != nil {
		t.Fatalf("mkdir default template: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WS1"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive", "OLD1"), 0o755); err != nil {
		t.Fatalf("mkdir archived workspace: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "apply: 2 action(s)") {
		t.Fatalf("stdout missing apply count: %q", out.String())
	}
	for _, path := range []string{
		filepath.Join(root, "templates", "default", workspaceDocumentFilename),
		filepath.Join(root, "workspaces", "WS1", workspaceDocumentFilename),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected migrated path %s: %v", path, statErr)
		}
	}
	for _, path := range []string{
		filepath.Join(root, ".cmux", "dock.json"),
		filepath.Join(root, "templates", "default", ".cmux", "dock.json"),
		filepath.Join(root, "workspaces", "WS1", ".cmux", "dock.json"),
		filepath.Join(root, "archive", "OLD1", workspaceDocumentFilename),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("path should not be migrated %s, stat err=%v", path, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(root, "archive", "OLD1", ".cmux", "dock.json")); !os.IsNotExist(statErr) {
		t.Fatalf("archived workspace should not be migrated by default, stat err=%v", statErr)
	}
}

func TestCLI_RootMigrate_DoesNotOverwriteExistingFiles(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	ws := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(filepath.Join(root, "templates", "default"), 0o755); err != nil {
		t.Fatalf("mkdir template: %v", err)
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "default", workspaceDocumentFilename), []byte("custom template tasks\n"), 0o644); err != nil {
		t.Fatalf("write template tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws, workspaceDocumentFilename), []byte("custom tasks\n"), 0o644); err != nil {
		t.Fatalf("write workspace tasks: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "repos", "repo1"), 0o755); err != nil {
		t.Fatalf("mkdir repos: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws, "repos", "repo1", workspaceDocumentFilename), []byte("repo tasks\n"), 0o644); err != nil {
		t.Fatalf("write repo tasks: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	templateTasks, readErr := os.ReadFile(filepath.Join(root, "templates", "default", workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read template tasks: %v", readErr)
	}
	if string(templateTasks) != "custom template tasks\n" {
		t.Fatalf("template tasks were overwritten: %q", string(templateTasks))
	}
	workspaceTasks, readErr := os.ReadFile(filepath.Join(ws, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace tasks: %v", readErr)
	}
	if string(workspaceTasks) != "custom tasks\n" {
		t.Fatalf("workspace tasks were overwritten: %q", string(workspaceTasks))
	}
	repoTasks, readErr := os.ReadFile(filepath.Join(ws, "repos", "repo1", workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read repo tasks: %v", readErr)
	}
	if string(repoTasks) != "repo tasks\n" {
		t.Fatalf("repo tasks were touched: %q", string(repoTasks))
	}
}

func TestCLI_RootMigrate_ApplyMigratesManagedLegacyDockToGlobal(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	ws := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(filepath.Join(root, "templates", "default", ".cmux"), 0o755); err != nil {
		t.Fatalf("mkdir template .cmux: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".cmux"), 0o755); err != nil {
		t.Fatalf("mkdir workspace .cmux: %v", err)
	}
	oldDock := `{
  "controls": [
    {
      "id": "kra-tasks",
      "title": "Status",
      "command": "kra ws status --current",
      "cwd": ".",
      "height": 420
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".cmux", "dock.json"), []byte(oldDock), 0o644); err != nil {
		t.Fatalf("write workspace dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(filepath.Join(ws, ".cmux", "dock.json")); !os.IsNotExist(statErr) {
		t.Fatalf("managed workspace dock should be removed, stat err=%v", statErr)
	}
	dockBytes, readErr := os.ReadFile(filepath.Join(home, ".config", "cmux", "dock.json"))
	if readErr != nil {
		t.Fatalf("read global dock: %v", readErr)
	}
	var dock cmuxDockConfig
	if err := json.Unmarshal(dockBytes, &dock); err != nil {
		t.Fatalf("unmarshal dock: %v", err)
	}
	if len(dock.Controls) != 1 {
		t.Fatalf("controls len = %d, want 1", len(dock.Controls))
	}
	want := "kra ws status --cmux-current"
	if dock.Controls[0].Command != want {
		t.Fatalf("dock command = %q, want %q", dock.Controls[0].Command, want)
	}
}

func TestCLI_RootMigrate_GlobalDockPreservesExistingControlsAndUpdatesKraTasks(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	globalPath := filepath.Join(home, ".config", "cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global cmux: %v", err)
	}
	existing := `{
  "controls": [
    {
      "id": "custom",
      "title": "Custom",
      "command": "echo custom",
      "height": 100
    },
    {
      "id": "kra-tasks",
      "title": "Old",
      "command": "kra ws task view --current --watch",
      "height": 360
    }
  ]
}
`
	if err := os.WriteFile(globalPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write global dock: %v", err)
	}
	rootDock := filepath.Join(root, ".cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(rootDock), 0o755); err != nil {
		t.Fatalf("mkdir root dock dir: %v", err)
	}
	if err := os.WriteFile(rootDock, []byte(defaultWorkspaceCMUXDockContent()), 0o644); err != nil {
		t.Fatalf("write root dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	b, readErr := os.ReadFile(globalPath)
	if readErr != nil {
		t.Fatalf("read global dock: %v", readErr)
	}
	var dock cmuxDockConfig
	if err := json.Unmarshal(b, &dock); err != nil {
		t.Fatalf("unmarshal global dock: %v", err)
	}
	if len(dock.Controls) != 2 {
		t.Fatalf("controls len=%d, want 2: %+v", len(dock.Controls), dock.Controls)
	}
	if dock.Controls[0].ID != "custom" || dock.Controls[0].Command != "echo custom" {
		t.Fatalf("custom control not preserved: %+v", dock.Controls)
	}
	want := "kra ws status --cmux-current"
	if dock.Controls[1].ID != "kra-tasks" || dock.Controls[1].Command != want || dock.Controls[1].Height != 420 {
		t.Fatalf("kra-tasks not updated to standard: %+v", dock.Controls[1])
	}
}

func TestCLI_RootMigrate_GlobalDockUpdatesOldKraTasksWithoutLegacyProjectDock(t *testing.T) {
	prepareCurrentRootForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	globalPath := filepath.Join(home, ".config", "cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global cmux: %v", err)
	}
	existing := `{
  "controls": [
    {
      "id": "kra-tasks",
      "title": "Tasks",
      "command": "kra ws task tui --cmux-current --refresh 2s",
      "height": 420
    }
  ]
}
`
	if err := os.WriteFile(globalPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write global dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	b, readErr := os.ReadFile(globalPath)
	if readErr != nil {
		t.Fatalf("read global dock: %v", readErr)
	}
	var dock cmuxDockConfig
	if err := json.Unmarshal(b, &dock); err != nil {
		t.Fatalf("unmarshal global dock: %v", err)
	}
	if len(dock.Controls) != 1 {
		t.Fatalf("controls len=%d, want 1: %+v", len(dock.Controls), dock.Controls)
	}
	if dock.Controls[0].Title != "Status" || dock.Controls[0].Command != "kra ws status --cmux-current" {
		t.Fatalf("global kra-tasks not migrated: %+v", dock.Controls[0])
	}
}

func TestCLI_RootMigrate_AddsTaskHandoffGuideToExistingWorkspaceDocument(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	ws := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	content := "# Workspace\n\n## Current State\n\nExisting state.\n\n## Tasks\n\n### TASK-001 First\nstatus: todo\n"
	if err := os.WriteFile(filepath.Join(ws, workspaceDocumentFilename), []byte(content), 0o644); err != nil {
		t.Fatalf("write workspace.md: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	b, readErr := os.ReadFile(filepath.Join(ws, workspaceDocumentFilename))
	if readErr != nil {
		t.Fatalf("read workspace.md: %v", readErr)
	}
	got := string(b)
	if !strings.Contains(got, "This file is the workspace handoff state. Keep it current.") {
		t.Fatalf("workspace.md missing handoff guide: %q", got)
	}
	if strings.Contains(got, "## Next") {
		t.Fatalf("workspace.md should not add dedicated Next section: %q", got)
	}
}

func TestCLI_RootMigrate_LeavesMixedAndCustomProjectDockUnchanged(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	ws := filepath.Join(root, "workspaces", "WS1")
	mixedPath := filepath.Join(root, "templates", "default", ".cmux", "dock.json")
	customPath := filepath.Join(ws, ".cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(mixedPath), 0o755); err != nil {
		t.Fatalf("mkdir mixed dock dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(customPath), 0o755); err != nil {
		t.Fatalf("mkdir custom dock dir: %v", err)
	}
	mixed := `{
  "controls": [
    {
      "id": "kra-tasks",
      "title": "Status",
      "command": "kra ws task view --current --watch",
      "height": 420
    },
    {
      "id": "custom",
      "title": "Custom",
      "command": "echo custom",
      "height": 100
    }
  ]
}
`
	custom := `{
  "controls": [
    {
      "id": "custom",
      "title": "Custom",
      "command": "echo custom",
      "height": 100
    }
  ]
}
`
	if err := os.WriteFile(mixedPath, []byte(mixed), 0o644); err != nil {
		t.Fatalf("write mixed dock: %v", err)
	}
	if err := os.WriteFile(customPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("write custom dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	for path, want := range map[string]string{mixedPath: mixed, customPath: custom} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if string(got) != want {
			t.Fatalf("%s changed:\n%s", path, string(got))
		}
	}
	if !strings.Contains(out.String(), "project-local Dock config remains") {
		t.Fatalf("stdout missing warning: %q", out.String())
	}
}

func TestCLI_RootMigrate_InvalidGlobalDockFailsClosed(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	globalPath := filepath.Join(home, ".config", "cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("mkdir global cmux: %v", err)
	}
	if err := os.WriteFile(globalPath, []byte("{not json\n"), 0o644); err != nil {
		t.Fatalf("write invalid global dock: %v", err)
	}
	legacyPath := filepath.Join(root, ".cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy dock dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(defaultWorkspaceCMUXDockContent()), 0o644); err != nil {
		t.Fatalf("write legacy dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code == exitOK {
		t.Fatalf("exit code=%d, want failure (out=%q)", code, out.String())
	}
	got, readErr := os.ReadFile(globalPath)
	if readErr != nil {
		t.Fatalf("read global dock: %v", readErr)
	}
	if string(got) != "{not json\n" {
		t.Fatalf("invalid global dock was modified: %q", string(got))
	}
	if _, statErr := os.Stat(legacyPath); statErr != nil {
		t.Fatalf("legacy dock should remain on failure: %v", statErr)
	}
}

func TestCLI_RootMigrate_InvalidProjectDockFailsClosed(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	legacyPath := filepath.Join(root, "templates", "default", ".cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy dock dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("{not json\n"), 0o644); err != nil {
		t.Fatalf("write invalid legacy dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code == exitOK {
		t.Fatalf("exit code=%d, want failure (out=%q)", code, out.String())
	}
	got, readErr := os.ReadFile(legacyPath)
	if readErr != nil {
		t.Fatalf("read legacy dock: %v", readErr)
	}
	if string(got) != "{not json\n" {
		t.Fatalf("invalid legacy dock was modified: %q", string(got))
	}
}

func TestCLI_RootMigrate_JSONIncludesGlobalAndLegacyDockResults(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	legacyPath := filepath.Join(root, ".cmux", "dock.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy dock dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(defaultWorkspaceCMUXDockContent()), 0o644); err != nil {
		t.Fatalf("write legacy dock: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	var resp testJSONResponse
	if uerr := json.Unmarshal(out.Bytes(), &resp); uerr != nil {
		t.Fatalf("json unmarshal error: %v", uerr)
	}
	if resp.Action != "root.migrate" {
		t.Fatalf("action=%q, want root.migrate", resp.Action)
	}
	global, ok := resp.Result["global_dock"].(map[string]any)
	if !ok {
		t.Fatalf("missing global_dock: %+v", resp.Result)
	}
	if global["path"] == "" || global["changed"] != true || global["created"] != true {
		t.Fatalf("unexpected global_dock: %+v", global)
	}
	legacy, ok := resp.Result["legacy_project_docks"].([]any)
	if !ok || len(legacy) != 1 {
		t.Fatalf("unexpected legacy_project_docks: %+v", resp.Result["legacy_project_docks"])
	}
	item, ok := legacy[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected legacy item: %+v", legacy[0])
	}
	if item["path"] != legacyPath || item["kind"] != "root" || item["action"] != "remove" {
		t.Fatalf("unexpected legacy item: %+v", item)
	}
}
