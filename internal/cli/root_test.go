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
