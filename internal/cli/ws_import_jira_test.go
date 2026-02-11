package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Import_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws import") {
		t.Fatalf("stdout missing ws import usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws import jira") {
		t.Fatalf("stdout missing ws import jira usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_RequiresMode(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "one of --sprint or --jql is required") {
		t.Fatalf("stderr missing required mode error: %q", err.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
}

func TestCLI_WS_Import_Jira_RejectsSprintAndJQLCombination(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "Sprint 1", "--space", "DEMO", "--jql", "assignee=currentUser()"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--sprint and --jql cannot be combined") {
		t.Fatalf("stderr missing combination error: %q", err.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
}

func TestCLI_WS_Import_Jira_RequiresSpaceWithSprint(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "Sprint 1"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--sprint requires --space (or --project)") {
		t.Fatalf("stderr missing space-required error: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_RejectsSpaceAndProjectTogether(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "Sprint 1", "--space", "DEMO", "--project", "DEMO"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--space and --project cannot be combined") {
		t.Fatalf("stderr missing mutual exclusion error: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_RejectsBoardWithoutSprint(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()", "--board", "TEAM"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--board is only valid with --sprint") {
		t.Fatalf("stderr missing board validation error: %q", err.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
}

func TestClassifyWSImportJiraCreateFailureReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "permission denied", err: errors.New("open x: permission denied"), want: "permission_denied"},
		{name: "not found", err: errors.New("stat x: no such file or directory"), want: "not_found"},
		{name: "other", err: errors.New("create workspace dir: not a directory"), want: "create_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyWSImportJiraCreateFailureReason(tc.err); got != tc.want {
				t.Fatalf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderWSImportJiraApplyPrompt_UsesBulletedPlanAlignment(t *testing.T) {
	got := renderWSImportJiraApplyPrompt(false)
	want := "  • apply this plan? [Enter=yes / n=no]: "
	if got != want {
		t.Fatalf("renderWSImportJiraApplyPrompt() = %q, want %q", got, want)
	}
}

func TestRenderWSImportJiraPlanItemLabel_SkipReasonPolicy(t *testing.T) {
	got := renderWSImportJiraPlanItemLabel(wsImportJiraItem{
		IssueKey: "PROJ-101",
		Title:    "Already exists",
		Action:   "skip",
		Reason:   "already_active",
	})
	if strings.Contains(got, "(") || strings.Contains(got, ")") {
		t.Fatalf("already_active reason should be hidden, got %q", got)
	}
	if got != "PROJ-101: Already exists" {
		t.Fatalf("unexpected label for already_active: %q", got)
	}

	got = renderWSImportJiraPlanItemLabel(wsImportJiraItem{
		IssueKey: "PROJ-102",
		Title:    "Archived exists",
		Action:   "skip",
		Reason:   "archived_exists",
	})
	if got != "PROJ-102: Archived exists (archived_exists)" {
		t.Fatalf("non already_active reason should be shown, got %q", got)
	}
}

func TestCLI_WS_Import_Jira_NoPromptWithoutApply_PrintsPlanWithSkipAndFail(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	if err := os.MkdirAll(filepath.Join(env.Root, "workspaces", "PROJ-101"), 0o755); err != nil {
		t.Fatalf("seed active workspace: %v", err)
	}

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("request path = %q, want /rest/api/3/search/jql", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-101","fields":{"summary":"Already exists"}},{"key":"BAD/1","fields":{"summary":"Invalid key"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()", "--no-prompt"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	got := out.String()
	if !strings.Contains(got, "Plan:") {
		t.Fatalf("stdout missing plan heading: %q", got)
	}
	if !strings.Contains(got, "PROJ-101: Already exists") {
		t.Fatalf("stdout missing skipped item label: %q", got)
	}
	if strings.Contains(got, "(already_active)") {
		t.Fatalf("stdout should hide already_active reason: %q", got)
	}
	if !strings.Contains(got, "invalid_workspace_id") {
		t.Fatalf("stdout missing fail reason: %q", got)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("dev@example.com:token-123"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, wantAuth)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_NoPromptWithoutApply_UsesBulletedPlanLayout(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("request path = %q, want /rest/api/3/search/jql", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-201","fields":{"summary":"Implement import"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()", "--no-prompt"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	got := out.String()
	for _, want := range []string{
		"Plan:",
		"  • source: jira mode=jql",
		"  • filters: assignee=currentUser() statusCategory!=Done limit=30",
		"  • to create (1)",
		"    └─ PROJ-201: Implement import",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in plan output:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Summary:") {
		t.Fatalf("plan output should not include Summary section:\n%s", got)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_JSON_NoPromptWithoutApply_Contract(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)
	if err := os.MkdirAll(filepath.Join(env.Root, "workspaces", "PROJ-101"), 0o755); err != nil {
		t.Fatalf("seed active workspace: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(env.Root, "archive", "PROJ-102"), 0o755); err != nil {
		t.Fatalf("seed archived workspace: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("request path = %q, want /rest/api/3/search/jql", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-101","fields":{"summary":"Already exists"}},{"key":"PROJ-102","fields":{"summary":"Archived exists"}},{"key":"BAD/1","fields":{"summary":"Invalid key"}},{"key":"PROJ-103","fields":{"summary":"Create me"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()", "--no-prompt", "--json"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	if strings.Contains(out.String(), "Plan:") || strings.Contains(out.String(), "apply this plan?") {
		t.Fatalf("stdout should contain JSON only: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}

	var plan wsImportJiraPlan
	if decodeErr := json.Unmarshal(out.Bytes(), &plan); decodeErr != nil {
		t.Fatalf("stdout is not valid json: %v (stdout=%q)", decodeErr, out.String())
	}
	if plan.Summary.Candidates != 4 || plan.Summary.ToCreate != 1 || plan.Summary.Skipped != 2 || plan.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", plan.Summary)
	}

	items := map[string]wsImportJiraItem{}
	for _, it := range plan.Items {
		items[it.IssueKey] = it
	}
	if got := items["PROJ-101"]; got.Action != "skip" || got.Reason != "already_active" {
		t.Fatalf("PROJ-101 item = %+v", got)
	}
	if got := items["PROJ-102"]; got.Action != "skip" || got.Reason != "archived_exists" {
		t.Fatalf("PROJ-102 item = %+v", got)
	}
	if got := items["BAD/1"]; got.Action != "fail" || got.Reason != "invalid_workspace_id" {
		t.Fatalf("BAD/1 item = %+v", got)
	}
	if got := items["PROJ-103"]; got.Action != "create" || got.Reason != "" {
		t.Fatalf("PROJ-103 item = %+v", got)
	}
}

func TestCLI_WS_Import_Jira_PromptDecline_WithFailedPlan_ReturnsError(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("request path = %q, want /rest/api/3/search/jql", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"BAD/1","fields":{"summary":"Invalid key"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var in bytes.Buffer
	in.WriteString("n\n")
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = &in

	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stdout=%q, stderr=%q)", code, exitError, out.String(), err.String())
	}
	if !strings.Contains(out.String(), "invalid_workspace_id") {
		t.Fatalf("stdout missing fail reason: %q", out.String())
	}
	if !strings.Contains(out.String(), "apply this plan? [Enter=yes / n=no]:") {
		t.Fatalf("stdout missing apply prompt line: %q", out.String())
	}
	if !strings.HasSuffix(out.String(), ": \n") {
		t.Fatalf("stdout should end with apply prompt and newline: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_PromptAccept_PrintsResultSummary(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("request path = %q, want /rest/api/3/search/jql", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-310","fields":{"summary":"Prompt apply"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var in bytes.Buffer
	in.WriteString("\n")
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = &in

	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q, stderr=%q)", code, exitOK, out.String(), err.String())
	}
	if !strings.Contains(out.String(), "Result:") {
		t.Fatalf("stdout missing result heading: %q", out.String())
	}
	if !strings.Contains(out.String(), "create=1 skipped=0 failed=0") {
		t.Fatalf("stdout missing result summary: %q", out.String())
	}
	if !strings.Contains(out.String(), "import completed") {
		t.Fatalf("stdout missing success message: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_JSON_Prompt_PrintsPromptToStderr(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("request path = %q, want /rest/api/3/search/jql", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-301","fields":{"summary":"JSON prompt test"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var in bytes.Buffer
	in.WriteString("n\n")
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = &in

	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()", "--json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q, stderr=%q)", code, exitOK, out.String(), err.String())
	}
	if strings.Contains(out.String(), "apply this plan? [Enter=yes / n=no]") {
		t.Fatalf("stdout should not include prompt: %q", out.String())
	}
	if !strings.Contains(err.String(), "apply this plan? [Enter=yes / n=no]") {
		t.Fatalf("stderr missing prompt: %q", err.String())
	}

	var plan wsImportJiraPlan
	if decodeErr := json.Unmarshal(out.Bytes(), &plan); decodeErr != nil {
		t.Fatalf("stdout is not valid json: %v (stdout=%q)", decodeErr, out.String())
	}
	if plan.Summary.Failed != 0 || plan.Summary.ToCreate != 1 {
		t.Fatalf("unexpected summary: %+v", plan.Summary)
	}
}

func TestCLI_WS_Import_Jira_NoPromptApply_CreatesWorkspace(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-201","fields":{"summary":"Implement import"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()", "--no-prompt", "--apply"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	wsPath := filepath.Join(env.Root, "workspaces", "PROJ-201")
	if _, statErr := os.Stat(wsPath); statErr != nil {
		t.Fatalf("workspace was not created: %v", statErr)
	}
	metaBytes, readErr := os.ReadFile(filepath.Join(wsPath, workspaceMetaFilename))
	if readErr != nil {
		t.Fatalf("read meta: %v", readErr)
	}
	if !strings.Contains(string(metaBytes), `"source_url": "`+server.URL+`/browse/PROJ-201"`) {
		t.Fatalf("meta missing source_url: %q", string(metaBytes))
	}
	if !strings.Contains(string(metaBytes), `"title": "Implement import"`) {
		t.Fatalf("meta missing title: %q", string(metaBytes))
	}
	if !strings.Contains(out.String(), "Result:") {
		t.Fatalf("stdout missing result heading: %q", out.String())
	}
	if !strings.Contains(out.String(), "create=1 skipped=0 failed=0") {
		t.Fatalf("stdout missing result summary: %q", out.String())
	}
	if !strings.Contains(out.String(), "import completed") {
		t.Fatalf("stdout missing success message: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_JSON_NoPromptApply_CreateFailureReason(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	workspacesDir := filepath.Join(env.Root, "workspaces")
	if err := os.Chmod(workspacesDir, 0o555); err != nil {
		t.Fatalf("chmod workspaces dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(workspacesDir, 0o755) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("request path = %q, want /rest/api/3/search/jql", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-201","fields":{"summary":"Create fails"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "import", "jira", "--jql", "assignee=currentUser()", "--no-prompt", "--apply", "--json"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stdout=%q, stderr=%q)", code, exitError, out.String(), err.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}

	var plan wsImportJiraPlan
	if decodeErr := json.Unmarshal(out.Bytes(), &plan); decodeErr != nil {
		t.Fatalf("stdout is not valid json: %v (stdout=%q)", decodeErr, out.String())
	}
	if plan.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", plan.Summary)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(plan.Items))
	}
	foundFail := false
	for _, it := range plan.Items {
		if it.IssueKey == "PROJ-201" && it.Action == "fail" {
			foundFail = true
			if it.Reason != "permission_denied" {
				t.Fatalf("unexpected fail reason: %+v", it)
			}
		}
	}
	if !foundFail {
		t.Fatalf("fail item for PROJ-201 not found: %+v", plan.Items)
	}
}

func TestCLI_WS_Import_Jira_SprintNoValue_PromptSelectsFromSpaceSprintList(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		got := r.URL.Query().Get("jql")
		if strings.Contains(got, "openSprints()") || strings.Contains(got, "futureSprints()") {
			_, _ = w.Write([]byte(`{"issues":[{"fields":{"sprint":{"id":16,"name":"DEMO スプリント 16","state":"active","originBoardId":98}}}]}`))
			return
		}
		{
			got := r.URL.Query().Get("jql")
			if !strings.Contains(got, `project = DEMO`) || !strings.Contains(got, `sprint = 16`) {
				t.Fatalf("jql = %q, want project+sprint(id) filters", got)
			}
			_, _ = w.Write([]byte(`{"issues":[{"key":"PROJ-300","fields":{"summary":"Sprint import"}}]}`))
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var in bytes.Buffer
	in.WriteString("1\nn\n")
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = &in

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "--space", "DEMO"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Select sprint:") {
		t.Fatalf("stdout missing sprint selection prompt: %q", out.String())
	}
	if !strings.Contains(out.String(), "sprint=DEMO スプリント 16") {
		t.Fatalf("stdout missing selected sprint in plan source: %q", out.String())
	}
	if strings.Contains(err.String(), "resolve sprint:") || strings.Contains(err.String(), "resolve jira issues:") {
		t.Fatalf("stderr contains unexpected error: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_SprintNoValue_ShowsOnlyActiveFuture(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		got := r.URL.Query().Get("jql")
		if strings.Contains(got, "openSprints()") || strings.Contains(got, "futureSprints()") {
			_, _ = w.Write([]byte(`{"issues":[{"fields":{"customfield_10020":[{"id":16,"name":"DEMO スプリント 16","state":"active","originBoardId":98},{"id":15,"name":"DEMO スプリント 15","state":"closed","originBoardId":98}],"status":{"id":"1","name":"To Do"}}}]}`))
			return
		}
		if !strings.Contains(got, `project = DEMO`) || !strings.Contains(got, `sprint = 16`) {
			t.Fatalf("jql = %q, want project+sprint(id) filters", got)
		}
		_, _ = w.Write([]byte(`{"issues":[]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var in bytes.Buffer
	in.WriteString("1\nn\n")
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = &in

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "--space", "DEMO"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	stdout := out.String()
	if !strings.Contains(stdout, "Select sprint:") {
		t.Fatalf("stdout missing sprint selection prompt: %q", stdout)
	}
	if !strings.Contains(stdout, "DEMO スプリント 16") {
		t.Fatalf("stdout missing active sprint: %q", stdout)
	}
	if strings.Contains(stdout, "DEMO スプリント 15") {
		t.Fatalf("stdout should not include closed sprint: %q", stdout)
	}
	if strings.Contains(stdout, "To Do") {
		t.Fatalf("stdout should not include non-sprint options: %q", stdout)
	}
}

func TestCLI_WS_Import_Jira_SprintName_NoPrompt_Works(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		got := r.URL.Query().Get("jql")
		if !strings.Contains(got, `project = DEMO`) || !strings.Contains(got, `sprint = "Sprint X"`) {
			t.Fatalf("jql = %q, want project+sprint filters", got)
		}
		_, _ = w.Write([]byte(`{"issues":[]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "Sprint X", "--space", "DEMO", "--no-prompt"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "sprint=Sprint X") {
		t.Fatalf("stdout missing sprint source: %q", out.String())
	}
}

func TestCLI_WS_Import_Jira_SprintNumericID_UsesJQLDirect(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		got := r.URL.Query().Get("jql")
		if !strings.Contains(got, "project = DEMO") || !strings.Contains(got, "sprint = 55") {
			t.Fatalf("jql = %q, want project+sprint id filters", got)
		}
		_, _ = w.Write([]byte(`{"issues":[]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "55", "--space", "DEMO", "--no-prompt"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "sprint=55") {
		t.Fatalf("stdout missing sprint source value: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Import_Jira_ProjectAlias_WorksLikeSpace(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		got := r.URL.Query().Get("jql")
		if !strings.Contains(got, "project = DEMO") || !strings.Contains(got, "sprint = 55") {
			t.Fatalf("jql = %q, want project+sprint id filters", got)
		}
		_, _ = w.Write([]byte(`{"issues":[]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("GIONX_JIRA_BASE_URL", server.URL)
	t.Setenv("GIONX_JIRA_EMAIL", "dev@example.com")
	t.Setenv("GIONX_JIRA_API_TOKEN", "token-123")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "import", "jira", "--sprint", "55", "--project", "DEMO", "--no-prompt"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "sprint=55") {
		t.Fatalf("stdout missing resolved sprint: %q", out.String())
	}
}
