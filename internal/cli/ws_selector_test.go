package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tasuku43/gion-core/workspacerisk"
)

func TestCloseSelectorModel_SpaceTogglesSelection(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.selected[0] {
		t.Fatalf("space key should toggle selection on")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.selected[0] {
		t.Fatalf("second space key should toggle selection off")
	}
}

func TestCloseSelectorModel_EnterRequiresSelection(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.done {
		t.Fatalf("enter without selection should not complete")
	}
	if next.message == "" {
		t.Fatalf("enter without selection should set guidance message")
	}
}

func TestCloseSelectorModel_FullWidthSpaceTogglesSelection(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("　")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.selected[0] {
		t.Fatalf("full-width space key should toggle selection on")
	}
}

func TestCloseSelectorModel_FilterPersistsAfterToggle(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{
		{ID: "WS1", Description: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS2", Description: "beta", Risk: workspacerisk.WorkspaceRiskClean},
	}, "active", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.filterMode {
		t.Fatalf("filter mode should be enabled")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "a" {
		t.Fatalf("filter = %q, want %q", next.filter, "a")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filterMode {
		t.Fatalf("filter mode should be disabled after enter")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "a" {
		t.Fatalf("filter should persist after toggle, got %q", next.filter)
	}
	if !next.selected[0] {
		t.Fatalf("toggle should select visible candidate")
	}
}

func TestCloseSelectorModel_FilterClearsOnlyByDelete(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "x" {
		t.Fatalf("filter = %q, want %q", next.filter, "x")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "x" {
		t.Fatalf("filter should remain until explicit delete, got %q", next.filter)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "" {
		t.Fatalf("filter should be cleared by backspace, got %q", next.filter)
	}
}
