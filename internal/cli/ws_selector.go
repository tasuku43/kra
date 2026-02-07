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

	useColor := writerSupportsColor(c.Err)

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
		case ' ': // Space
			selected[cursor] = !selected[cursor]
			message = ""
			render()
		case '\r', '\n': // Enter -> proceed
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
			if reader.Buffered() == 0 {
				return nil, errSelectorCanceled
			}
			b2, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			if b2 != '[' {
				return nil, errSelectorCanceled
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
	for i := range candidates {
		if selected[i] {
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
				bodyStyled = styleBold(bodyStyled, true)
			}
			if i == cursor {
				focusAccent := "\x1b[96;1m>\x1b[0m "
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
