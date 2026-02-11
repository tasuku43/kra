package cli

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gion-core/workspacerisk"
	"github.com/tasuku43/gionx/internal/infra/statestore"
)

func TestGolden_WSActionSelectorSingle(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"Action:",
		"run",
		[]workspaceSelectorCandidate{
			{ID: "add-repo", Description: "add repositories", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "close", Description: "archive this workspace", Risk: workspacerisk.WorkspaceRiskClean},
		},
		map[int]bool{},
		0,
		"",
		selectorMessageLevelMuted,
		"",
		true,
		true,
		true,
		false,
		false,
		120,
	)
	assertGolden(t, "ws_action_selector_single.golden", strings.Join(lines, "\n")+"\n")
}

func TestGolden_RepoPoolSelectorMulti(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"Repos(pool):",
		"add",
		[]workspaceSelectorCandidate{
			{ID: "example-org/helmfiles", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "example-org/sre-apps", Risk: workspacerisk.WorkspaceRiskClean},
		},
		map[int]bool{1: true},
		0,
		"",
		selectorMessageLevelMuted,
		"",
		false,
		true,
		false,
		false,
		false,
		120,
	)
	assertGolden(t, "repo_pool_selector_multi.golden", strings.Join(lines, "\n")+"\n")
}

func TestGolden_WSAddRepoPlan(t *testing.T) {
	var out bytes.Buffer
	plan := []addRepoPlanItem{
		{Candidate: addRepoPoolCandidate{RepoKey: "example-org/terraforms"}},
		{Candidate: addRepoPoolCandidate{RepoKey: "tasuku43/gionx"}},
	}
	printAddRepoPlan(&out, "DEMO-0000", plan, false)
	assertGolden(t, "ws_add_repo_plan.golden", out.String())
}

func TestGolden_WSImportJiraPlanInlineConfirm(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	plan := wsImportJiraPlan{
		Source: wsImportJiraSource{
			Type:   "jira",
			Mode:   "sprint",
			Sprint: "DEMO スプリント 16",
		},
		Filters: wsImportJiraFilters{
			Assignee:       "currentUser()",
			StatusCategory: "not_done",
			Limit:          30,
		},
		Summary: wsImportJiraSummary{
			Candidates: 3,
			ToCreate:   2,
			Skipped:    1,
			Failed:     0,
		},
		Items: []wsImportJiraItem{
			{
				IssueKey: "DEMO-0000",
				Title:    "Prepare baseline project setup",
				Action:   "create",
			},
			{
				IssueKey: "DEMO-0000",
				Title:    "Audit current manifest structure",
				Action:   "create",
			},
			{
				IssueKey: "DEMO-0000",
				Title:    "Evaluate cloud execution feasibility",
				Action:   "skip",
				Reason:   "already_active",
			},
		},
	}

	c.printWSImportJiraPlanHuman(plan)
	assertGolden(t, "ws_import_jira_plan_inline_confirm.golden", out.String())
}

func TestGolden_WSImportJiraResult(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	plan := wsImportJiraPlan{
		Summary: wsImportJiraSummary{
			ToCreate: 2,
			Skipped:  1,
			Failed:   0,
		},
	}

	c.printWSImportJiraResultHuman(plan)
	assertGolden(t, "ws_import_jira_result.golden", out.String())
}

func TestGolden_WSListHuman_Empty(t *testing.T) {
	var out bytes.Buffer
	printWSListHuman(&out, nil, "active", false, false)
	assertGolden(t, "ws_list_human_empty.golden", out.String())
}

func TestGolden_WSListHuman_Tree(t *testing.T) {
	var out bytes.Buffer
	rows := []wsListRow{
		{
			ID:    "DEMO-0000",
			Title: "Audit current manifest structure",
			Repos: []statestore.WorkspaceRepo{
				{
					Alias:  "gionx",
					Branch: "main",
					MissingAt: sql.NullInt64{
						Valid: false,
					},
				},
				{
					Alias:  "infra",
					Branch: "develop",
					MissingAt: sql.NullInt64{
						Int64: 1700000000,
						Valid: true,
					},
				},
			},
		},
		{
			ID:    "TEST-002",
			Title: "",
			Repos: nil,
		},
	}
	printWSListHuman(&out, rows, "active", true, false)
	assertGolden(t, "ws_list_human_tree.golden", out.String())
}

func TestGolden_WSFlowResult(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	c.printWorkspaceFlowResult("Purged", "✔", []string{"TEST-001", "TEST-002"}, 2, false)
	assertGolden(t, "ws_flow_result.golden", out.String())
}

func TestGolden_WSFlowAbortedResult(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	c.printWorkspaceFlowAbortedResult("canceled at Risk", false)
	assertGolden(t, "ws_flow_aborted_result.golden", out.String())
}

func TestGolden_WSCloseRiskSection(t *testing.T) {
	var out bytes.Buffer
	items := []workspaceRiskDetail{
		{
			id:   "WS1",
			risk: workspacerisk.WorkspaceRiskDirty,
			perRepo: []repoRiskItem{
				{alias: "repo-a", state: workspacerisk.RepoStateDirty},
				{alias: "repo-b", state: workspacerisk.RepoStateUnpushed},
			},
		},
		{
			id:      "WS2",
			risk:    workspacerisk.WorkspaceRiskClean,
			perRepo: []repoRiskItem{{alias: "repo-c", state: workspacerisk.RepoStateClean}},
		},
	}

	printRiskSection(&out, items, false)
	assertGolden(t, "ws_close_risk_section.golden", out.String())
}

func TestGolden_WSPurgeRiskSection(t *testing.T) {
	var out bytes.Buffer
	selectedIDs := []string{"WS1"}
	riskMeta := map[string]purgeWorkspaceMeta{
		"WS1": {
			status: "active",
			risk:   workspacerisk.WorkspaceRiskDirty,
			perRepo: []repoRiskItem{
				{alias: "repo1", state: workspacerisk.RepoStateDirty},
			},
		},
	}

	printPurgeRiskSection(&out, selectedIDs, riskMeta, false)
	assertGolden(t, "ws_purge_risk_section.golden", out.String())
}

func assertGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %s: %v", path, err)
	}
	if got != string(wantBytes) {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, string(wantBytes), got)
	}
}
