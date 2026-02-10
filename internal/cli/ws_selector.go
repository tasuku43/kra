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

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/tasuku43/gion-core/workspacerisk"
)

var errSelectorCanceled = errors.New("selector canceled")

const selectorSingleConfirmDelay = 200 * time.Millisecond

type selectorConfirmDoneMsg struct{}
type selectorMessageLevel string

const (
	selectorMessageLevelMuted selectorMessageLevel = "muted"
	selectorMessageLevelError selectorMessageLevel = "error"
)

type workspaceSelectorCandidate struct {
	ID          string
	Title       string
	Description string
	Risk        workspacerisk.WorkspaceRisk
}

func (c workspaceSelectorCandidate) secondaryText() string {
	desc := strings.TrimSpace(c.Description)
	if desc != "" {
		return desc
	}
	return strings.TrimSpace(c.Title)
}

type workspaceSelectorModel struct {
	candidates    []workspaceSelectorCandidate
	selected      map[int]bool
	cursor        int // cursor position within filtered indices
	width         int
	height        int
	status        string
	title         string
	itemLabel     string
	showDesc      bool
	action        string
	useColor      bool
	debugf        func(string, ...any)
	message       string
	msgLevel      selectorMessageLevel
	filter        string
	filterInput   textinput.Model
	canceled      bool
	done          bool
	single        bool
	confirming    bool
	reducedMotion bool
}

func newWorkspaceSelectorModel(candidates []workspaceSelectorCandidate, status string, action string, useColor bool, debugf func(string, ...any)) workspaceSelectorModel {
	return newWorkspaceSelectorModelWithOptionsAndMode(candidates, status, action, "", "workspace", false, useColor, debugf)
}

func newWorkspaceSelectorModelWithOptions(candidates []workspaceSelectorCandidate, status string, action string, title string, itemLabel string, useColor bool, debugf func(string, ...any)) workspaceSelectorModel {
	return newWorkspaceSelectorModelWithOptionsAndMode(candidates, status, action, title, itemLabel, false, useColor, debugf)
}

func newWorkspaceSelectorModelWithOptionsAndMode(candidates []workspaceSelectorCandidate, status string, action string, title string, itemLabel string, single bool, useColor bool, debugf func(string, ...any)) workspaceSelectorModel {
	if debugf == nil {
		debugf = func(string, ...any) {}
	}
	if strings.TrimSpace(action) == "" {
		action = "proceed"
	}
	if strings.TrimSpace(itemLabel) == "" {
		itemLabel = "workspace"
	}
	filterInput := newCLITextInput()
	return workspaceSelectorModel{
		candidates:    candidates,
		selected:      make(map[int]bool, len(candidates)),
		cursor:        0,
		width:         80,
		height:        24,
		status:        status,
		title:         title,
		itemLabel:     itemLabel,
		showDesc:      strings.ToLower(strings.TrimSpace(itemLabel)) != "repo",
		action:        action,
		useColor:      useColor,
		debugf:        debugf,
		msgLevel:      selectorMessageLevelMuted,
		single:        single,
		reducedMotion: isReducedMotionEnabled(),
		filterInput:   filterInput,
	}
}

func (m workspaceSelectorModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m workspaceSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil
	case selectorConfirmDoneMsg:
		if !m.confirming || m.canceled {
			return m, nil
		}
		m.done = true
		m.debugf("selector done (confirm) selected=%v", m.selectedIDs())
		return m, tea.Quit
	case tea.KeyMsg:
		if m.confirming {
			// Lock input while showing the selected single item before transition.
			return m, nil
		}
		m.debugf("selector key type=%v runes=%q cursor=%d filter=%q", msg.Type, string(msg.Runes), m.cursor, m.filter)
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.canceled = true
			m.debugf("selector canceled")
			return m, tea.Quit
		case tea.KeyBackspace, tea.KeyDelete:
			if m.deleteFilterRuneBeforeCursor() {
				m.ensureCursorInFilteredRange()
				m.clearMessage()
			}
			return m, nil
		case tea.KeyEnter:
			if m.single {
				visible := m.filteredIndices()
				if len(visible) == 0 {
					m.setErrorMessage(fmt.Sprintf("at least one %s must be visible", m.itemLabel))
					m.debugf("selector enter rejected: no visible candidate")
					return m, nil
				}
				idx := visible[m.cursor]
				m.selected = map[int]bool{idx: true}
				m.filterInput.Blur()
				if m.reducedMotion {
					m.done = true
					m.debugf("selector done (single reduced motion) selected=%v", m.selectedIDs())
					return m, tea.Quit
				}
				m.confirming = true
				return m, selectorSingleConfirmCmd()
			} else {
				if m.selectedCount() == 0 {
					m.setErrorMessage(fmt.Sprintf("at least one %s must be selected", m.itemLabel))
					m.debugf("selector enter rejected: no selection")
					return m, nil
				}
				m.filterInput.Blur()
				if m.reducedMotion {
					m.done = true
					m.debugf("selector done (multi reduced motion) selected=%v", m.selectedIDs())
					return m, tea.Quit
				}
				m.confirming = true
				return m, selectorSingleConfirmCmd()
			}
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			m.clearMessage()
			m.debugf("selector move up cursor=%d", m.cursor)
			return m, nil
		case tea.KeyDown:
			visible := m.filteredIndices()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
			m.clearMessage()
			m.debugf("selector move down cursor=%d", m.cursor)
			return m, nil
		case tea.KeySpace:
			if m.single {
				return m, nil
			}
			m.toggleCurrentSelection()
			return m, nil
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			switch r {
			case ' ', '　':
				if m.single {
					return m, nil
				}
				m.toggleCurrentSelection()
				return m, nil
			default:
				if isFilterAppendableRune(r) {
					m.setFilter(m.filter + string(r))
					m.ensureCursorInFilteredRange()
					m.clearMessage()
					return m, nil
				}
			}
		}
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		if next := m.filterInput.Value(); next != m.filter {
			m.filter = next
			m.filterInput.CursorEnd()
			m.ensureCursorInFilteredRange()
			m.clearMessage()
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	if next := m.filterInput.Value(); next != m.filter {
		m.filter = next
		m.filterInput.CursorEnd()
		m.ensureCursorInFilteredRange()
		m.clearMessage()
	}
	return m, cmd
}

func (m workspaceSelectorModel) View() string {
	filterInput := m.filterInput
	filterInput.Width = selectorFilterWidth(m.width)
	lines := renderWorkspaceSelectorLinesWithFilterView(
		m.itemLabel,
		m.status,
		m.title,
		m.action,
		m.candidates,
		m.selected,
		m.cursor,
		m.message,
		m.msgLevel,
		m.filter,
		filterInput.View(),
		m.showDesc,
		false,
		m.single,
		m.confirming,
		m.useColor,
		m.width,
		m.height,
	)
	return strings.Join(lines, "\n")
}

func selectorSingleConfirmCmd() tea.Cmd {
	return tea.Tick(selectorSingleConfirmDelay, func(time.Time) tea.Msg {
		return selectorConfirmDoneMsg{}
	})
}

func (m *workspaceSelectorModel) toggleCurrentSelection() {
	visible := m.filteredIndices()
	if len(visible) == 0 {
		m.message = "no workspaces match current filter"
		m.msgLevel = selectorMessageLevelError
		return
	}
	idx := visible[m.cursor]
	m.selected[idx] = !m.selected[idx]
	m.clearMessage()
	m.debugf("selector toggle idx=%d selected=%t", idx, m.selected[idx])
}

func (m *workspaceSelectorModel) clearMessage() {
	m.message = ""
	m.msgLevel = selectorMessageLevelMuted
}

func (m *workspaceSelectorModel) setErrorMessage(message string) {
	m.message = message
	m.msgLevel = selectorMessageLevelError
}

func (m *workspaceSelectorModel) setFilter(filter string) {
	m.filter = filter
	m.filterInput.SetValue(filter)
	m.filterInput.CursorEnd()
}

func (m *workspaceSelectorModel) deleteFilterRuneBeforeCursor() bool {
	filterRunes := []rune(m.filterInput.Value())
	if len(filterRunes) == 0 {
		return false
	}
	pos := m.filterInput.Position()
	if pos <= 0 {
		return false
	}
	if pos > len(filterRunes) {
		pos = len(filterRunes)
	}
	next := append(filterRunes[:pos-1], filterRunes[pos:]...)
	m.filter = string(next)
	m.filterInput.SetValue(m.filter)
	m.filterInput.SetCursor(pos - 1)
	return true
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
	return c.promptWorkspaceSelectorWithOptions(status, action, "", "workspace", candidates)
}

func (c *CLI) promptWorkspaceSelectorSingle(status string, action string, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptionsAndMode(status, action, "", "workspace", candidates, true)
}

func (c *CLI) promptWorkspaceSelectorWithOptions(status string, action string, title string, itemLabel string, candidates []workspaceSelectorCandidate) ([]string, error) {
	return c.promptWorkspaceSelectorWithOptionsAndMode(status, action, title, itemLabel, candidates, false)
}

func (c *CLI) promptWorkspaceSelectorWithOptionsAndMode(status string, action string, title string, itemLabel string, candidates []workspaceSelectorCandidate, single bool) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}

	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		return nil, fmt.Errorf("interactive workspace selection requires a TTY")
	}

	useColor := writerSupportsColor(c.Err)
	return runWorkspaceSelectorWithOptionsAndMode(inFile, c.Err, status, action, title, itemLabel, candidates, single, useColor, c.debugf)
}

func runWorkspaceSelector(in *os.File, out io.Writer, status string, action string, candidates []workspaceSelectorCandidate, useColor bool, debugf func(string, ...any)) ([]string, error) {
	return runWorkspaceSelectorWithOptions(in, out, status, action, "", "workspace", candidates, useColor, debugf)
}

func runWorkspaceSelectorWithOptions(in *os.File, out io.Writer, status string, action string, title string, itemLabel string, candidates []workspaceSelectorCandidate, useColor bool, debugf func(string, ...any)) ([]string, error) {
	return runWorkspaceSelectorWithOptionsAndMode(in, out, status, action, title, itemLabel, candidates, false, useColor, debugf)
}

func runWorkspaceSelectorWithOptionsAndMode(in *os.File, out io.Writer, status string, action string, title string, itemLabel string, candidates []workspaceSelectorCandidate, single bool, useColor bool, debugf func(string, ...any)) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}
	if strings.TrimSpace(status) == "" {
		status = "active"
	}

	model := newWorkspaceSelectorModelWithOptionsAndMode(candidates, status, action, title, itemLabel, single, useColor, debugf)
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
	return renderWorkspaceSelectorLinesWithOptions(status, "", action, candidates, selected, cursor, message, selectorMessageLevelMuted, filter, true, showCaret, false, false, useColor, termWidth)
}

func renderWorkspaceSelectorLinesWithOptions(status string, title string, action string, candidates []workspaceSelectorCandidate, selected map[int]bool, cursor int, message string, msgLevel selectorMessageLevel, filter string, showDesc bool, showCaret bool, single bool, confirming bool, useColor bool, termWidth int) []string {
	return renderWorkspaceSelectorLinesWithFilterView("", status, title, action, candidates, selected, cursor, message, msgLevel, filter, filter, showDesc, showCaret, single, confirming, useColor, termWidth, 0)
}

func renderWorkspaceSelectorLinesWithFilterView(itemLabel string, status string, title string, action string, candidates []workspaceSelectorCandidate, selected map[int]bool, cursor int, message string, msgLevel selectorMessageLevel, filter string, filterView string, showDesc bool, showCaret bool, single bool, confirming bool, useColor bool, termWidth int, termHeight int) []string {
	idWidth := len("workspace")
	for _, it := range candidates {
		if n := len(it.ID); n > idWidth {
			idWidth = n
		}
	}
	if idWidth < 10 {
		idWidth = 10
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

	titleLine := renderWorkspacesTitle(status, useColor)
	if strings.TrimSpace(title) != "" {
		titleLine = styleBold(title, useColor)
	}
	if termWidth < 24 {
		termWidth = 24
	}
	maxCols := termWidth - 1
	footerRaw := renderSelectorFooterLine(selectedCount, len(candidates), action, single, maxCols)
	footer := styleMuted(footerRaw, useColor)
	compactTitle := !showDesc

	// Keep workspace/action rows stable, but let compact rows (repo/context etc.)
	// use terminal width so truncation happens only when the row truly overflows.
	if compactTitle {
		maxIDWidth := maxCols - displayWidth("○ ") - 2
		if maxIDWidth < 1 {
			maxIDWidth = 1
		}
		if idWidth > maxIDWidth {
			idWidth = maxIDWidth
		}
	} else if idWidth > 24 {
		idWidth = 24
	}

	rowStyle := selectorRowStyleFromContext(itemLabel, title, showDesc)
	bodyRows := selectorBodyRowsLimit(termHeight, compactTitle)
	start, end := selectorViewportRange(totalVisible, cursor, bodyRows)

	bodyLines := make([]string, 0, len(candidates))
	if totalVisible == 0 {
		empty := uiIndent + "(no matches)"
		if useColor {
			empty = styleMuted(empty, true)
		}
		bodyLines = append(bodyLines, empty)
	}
	for visiblePos := start; visiblePos < end; visiblePos++ {
		sourceIdx := visible[visiblePos]
		it := candidates[sourceIdx]
		focus := " "
		if visiblePos == cursor {
			focus = ">"
		}

		idRaw := truncateDisplay(it.ID, idWidth)
		idPlain := fmt.Sprintf("%-*s", idWidth, idRaw)
		prefixPlain := idPlain + "  "

		idText := colorizeRiskID(idPlain, it.Risk, useColor)
		if compactTitle {
			// Repo selection keeps IDs visually stable: plain/bold without risk hue.
			idText = styleBold(idPlain, useColor)
		}
		prefix := idText + "  "
		mark := "○"
		if selected[sourceIdx] {
			mark = "●"
		}
		prefixPlain = fmt.Sprintf("%s %s  ", mark, idPlain)
		markerText := mark
		if useColor && selected[sourceIdx] {
			markerText = styleAccent(mark, true)
		}
		prefix = fmt.Sprintf("%s %s  ", markerText, idText)
		bodyRaw := prefix
		if showDesc {
			switch rowStyle {
			case selectorRowStyleWorkspace:
				bodyRaw, prefixPlain = renderWorkspaceSelectorRow(markerText, mark, idRaw, it, maxCols, useColor)
			case selectorRowStyleAction:
				bodyRaw, prefixPlain = renderActionSelectorRow(markerText, mark, idRaw, it, idWidth, maxCols, useColor)
			default:
				desc := it.secondaryText()
				if desc == "" {
					desc = "(no title)"
				}
				availableDescCols := maxCols - displayWidth(prefixPlain) - 2 // include focus + space
				if availableDescCols < 8 {
					availableDescCols = 8
				}
				bodyRaw = prefix + truncateDisplay(desc, availableDescCols)
			}
		}

		lineRaw := focus + " " + bodyRaw
		lineRaw = truncateDisplay(lineRaw, maxCols)

		line := lineRaw
		if useColor {
			bodyStyled := bodyRaw
			if confirming && !selected[sourceIdx] {
				bodyStyled = styleMuted(bodyRaw, true)
			}
			if !single && selected[sourceIdx] {
				bodyStyled = lipgloss.NewStyle().Bold(true).Render(bodyRaw)
			}
			if visiblePos == cursor {
				// Keep cursor emphasis visible but subtle across light/dark terminal themes.
				// Strip inline ANSI first so background highlight spans the full row text,
				// including title segments that might otherwise reset attributes.
				if rowStyle == selectorRowStyleWorkspace {
					bodyStyled = stripANSISequences(bodyStyled)
				}
				bodyStyled = lipgloss.NewStyle().
					Background(lipgloss.AdaptiveColor{Light: "252", Dark: "236"}).
					Render(bodyStyled)
				focusAccent := styleBold(styleTokenize(">", tokenFocus, true), true) + " "
				line = focusAccent + bodyStyled
			} else {
				line = focus + " " + bodyStyled
			}
		}
		bodyLines = append(bodyLines, line)
	}

	sectionBody := make([]string, 0, len(bodyLines)+4)
	sectionBody = append(sectionBody, bodyLines...)
	sectionBody = append(sectionBody, "")
	if useColor {
		prefix := styleMuted(fmt.Sprintf("%sfilter: ", uiIndent), true)
		sectionBody = append(sectionBody, prefix+filterView)
	} else {
		availableFilterCols := maxCols - displayWidth(uiIndent+"filter: ") - 1
		if availableFilterCols < 1 {
			availableFilterCols = 1
		}
		filterBody := truncateDisplay(filterView, availableFilterCols)
		sectionBody = append(sectionBody, fmt.Sprintf("%sfilter: %s", uiIndent, filterBody))
	}
	sectionBody = append(sectionBody, footer)

	if strings.TrimSpace(message) == "" {
		sectionBody = append(sectionBody, "")
	} else {
		msgCols := maxCols - displayWidth(uiIndent)
		if msgCols < 1 {
			msgCols = 1
		}
		msgLine := uiIndent + truncateDisplay(message, msgCols)
		if useColor && msgLevel == selectorMessageLevelError {
			sectionBody = append(sectionBody, styleError(msgLine, true))
		} else if useColor {
			sectionBody = append(sectionBody, styleMuted(msgLine, true))
		} else {
			sectionBody = append(sectionBody, msgLine)
		}
	}

	lines := make([]string, 0, len(candidates)+8)
	lines = appendSectionLines(lines, titleLine, sectionBody, sectionRenderOptions{
		blankAfterHeading: !compactTitle,
		trailingBlank:     true,
	})
	return lines
}

func selectorBodyRowsLimit(termHeight int, compactTitle bool) int {
	if termHeight <= 0 {
		return 0
	}
	chromeRows := 1 // title
	if !compactTitle {
		chromeRows++ // blank after title
	}
	chromeRows += 1 // blank before filter
	chromeRows += 1 // filter line
	chromeRows += 1 // footer line
	chromeRows += 1 // message/blank line
	bodyRows := termHeight - chromeRows
	if bodyRows < 1 {
		return 1
	}
	return bodyRows
}

func selectorViewportRange(total int, cursor int, bodyRows int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if bodyRows <= 0 || total <= bodyRows {
		return 0, total
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	start := cursor - bodyRows/2
	if start < 0 {
		start = 0
	}
	end := start + bodyRows
	if end > total {
		end = total
		start = end - bodyRows
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

type selectorRowStyle string

const (
	selectorRowStyleDefault   selectorRowStyle = "default"
	selectorRowStyleWorkspace selectorRowStyle = "workspace"
	selectorRowStyleAction    selectorRowStyle = "action"
)

func selectorRowStyleFromContext(itemLabel string, title string, showDesc bool) selectorRowStyle {
	if !showDesc {
		return selectorRowStyleDefault
	}
	label := strings.ToLower(strings.TrimSpace(itemLabel))
	switch label {
	case "workspace":
		return selectorRowStyleWorkspace
	case "action":
		return selectorRowStyleAction
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), "action:") {
		return selectorRowStyleAction
	}
	if strings.TrimSpace(title) == "" {
		return selectorRowStyleWorkspace
	}
	return selectorRowStyleDefault
}

func renderWorkspaceSelectorRow(markerText string, mark string, idRaw string, it workspaceSelectorCandidate, maxCols int, useColor bool) (string, string) {
	desc := it.secondaryText()
	emptyTitle := false
	if desc == "" {
		desc = "(no title)"
		emptyTitle = true
	}
	separatorPlain := ": "
	prefixPlain := fmt.Sprintf("%s %s%s", mark, idRaw, separatorPlain)
	availableDescCols := maxCols - displayWidth(prefixPlain) - 2 // include focus + space
	if availableDescCols < 8 {
		availableDescCols = 8
	}
	descText := truncateDisplay(desc, availableDescCols)
	descStyled := descText
	separatorStyled := separatorPlain
	idStyled := styleBold(colorizeRiskID(idRaw, it.Risk, useColor), useColor)
	if useColor {
		separatorStyled = styleMuted(separatorPlain, true)
		if emptyTitle {
			descStyled = styleMuted(descText, true)
		}
	}
	return fmt.Sprintf("%s %s%s%s", markerText, idStyled, separatorStyled, descStyled), prefixPlain
}

func renderActionSelectorRow(markerText string, mark string, actionRaw string, it workspaceSelectorCandidate, idWidth int, maxCols int, useColor bool) (string, string) {
	actionPadded := fmt.Sprintf("%-*s", idWidth, actionRaw)
	prefixPlain := fmt.Sprintf("%s %s  ", mark, actionPadded)
	desc := it.secondaryText()
	if desc == "" {
		desc = "no description"
	}
	suffixPlain := fmt.Sprintf("(%s)", desc)
	availableSuffixCols := maxCols - displayWidth(prefixPlain) - 2 // include focus + space
	if availableSuffixCols < 6 {
		availableSuffixCols = 6
	}
	suffixText := truncateDisplay(suffixPlain, availableSuffixCols)
	suffixStyled := suffixText
	if useColor {
		suffixStyled = styleMuted(suffixText, true)
	}
	actionStyled := colorizeRiskID(actionPadded, it.Risk, useColor)
	return fmt.Sprintf("%s %s  %s", markerText, actionStyled, suffixStyled), prefixPlain
}

func isReducedMotionEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GIONX_REDUCED_MOTION"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func selectorFilterWidth(termWidth int) int {
	if termWidth < 24 {
		termWidth = 24
	}
	maxCols := termWidth - 1
	availableFilterCols := maxCols - displayWidth(uiIndent+"filter: ") - 1
	if availableFilterCols < 1 {
		availableFilterCols = 1
	}
	return availableFilterCols
}

func renderSelectorFooterLine(selectedCount int, total int, action string, single bool, maxCols int) string {
	if single {
		base := uiIndent + "↑↓ move"
		parts := []string{
			fmt.Sprintf("enter %s", action),
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
	out := make([]int, 0, len(candidates))
	for i, c := range candidates {
		if fuzzyFilterMatch(c.ID, filter) || fuzzyFilterMatch(c.secondaryText(), filter) {
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

func stripANSISequences(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inEscape := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !inEscape {
			if ch == 0x1b && i+1 < len(s) && s[i+1] == '[' {
				inEscape = true
				i++
				continue
			}
			b.WriteByte(ch)
			continue
		}
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			inEscape = false
		}
	}
	return b.String()
}
