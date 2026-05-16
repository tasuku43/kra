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
	if !strings.Contains(out.String(), "plan: 5 action(s)") {
		t.Fatalf("stdout missing plan count: %q", out.String())
	}
	for _, path := range []string{
		filepath.Join(root, ".cmux", "dock.json"),
		filepath.Join(root, "templates", "default", ".cmux", "dock.json"),
		filepath.Join(root, "templates", "default", "tasks.md"),
		filepath.Join(root, "workspaces", "WS1", ".cmux", "dock.json"),
		filepath.Join(root, "workspaces", "WS1", "tasks.md"),
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
	if !strings.Contains(out.String(), "apply: 5 action(s)") {
		t.Fatalf("stdout missing apply count: %q", out.String())
	}
	for _, path := range []string{
		filepath.Join(root, ".cmux", "dock.json"),
		filepath.Join(root, "templates", "default", ".cmux", "dock.json"),
		filepath.Join(root, "templates", "default", "tasks.md"),
		filepath.Join(root, "workspaces", "WS1", ".cmux", "dock.json"),
		filepath.Join(root, "workspaces", "WS1", "tasks.md"),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected migrated path %s: %v", path, statErr)
		}
	}
	rootDock, readErr := os.ReadFile(filepath.Join(root, ".cmux", "dock.json"))
	if readErr != nil {
		t.Fatalf("read root dock: %v", readErr)
	}
	assertDockJSONCommandContains(t, rootDock, "kra ws task tui --all --todo-only --refresh 2s")
	if _, statErr := os.Stat(filepath.Join(root, "archive", "OLD1", ".cmux", "dock.json")); !os.IsNotExist(statErr) {
		t.Fatalf("archived workspace should not be migrated by default, stat err=%v", statErr)
	}
}

func TestCLI_RootMigrate_DoesNotOverwriteExistingFiles(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	ws := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(filepath.Join(root, "templates", "default", ".cmux"), 0o755); err != nil {
		t.Fatalf("mkdir template .cmux: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".cmux"), 0o755); err != nil {
		t.Fatalf("mkdir workspace .cmux: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "default", ".cmux", "dock.json"), []byte("custom dock\n"), 0o644); err != nil {
		t.Fatalf("write template dock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws, "tasks.md"), []byte("custom tasks\n"), 0o644); err != nil {
		t.Fatalf("write workspace tasks: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"root", "migrate", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	templateDock, readErr := os.ReadFile(filepath.Join(root, "templates", "default", ".cmux", "dock.json"))
	if readErr != nil {
		t.Fatalf("read template dock: %v", readErr)
	}
	if string(templateDock) != "custom dock\n" {
		t.Fatalf("template dock was overwritten: %q", string(templateDock))
	}
	workspaceTasks, readErr := os.ReadFile(filepath.Join(ws, "tasks.md"))
	if readErr != nil {
		t.Fatalf("read workspace tasks: %v", readErr)
	}
	if string(workspaceTasks) != "custom tasks\n" {
		t.Fatalf("workspace tasks were overwritten: %q", string(workspaceTasks))
	}
}

func TestCLI_RootMigrate_ApplyUsesDetectedShellInitForManagedDockCommand(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		t.Fatalf("user home dir: %v", homeErr)
	}
	t.Setenv("SHELL", "/bin/zsh")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export PATH=$PATH\n"), 0o644); err != nil {
		t.Fatalf("write .zshrc: %v", err)
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
      "title": "Tasks",
      "command": "kra ws task tui --current --refresh 2s",
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
	dockBytes, readErr := os.ReadFile(filepath.Join(ws, ".cmux", "dock.json"))
	if readErr != nil {
		t.Fatalf("read workspace dock: %v", readErr)
	}
	var dock cmuxDockConfig
	if err := json.Unmarshal(dockBytes, &dock); err != nil {
		t.Fatalf("unmarshal dock: %v", err)
	}
	if len(dock.Controls) != 1 {
		t.Fatalf("controls len = %d, want 1", len(dock.Controls))
	}
	want := "source ~/.zshrc; kra ws task tui --current --refresh 2s"
	if dock.Controls[0].Command != want {
		t.Fatalf("dock command = %q, want %q", dock.Controls[0].Command, want)
	}
}
