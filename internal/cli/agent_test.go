package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLI_Agent_Help(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "--help"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra agent") {
		t.Fatalf("stdout missing usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_Agent_Prompt_Text(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "prompt"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "kra agent prompt") {
		t.Fatalf("stdout missing heading: %q", text)
	}
	if !strings.Contains(text, "kra help ws close --mode agent") {
		t.Fatalf("stdout missing drill-down command: %q", text)
	}
}

func TestCLI_Agent_Prompt_JSON(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "prompt", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	var payload cliJSONResponse
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if !payload.OK || payload.Action != "agent.prompt" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	result, ok := payload.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", payload.Result)
	}
	drillDown, ok := result["drill_down_commands"].([]any)
	if !ok || len(drillDown) == 0 {
		t.Fatalf("drill_down_commands missing or empty: %#v", result["drill_down_commands"])
	}
}

func TestCLI_Agent_Prompt_Brief_Text(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "prompt", "--brief"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if strings.Contains(text, "Recommended workflow:") {
		t.Fatalf("brief stdout should omit verbose sections: %q", text)
	}
	if !strings.Contains(text, "Automation: prefer --id over --current/--select, --format json where supported.") {
		t.Fatalf("brief stdout missing automation summary: %q", text)
	}
	if !strings.Contains(text, "Drill down: kra help <command-path> --mode agent") {
		t.Fatalf("brief stdout missing drill-down summary: %q", text)
	}
}

func TestCLI_Agent_Prompt_Brief_JSON(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"agent", "prompt", "--brief", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	var payload cliJSONResponse
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	result, ok := payload.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", payload.Result)
	}
	workflow, ok := result["recommended_workflow"].([]any)
	if ok && len(workflow) != 0 {
		t.Fatalf("brief json should omit recommended_workflow: %#v", workflow)
	}
	what, ok := result["what_kra_is"].([]any)
	if !ok || len(what) != 1 {
		t.Fatalf("brief json what_kra_is unexpected: %#v", result["what_kra_is"])
	}
}
