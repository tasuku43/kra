package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLI_RootUsage_DoesNotIncludeCMUXCommand(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if strings.Contains(out.String(), "\n  cmux") {
		t.Fatalf("root usage should not include cmux command: %q", out.String())
	}
}

func TestCLI_WSOpen_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "open", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws open") {
		t.Fatalf("stdout missing ws open usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WSSwitch_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "switch", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws switch") {
		t.Fatalf("stdout missing ws switch usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WSOpen_NoRoot_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "open", "--id", "WS1"})
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

func TestCLI_WSSwitch_NoRoot_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "switch", "--id", "WS1"})
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

func TestCLI_WSSwitch_CMUXFlag_ReturnsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "switch", "--id", "WS1", "--cmux", "cmux-1"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--cmux is no longer supported") {
		t.Fatalf("stderr missing unsupported flag message: %q", err.String())
	}
}
