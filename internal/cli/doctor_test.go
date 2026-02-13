package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_Doctor_Help(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra doctor") {
		t.Fatalf("stdout missing doctor usage: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr not empty: %q", errBuf.String())
	}
}

func TestCLI_Doctor_HealthyRoot(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS-1", "title", "", 100)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	got := out.String()
	if !strings.Contains(got, "Result:") || !strings.Contains(got, "warn: 0") || !strings.Contains(got, "error: 0") {
		t.Fatalf("stdout missing healthy summary: %q", got)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr not empty: %q", errBuf.String())
	}
}

func TestCLI_Doctor_DetectsInvalidWorkspaceMeta(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	metaPath := filepath.Join(wsPath, workspaceMetaFilename)
	if err := os.WriteFile(metaPath, []byte("{broken-json"), 0o644); err != nil {
		t.Fatalf("write broken meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	got := out.String()
	if !strings.Contains(got, "[workspace_meta_invalid]") {
		t.Fatalf("stdout missing invalid meta finding: %q", got)
	}
}

func TestCLI_Doctor_DetectsBindingMissingWorktree(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	meta := newWorkspaceMetaFileForCreate("WS-1", "title", "", 100)
	meta.ReposRestore = []workspaceMetaRepoRestore{{
		RepoUID: "github.com/example/repo",
		RepoKey: "example/repo",
		Alias:   "repo",
		Branch:  "WS-1",
		BaseRef: "origin/main",
	}}
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q)", code, exitOK, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "[binding_missing_worktree]") || !strings.Contains(got, "warn: 1") {
		t.Fatalf("stdout missing warning finding: %q", got)
	}
}

func TestCLI_Doctor_JSONAndFixValidation(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--format", "json", "--fix"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	var resp cliJSONResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if resp.OK || resp.Action != "doctor" || resp.Error == nil || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode, got %q", errBuf.String())
	}
}
