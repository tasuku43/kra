package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/gionx/internal/testutil"
)

func TestCLI_WS_Go_Help_ShowsUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "go", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "gionx ws go") {
		t.Fatalf("stdout missing ws go usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Go_DirectActive_PrintsDestination(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "go", "WS1"})
	if code != exitOK {
		t.Fatalf("ws go exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	want := filepath.Join(env.Root, "workspaces", "WS1")
	wantOut := "cd '" + want + "'\n"
	if out.String() != wantOut {
		t.Fatalf("stdout mismatch: got=%q want=%q", out.String(), wantOut)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Go_DirectArchived_PrintsDestination(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}
	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "go", "--archived", "WS1"})
	if code != exitOK {
		t.Fatalf("ws go --archived exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	want := filepath.Join(env.Root, "archive", "WS1")
	wantOut := "cd '" + want + "'\n"
	if out.String() != wantOut {
		t.Fatalf("stdout mismatch: got=%q want=%q", out.String(), wantOut)
	}
}

func TestCLI_WS_Go_EmitCD_PrintsShellSnippetOnly(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "go", "--emit-cd", "WS1"})
	if code != exitOK {
		t.Fatalf("ws go --emit-cd exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	want := "cd '" + filepath.Join(env.Root, "workspaces", "WS1") + "'\n"
	if out.String() != want {
		t.Fatalf("stdout = %q, want %q", out.String(), want)
	}
}

func TestCLI_WS_Go_UI_PrintsResultSection(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "go", "--ui", "WS1"})
	if code != exitOK {
		t.Fatalf("ws go --ui exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	want := filepath.Join(env.Root, "workspaces", "WS1")
	if !strings.Contains(out.String(), "Result:") || !strings.Contains(out.String(), "Destination: "+want) {
		t.Fatalf("stdout missing ui result: %q", out.String())
	}
	if strings.Contains(out.String(), "cd '") {
		t.Fatalf("ui mode should not print cd snippet: %q", out.String())
	}
}

func TestCLI_WS_Go_UIAndEmitCD_Fails(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "go", "--ui", "--emit-cd", "WS1"})
	if code != exitUsage {
		t.Fatalf("ws go --ui --emit-cd exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "cannot be used together") {
		t.Fatalf("stderr missing conflict error: %q", err.String())
	}
}

func TestCLI_WS_Go_ArchivedScopeRejectsActiveWorkspace(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "go", "--archived", "WS1"})
	if code != exitError {
		t.Fatalf("ws go --archived exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "workspace is not archived") {
		t.Fatalf("stderr missing scope mismatch: %q", err.String())
	}
}

func TestCLI_WS_Go_SelectorModeWithoutTTY_Errors(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "create", "--no-prompt", "WS1"})
		if code != exitOK {
			t.Fatalf("ws create exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")

		code := c.Run([]string{"ws", "go"})
		if code != exitUsage {
			t.Fatalf("ws go exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
		}
		if !strings.Contains(err.String(), "ws go requires --id <id> or positional <id>") {
			t.Fatalf("stderr missing id requirement: %q", err.String())
		}
	}
}
