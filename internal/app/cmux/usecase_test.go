package cmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellQuoteCDPath_UsesHomeVariableWhenUnderHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		t.Skip("home dir is not available")
	}
	got := shellQuoteCDPath(filepath.Join(home, "work/root/workspaces/SREP-4084"))
	want := `"$HOME/work/root/workspaces/SREP-4084"`
	if got != want {
		t.Fatalf("shellQuoteCDPath() = %q, want %q", got, want)
	}
}
