package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var errInputCanceled = errors.New("input canceled")

func newCLITextInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Focus()
	return input
}

type inlineTextInputModel struct {
	prompt   string
	input    textinput.Model
	initial  string
	value    string
	edited   bool
	canceled bool
}

func newInlineTextInputModel(prompt string) inlineTextInputModel {
	return newInlineTextInputModelWithInitial(prompt, "")
}

func newInlineTextInputModelWithInitial(prompt string, initialValue string) inlineTextInputModel {
	initial := strings.TrimSpace(initialValue)
	input := newCLITextInput()
	input.SetValue(initial)
	input.CursorEnd()
	return inlineTextInputModel{
		prompt:  prompt,
		input:   input,
		initial: initial,
	}
}

func (m inlineTextInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m inlineTextInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			value := strings.TrimSpace(m.input.Value())
			if value == "" && m.initial != "" {
				value = m.initial
			}
			m.value = value
			m.edited = value != m.initial
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.canceled = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m inlineTextInputModel) View() string {
	return fmt.Sprintf("%s%s", m.prompt, m.input.View())
}

func runInlineTextInput(in io.Reader, out io.Writer, prompt string) (string, error) {
	value, _, err := runInlineTextInputWithInitial(in, out, prompt, "")
	return value, err
}

func runInlineTextInputWithInitial(in io.Reader, out io.Writer, prompt string, initialValue string) (string, bool, error) {
	model := newInlineTextInputModelWithInitial(prompt, initialValue)
	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	)
	finalModel, err := program.Run()
	if err != nil {
		return "", false, err
	}
	next, ok := finalModel.(inlineTextInputModel)
	if !ok {
		return "", false, fmt.Errorf("unexpected inline input model type")
	}
	if next.canceled {
		return "", false, errInputCanceled
	}
	return next.value, next.edited, nil
}
