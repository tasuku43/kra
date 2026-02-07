package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tasuku43/gion-core/workspacerisk"
)

func TestWorkspaceSelectorModel_SpaceTogglesSelection(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", "proceed", false, nil)

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

func TestWorkspaceSelectorModel_BlinkTogglesCaretVisibility(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", "proceed", false, nil)
	if !m.showCaret {
		t.Fatalf("initial caret visibility should be true")
	}

	updated, cmd := m.Update(selectorCaretBlinkMsg{})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.showCaret {
		t.Fatalf("caret visibility should toggle to false")
	}
	if cmd == nil {
		t.Fatalf("blink should schedule next tick")
	}
}

func TestWorkspaceSelectorModel_EnterRequiresSelection(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", "proceed", false, nil)

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

func TestWorkspaceSelectorModel_FullWidthSpaceTogglesSelection(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", "proceed", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("　")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.selected[0] {
		t.Fatalf("full-width space key should toggle selection on")
	}
}

func TestWorkspaceSelectorModel_FilterPersistsAfterToggle(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{
		{ID: "WS1", Description: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS2", Description: "beta", Risk: workspacerisk.WorkspaceRiskClean},
	}, "active", "proceed", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "b" {
		t.Fatalf("filter = %q, want %q", next.filter, "b")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "b" {
		t.Fatalf("filter should persist after toggle, got %q", next.filter)
	}
	if !next.selected[1] {
		t.Fatalf("toggle should select current visible candidate")
	}
}

func TestWorkspaceSelectorModel_FilterClearsByDeleteOneRuneAtATime(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", "proceed", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "ab" {
		t.Fatalf("filter = %q, want %q", next.filter, "ab")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "a" {
		t.Fatalf("backspace should delete one rune from filter, got %q", next.filter)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyDelete})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "" {
		t.Fatalf("filter should be cleared only by explicit delete, got %q", next.filter)
	}
}

func TestWorkspaceSelectorModel_SpaceDoesNotAppendFilter(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", "proceed", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "w" {
		t.Fatalf("filter = %q, want %q", next.filter, "w")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "w" {
		t.Fatalf("space should not be appended to filter, got %q", next.filter)
	}
	if !next.selected[0] {
		t.Fatalf("space should toggle selection")
	}
}

func TestWorkspaceSelectorModel_FilterNarrowingResetsCursorIntoRange(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{
		{ID: "WS1", Description: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS2", Description: "beta", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS3", Description: "gamma", Risk: workspacerisk.WorkspaceRiskClean},
	}, "active", "proceed", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyDown})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", next.cursor)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.cursor != 0 {
		t.Fatalf("cursor should be reset into filtered range, got %d", next.cursor)
	}
}

func TestRenderWorkspaceSelectorLines_AlwaysShowsFilterLine(t *testing.T) {
	lines := renderWorkspaceSelectorLines(
		"active",
		"proceed",
		[]workspaceSelectorCandidate{{ID: "WS1", Description: "d", Risk: workspacerisk.WorkspaceRiskClean}},
		map[int]bool{},
		0,
		"",
		"",
		true,
		false,
		80,
	)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "filter:") {
		t.Fatalf("expected filter line to be shown, got %q", joined)
	}
	if !strings.Contains(joined, "filter: |") {
		t.Fatalf("expected filter caret to be shown, got %q", joined)
	}
}

func TestRenderWorkspaceSelectorLines_UsesActionLabelInFooter(t *testing.T) {
	lines := renderWorkspaceSelectorLines(
		"active",
		"close",
		[]workspaceSelectorCandidate{{ID: "WS1", Description: "d", Risk: workspacerisk.WorkspaceRiskClean}},
		map[int]bool{},
		0,
		"",
		"",
		true,
		false,
		120,
	)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "enter close") {
		t.Fatalf("expected action label in footer, got %q", joined)
	}
}

func TestRenderSelectorFooterLine_AlwaysKeepsSelectedSummary(t *testing.T) {
	line := renderSelectorFooterLine(2, 10, "close", 18)
	if !strings.Contains(line, "selected:") {
		t.Fatalf("footer should keep selected summary, got %q", line)
	}
}

func TestRenderSelectorFooterLine_DropsHintsDeterministically(t *testing.T) {
	line := renderSelectorFooterLine(2, 10, "close", 46)
	if !strings.Contains(line, "selected: 2/10") {
		t.Fatalf("footer missing selected summary: %q", line)
	}
	if !strings.Contains(line, "↑↓ move") {
		t.Fatalf("footer should keep first hint, got %q", line)
	}
	if strings.Contains(line, "type filter") {
		t.Fatalf("footer should truncate later hints first, got %q", line)
	}
}
