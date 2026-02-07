package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildAddRepoInputsLines_BaseOnlySingleRepo(t *testing.T) {
	rows := []addRepoInputProgress{
		{
			RepoKey: "tasuku43/puml-parser-php",
			BaseRef: "origin/main",
		},
	}

	lines := buildAddRepoInputsLines("TEST-010", rows, 0, false)
	got := strings.Join(lines, "\n")
	want := strings.Join([]string{
		"  • repos:",
		"    └─ tasuku43/puml-parser-php",
		"       ├─ base_ref: origin/main",
		"       └─ branch: TEST-010",
	}, "\n")

	if !strings.Contains(got, want) {
		t.Fatalf("unexpected inputs block:\n%s", got)
	}
}

func TestBuildAddRepoInputsLines_BaseOnlyNonActiveRepo(t *testing.T) {
	rows := []addRepoInputProgress{
		{
			RepoKey: "tasuku43/puml-parser-php",
			BaseRef: "origin/main",
		},
	}

	lines := buildAddRepoInputsLines("TEST-010", rows, -1, false)
	got := strings.Join(lines, "\n")
	want := strings.Join([]string{
		"  • repos:",
		"    └─ tasuku43/puml-parser-php",
		"       └─ base_ref: origin/main",
	}, "\n")

	if !strings.Contains(got, want) {
		t.Fatalf("unexpected inputs block:\n%s", got)
	}
}

func TestBuildAddRepoInputsLines_BaseAndBranchSingleRepo(t *testing.T) {
	rows := []addRepoInputProgress{
		{
			RepoKey: "tasuku43/puml-parser-php",
			BaseRef: "origin/main",
			Branch:  "dddd",
		},
	}

	lines := buildAddRepoInputsLines("TEST-010", rows, 0, false)
	got := strings.Join(lines, "\n")
	want := strings.Join([]string{
		"  • repos:",
		"    └─ tasuku43/puml-parser-php",
		"       ├─ base_ref: origin/main",
		"       └─ branch: dddd",
	}, "\n")

	if !strings.Contains(got, want) {
		t.Fatalf("unexpected inputs block:\n%s", got)
	}
}

func TestBuildAddRepoInputsLines_FirstRepoFinalizedThenSecondBaseOnly(t *testing.T) {
	rows := []addRepoInputProgress{
		{
			RepoKey: "tasuku43/puml-parser-php",
			BaseRef: "origin/main",
			Branch:  "dddd",
		},
		{
			RepoKey: "tasuku43/dependency-analyzer",
			BaseRef: "origin/main",
		},
	}

	lines := buildAddRepoInputsLines("TEST-010", rows, 1, false)
	got := strings.Join(lines, "\n")
	want := strings.Join([]string{
		"  • repos:",
		"    ├─ tasuku43/puml-parser-php",
		"    │  ├─ base_ref: origin/main",
		"    │  └─ branch: dddd",
		"    └─ tasuku43/dependency-analyzer",
		"       ├─ base_ref: origin/main",
		"       └─ branch: TEST-010",
	}, "\n")

	if !strings.Contains(got, want) {
		t.Fatalf("unexpected inputs block:\n%s", got)
	}
}

func TestResolveBranchInput_UsesRawInputWithoutPrefixEnforcement(t *testing.T) {
	got := resolveBranchInput("feature/x", "TEST-010")
	if got != "feature/x" {
		t.Fatalf("expected raw branch input, got=%q", got)
	}
}

func TestResolveBranchInput_KeepsEmptyWhenEmpty(t *testing.T) {
	got := resolveBranchInput("", "TEST-010")
	if got != "" {
		t.Fatalf("expected empty branch for empty input, got=%q", got)
	}
}

func TestResolveBaseRefInput_RejectsNonOriginPrefix(t *testing.T) {
	if _, err := resolveBaseRefInput("main", "origin/main"); err == nil {
		t.Fatal("expected error for non origin/ base_ref")
	}
}

func TestResolveBaseRefInput_UsesDefaultWhenEmpty(t *testing.T) {
	got, err := resolveBaseRefInput("", "origin/main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "origin/main" {
		t.Fatalf("expected default base_ref, got=%q", got)
	}
}

func TestRenderAddRepoInputPrompt(t *testing.T) {
	got := renderAddRepoInputPrompt("prefix ", "base_ref", "origin/main", false)
	if got != "prefix base_ref: origin/main" {
		t.Fatalf("unexpected prompt: %q", got)
	}
}

func TestBuildAddRepoInputsLines_StateTransitionSnapshots(t *testing.T) {
	rows := []addRepoInputProgress{
		{
			RepoKey: "tasuku43/puml-parser-php",
			BaseRef: "origin/main",
		},
		{
			RepoKey: "tasuku43/dependency-analyzer",
			BaseRef: "origin/main",
		},
	}

	// 1) first repo base_ref fixed, branch pending should already be visible.
	step1 := strings.Join(buildAddRepoInputsLines("TEST-010", rows, 0, false), "\n")
	wantStep1 := strings.Join([]string{
		"  • repos:",
		"    └─ tasuku43/puml-parser-php",
		"       ├─ base_ref: origin/main",
		"       └─ branch: TEST-010",
	}, "\n")
	if !strings.Contains(step1, wantStep1) {
		t.Fatalf("step1 unexpected:\n%s", step1)
	}

	// 2) first repo branch fixed.
	rows[0].Branch = "dddd"
	step2 := strings.Join(buildAddRepoInputsLines("TEST-010", rows, 0, false), "\n")
	wantStep2 := strings.Join([]string{
		"  • repos:",
		"    └─ tasuku43/puml-parser-php",
		"       ├─ base_ref: origin/main",
		"       └─ branch: dddd",
	}, "\n")
	if !strings.Contains(step2, wantStep2) {
		t.Fatalf("step2 unexpected:\n%s", step2)
	}

	// 3) second repo base_ref fixed, first stays finalized and second shows pending branch.
	step3 := strings.Join(buildAddRepoInputsLines("TEST-010", rows, 1, false), "\n")
	wantStep3 := strings.Join([]string{
		"  • repos:",
		"    ├─ tasuku43/puml-parser-php",
		"    │  ├─ base_ref: origin/main",
		"    │  └─ branch: dddd",
		"    └─ tasuku43/dependency-analyzer",
		"       ├─ base_ref: origin/main",
		"       └─ branch: TEST-010",
	}, "\n")
	if !strings.Contains(step3, wantStep3) {
		t.Fatalf("step3 unexpected:\n%s", step3)
	}
}

func TestRenderAddRepoInputsProgress_NonTTY_NoEscapeAndStableLineCount(t *testing.T) {
	rows := []addRepoInputProgress{
		{RepoKey: "tasuku43/gionx", BaseRef: "origin/main"},
	}
	var out bytes.Buffer
	rendered := renderAddRepoInputsProgress(&out, "TEST-010", rows, 0, false, 99, true)
	if rendered == 0 {
		t.Fatal("expected rendered line count > 0")
	}
	got := out.String()
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("non-TTY render should not emit escape sequences, got %q", got)
	}
	if !strings.Contains(got, "Inputs:") || !strings.Contains(got, "branch: TEST-010") {
		t.Fatalf("unexpected render output:\n%s", got)
	}
}
