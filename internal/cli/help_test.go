package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLI_Help_NoTarget_ShowsRootUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("stdout missing root usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_Help_Human_DelegatesToUsage(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"help", "ws", "close"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws close") {
		t.Fatalf("stdout missing delegated usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_Help_Agent_Text(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"help", "ws", "close", "--mode", "agent"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "Command: kra ws close") {
		t.Fatalf("stdout missing command header: %q", text)
	}
	if !strings.Contains(text, "recommended_invocation: kra ws close --id <id> --force --format json") {
		t.Fatalf("stdout missing recommended invocation: %q", text)
	}
	if !strings.Contains(text, "avoid_flags_in_automation:") {
		t.Fatalf("stdout missing automation section: %q", text)
	}
}

func TestCLI_Help_Agent_JSON(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"help", "init", "--mode", "agent", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	var payload cliJSONResponse
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if !payload.OK || payload.Action != "help" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	result, ok := payload.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", payload.Result)
	}
	if result["mode"] != "agent" {
		t.Fatalf("mode = %v, want agent", result["mode"])
	}
	spec, ok := result["spec"].(map[string]any)
	if !ok {
		t.Fatalf("spec type = %T, want map[string]any", result["spec"])
	}
	if spec["command"] != "kra init" {
		t.Fatalf("command = %v, want kra init", spec["command"])
	}
}

func TestCLI_Help_UnknownTarget(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"help", "ws", "unknown", "--mode", "agent"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "unknown help target") {
		t.Fatalf("stderr missing unknown target message: %q", err.String())
	}
}
