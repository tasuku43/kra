package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/paths"
)

func TestCLI_Run_StateChangingCommand_BootstrapsGlobalConfig(t *testing.T) {
	kraHome := setKraHomeForTest(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatalf("create workspaces/: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("create archive/: %v", err)
	}
	seedDefaultTemplate(t, root)
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output=%s)", args, err, string(out))
		}
	}
	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")
	writeCurrentContextForTest(t, root)

	globalConfigPath := filepath.Join(kraHome, "config.yaml")
	if _, err := os.Stat(globalConfigPath); !os.IsNotExist(err) {
		t.Fatalf("global config should not exist before run: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "create", "--no-prompt", "CFG-BOOT-001"})
	if code != exitOK {
		t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}

	b, readErr := os.ReadFile(globalConfigPath)
	if readErr != nil {
		t.Fatalf("read global config: %v", readErr)
	}
	if !strings.Contains(string(b), "Precedence (high -> low)") {
		t.Fatalf("global config missing precedence comment: %q", string(b))
	}
}

func TestCLI_Run_ReadOnlyCommand_DoesNotBootstrapGlobalConfig(t *testing.T) {
	kraHome := setKraHomeForTest(t)
	globalConfigPath := filepath.Join(kraHome, "config.yaml")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"version"})
	if code != exitOK {
		t.Fatalf("version exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(globalConfigPath); !os.IsNotExist(statErr) {
		t.Fatalf("global config should not be created for read-only command: %v", statErr)
	}
}

func TestShouldBootstrapGlobalConfig_WSActPolicy(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "ws create", args: []string{"ws", "create", "--no-prompt", "WS1"}, want: true},
		{name: "ws act close", args: []string{"ws", "close", "--id", "WS1"}, want: true},
		{name: "ws act save", args: []string{"ws", "save", "--id", "WS1"}, want: true},
		{name: "ws act resume", args: []string{"ws", "resume", "--id", "WS1", "--latest"}, want: true},
		{name: "ws act remove-repo", args: []string{"ws", "remove-repo", "--id", "WS1"}, want: true},
		{name: "ws act go", args: []string{"ws", "go", "--id", "WS1"}, want: false},
		{name: "ws list", args: []string{"ws", "list"}, want: false},
		{name: "ws act close help", args: []string{"ws", "close", "--help"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldBootstrapGlobalConfig(tc.args); got != tc.want {
				t.Fatalf("shouldBootstrapGlobalConfig(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func writeCurrentContextForTest(t *testing.T, root string) {
	t.Helper()
	if err := paths.WriteCurrentContext(root); err != nil {
		t.Fatalf("WriteCurrentContext() error: %v", err)
	}
}
