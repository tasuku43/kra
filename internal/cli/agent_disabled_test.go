//go:build !experimental

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLI_Agent_Disabled_ReturnsUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "list"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), `unknown command: "agent"`) {
		t.Fatalf("stderr should include unknown command: %q", err.String())
	}
}
