package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestTokenANSI_KnownTokens(t *testing.T) {
	tests := []struct {
		name  string
		token styleToken
		want  string
	}{
		{name: "text.primary", token: tokenTextPrimary, want: ""},
		{name: "text.muted", token: tokenTextMuted, want: ansiMuted},
		{name: "accent", token: tokenAccent, want: ansiCyan},
		{name: "status.success", token: tokenStatusSuccess, want: ansiGreen},
		{name: "status.warning", token: tokenStatusWarning, want: ansiYellow},
		{name: "status.error", token: tokenStatusError, want: ansiRed},
		{name: "status.info", token: tokenStatusInfo, want: ansiBlue},
		{name: "focus", token: tokenFocus, want: ansiCyan},
		{name: "selection", token: tokenSelection, want: ansiBlack},
		{name: "diff.add", token: tokenDiffAdd, want: ansiGreen},
		{name: "diff.remove", token: tokenDiffRemove, want: ansiRed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenANSI(tt.token)
			if got != tt.want {
				t.Fatalf("tokenANSI(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

func TestStyleWarnAndInfo_Colorized(t *testing.T) {
	warn := styleWarn("warn", true)
	info := styleInfo("info", true)

	if !strings.Contains(warn, ansiYellow) || !strings.Contains(warn, ansiReset) {
		t.Fatalf("styleWarn should include ANSI yellow/reset, got %q", warn)
	}
	if !strings.Contains(info, ansiBlue) || !strings.Contains(info, ansiReset) {
		t.Fatalf("styleInfo should include ANSI blue/reset, got %q", info)
	}
}

func TestRenderWorkspaceStatusLabel_UnknownAndNoColor(t *testing.T) {
	if got := renderWorkspaceStatusLabel("unknown", true); got != "unknown" {
		t.Fatalf("unknown status should remain plain, got %q", got)
	}
	if got := renderWorkspaceStatusLabel("active", false); got != "active" {
		t.Fatalf("no-color active should stay plain, got %q", got)
	}
	if got := renderWorkspaceStatusLabel("archived", false); got != "archived" {
		t.Fatalf("no-color archived should stay plain, got %q", got)
	}
}

func TestPrintResultSection_UsesSharedIndent(t *testing.T) {
	var out bytes.Buffer
	printResultSection(&out, false, "line-1", "line-2")

	got := out.String()
	want := "\nResult:\n" + uiIndent + "line-1\n" + uiIndent + "line-2\n"
	if got != want {
		t.Fatalf("unexpected result section:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}
