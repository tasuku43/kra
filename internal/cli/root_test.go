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

func TestCLI_RootMigrate_PreservesExistingDockFiles(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}
	files := map[string]string{
		filepath.Join(root, ".cmux", "dock.json"):                         "{root user content\n",
		filepath.Join(root, "templates", "default", ".cmux", "dock.json"): "{template user content\n",
		filepath.Join(root, "workspaces", "WS1", ".cmux", "dock.json"):    "{workspace user content\n",
		filepath.Join(home, ".config", "cmux", "dock.json"):               "{global user content\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := New(&out, &errOut).Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, errOut.String())
	}
	for path, want := range files {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if string(got) != want {
			t.Fatalf("Dock file changed at %s: got %q, want %q", path, string(got), want)
		}
	}
	if strings.Contains(out.String(), "global Dock") || strings.Contains(out.String(), "legacy project Dock") || strings.Contains(out.String(), "kra-tasks") {
		t.Fatalf("root migrate still reports retired Dock migration: %q", out.String())
	}
}

func TestCLI_RootMigrate_JSONOmitsRetiredDockFields(t *testing.T) {
	prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	code := New(&out, &err).Run([]string{"root", "migrate", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	var resp testJSONResponse
	if decodeErr := json.Unmarshal(out.Bytes(), &resp); decodeErr != nil {
		t.Fatalf("json unmarshal error: %v", decodeErr)
	}
	for _, key := range []string{"global_dock", "legacy_project_docks", "recommendations"} {
		if _, ok := resp.Result[key]; ok {
			t.Fatalf("retired field %q is still present: %+v", key, resp.Result)
		}
	}
}
