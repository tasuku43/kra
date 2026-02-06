package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-isatty"
	"github.com/tasuku43/gion-core/workspacerisk"
	"golang.org/x/term"
)

var errSelectorCanceled = errors.New("selector canceled")

type closeSelectorCandidate struct {
	ID          string
	Description string
	Risk        workspacerisk.WorkspaceRisk
}

func (c *CLI) promptWorkspaceCloseSelector(candidates []closeSelectorCandidate) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}

	inFile, ok := c.In.(*os.File)
	if !ok || !isatty.IsTerminal(inFile.Fd()) {
		return nil, fmt.Errorf("interactive workspace selection requires a TTY")
	}

	useColor := false
	if outFile, ok := c.Err.(*os.File); ok && isatty.IsTerminal(outFile.Fd()) {
		useColor = true
	}

	return runCloseSelector(inFile, c.Err, candidates, useColor)
}

func runCloseSelector(in *os.File, out io.Writer, candidates []closeSelectorCandidate, useColor bool) ([]string, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no workspace candidates")
	}

	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = term.Restore(int(in.Fd()), state) }()

	fmt.Fprint(out, "\x1b[?25l")
	defer fmt.Fprint(out, "\x1b[?25h")

	selected := make([]bool, len(candidates))
	cursor := 0
	message := ""
	prevLines := 0

	render := func() {
		width, _, sizeErr := term.GetSize(int(in.Fd()))
		if sizeErr != nil || width <= 0 {
			width = 80
		}
		lines := renderCloseSelectorLines(candidates, selected, cursor, message, useColor, width)
		if prevLines > 0 {
			fmt.Fprintf(out, "\x1b[%dA", prevLines)
		}
		for _, line := range lines {
			fmt.Fprintf(out, "\r\x1b[2K%s\n", line)
		}
		prevLines = len(lines)
	}

	render()
	reader := bufio.NewReader(in)

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}

		switch b {
		case 0x03: // Ctrl+C
			return nil, errSelectorCanceled
		case 0x04: // Ctrl+D
			ids := make([]string, 0, len(candidates))
			for i, picked := range selected {
				if picked {
					ids = append(ids, candidates[i].ID)
				}
			}
			if len(ids) == 0 {
				message = "at least one workspace must be selected"
				render()
				continue
			}
			return ids, nil
		case '\r', '\n':
			selected[cursor] = !selected[cursor]
			message = ""
			render()
		case 'j', 'J':
			if cursor < len(candidates)-1 {
				cursor++
			}
			message = ""
			render()
		case 'k', 'K':
			if cursor > 0 {
				cursor--
			}
			message = ""
			render()
		case 0x1b: // ESC sequence (arrow keys)
			b2, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			if b2 != '[' {
				continue
			}
			b3, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			switch b3 {
			case 'A':
				if cursor > 0 {
					cursor--
				}
				message = ""
				render()
			case 'B':
				if cursor < len(candidates)-1 {
					cursor++
				}
				message = ""
				render()
			}
		}
	}
}

func renderCloseSelectorLines(candidates []closeSelectorCandidate, selected []bool, cursor int, message string, useColor bool, termWidth int) []string {
	idWidth := len("workspace")
	riskWidth := len("[unknown]")
	for _, it := range candidates {
		if n := len(it.ID); n > idWidth {
			idWidth = n
		}
	}

	selectedCount := 0
	cleanCount := 0
	warningCount := 0
	dangerCount := 0
	for i, it := range candidates {
		if selected[i] {
			selectedCount++
		}
		switch it.Risk {
		case workspacerisk.WorkspaceRiskClean:
			cleanCount++
		case workspacerisk.WorkspaceRiskUnpushed, workspacerisk.WorkspaceRiskDiverged:
			warningCount++
		case workspacerisk.WorkspaceRiskDirty, workspacerisk.WorkspaceRiskUnknown:
			dangerCount++
		default:
			dangerCount++
		}
	}

	title := "ws close  | scope: active"
	hintsRaw := "Enter: toggle  Ctrl+D: confirm  Ctrl+C: cancel"
	statusRaw := fmt.Sprintf("selected: %d/%d  clean: %d  warning: %d  danger: %d", selectedCount, len(candidates), cleanCount, warningCount, dangerCount)
	hints := hintsRaw
	statusLine := statusRaw
	if useColor {
		statusLine = "\x1b[90m" + statusLine + "\x1b[0m"
		hints = "\x1b[90m" + hints + "\x1b[0m"
	}

	if termWidth < 48 {
		termWidth = 48
	}
	maxCols := termWidth - 1

	bodyLines := make([]string, 0, len(candidates))
	maxWidth := len(title)
	for i, it := range candidates {
		focus := " "
		if i == cursor {
			focus = ">"
		}
		mark := "○"
		if selected[i] {
			mark = "✔︎"
		}
		risk := "[" + string(it.Risk) + "]"
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			desc = "(no description)"
		}
		prefix := fmt.Sprintf("%s %-*s %-*s - ", mark, idWidth, it.ID, riskWidth, risk)
		availableDescCols := maxCols - displayWidth(prefix) - 2 // include focus + space
		if availableDescCols < 8 {
			availableDescCols = 8
		}
		desc = truncateDisplay(desc, availableDescCols)

		bodyRaw := prefix + desc
		bodyStyled := bodyRaw
		if useColor && !selected[i] {
			coloredRisk := colorizeRiskTag(risk, it.Risk)
			bodyStyled = fmt.Sprintf("%s %-*s %-*s - %s", mark, idWidth, it.ID, riskWidth, coloredRisk, desc)
		}
		lineRaw := focus + " " + bodyRaw
		lineRaw = truncateDisplay(lineRaw, maxCols)
		if len(lineRaw) > maxWidth {
			maxWidth = len(lineRaw)
		}

		line := lineRaw
		if useColor {
			if i == cursor {
				focusAccent := "\x1b[96;1m>\x1b[0m "
				if selected[i] {
					// Keep selected rows muted, but keep cursor visibility with accent marker.
					line = focusAccent + "\x1b[90m" + bodyRaw + "\x1b[0m"
				} else {
					// Subtle focus emphasis (fzf-like): accent marker + risk-tag color.
					line = focusAccent + bodyStyled
				}
			} else if selected[i] {
				line = "\x1b[90m" + line + "\x1b[0m"
			} else {
				line = focus + " " + bodyStyled
			}
		}
		bodyLines = append(bodyLines, line)
	}
	title = truncateDisplay(title, maxCols)
	hintsRaw = truncateDisplay(hintsRaw, maxCols)
	statusRaw = truncateDisplay(statusRaw, maxCols)

	if len(statusRaw) > maxWidth {
		maxWidth = len(statusRaw)
	}
	if len(hintsRaw) > maxWidth {
		maxWidth = len(hintsRaw)
	}

	sepWidth := maxWidth
	if sepWidth < 44 {
		sepWidth = 44
	}
	if sepWidth > maxCols {
		sepWidth = maxCols
	}
	sep := strings.Repeat("─", sepWidth)
	if useColor {
		sep = "\x1b[90m" + sep + "\x1b[0m"
	}

	lines := make([]string, 0, len(candidates)+8)
	lines = append(lines, title)
	lines = append(lines, sep)
	lines = append(lines, bodyLines...)
	lines = append(lines, sep)
	lines = append(lines, statusLine)
	lines = append(lines, hints)

	if strings.TrimSpace(message) == "" {
		lines = append(lines, "")
	} else if useColor {
		lines = append(lines, "\x1b[90m"+truncateDisplay(message, maxCols)+"\x1b[0m")
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

func colorizeRiskTag(tag string, risk workspacerisk.WorkspaceRisk) string {
	switch risk {
	case workspacerisk.WorkspaceRiskUnpushed, workspacerisk.WorkspaceRiskDiverged:
		return "\x1b[33m" + tag + "\x1b[0m"
	case workspacerisk.WorkspaceRiskDirty, workspacerisk.WorkspaceRiskUnknown:
		return "\x1b[31m" + tag + "\x1b[0m"
	default:
		return tag
	}
}
