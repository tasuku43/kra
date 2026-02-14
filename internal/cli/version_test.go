package cli

import (
	"bytes"
	"testing"
)

func TestCLI_Version_DefaultDev(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"version"})
	if code != exitOK {
		t.Fatalf("version exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if got, want := out.String(), "dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestCLI_Version_IncludesBuildMetadataWhenSet(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.Version = "v0.1.0"
	c.Commit = "abc1234"
	c.Date = "2026-02-14T00:00:00Z"

	code := c.Run([]string{"version"})
	if code != exitOK {
		t.Fatalf("version exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if got, want := out.String(), "v0.1.0 abc1234 2026-02-14T00:00:00Z\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
