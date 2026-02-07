package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/tasuku43/gion-core/workspacerisk"
)

var errSelectorCanceled = errors.New("selector canceled")

type closeSelectorCandidate struct {
	ID          string
	Description string
	Risk        workspacerisk.WorkspaceRisk
}

type closeSelectorModel struct {
	candidates []closeSelectorCandidate
	selected   map[int]bool
	cursor     int
	width      int
	useColor   bool
	debugf     func(string, ...any)
	message    string
	canceled   bool
	done       bool
}

func newCloseSelectorModel(candidates []closeSelectorCandidate, useColor bool, debugf func(string, ...any)) closeSelectorModel {
	if debugf == nil {
		debugf = func(string, ...any) {}
	}
	return closeSelectorModel{
		candidates: candidates,
		selected:   make(map[int]bool, len(candidates)),
		cursor:     0,
		width:      80,
		useColor:   useColor,
		debugf:     debugf,
	}
}

func (m closeSelectorModel) Init() tea.Cmd {
	return nil
}

func (m closeSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyMsg:
		m.debugf("selector key type=%v runes=%q cursor=%d", msg.Type, string(msg.Runes), m.cursor)
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.canceled = true
			m.debugf("selector canceled")
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			m.message = ""
			m.debugf("selector move up cursor=%d", m.cursor)
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.candidates)-1 {
				m.cursor++
			}
			m.message = ""
			m.debugf("selector move down cursor=%d", m.cursor)
			return m, nil
		case tea.KeySpace:
			m.selected[m.cursor] = !m.selected[m.cursor]
			m.message = ""
			m.debugf("selector toggle cursor=%d selected=%t", m.cursor, m.selected[m.cursor])
			return m, nil
		case tea.KeyEnter:
			if m.selectedCount() == 0 {
				m.message = "at least one workspace must be selected"
				m.debugf("selector enter rejected: no selection")
				return m, nil
			}
			m.done = true
			m.debugf("selector done selected=%v", m.selectedIDs())
			return m, tea.Quit
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'j', 'J':
				if m.cursor < len(m.candidates)-1 {
					m.cursor++
				}
				m.message = ""
				m.debugf("selector move down cursor=%d", m.cursor)
				return m, nil
			case 'k', 'K':
				if m.cursor > 0 {
					m.cursor--
				}
				m.message = ""
				m.debugf("selector move up cursor=%d", m.cursor)
				return m, nil
			case ' ', '　':
				m.selected[m.cursor] = !m.selected[m.cursor]
				m.message = ""
				m.debugf("selector toggle cursor=%d selected=%t", m.cursor, m.selected[m.cursor])
				return m, nil
			}
		}
	}

	return m, nil
}

func (m closeSelectorModel) View() string {
	lines := renderCloseSelectorLines(m.candidates, m.selected, m.cursor, m.message, m.useColor, m.width)
	return strings.Join(lines, "\n")
}

func (m closeSelectorModel) selectedCount() int {
	count := 0
	for _, picked := range m.selected {
		if picked {
			count++
		}
	}
	return count
}

func (m closeSelectorModel) selectedIDs() []string {
	ids := make([]string, 0, len(m.candidates))
	for i, it := range m.candidates {
		if m.selected[i] {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

func (c *CLI) promptWorkspaceCloseSelector(candidates []closeSelectorCandidate) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}

	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		return nil, fmt.Errorf("interactive workspace selection requires a TTY")
	}

	useColor := writerSupportsColor(c.Err)
	return runCloseSelector(inFile, c.Err, candidates, useColor, c.debugf)
}

func runCloseSelector(in *os.File, out io.Writer, candidates []closeSelectorCandidate, useColor bool, debugf func(string, ...any)) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}

	model := newCloseSelectorModel(candidates, useColor, debugf)
	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	)

	finalModel, err := program.Run()
	if err != nil {
		return nil, err
	}
	m, ok := finalModel.(closeSelectorModel)
	if !ok {
		return nil, fmt.Errorf("unexpected selector model type")
	}
	if m.canceled {
		return nil, errSelectorCanceled
	}

	ids := m.selectedIDs()
	if len(ids) == 0 {
		return nil, errSelectorCanceled
	}
	return ids, nil
}

func renderCloseSelectorLines(candidates []closeSelectorCandidate, selected map[int]bool, cursor int, message string, useColor bool, termWidth int) []string {
	idWidth := len("workspace")
	for _, it := range candidates {
		if n := len(it.ID); n > idWidth {
			idWidth = n
		}
	}
	if idWidth < 10 {
		idWidth = 10
	}
	if idWidth > 24 {
		idWidth = 24
	}

	selectedCount := 0
	for _, picked := range selected {
		if picked {
			selectedCount++
		}
	}

	titleLine := renderWorkspacesTitle("active", useColor)
	footerRaw := fmt.Sprintf("selected: %d/%d  ↑↓ move  space toggle  enter proceed  esc/c-c cancel", selectedCount, len(candidates))
	footer := styleMuted(footerRaw, useColor)

	if termWidth < 48 {
		termWidth = 48
	}
	maxCols := termWidth - 1

	bodyLines := make([]string, 0, len(candidates))
	for i, it := range candidates {
		focus := " "
		if i == cursor {
			focus = ">"
		}

		mark := " "
		if selected[i] {
			mark = "x"
		}

		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			desc = "(no description)"
		}

		idPlain := fmt.Sprintf("%-*s", idWidth, truncateDisplay(it.ID, idWidth))
		prefixPlain := fmt.Sprintf("[%s] %s  ", mark, idPlain)
		availableDescCols := maxCols - displayWidth(prefixPlain) - 2 // include focus + space
		if availableDescCols < 8 {
			availableDescCols = 8
		}
		desc = truncateDisplay(desc, availableDescCols)

		idText := colorizeRiskID(idPlain, it.Risk, useColor)
		prefix := fmt.Sprintf("[%s] %s  ", mark, idText)
		bodyRaw := prefix + desc

		lineRaw := focus + " " + bodyRaw
		lineRaw = truncateDisplay(lineRaw, maxCols)

		line := lineRaw
		if useColor {
			bodyStyled := bodyRaw
			if selected[i] {
				bodyStyled = lipgloss.NewStyle().Bold(true).Render(bodyRaw)
			}
			if i == cursor {
				// Keep cursor emphasis visible but subtle across light/dark terminal themes.
				bodyStyled = lipgloss.NewStyle().
					Background(lipgloss.AdaptiveColor{Light: "252", Dark: "236"}).
					Render(bodyStyled)
				focusAccent := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true).Render(">") + " "
				line = focusAccent + bodyStyled
			} else {
				line = focus + " " + bodyStyled
			}
		}
		bodyLines = append(bodyLines, line)
	}

	footerRaw = truncateDisplay(footerRaw, maxCols)
	footer = styleMuted(footerRaw, useColor)

	lines := make([]string, 0, len(candidates)+7)
	lines = append(lines, titleLine)
	lines = append(lines, "")
	lines = append(lines, bodyLines...)
	lines = append(lines, "")
	lines = append(lines, footer)

	if strings.TrimSpace(message) == "" {
		lines = append(lines, "")
	} else if useColor {
		lines = append(lines, styleMuted(truncateDisplay(message, maxCols), true))
	} else {
		lines = append(lines, truncateDisplay(message, maxCols))
	}

	return lines
}

func runeDisplayWidth(r rune) int {
	if r < 128 {
		return 1
	}
	return 2
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeDisplayWidth(r)
	}
	return w
}

func truncateDisplay(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	if displayWidth(s) <= maxCols {
		return s
	}
	if maxCols == 1 {
		return "…"
	}

	var b strings.Builder
	cols := 0
	limit := maxCols - 1
	for _, r := range s {
		rw := runeDisplayWidth(r)
		if cols+rw > limit {
			break
		}
		b.WriteRune(r)
		cols += rw
	}
	if b.Len() == 0 && utf8.RuneCountInString(s) > 0 {
		return "…"
	}
	return b.String() + "…"
}

func colorizeRiskID(id string, risk workspacerisk.WorkspaceRisk, useColor bool) string {
	if !useColor {
		return id
	}
	switch risk {
	case workspacerisk.WorkspaceRiskDirty, workspacerisk.WorkspaceRiskUnknown:
		return styleError(id, true)
	case workspacerisk.WorkspaceRiskDiverged, workspacerisk.WorkspaceRiskUnpushed:
		return styleWarn(id, true)
	default:
		return id
	}
}
