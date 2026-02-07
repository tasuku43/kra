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

type workspaceSelectorCandidate struct {
	ID          string
	Description string
	Risk        workspacerisk.WorkspaceRisk
}

type workspaceSelectorModel struct {
	candidates []workspaceSelectorCandidate
	selected   map[int]bool
	cursor     int // cursor position within filtered indices
	width      int
	status     string
	useColor   bool
	debugf     func(string, ...any)
	message    string
	filter     string
	filterMode bool
	canceled   bool
	done       bool
}

func newWorkspaceSelectorModel(candidates []workspaceSelectorCandidate, status string, useColor bool, debugf func(string, ...any)) workspaceSelectorModel {
	if debugf == nil {
		debugf = func(string, ...any) {}
	}
	return workspaceSelectorModel{
		candidates: candidates,
		selected:   make(map[int]bool, len(candidates)),
		cursor:     0,
		width:      80,
		status:     status,
		useColor:   useColor,
		debugf:     debugf,
	}
}

func (m workspaceSelectorModel) Init() tea.Cmd {
	return nil
}

func (m workspaceSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyMsg:
		m.debugf("selector key type=%v runes=%q cursor=%d filter=%q filterMode=%t", msg.Type, string(msg.Runes), m.cursor, m.filter, m.filterMode)
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.canceled = true
			m.debugf("selector canceled")
			return m, tea.Quit
		case tea.KeyBackspace, tea.KeyDelete:
			if m.filter != "" {
				m.filter = trimLastRune(m.filter)
				m.ensureCursorInFilteredRange()
				m.message = ""
				return m, nil
			}
			return m, nil
		case tea.KeyEnter:
			if m.filterMode {
				m.filterMode = false
				m.message = ""
				return m, nil
			}
			if m.selectedCount() == 0 {
				m.message = "at least one workspace must be selected"
				m.debugf("selector enter rejected: no selection")
				return m, nil
			}
			m.done = true
			m.debugf("selector done selected=%v", m.selectedIDs())
			return m, tea.Quit
		case tea.KeyUp:
			if m.filterMode {
				return m, nil
			}
			if m.cursor > 0 {
				m.cursor--
			}
			m.message = ""
			m.debugf("selector move up cursor=%d", m.cursor)
			return m, nil
		case tea.KeyDown:
			if m.filterMode {
				return m, nil
			}
			visible := m.filteredIndices()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
			m.message = ""
			m.debugf("selector move down cursor=%d", m.cursor)
			return m, nil
		case tea.KeySpace:
			if m.filterMode {
				return m, nil
			}
			visible := m.filteredIndices()
			if len(visible) == 0 {
				m.message = "no workspaces match current filter"
				return m, nil
			}
			idx := visible[m.cursor]
			m.selected[idx] = !m.selected[idx]
			m.message = ""
			m.debugf("selector toggle idx=%d selected=%t", idx, m.selected[idx])
			return m, nil
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case '/':
				m.filterMode = true
				m.message = ""
				return m, nil
			case 'j', 'J':
				if m.filterMode {
					m.filter += string(msg.Runes[0])
					m.ensureCursorInFilteredRange()
					m.message = ""
					return m, nil
				}
				visible := m.filteredIndices()
				if m.cursor < len(visible)-1 {
					m.cursor++
				}
				m.message = ""
				m.debugf("selector move down cursor=%d", m.cursor)
				return m, nil
			case 'k', 'K':
				if m.filterMode {
					m.filter += string(msg.Runes[0])
					m.ensureCursorInFilteredRange()
					m.message = ""
					return m, nil
				}
				if m.cursor > 0 {
					m.cursor--
				}
				m.message = ""
				m.debugf("selector move up cursor=%d", m.cursor)
				return m, nil
			case ' ', '　':
				if m.filterMode {
					m.filter += string(msg.Runes[0])
					m.ensureCursorInFilteredRange()
					m.message = ""
					return m, nil
				}
				visible := m.filteredIndices()
				if len(visible) == 0 {
					m.message = "no workspaces match current filter"
					return m, nil
				}
				idx := visible[m.cursor]
				m.selected[idx] = !m.selected[idx]
				m.message = ""
				m.debugf("selector toggle idx=%d selected=%t", idx, m.selected[idx])
				return m, nil
			default:
				if m.filterMode {
					m.filter += string(msg.Runes[0])
					m.ensureCursorInFilteredRange()
					m.message = ""
					return m, nil
				}
			}
		}
	}

	return m, nil
}

func (m workspaceSelectorModel) View() string {
	lines := renderWorkspaceSelectorLines(m.status, m.candidates, m.selected, m.cursor, m.message, m.filter, m.filterMode, m.useColor, m.width)
	return strings.Join(lines, "\n")
}

func (m *workspaceSelectorModel) ensureCursorInFilteredRange() {
	visible := m.filteredIndices()
	if len(visible) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(visible) {
		m.cursor = len(visible) - 1
	}
}

func (m workspaceSelectorModel) filteredIndices() []int {
	return filteredCandidateIndices(m.candidates, m.filter)
}

func (m workspaceSelectorModel) selectedCount() int {
	count := 0
	for _, picked := range m.selected {
		if picked {
			count++
		}
	}
	return count
}

func (m workspaceSelectorModel) selectedIDs() []string {
	ids := make([]string, 0, len(m.candidates))
	for i, it := range m.candidates {
		if m.selected[i] {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

func (c *CLI) promptWorkspaceSelector(status string, candidates []workspaceSelectorCandidate) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}

	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		return nil, fmt.Errorf("interactive workspace selection requires a TTY")
	}

	useColor := writerSupportsColor(c.Err)
	return runWorkspaceSelector(inFile, c.Err, status, candidates, useColor, c.debugf)
}

func runWorkspaceSelector(in *os.File, out io.Writer, status string, candidates []workspaceSelectorCandidate, useColor bool, debugf func(string, ...any)) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}
	if strings.TrimSpace(status) == "" {
		status = "active"
	}

	model := newWorkspaceSelectorModel(candidates, status, useColor, debugf)
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
	m, ok := finalModel.(workspaceSelectorModel)
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

func renderWorkspaceSelectorLines(status string, candidates []workspaceSelectorCandidate, selected map[int]bool, cursor int, message string, filter string, filterMode bool, useColor bool, termWidth int) []string {
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
	visible := filteredCandidateIndices(candidates, filter)
	totalVisible := len(visible)
	filterLabel := strings.TrimSpace(filter)

	titleLine := renderWorkspacesTitle(status, useColor)
	footerRaw := fmt.Sprintf("%sselected: %d/%d  ↑↓ move  space toggle  enter proceed  / filter  esc/c-c cancel", uiIndent, selectedCount, len(candidates))
	footer := styleMuted(footerRaw, useColor)

	if termWidth < 48 {
		termWidth = 48
	}
	maxCols := termWidth - 1

	bodyLines := make([]string, 0, len(candidates))
	if totalVisible == 0 {
		empty := uiIndent + "(no matches)"
		if useColor {
			empty = styleMuted(empty, true)
		}
		bodyLines = append(bodyLines, empty)
	}
	for visiblePos, sourceIdx := range visible {
		it := candidates[sourceIdx]
		focus := " "
		if visiblePos == cursor {
			focus = ">"
		}

		mark := " "
		if selected[sourceIdx] {
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
			if selected[sourceIdx] {
				bodyStyled = lipgloss.NewStyle().Bold(true).Render(bodyRaw)
			}
			if visiblePos == cursor {
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
	if filterLabel != "" {
		filterLine := fmt.Sprintf("%sfilter: %s", uiIndent, truncateDisplay(filterLabel, maxCols-8))
		if filterMode {
			filterLine += " _"
		}
		if useColor {
			lines = append(lines, styleMuted(filterLine, true))
		} else {
			lines = append(lines, filterLine)
		}
	}
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

func filteredCandidateIndices(candidates []workspaceSelectorCandidate, filter string) []int {
	query := strings.ToLower(strings.TrimSpace(filter))
	out := make([]int, 0, len(candidates))
	for i, c := range candidates {
		if query == "" {
			out = append(out, i)
			continue
		}
		id := strings.ToLower(c.ID)
		desc := strings.ToLower(c.Description)
		if strings.Contains(id, query) || strings.Contains(desc, query) {
			out = append(out, i)
		}
	}
	return out
}

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
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
