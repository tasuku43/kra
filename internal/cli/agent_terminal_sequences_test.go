package cli

import "testing"

func TestAgentTerminalSequenceParser_OSC9_BELTerminated(t *testing.T) {
	p := newAgentTerminalSequenceParser()
	ev := p.Feed([]byte("\x1b]9;done\x07"))
	if len(ev) != 1 {
		t.Fatalf("event count=%d, want=1", len(ev))
	}
	if ev[0].Name != "osc_9_notify" {
		t.Fatalf("event name=%q, want=osc_9_notify", ev[0].Name)
	}
	if ev[0].StateHint != "idle" {
		t.Fatalf("state hint=%q, want=idle", ev[0].StateHint)
	}
}

func TestAgentTerminalSequenceParser_OSC777_STTerminatedAcrossChunks(t *testing.T) {
	p := newAgentTerminalSequenceParser()
	if got := p.Feed([]byte("\x1b]777;notify;title")); len(got) != 0 {
		t.Fatalf("partial chunk should not emit events, got=%d", len(got))
	}
	ev := p.Feed([]byte(";body\x1b\\"))
	if len(ev) != 1 {
		t.Fatalf("event count=%d, want=1", len(ev))
	}
	if ev[0].Name != "osc_777_notify" {
		t.Fatalf("event name=%q, want=osc_777_notify", ev[0].Name)
	}
	if ev[0].StateHint != "idle" {
		t.Fatalf("state hint=%q, want=idle", ev[0].StateHint)
	}
}

func TestAgentTerminalSequenceParser_OSC133_C_And_D(t *testing.T) {
	p := newAgentTerminalSequenceParser()
	ev := p.Feed([]byte("\x1b]133;C\x07\x1b]133;D;0\x07"))
	if len(ev) != 2 {
		t.Fatalf("event count=%d, want=2", len(ev))
	}
	if ev[0].Name != "osc_133_c" || ev[0].StateHint != "running" {
		t.Fatalf("first event unexpected: %+v", ev[0])
	}
	if ev[1].Name != "osc_133_d" || ev[1].StateHint != "idle" {
		t.Fatalf("second event unexpected: %+v", ev[1])
	}
}

func TestApplyRuntimeStateHints_LastHintWins(t *testing.T) {
	state := applyRuntimeStateHints("running", []agentRuntimeSignalEvent{
		{Name: "osc_133_c", StateHint: "running"},
		{Name: "osc_9_notify", StateHint: "idle"},
	})
	if state != "idle" {
		t.Fatalf("state=%q, want=idle", state)
	}
}
