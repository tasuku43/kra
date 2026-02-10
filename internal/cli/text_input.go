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
	value    string
	canceled bool
}

func newInlineTextInputModel(prompt string) inlineTextInputModel {
	return inlineTextInputModel{
		prompt: prompt,
		input:  newCLITextInput(),
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
			m.value = strings.TrimSpace(m.input.Value())
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
	model := newInlineTextInputModel(prompt)
	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	)
	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintln(out)
	next, ok := finalModel.(inlineTextInputModel)
	if !ok {
		return "", fmt.Errorf("unexpected inline input model type")
	}
	if next.canceled {
		return "", errInputCanceled
	}
	return next.value, nil
}
