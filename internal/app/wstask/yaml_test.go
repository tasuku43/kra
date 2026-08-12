package wstask

import (
	"strings"
	"testing"
)

func TestParseAndRenderYAMLContent_PreservesDependsOn(t *testing.T) {
	content := []byte("schema_version: 1\n" +
		"tasks:\n" +
		"  - id: TASK-001\n" +
		"    title: Foundation\n" +
		"    status: done\n" +
		"    description: Base work\n" +
		"    depends_on: []\n" +
		"  - id: TASK-002\n" +
		"    title: Integration\n" +
		"    status: todo\n" +
		"    depends_on: [TASK-001]\n")

	items, diags := ParseYAMLContent(content)
	if diags.HasErrors() {
		t.Fatalf("ParseYAMLContent() errors = %v", diags.Errors)
	}
	if got := items[1].DependsOn; len(got) != 1 || got[0] != "TASK-001" {
		t.Fatalf("parsed depends_on = %v, want [TASK-001]", got)
	}

	rendered, err := RenderWorkspaceYAML(items)
	if err != nil {
		t.Fatalf("RenderWorkspaceYAML() error = %v", err)
	}
	ws, err := ParseYAMLWorkspace(rendered)
	if err != nil {
		t.Fatalf("ParseYAMLWorkspace(rendered) error = %v", err)
	}
	if got := ws.Tasks[1].DependsOn; len(got) != 1 || got[0] != "TASK-001" {
		t.Fatalf("rendered depends_on = %v, want [TASK-001]", got)
	}
}

func TestParseYAMLContent_RequiresExplicitTaskLists(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "missing tasks", content: "schema_version: 1\n", want: "tasks list"},
		{name: "null tasks", content: "schema_version: 1\ntasks: null\n", want: "tasks list"},
		{name: "missing depends_on", content: "schema_version: 1\ntasks:\n  - id: TASK-001\n    title: One\n    status: todo\n", want: "depends_on list"},
		{name: "null depends_on", content: "schema_version: 1\ntasks:\n  - id: TASK-001\n    title: One\n    status: todo\n    depends_on: null\n", want: "depends_on list"},
		{name: "empty dependency id", content: "schema_version: 1\ntasks:\n  - id: TASK-001\n    title: One\n    status: todo\n    depends_on: [\"\"]\n", want: "empty task ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := ParseYAMLContent([]byte(tt.content))
			if !diags.HasErrors() || !containsDiagnostic(diags.Errors, tt.want) {
				t.Fatalf("diagnostics = %+v, want error containing %q", diags, tt.want)
			}
		})
	}
}

func TestParseYAMLContent_AcceptsExplicitEmptyLists(t *testing.T) {
	items, diags := ParseYAMLContent([]byte("schema_version: 1\ntasks: []\n"))
	if diags.HasErrors() || len(items) != 0 {
		t.Fatalf("items=%+v diagnostics=%+v, want valid empty list", items, diags)
	}

	items, diags = ParseYAMLContent([]byte("schema_version: 1\ntasks:\n  - id: TASK-001\n    title: One\n    status: todo\n    depends_on: []\n"))
	if diags.HasErrors() || len(items) != 1 || items[0].DependsOn == nil {
		t.Fatalf("items=%+v diagnostics=%+v, want explicit empty depends_on", items, diags)
	}
}

func containsDiagnostic(diagnostics []string, substring string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, substring) {
			return true
		}
	}
	return false
}
