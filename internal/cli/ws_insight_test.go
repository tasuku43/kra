package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCLI_WS_Insight_Help(t *testing.T) {
	t.Setenv(experimentsEnvKey, experimentInsightCapture)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "insight", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws insight") {
		t.Fatalf("stdout missing insight usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Insight_DisabledByDefault(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "insight", "--help"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "ws insight is experimental") {
		t.Fatalf("stderr missing experimental guidance: %q", err.String())
	}
}

func TestCLI_WS_InsightAdd_Human_Succeeds(t *testing.T) {
	t.Setenv(experimentsEnvKey, experimentInsightCapture)
	root := prepareCurrentRootForTest(t)
	workspacePath := filepath.Join(root, "workspaces", "WS1")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{
		"ws", "insight", "add",
		"--id", "WS1",
		"--ticket", "OPS-011",
		"--session-id", "session-123",
		"--what", "Retry policy with bounded backoff reduced failure retries",
		"--context", "Incident triage in checkout service",
		"--why", "It reduced noisy retries and clarified root cause",
		"--next", "Reuse the policy in payment worker",
		"--tag", "incident",
		"--tag", "retry",
		"--approved",
	})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
	if !strings.Contains(out.String(), "Insight saved:") {
		t.Fatalf("stdout missing result summary: %q", out.String())
	}

	insightsDir := filepath.Join(workspacePath, "worklog", "insights")
	entries, readErr := os.ReadDir(insightsDir)
	if readErr != nil {
		t.Fatalf("read insights dir: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("insight file count = %d, want 1", len(entries))
	}
	name := entries[0].Name()
	if !regexp.MustCompile(`^\d{8}-\d{6}-[a-z0-9-]+\.md$`).MatchString(name) {
		t.Fatalf("filename %q does not match expected pattern", name)
	}
	contentBytes, readFileErr := os.ReadFile(filepath.Join(insightsDir, name))
	if readFileErr != nil {
		t.Fatalf("read insight file: %v", readFileErr)
	}
	content := string(contentBytes)
	for _, want := range []string{
		"kind: insight",
		`ticket: "OPS-011"`,
		`workspace: "WS1"`,
		`session_id: "session-123"`,
		"## Context",
		"## What happened",
		"## Why it matters",
		"## Next reuse",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("insight content missing %q: %q", want, content)
		}
	}
}

func TestCLI_WS_InsightAdd_JSON_SucceedsForArchivedWorkspace(t *testing.T) {
	t.Setenv(experimentsEnvKey, experimentInsightCapture)
	root := prepareCurrentRootForTest(t)
	if err := os.MkdirAll(filepath.Join(root, "archive", "WSA"), 0o755); err != nil {
		t.Fatalf("mkdir archived workspace: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{
		"ws", "insight", "add",
		"--id", "WSA",
		"--ticket", "OPS-011",
		"--session-id", "session-archived",
		"--what", "Archived workspace insight write succeeded",
		"--approved",
		"--format", "json",
	})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.insight.add" || resp.WorkspaceID != "WSA" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	path, _ := resp.Result["path"].(string)
	if strings.TrimSpace(path) == "" {
		t.Fatalf("result.path is empty: %+v", resp.Result)
	}
	if !strings.Contains(path, filepath.Join("archive", "WSA", "worklog", "insights")) {
		t.Fatalf("result.path should point archive workspace, got %q", path)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("insight file missing: %v", statErr)
	}
}

func TestCLI_WS_InsightAdd_JSON_RequiresApproved(t *testing.T) {
	t.Setenv(experimentsEnvKey, experimentInsightCapture)
	root := prepareCurrentRootForTest(t)
	if err := os.MkdirAll(filepath.Join(root, "workspaces", "WS1"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{
		"ws", "insight", "add",
		"--id", "WS1",
		"--ticket", "OPS-011",
		"--session-id", "session-123",
		"--what", "Need approval",
		"--format", "json",
	})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "ws.insight.add" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if !strings.Contains(resp.Error.Message, "--approved is required") {
		t.Fatalf("error message mismatch: %q", resp.Error.Message)
	}
	if _, statErr := os.Stat(filepath.Join(root, "workspaces", "WS1", "worklog", "insights")); !os.IsNotExist(statErr) {
		t.Fatalf("insight dir should not be created when approval missing")
	}
}

func TestCLI_WS_InsightAdd_JSON_WorkspaceNotFound(t *testing.T) {
	t.Setenv(experimentsEnvKey, experimentInsightCapture)
	prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{
		"ws", "insight", "add",
		"--id", "WS404",
		"--ticket", "OPS-011",
		"--session-id", "session-123",
		"--what", "workspace missing",
		"--approved",
		"--format", "json",
	})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "ws.insight.add" || resp.Error.Code != "not_found" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
}
