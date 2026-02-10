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
		{ID: "WS1", Title: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS2", Title: "beta", Risk: workspacerisk.WorkspaceRiskClean},
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

func TestFilteredCandidateIndices_UsesFuzzyMatch(t *testing.T) {
	candidates := []workspaceSelectorCandidate{
		{ID: "example-org/helmfiles", Title: "platform", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "tasuku43/gionx", Title: "command line tool", Risk: workspacerisk.WorkspaceRiskClean},
	}

	got := filteredCandidateIndices(candidates, "cs")
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("filteredCandidateIndices should fuzzy-match id: got=%v", got)
	}

	got = filteredCandidateIndices(candidates, "d l t")
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("filteredCandidateIndices should fuzzy-match title with whitespace-insensitive query: got=%v", got)
	}
}

func TestWorkspaceSelectorModel_FilterClearsByDeleteOneRuneAtATime(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean}}, "active", "proceed", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "cd" {
		t.Fatalf("filter = %q, want %q", next.filter, "cd")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "c" {
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

func TestWorkspaceSelectorModel_LetterAIsFilterInput(t *testing.T) {
	m := newWorkspaceSelectorModel([]workspaceSelectorCandidate{
		{ID: "WS1", Title: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS2", Title: "beta", Risk: workspacerisk.WorkspaceRiskClean},
	}, "active", "proceed", false, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.filter != "a" {
		t.Fatalf("a should be appended to filter, got %q", next.filter)
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
		{ID: "WS1", Title: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS2", Title: "beta", Risk: workspacerisk.WorkspaceRiskClean},
		{ID: "WS3", Title: "gamma", Risk: workspacerisk.WorkspaceRiskClean},
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
		[]workspaceSelectorCandidate{{ID: "WS1", Title: "d", Risk: workspacerisk.WorkspaceRiskClean}},
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
	if strings.Contains(joined, "filter: |") {
		t.Fatalf("filter line should not render synthetic caret, got %q", joined)
	}
}

func TestRenderWorkspaceSelectorLines_UsesActionLabelInFooter(t *testing.T) {
	lines := renderWorkspaceSelectorLines(
		"active",
		"close",
		[]workspaceSelectorCandidate{{ID: "WS1", Title: "d", Risk: workspacerisk.WorkspaceRiskClean}},
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

func TestRenderWorkspaceSelectorLines_MessageIsIndented(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"",
		"close",
		[]workspaceSelectorCandidate{{ID: "WS1", Title: "d", Risk: workspacerisk.WorkspaceRiskClean}},
		map[int]bool{},
		0,
		"at least one workspace must be selected",
		selectorMessageLevelError,
		"",
		true,
		true,
		false,
		false,
		false,
		120,
	)
	msg := lines[len(lines)-1]
	if !strings.HasPrefix(msg, uiIndent) {
		t.Fatalf("message should start with shared indent %q, got %q", uiIndent, msg)
	}
}

func TestRenderWorkspaceSelectorLines_ErrorMessageUsesErrorToken(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"",
		"close",
		[]workspaceSelectorCandidate{{ID: "WS1", Title: "d", Risk: workspacerisk.WorkspaceRiskClean}},
		map[int]bool{},
		0,
		"at least one workspace must be selected",
		selectorMessageLevelError,
		"",
		true,
		true,
		false,
		false,
		true,
		120,
	)
	msg := lines[len(lines)-1]
	if !strings.Contains(msg, ansiError) {
		t.Fatalf("error message should include error color token, got %q", msg)
	}
}

func TestRenderSelectorFooterLine_AlwaysKeepsSelectedSummary(t *testing.T) {
	line := renderSelectorFooterLine(2, 10, "close", false, 18)
	if !strings.Contains(line, "selected:") {
		t.Fatalf("footer should keep selected summary, got %q", line)
	}
}

func TestRenderSelectorFooterLine_DropsHintsDeterministically(t *testing.T) {
	line := renderSelectorFooterLine(2, 10, "close", false, 46)
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

func TestRenderWorkspaceSelectorLinesWithOptions_SingleModeShowsSelectionMarkerAndHidesSelectedSummary(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"",
		"go",
		[]workspaceSelectorCandidate{
			{ID: "WS1", Title: "desc", Risk: workspacerisk.WorkspaceRiskClean},
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
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "○ WS1") {
		t.Fatalf("single mode should show circle marker: %q", joined)
	}
	if strings.Contains(joined, "selected:") {
		t.Fatalf("single mode footer should not show selected summary: %q", joined)
	}
	if !strings.Contains(joined, "enter go") {
		t.Fatalf("single mode footer should keep enter action hint: %q", joined)
	}
}

func TestRenderWorkspaceSelectorLinesWithOptions_MultiModeUsesCircleSelectionMarker(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"",
		"add",
		[]workspaceSelectorCandidate{
			{ID: "WS1", Title: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "WS2", Title: "beta", Risk: workspacerisk.WorkspaceRiskClean},
		},
		map[int]bool{1: true},
		0,
		"",
		selectorMessageLevelMuted,
		"",
		true,
		true,
		false,
		false,
		false,
		120,
	)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "○ WS1") {
		t.Fatalf("multi mode should render unselected marker, got %q", joined)
	}
	if !strings.Contains(joined, "● WS2") {
		t.Fatalf("multi mode should render selected marker, got %q", joined)
	}
}

func TestRenderWorkspaceSelectorLinesWithOptions_RepoModeCompactsHeaderSpacing(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"Repos(pool):",
		"add",
		[]workspaceSelectorCandidate{
			{ID: "example-org/helmfiles", Risk: workspacerisk.WorkspaceRiskUnknown},
		},
		map[int]bool{},
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
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "Repos(pool):\n\n") {
		t.Fatalf("repo mode should not keep blank line after title: %q", joined)
	}
	if !strings.Contains(joined, "Repos(pool):\n> ○ ") {
		t.Fatalf("repo mode should render row right after title: %q", joined)
	}
}

func TestRenderWorkspaceSelectorLinesWithOptions_RepoModeSelectedMarkerUsesAccentColor(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"Repos(pool):",
		"add",
		[]workspaceSelectorCandidate{
			{ID: "example-org/helmfiles", Risk: workspacerisk.WorkspaceRiskUnknown},
		},
		map[int]bool{0: true},
		0,
		"",
		selectorMessageLevelMuted,
		"",
		false,
		true,
		false,
		false,
		true,
		120,
	)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, ansiAccent+"●"+ansiReset) {
		t.Fatalf("repo mode selected marker should use accent token, got %q", joined)
	}
}

func TestWorkspaceSelectorModel_SingleModeEnterSelectsCurrent(t *testing.T) {
	m := newWorkspaceSelectorModelWithOptionsAndMode(
		[]workspaceSelectorCandidate{
			{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "WS2", Risk: workspacerisk.WorkspaceRiskClean},
		},
		"active",
		"go",
		"",
		"workspace",
		true,
		false,
		nil,
	)
	m.cursor = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.done {
		t.Fatalf("single mode enter should wait confirm delay before completion")
	}
	if !next.confirming {
		t.Fatalf("single mode should enter confirming state")
	}
	if next.filterInput.Focused() {
		t.Fatalf("single mode confirm should hide filter cursor before transition")
	}
	ids := next.selectedIDs()
	if len(ids) != 1 || ids[0] != "WS2" {
		t.Fatalf("single mode should select focused row, got=%v", ids)
	}

	updated, _ = next.Update(selectorConfirmDoneMsg{})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.done {
		t.Fatalf("single mode should complete after confirm delay")
	}
}

func TestWorkspaceSelectorModel_SingleModeLocksInputWhileConfirming(t *testing.T) {
	m := newWorkspaceSelectorModelWithOptionsAndMode(
		[]workspaceSelectorCandidate{
			{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "WS2", Risk: workspacerisk.WorkspaceRiskClean},
		},
		"active",
		"go",
		"",
		"workspace",
		true,
		false,
		nil,
	)
	m.cursor = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.confirming {
		t.Fatalf("single mode should enter confirming state")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyUp})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.cursor != 1 {
		t.Fatalf("cursor should be locked while confirming, got=%d", next.cursor)
	}
}

func TestWorkspaceSelectorModel_MultiModeEnterWaitsConfirmDelay(t *testing.T) {
	m := newWorkspaceSelectorModel(
		[]workspaceSelectorCandidate{
			{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "WS2", Risk: workspacerisk.WorkspaceRiskClean},
		},
		"active",
		"close",
		false,
		nil,
	)
	m.selected[1] = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.done {
		t.Fatalf("multi mode enter should wait confirm delay before completion")
	}
	if !next.confirming {
		t.Fatalf("multi mode should enter confirming state")
	}
	if next.filterInput.Focused() {
		t.Fatalf("multi mode confirm should hide filter cursor before transition")
	}

	updated, _ = next.Update(selectorConfirmDoneMsg{})
	next, ok = updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.done {
		t.Fatalf("multi mode should complete after confirm delay")
	}
}

func TestWorkspaceSelectorModel_MultiModeReducedMotionSkipsConfirmDelay(t *testing.T) {
	t.Setenv("GIONX_REDUCED_MOTION", "1")

	m := newWorkspaceSelectorModel(
		[]workspaceSelectorCandidate{
			{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "WS2", Risk: workspacerisk.WorkspaceRiskClean},
		},
		"active",
		"close",
		false,
		nil,
	)
	m.selected[0] = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.done {
		t.Fatalf("multi mode should complete immediately when reduced motion is enabled")
	}
	if next.confirming {
		t.Fatalf("multi mode should not enter confirming state with reduced motion")
	}
	if next.filterInput.Focused() {
		t.Fatalf("multi mode reduced motion should hide filter cursor before transition")
	}
}

func TestWorkspaceSelectorModel_SingleModeReducedMotionSkipsConfirmDelay(t *testing.T) {
	t.Setenv("GIONX_REDUCED_MOTION", "1")

	m := newWorkspaceSelectorModelWithOptionsAndMode(
		[]workspaceSelectorCandidate{
			{ID: "WS1", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "WS2", Risk: workspacerisk.WorkspaceRiskClean},
		},
		"active",
		"go",
		"",
		"workspace",
		true,
		false,
		nil,
	)
	m.cursor = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok := updated.(workspaceSelectorModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.done {
		t.Fatalf("single mode should complete immediately when reduced motion is enabled")
	}
	if next.confirming {
		t.Fatalf("single mode should not enter confirming state with reduced motion")
	}
	if next.filterInput.Focused() {
		t.Fatalf("single mode reduced motion should hide filter cursor before transition")
	}
}

func TestRenderWorkspaceSelectorLinesWithOptions_SingleConfirmingMutesNonSelectedRows(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"",
		"go",
		[]workspaceSelectorCandidate{
			{ID: "WS1", Title: "alpha", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "WS2", Title: "beta", Risk: workspacerisk.WorkspaceRiskClean},
		},
		map[int]bool{1: true},
		1,
		"",
		selectorMessageLevelMuted,
		"",
		true,
		true,
		true,
		true,
		true,
		120,
	)

	targetLine := ""
	for _, line := range lines {
		if strings.Contains(line, "○ WS1") {
			targetLine = line
			break
		}
	}
	if targetLine == "" {
		t.Fatalf("expected unselected line in output, got=%q", strings.Join(lines, "\n"))
	}
	if !strings.Contains(targetLine, ansiMuted) {
		t.Fatalf("unselected line should be muted while confirming, got=%q", targetLine)
	}
}

func TestRenderWorkspaceSelectorLinesWithOptions_MultiConfirmingMutesNonSelectedRows(t *testing.T) {
	lines := renderWorkspaceSelectorLinesWithOptions(
		"active",
		"Repos(pool):",
		"add",
		[]workspaceSelectorCandidate{
			{ID: "example-org/helmfiles", Risk: workspacerisk.WorkspaceRiskClean},
			{ID: "example-org/sre-apps", Risk: workspacerisk.WorkspaceRiskClean},
		},
		map[int]bool{1: true},
		1,
		"",
		selectorMessageLevelMuted,
		"",
		false,
		true,
		false,
		true,
		true,
		120,
	)

	targetLine := ""
	for _, line := range lines {
		if strings.Contains(line, "example-org/helmfiles") {
			targetLine = line
			break
		}
	}
	if targetLine == "" {
		t.Fatalf("expected unselected line in output, got=%q", strings.Join(lines, "\n"))
	}
	if !strings.Contains(targetLine, ansiMuted) {
		t.Fatalf("unselected line should be muted while multi confirming, got=%q", targetLine)
	}
}
