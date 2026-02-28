package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLI_RootUsage_IncludesCMUXCommand(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "cmux") {
		t.Fatalf("root usage should include cmux command: %q", out.String())
	}
}

func TestCLI_CMUX_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"cmux", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra cmux <subcommand>") {
		t.Fatalf("stdout missing cmux usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_CMUX_NoArgs_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"cmux"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "kra cmux <subcommand>") {
		t.Fatalf("stderr missing cmux usage: %q", err.String())
	}
}

func TestCLI_CMUX_UnknownSubcommand_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"cmux", "nope"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", out.String())
	}
	if !strings.Contains(err.String(), `unknown command: "cmux nope"`) {
		t.Fatalf("stderr missing unknown command: %q", err.String())
	}
	if !strings.Contains(err.String(), "kra cmux <subcommand>") {
		t.Fatalf("stderr missing usage: %q", err.String())
	}
}

func TestCLI_CMUX_SubcommandHelpRoutes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "open",
			args: []string{"cmux", "open", "--help"},
			want: "kra cmux open",
		},
		{
			name: "switch",
			args: []string{"cmux", "switch", "--help"},
			want: "kra cmux switch",
		},
		{
			name: "list",
			args: []string{"cmux", "list", "--help"},
			want: "kra cmux list",
		},
		{
			name: "status",
			args: []string{"cmux", "status", "--help"},
			want: "kra cmux status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var err bytes.Buffer
			c := New(&out, &err)

			code := c.Run(tc.args)
			if code != exitOK {
				t.Fatalf("exit code = %d, want %d", code, exitOK)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("stdout missing usage marker %q: %q", tc.want, out.String())
			}
			if err.Len() != 0 {
				t.Fatalf("stderr not empty: %q", err.String())
			}
		})
	}
}

func TestCLI_CMUX_Open_NoRoot_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"cmux", "open", "WS1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "cmux open (WS1): resolve KRA_ROOT:") {
		t.Fatalf("stderr missing root resolution error: %q", err.String())
	}
}

func TestCLI_CMUX_List_NoRoot_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"cmux", "list"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "cmux list: resolve KRA_ROOT:") {
		t.Fatalf("stderr missing root resolution error: %q", err.String())
	}
}

func TestCLI_CMUX_Status_NoRoot_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"cmux", "status"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "cmux status: resolve KRA_ROOT:") {
		t.Fatalf("stderr missing root resolution error: %q", err.String())
	}
}

func TestCLI_CMUX_Switch_NoRoot_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"cmux", "switch", "--workspace", "WS1", "--cmux", "cmux-1"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", out.String())
	}
	if !strings.Contains(err.String(), "cmux switch (WS1): resolve KRA_ROOT:") {
		t.Fatalf("stderr missing root resolution error: %q", err.String())
	}
}
