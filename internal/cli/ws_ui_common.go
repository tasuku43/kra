package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiBoldOff = "\x1b[22m"
	ansiFGReset = "\x1b[39m"
	ansiBlack   = "\x1b[30m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiCyan    = "\x1b[36m"
	ansiMuted   = "\x1b[90m"
	ansiBody    = "\x1b[38;5;250m"
	// 256-color accents for subdued git reference styling.
	ansiGitRefLocalMuted  = "\x1b[2;38;5;65m"
	ansiGitRefRemoteMuted = "\x1b[38;5;67m"
	ansiErrorSubtle       = "\x1b[2;38;5;174m"

	// Backward-compatible aliases for existing tests/callers.
	ansiAccent = ansiCyan
	ansiWarn   = ansiYellow
	ansiError  = ansiRed

	uiIndent = "  "
)

type styleToken string

const (
	tokenTextPrimary styleToken = "text.primary"
	tokenTextBody    styleToken = "text.body"
	tokenTextMuted   styleToken = "text.muted"
	tokenAccent      styleToken = "accent"

	tokenStatusSuccess     styleToken = "status.success"
	tokenStatusWarning     styleToken = "status.warning"
	tokenStatusError       styleToken = "status.error"
	tokenStatusErrorSubtle styleToken = "status.error.subtle"
	tokenStatusInfo        styleToken = "status.info"

	tokenFocus     styleToken = "focus"
	tokenSelection styleToken = "selection"

	tokenDiffAdd    styleToken = "diff.add"
	tokenDiffRemove styleToken = "diff.remove"

	tokenGitRefLocal  styleToken = "git.ref.local"
	tokenGitRefRemote styleToken = "git.ref.remote"
)

func tokenANSI(token styleToken) string {
	switch token {
	case tokenTextPrimary:
		return ""
	case tokenTextBody:
		return ansiBody
	case tokenTextMuted:
		return ansiMuted
	case tokenAccent:
		return ansiCyan
	case tokenStatusSuccess:
		return ansiGreen
	case tokenStatusWarning:
		return ansiYellow
	case tokenStatusError:
		return ansiRed
	case tokenStatusErrorSubtle:
		return ansiErrorSubtle
	case tokenStatusInfo:
		return ansiBlue
	case tokenFocus:
		return ansiCyan
	case tokenSelection:
		return ansiBlack
	case tokenDiffAdd:
		return ansiGreen
	case tokenDiffRemove:
		return ansiRed
	case tokenGitRefLocal:
		return ansiGitRefLocalMuted
	case tokenGitRefRemote:
		return ansiGitRefRemoteMuted
	default:
		return ""
	}
}

func writerSupportsColor(w io.Writer) bool {
	return writerIsTTY(w)
}

func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

func styleBold(text string, useColor bool) string {
	if !useColor {
		return text
	}
	return ansiBold + text + ansiReset
}

func styleBoldKeepBG(text string, useColor bool) string {
	if !useColor {
		return text
	}
	// Use bold on/off instead of full reset so outer background styling is preserved.
	return ansiBold + text + ansiBoldOff
}

func styleTokenize(text string, token styleToken, useColor bool) string {
	if !useColor {
		return text
	}
	ansi := tokenANSI(token)
	if ansi == "" {
		return text
	}
	return ansi + text + ansiReset
}

func styleTokenizeKeepBG(text string, token styleToken, useColor bool) string {
	if !useColor {
		return text
	}
	ansi := tokenANSI(token)
	if ansi == "" {
		return text
	}
	// Reset only foreground so outer background styling remains intact.
	return ansi + text + ansiFGReset
}

func styleMuted(text string, useColor bool) string {
	return styleTokenize(text, tokenTextMuted, useColor)
}

func styleBody(text string, useColor bool) string {
	return styleTokenize(text, tokenTextBody, useColor)
}

func styleMutedKeepBG(text string, useColor bool) string {
	return styleTokenizeKeepBG(text, tokenTextMuted, useColor)
}

func styleAccent(text string, useColor bool) string {
	return styleTokenize(text, tokenAccent, useColor)
}

func styleAccentKeepBG(text string, useColor bool) string {
	return styleTokenizeKeepBG(text, tokenAccent, useColor)
}

func styleWarn(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusWarning, useColor)
}

func styleError(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusError, useColor)
}

func styleErrorSubtle(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusErrorSubtle, useColor)
}

func styleSuccess(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusSuccess, useColor)
}

func styleInfo(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusInfo, useColor)
}

func styleGitRefLocal(text string, useColor bool) string {
	return styleTokenize(text, tokenGitRefLocal, useColor)
}

func styleGitRefRemote(text string, useColor bool) string {
	return styleTokenize(text, tokenGitRefRemote, useColor)
}

func styleGitStatusShortLine(line string, useColor bool) string {
	if !useColor {
		return line
	}
	if len(line) < 2 {
		return line
	}

	x := styleGitStatusShortChar(line[0], true, useColor)
	y := styleGitStatusShortChar(line[1], false, useColor)
	return x + y + line[2:]
}

func styleGitStatusShortChar(ch byte, staged bool, useColor bool) string {
	switch ch {
	case ' ':
		return " "
	case '?':
		return styleTokenize("?", tokenDiffRemove, useColor)
	default:
		if staged {
			return styleTokenize(string(ch), tokenDiffAdd, useColor)
		}
		return styleTokenize(string(ch), tokenDiffRemove, useColor)
	}
}

func renderWorkspaceStatusLabel(status string, useColor bool) string {
	if !useColor {
		return status
	}
	switch status {
	case "active":
		return styleAccent(status, useColor)
	case "archived":
		return styleMuted(status, useColor)
	default:
		return status
	}
}

func renderWorkspacesTitle(status string, useColor bool) string {
	label := fmt.Sprintf("Workspaces(%s):", renderWorkspaceStatusLabel(status, useColor))
	return styleBold(label, useColor)
}

func renderRiskTitle(useColor bool) string {
	return styleBold("Risk:", useColor)
}

func renderProgressTitle(useColor bool) string {
	return styleBold("Progress:", useColor)
}

func renderResultTitle(useColor bool) string {
	return styleBold("Result:", useColor)
}

func printResultSection(out io.Writer, useColor bool, lines ...string) {
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		body = append(body, fmt.Sprintf("%s%s", uiIndent, line))
	}
	fmt.Fprintln(out)
	printSection(out, renderResultTitle(useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

type sectionRenderOptions struct {
	blankAfterHeading bool
	trailingBlank     bool
}

type sectionAtom struct {
	heading string
	body    []string
	opts    sectionRenderOptions
}

func newSectionAtom(heading string, body []string, opts sectionRenderOptions) sectionAtom {
	return sectionAtom{
		heading: heading,
		body:    append([]string(nil), body...),
		opts:    opts,
	}
}

func appendSectionLines(lines []string, heading string, body []string, opts sectionRenderOptions) []string {
	section := make([]string, 0, 2+len(body))
	section = append(section, heading)
	if opts.blankAfterHeading {
		section = append(section, "")
	}
	section = append(section, body...)
	if opts.trailingBlank {
		for len(section) > 0 && strings.TrimSpace(section[len(section)-1]) == "" {
			section = section[:len(section)-1]
		}
		section = append(section, "")
	}
	return append(lines, section...)
}

func renderSectionAtoms(atoms ...sectionAtom) []string {
	lines := make([]string, 0, len(atoms)*4)
	for _, atom := range atoms {
		lines = appendSectionLines(lines, atom.heading, atom.body, atom.opts)
	}
	return lines
}

func printSection(out io.Writer, heading string, body []string, opts sectionRenderOptions) {
	lines := renderSectionAtoms(newSectionAtom(heading, body, opts))
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
}

func printSectionBoundary(out io.Writer) {
	fmt.Fprintln(out)
}
