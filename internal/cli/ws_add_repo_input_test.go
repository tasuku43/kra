package cli

import (
	"bytes"
	"strings"
	"testing"
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
		{RepoKey: "tasuku43/GIONX"},
		{RepoKey: "tasuku43/gion-core"},
		{RepoKey: "tasuku43/other"},
	}
	got := filterAddRepoPoolCandidates(cands, "gion")
	if len(got) != 2 {
		t.Fatalf("len(filtered)=%d, want=2", len(got))
	}
	if got[0].RepoKey != "tasuku43/GIONX" || got[1].RepoKey != "tasuku43/gion-core" {
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
		{Candidate: addRepoPoolCandidate{RepoKey: "tasuku43/gionx"}},
		{Candidate: addRepoPoolCandidate{RepoKey: "tasuku43/gion-core"}},
	}

	printAddRepoPlan(&out, "TEST-010", plan, false)
	got := out.String()

	for _, want := range []string{
		"Plan:",
		"add 2 repos to workspace TEST-010",
		"repos:",
		"├─ tasuku43/gionx",
		"└─ tasuku43/gion-core",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in plan output:\n%s", want, got)
		}
	}
}
