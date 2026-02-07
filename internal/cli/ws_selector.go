package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/tasuku43/gion-core/workspacerisk"
)

var errSelectorCanceled = errors.New("selector canceled")

const selectorCaretBlinkInterval = 500 * time.Millisecond

type selectorCaretBlinkMsg struct{}

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
	action     string
	useColor   bool
	debugf     func(string, ...any)
	message    string
	filter     string
	showCaret  bool
	canceled   bool
	done       bool
}

func newWorkspaceSelectorModel(candidates []workspaceSelectorCandidate, status string, action string, useColor bool, debugf func(string, ...any)) workspaceSelectorModel {
	if debugf == nil {
		debugf = func(string, ...any) {}
	}
	if strings.TrimSpace(action) == "" {
		action = "proceed"
	}
	return workspaceSelectorModel{
		candidates: candidates,
		selected:   make(map[int]bool, len(candidates)),
		cursor:     0,
		width:      80,
		status:     status,
		action:     action,
		useColor:   useColor,
		debugf:     debugf,
		showCaret:  true,
	}
}

func (m workspaceSelectorModel) Init() tea.Cmd {
	return selectorCaretBlinkCmd()
}

func (m workspaceSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case selectorCaretBlinkMsg:
		if m.done || m.canceled {
			return m, nil
		}
		m.showCaret = !m.showCaret
		return m, selectorCaretBlinkCmd()
	case tea.KeyMsg:
		m.debugf("selector key type=%v runes=%q cursor=%d filter=%q", msg.Type, string(msg.Runes), m.cursor, m.filter)
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
			if m.selectedCount() == 0 {
				m.message = "at least one workspace must be selected"
				m.debugf("selector enter rejected: no selection")
				return m, nil
			}
			m.done = true
			m.debugf("selector done selected=%v", m.selectedIDs())
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			m.message = ""
			m.debugf("selector move up cursor=%d", m.cursor)
			return m, nil
		case tea.KeyDown:
			visible := m.filteredIndices()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
			m.message = ""
			m.debugf("selector move down cursor=%d", m.cursor)
			return m, nil
		case tea.KeySpace:
			m.toggleCurrentSelection()
			return m, nil
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			switch r {
			case ' ', '　':
				m.toggleCurrentSelection()
				return m, nil
			case 'a':
				m.setVisibleSelection(true)
				return m, nil
			case 'A':
				m.setVisibleSelection(false)
				return m, nil
			default:
				if isFilterAppendableRune(r) {
					m.filter += string(r)
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
	lines := renderWorkspaceSelectorLines(m.status, m.action, m.candidates, m.selected, m.cursor, m.message, m.filter, m.showCaret, m.useColor, m.width)
	return strings.Join(lines, "\n")
}

func selectorCaretBlinkCmd() tea.Cmd {
	return tea.Tick(selectorCaretBlinkInterval, func(time.Time) tea.Msg {
		return selectorCaretBlinkMsg{}
	})
}

func (m *workspaceSelectorModel) toggleCurrentSelection() {
	visible := m.filteredIndices()
	if len(visible) == 0 {
		m.message = "no workspaces match current filter"
		return
	}
	idx := visible[m.cursor]
	m.selected[idx] = !m.selected[idx]
	m.message = ""
	m.debugf("selector toggle idx=%d selected=%t", idx, m.selected[idx])
}

func (m *workspaceSelectorModel) setVisibleSelection(selected bool) {
	visible := m.filteredIndices()
	if len(visible) == 0 {
		m.message = "no workspaces match current filter"
		return
	}
	for _, idx := range visible {
		m.selected[idx] = selected
	}
	m.message = ""
	if selected {
		m.debugf("selector select-all visible count=%d", len(visible))
	} else {
		m.debugf("selector clear-all visible count=%d", len(visible))
	}
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

func (c *CLI) promptWorkspaceSelector(status string, action string, candidates []workspaceSelectorCandidate) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}

	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		return nil, fmt.Errorf("interactive workspace selection requires a TTY")
	}

	useColor := writerSupportsColor(c.Err)
	return runWorkspaceSelector(inFile, c.Err, status, action, candidates, useColor, c.debugf)
}

func runWorkspaceSelector(in *os.File, out io.Writer, status string, action string, candidates []workspaceSelectorCandidate, useColor bool, debugf func(string, ...any)) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}
	if strings.TrimSpace(status) == "" {
		status = "active"
	}

	model := newWorkspaceSelectorModel(candidates, status, action, useColor, debugf)
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

func renderWorkspaceSelectorLines(status string, action string, candidates []workspaceSelectorCandidate, selected map[int]bool, cursor int, message string, filter string, showCaret bool, useColor bool, termWidth int) []string {
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
	if strings.TrimSpace(action) == "" {
		action = "proceed"
	}

	selectedCount := 0
	for _, picked := range selected {
		if picked {
			selectedCount++
		}
	}
	visible := filteredCandidateIndices(candidates, filter)
	totalVisible := len(visible)
	filterLabel := filter

	titleLine := renderWorkspacesTitle(status, useColor)
	if termWidth < 24 {
		termWidth = 24
	}
	maxCols := termWidth - 1
	footerRaw := renderSelectorFooterLine(selectedCount, len(candidates), action, maxCols)
	footer := styleMuted(footerRaw, useColor)

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

	lines := make([]string, 0, len(candidates)+7)
	lines = append(lines, titleLine)
	lines = append(lines, "")
	lines = append(lines, bodyLines...)
	lines = append(lines, "")
	availableFilterCols := maxCols - displayWidth(uiIndent+"filter: ") - 1
	if availableFilterCols < 1 {
		availableFilterCols = 1
	}
	filterBody := truncateDisplay(filterLabel, availableFilterCols)
	caret := " "
	if showCaret {
		caret = "|"
	}
	if useColor {
		base := styleMuted(fmt.Sprintf("%sfilter: %s", uiIndent, filterBody), true)
		caretStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true).Render(caret)
		lines = append(lines, base+caretStyled)
	} else {
		lines = append(lines, fmt.Sprintf("%sfilter: %s%s", uiIndent, filterBody, caret))
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

func renderSelectorFooterLine(selectedCount int, total int, action string, maxCols int) string {
	base := fmt.Sprintf("%sselected: %d/%d", uiIndent, selectedCount, total)
	if maxCols <= 0 {
		return ""
	}
	if displayWidth(base) > maxCols {
		return truncateDisplay(base, maxCols)
	}

	parts := []string{
		"↑↓ move",
		"space toggle",
		fmt.Sprintf("enter %s", action),
		"a all",
		"A none",
		"type filter",
		"esc cancel",
	}

	line := base
	for i, part := range parts {
		candidate := line + "  " + part
		if displayWidth(candidate) > maxCols {
			if i < len(parts)-1 {
				withEllipsis := line + "  …"
				if displayWidth(withEllipsis) <= maxCols {
					return withEllipsis
				}
			}
			return line
		}
		line = candidate
	}
	return line
}

func filteredCandidateIndices(candidates []workspaceSelectorCandidate, filter string) []int {
	query := strings.ToLower(filter)
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

func isFilterAppendableRune(r rune) bool {
	if unicode.IsControl(r) {
		return false
	}
	if unicode.IsSpace(r) {
		return false
	}
	return true
}

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
}

func runeDisplayWidth(r rune) int {
	if r == '…' {
		return 1
	}
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
