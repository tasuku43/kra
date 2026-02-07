package cli

import (
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
