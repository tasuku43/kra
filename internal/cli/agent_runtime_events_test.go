package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAgentRuntimeSignalEvents_WritesJSONL(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	record := agentRuntimeSessionRecord{
		SessionID: "s-1",
		RootPath:  root,
	}
	events := []agentRuntimeSignalEvent{
		{Name: "osc_9_notify", StateHint: "idle", Details: "done"},
		{Name: "bell"},
	}
	if err := appendAgentRuntimeSignalEvents(record, events); err != nil {
		t.Fatalf("append events: %v", err)
	}

	kraHome := strings.TrimSpace(os.Getenv("KRA_HOME"))
	if kraHome == "" {
		t.Fatalf("KRA_HOME should be set in tests")
	}
	path := filepath.Join(kraHome, "state", "agents", hashRootPath(root), "events", "s-1.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read event file: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, `"name":"osc_9_notify"`) {
		t.Fatalf("osc_9_notify event missing: %q", content)
	}
	if !strings.Contains(content, `"name":"bell"`) {
		t.Fatalf("bell event missing: %q", content)
	}
}
