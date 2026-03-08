package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_WSDashboard_JSON_ActiveDefault(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["scope"]; got != "active" {
		t.Fatalf("result.scope = %v, want %q", got, "active")
	}
	workspaces, ok := resp.Result["workspaces"].([]any)
	if !ok || len(workspaces) == 0 {
		t.Fatalf("result.workspaces missing: %+v", resp.Result)
	}
	row, ok := workspaces[0].(map[string]any)
	if !ok {
		t.Fatalf("workspace row missing: %+v", workspaces[0])
	}
	if got := row["coverage"]; got != string(workspaceOutputCoverageEmpty) {
		t.Fatalf("workspace coverage = %v, want %q", got, workspaceOutputCoverageEmpty)
	}
	summary, ok := resp.Result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("result.summary missing: %+v", resp.Result)
	}
	coverageTotals, ok := summary["coverage_totals"].(map[string]any)
	if !ok {
		t.Fatalf("summary.coverage_totals missing: %+v", summary)
	}
	if got := coverageTotals[string(workspaceOutputCoverageEmpty)]; got != float64(1) {
		t.Fatalf("summary.coverage_totals.empty = %v, want %v", got, float64(1))
	}
}

func TestCLI_WSDashboard_JSON_ArchivedScope(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "archived", "WS1")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--archived", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --archived --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Result["scope"]; got != "archived" {
		t.Fatalf("result.scope = %v, want %q", got, "archived")
	}
}

func TestCLI_WSDashboard_JSON_WorkspaceFilterKeepsGlobalSummaryCounts(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	seedWorkspaceMeta(t, env.Root, "archived", "WS2")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--workspace", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --workspace WS1 --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	summary, ok := resp.Result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("result.summary missing: %+v", resp.Result)
	}
	if got := summary["active"]; got != float64(1) {
		t.Fatalf("summary.active = %v, want %v", got, float64(1))
	}
	if got := summary["archived"]; got != float64(1) {
		t.Fatalf("summary.archived = %v, want %v", got, float64(1))
	}
	workspaces, ok := resp.Result["workspaces"].([]any)
	if !ok || len(workspaces) != 1 {
		t.Fatalf("result.workspaces = %+v, want single workspace", resp.Result["workspaces"])
	}
}

func TestCLI_WSDashboard_JSON_ReadOnlyWarningsDoNotMutateMeta(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")

	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		t.Fatalf("load workspace meta: %v", err)
	}
	meta.Workspace.WorkState = ""
	meta.Baseline = nil
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"ws", "dashboard", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --format json exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	warnings, ok := resp.Result["warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("result.warnings missing: %+v", resp.Result)
	}
	if !containsDashboardJSONWarning(warnings, formatWorkspaceListMissingWorkStateWarning("WS1")) {
		t.Fatalf("warnings should include missing work_state: %+v", warnings)
	}
	if !containsDashboardJSONWarning(warnings, formatWorkspaceListMissingBaselineWarning("WS1")) {
		t.Fatalf("warnings should include missing baseline: %+v", warnings)
	}

	updated, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		t.Fatalf("load workspace meta after dashboard: %v", err)
	}
	if updated.Workspace.WorkState != "" {
		t.Fatalf("dashboard should not backfill work_state, got %q", updated.Workspace.WorkState)
	}
	if updated.Baseline != nil {
		t.Fatalf("dashboard should not create baseline")
	}
}

func TestCLI_WSDashboard_JSON_IncludesCoverageSummaryAndDetail(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	wsPath := seedWorkspaceMeta(t, env.Root, "active", "WS1")

	if err := os.MkdirAll(filepath.Join(wsPath, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(wsPath, "artifacts"), 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "notes", "investigation.md"), []byte("note\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "artifacts", "evidence.txt"), []byte("artifact\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "dashboard", "--workspace", "WS1", "--format", "json"})
	if code != exitOK {
		t.Fatalf("ws dashboard --workspace WS1 --format json exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if !resp.OK || resp.Action != "ws.dashboard" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	summary, ok := resp.Result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("result.summary missing: %+v", resp.Result)
	}
	coverageTotals, ok := summary["coverage_totals"].(map[string]any)
	if !ok {
		t.Fatalf("summary.coverage_totals missing: %+v", summary)
	}
	if got := coverageTotals[string(workspaceOutputCoverageDocumented)]; got != float64(1) {
		t.Fatalf("summary.coverage_totals.documented = %v, want %v", got, float64(1))
	}
	workspaces, ok := resp.Result["workspaces"].([]any)
	if !ok || len(workspaces) != 1 {
		t.Fatalf("result.workspaces = %+v, want single workspace", resp.Result["workspaces"])
	}
	row, ok := workspaces[0].(map[string]any)
	if !ok {
		t.Fatalf("workspace row missing: %+v", workspaces[0])
	}
	if got := row["coverage"]; got != string(workspaceOutputCoverageDocumented) {
		t.Fatalf("workspace coverage = %v, want %q", got, workspaceOutputCoverageDocumented)
	}
	detail, ok := resp.Result["detail"].(map[string]any)
	if !ok {
		t.Fatalf("result.detail missing: %+v", resp.Result)
	}
	coverage, ok := detail["coverage"].(map[string]any)
	if !ok {
		t.Fatalf("detail.coverage missing: %+v", detail)
	}
	if got := coverage["state"]; got != string(workspaceOutputCoverageDocumented) {
		t.Fatalf("detail.coverage.state = %v, want %q", got, workspaceOutputCoverageDocumented)
	}
	if got := coverage["notes_count"]; got != float64(1) {
		t.Fatalf("detail.coverage.notes_count = %v, want %v", got, float64(1))
	}
	if got := coverage["artifacts_count"]; got != float64(1) {
		t.Fatalf("detail.coverage.artifacts_count = %v, want %v", got, float64(1))
	}
}

func containsDashboardJSONWarning(warnings []any, want string) bool {
	for _, warning := range warnings {
		if got, ok := warning.(string); ok && got == want {
			return true
		}
	}
	return false
}
