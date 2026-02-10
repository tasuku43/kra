package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInlineTextInputModel_EnterCommitsTrimmedValue(t *testing.T) {
	m := newInlineTextInputModel("prompt: ")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	next, ok := updated.(inlineTextInputModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	next, ok = updated.(inlineTextInputModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok = updated.(inlineTextInputModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.value != "a" {
		t.Fatalf("value = %q, want %q", next.value, "a")
	}
	if next.canceled {
		t.Fatalf("enter should not cancel input")
	}
}

func TestInlineTextInputModel_EscapeCancelsInput(t *testing.T) {
	m := newInlineTextInputModel("prompt: ")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, ok := updated.(inlineTextInputModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !next.canceled {
		t.Fatalf("escape should cancel input")
	}
}
