package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Create_Jira_Success(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	ticketKey := "PROJ-123"
	ticketURL := fmt.Sprintf("https://jira.example.com/browse/%s", ticketKey)
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/rest/api/3/issue/PROJ-123" {
			t.Fatalf("request path = %q, want %q", r.URL.Path, "/rest/api/3/issue/PROJ-123")
		}
		_, _ = w.Write([]byte(`{"key":"PROJ-123","fields":{"summary":"Implement login"}}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--jira", ticketURL})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	if !strings.Contains(out.String(), "✔ PROJ-123") {
		t.Fatalf("stdout missing created issue key: %q", out.String())
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("dev@example.com:token-123"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, wantAuth)
	}

	wsPath := filepath.Join(env.Root, "workspaces", "PROJ-123")
	if _, err := os.Stat(wsPath); err != nil {
		t.Fatalf("workspace dir not created: %v", err)
	}
	metaBytes, err := os.ReadFile(filepath.Join(wsPath, workspaceMetaFilename))
	if err != nil {
		t.Fatalf("read %s: %v", workspaceMetaFilename, err)
	}
	if !strings.Contains(string(metaBytes), `"source_url": "https://jira.example.com/browse/PROJ-123"`) {
		t.Fatalf("workspace metadata missing source_url: %q", string(metaBytes))
	}
	if !strings.Contains(string(metaBytes), `"title": "Implement login"`) {
		t.Fatalf("workspace metadata missing Jira summary title: %q", string(metaBytes))
	}
}

func TestCLI_WS_Create_Jira_MissingEnv_FailsFastWithoutWorkspaceCreation(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	t.Setenv("GIONX_JIRA_BASE_URL", "")
	t.Setenv("GIONX_JIRA_EMAIL", "")
	t.Setenv("GIONX_JIRA_API_TOKEN", "")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--jira", "https://jira.example.com/browse/PROJ-1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "missing jira env vars") {
		t.Fatalf("stderr missing missing-env error: %q", errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "PROJ-1")); !os.IsNotExist(err) {
		t.Fatalf("workspace dir should not exist, stat err=%v", err)
	}
}

func TestCLI_WS_Create_Jira_ConflictWithIDOrTitle_FailsUsage(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--id", "PROJ-123", "--jira", "https://jira.example.com/browse/PROJ-123"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitUsage, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "--jira cannot be combined with --id or --title") {
		t.Fatalf("stderr missing conflict error: %q", errBuf.String())
	}
}

func TestCLI_WS_Create_Jira_404_FailsFastWithoutStateMutation(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--jira", "https://jira.example.com/browse/PROJ-404"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "jira issue not found") {
		t.Fatalf("stderr missing jira not-found error: %q", errBuf.String())
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "PROJ-404")); !os.IsNotExist(err) {
		t.Fatalf("workspace dir should not exist, stat err=%v", err)
	}

}

func TestCLI_WS_Create_Jira_WithTemplateOption_AppliesTemplate(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	customTemplate := filepath.Join(env.Root, "templates", "custom")
	if err := os.MkdirAll(customTemplate, 0o755); err != nil {
		t.Fatalf("mkdir custom template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customTemplate, "CUSTOM.md"), []byte("custom\n"), 0o644); err != nil {
		t.Fatalf("write CUSTOM.md: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"key":"PROJ-200","fields":{"summary":"Template test"}}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"ws", "create", "--jira", "https://jira.example.com/browse/PROJ-200", "--template", "custom"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}

	wsPath := filepath.Join(env.Root, "workspaces", "PROJ-200")
	if _, err := os.Stat(filepath.Join(wsPath, "CUSTOM.md")); err != nil {
		t.Fatalf("workspace missing template file: %v", err)
	}
}
