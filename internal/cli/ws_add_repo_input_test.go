package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/testutil"
)

func TestParseMultiSelectIndices_DeduplicatesAndPreservesOrder(t *testing.T) {
	got, err := parseMultiSelectIndices("2,1,2 3", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 0, 2}
	if len(got) != len(want) {
		t.Fatalf("len(indices)=%d, want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("indices[%d]=%d, want=%d", i, got[i], want[i])
		}
	}
}

func TestParseMultiSelectIndices_InvalidInputs(t *testing.T) {
	cases := []string{"", "a", "0", "4"}
	for _, in := range cases {
		if _, err := parseMultiSelectIndices(in, 3); err == nil {
			t.Fatalf("expected error for input %q", in)
		}
	}
}

func TestFilterAddRepoPoolCandidates_CaseInsensitive(t *testing.T) {
	cands := []addRepoPoolCandidate{
		{RepoKey: "tasuku43/KRA"},
		{RepoKey: "tasuku43/gion-core"},
		{RepoKey: "tasuku43/other"},
	}
	got := filterAddRepoPoolCandidates(cands, "ra")
	if len(got) != 1 {
		t.Fatalf("len(filtered)=%d, want=1", len(got))
	}
	if got[0].RepoKey != "tasuku43/KRA" {
		t.Fatalf("unexpected filtered order/content: %+v", got)
	}
}

func TestFilterAddRepoPoolCandidates_FuzzyMatch(t *testing.T) {
	cands := []addRepoPoolCandidate{
		{RepoKey: "example-org/helmfiles"},
		{RepoKey: "example-org/sre-apps"},
	}
	got := filterAddRepoPoolCandidates(cands, "es")
	if len(got) != 2 {
		t.Fatalf("len(filtered)=%d, want=2", len(got))
	}
	if got[0].RepoKey != "example-org/helmfiles" || got[1].RepoKey != "example-org/sre-apps" {
		t.Fatalf("unexpected filtered order/content: %+v", got)
	}
}

func TestPromptAddRepoEditableInput_NonTTY_EmptyUsesInitial(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("\n")

	got, edited, promptErr := c.promptAddRepoEditableInput("  ", "branch", "TEST-010", false)
	if promptErr != nil {
		t.Fatalf("unexpected error: %v", promptErr)
	}
	if got != "TEST-010" {
		t.Fatalf("value=%q, want=%q", got, "TEST-010")
	}
	if edited {
		t.Fatalf("edited should be false when empty input uses initial")
	}
}

func TestPromptAddRepoEditableInput_NonTTY_InputOverridesInitial(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("feature/x\n")

	got, edited, promptErr := c.promptAddRepoEditableInput("  ", "branch", "TEST-010", false)
	if promptErr != nil {
		t.Fatalf("unexpected error: %v", promptErr)
	}
	if got != "feature/x" {
		t.Fatalf("value=%q, want=%q", got, "feature/x")
	}
	if !edited {
		t.Fatalf("edited should be true when input overrides initial")
	}
}

func TestPrintAddRepoPlan_ShowsConciseTree(t *testing.T) {
	var out bytes.Buffer
	plan := []addRepoPlanItem{
		{
			Candidate:     addRepoPoolCandidate{RepoKey: "tasuku43/kra"},
			FetchDecision: "required (stale, age=6m1s)",
		},
		{
			Candidate:     addRepoPoolCandidate{RepoKey: "tasuku43/gion-core"},
			FetchDecision: "skipped (fresh, age=2m0s <= 5m)",
		},
	}

	printAddRepoPlan(&out, "TEST-010", plan, false)
	got := out.String()

	for _, want := range []string{
		"Plan:",
		"add 2 repos (worktrees) to workspace TEST-010",
		"repos:",
		"├─ tasuku43/kra",
		"fetch: required (stale, age=6m1s)",
		"└─ tasuku43/gion-core",
		"fetch: skipped (fresh, age=2m0s <= 5m)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in plan output:\n%s", want, got)
		}
	}
}

func TestPrintAddRepoPlan_HidesFetchLineWhenDecisionIsEmpty(t *testing.T) {
	var out bytes.Buffer
	plan := []addRepoPlanItem{
		{
			Candidate:     addRepoPoolCandidate{RepoKey: "example-org/biryani-tools"},
			FetchDecision: "",
		},
	}
	printAddRepoPlan(&out, "DEMO-0000", plan, false)
	got := out.String()
	if strings.Contains(got, "fetch:") {
		t.Fatalf("plan should not include fetch line when decision is empty:\n%s", got)
	}
}

func TestMissingLocalFileRemotePath(t *testing.T) {
	missing := "file:///tmp/this-path-should-not-exist-kra-test"
	if p, ok := missingLocalFileRemotePath(missing); !ok || p == "" {
		t.Fatalf("missingLocalFileRemotePath(%q) = (%q, %v), want missing path", missing, p, ok)
	}
	if _, ok := missingLocalFileRemotePath("git@github.com:example-org/biryani-tools.git"); ok {
		t.Fatal("ssh remote should not be treated as missing local file remote")
	}
}

func TestEvaluateAddRepoFetchDecision_DoesNotRequireWhenOriginBranchMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	repoSpec := prepareRemoteRepoSpec(t, func(dir string, args ...string) {
		runGit(t, dir, args...)
	})
	_, repoKey, _ := seedRepoPoolAndState(t, env, repoSpec)

	spec, err := repospec.Normalize(repoSpec)
	if err != nil {
		t.Fatalf("Normalize(repoSpec): %v", err)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)

	decision, err := evaluateAddRepoFetchDecision(context.Background(), addRepoPlanItem{
		Candidate: addRepoPoolCandidate{
			RepoKey:  repoKey,
			BarePath: barePath,
		},
		BaseRefUsed: "origin/main",
		Branch:      "DEMO-0000",
	}, addRepoFetchOptions{})
	if err != nil {
		t.Fatalf("evaluateAddRepoFetchDecision() error: %v", err)
	}
	if decision.ShouldFetch {
		t.Fatalf("decision.ShouldFetch = true, want false (reason=%q)", decision.Reason)
	}
	if !strings.Contains(decision.Reason, "skipped (fresh") {
		t.Fatalf("decision.Reason = %q, want fresh-skip reason", decision.Reason)
	}
}

func TestRenderAddRepoApplyPrompt_UsesBulletedPlanAlignment(t *testing.T) {
	got := renderAddRepoApplyPrompt(false)
	want := "  • apply this plan? [Enter=yes / n=no]: "
	if got != want {
		t.Fatalf("renderAddRepoApplyPrompt() = %q, want %q", got, want)
	}
}

func TestPrintAddRepoResult_NoLeadingBlankLine(t *testing.T) {
	var out bytes.Buffer
	applied := []addRepoAppliedItem{
		{Plan: addRepoPlanItem{Candidate: addRepoPoolCandidate{RepoKey: "tasuku43/kra"}}},
		{Plan: addRepoPlanItem{Candidate: addRepoPoolCandidate{RepoKey: "tasuku43/gion-core"}}},
	}

	printAddRepoResult(&out, applied, false)
	got := out.String()

	if strings.HasPrefix(got, "\n") {
		t.Fatalf("result should not start with blank line:\n%q", got)
	}
	for _, want := range []string{
		"Result:",
		"  • Added 2 / 2",
		"  • ✔ tasuku43/kra",
		"  • ✔ tasuku43/gion-core",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in result output:\n%s", want, got)
		}
	}
}
