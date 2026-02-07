package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiBlack  = "\x1b[30m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiMuted  = "\x1b[90m"

	// Backward-compatible aliases for existing tests/callers.
	ansiAccent = ansiCyan
	ansiWarn   = ansiYellow
	ansiError  = ansiRed

	uiIndent = "  "
)

type styleToken string

const (
	tokenTextPrimary styleToken = "text.primary"
	tokenTextMuted   styleToken = "text.muted"
	tokenAccent      styleToken = "accent"

	tokenStatusSuccess styleToken = "status.success"
	tokenStatusWarning styleToken = "status.warning"
	tokenStatusError   styleToken = "status.error"
	tokenStatusInfo    styleToken = "status.info"

	tokenFocus     styleToken = "focus"
	tokenSelection styleToken = "selection"

	tokenDiffAdd    styleToken = "diff.add"
	tokenDiffRemove styleToken = "diff.remove"
)

func tokenANSI(token styleToken) string {
	switch token {
	case tokenTextPrimary:
		return ""
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

func styleMuted(text string, useColor bool) string {
	return styleTokenize(text, tokenTextMuted, useColor)
}

func styleAccent(text string, useColor bool) string {
	return styleTokenize(text, tokenAccent, useColor)
}

func styleWarn(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusWarning, useColor)
}

func styleError(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusError, useColor)
}

func styleSuccess(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusSuccess, useColor)
}

func styleInfo(text string, useColor bool) string {
	return styleTokenize(text, tokenStatusInfo, useColor)
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
	fmt.Fprintln(out)
	fmt.Fprintln(out, renderResultTitle(useColor))
	for _, line := range lines {
		fmt.Fprintf(out, "%s%s\n", uiIndent, line)
	}
}
