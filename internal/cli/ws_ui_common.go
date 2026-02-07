package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiMuted = "\x1b[90m"
	ansiWarn  = "\x1b[33m"
	ansiError = "\x1b[31m"

	ansiStatusActive   = "\x1b[36m"
	ansiStatusArchived = "\x1b[35m"

	uiIndent = "    "
)

func writerSupportsColor(w io.Writer) bool {
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

func styleMuted(text string, useColor bool) string {
	if !useColor {
		return text
	}
	return ansiMuted + text + ansiReset
}

func styleWarn(text string, useColor bool) string {
	if !useColor {
		return text
	}
	return ansiWarn + text + ansiReset
}

func styleError(text string, useColor bool) string {
	if !useColor {
		return text
	}
	return ansiError + text + ansiReset
}

func renderWorkspaceStatusLabel(status string, useColor bool) string {
	if !useColor {
		return status
	}
	switch status {
	case "active":
		return ansiStatusActive + status + ansiReset
	case "archived":
		return ansiStatusArchived + status + ansiReset
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

func renderResultTitle(useColor bool) string {
	return styleBold("Result:", useColor)
}
