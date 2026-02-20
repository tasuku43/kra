package cli

import (
	"testing"
	"time"
)

func TestAgentBrokerSessionOutput_IdleOutputBecomesRunning(t *testing.T) {
	now := time.Unix(200, 0)
	session := &agentBrokerSession{
		record: agentRuntimeSessionRecord{
			SessionID:    "s-1",
			RuntimeState: "idle",
			UpdatedAt:    100,
			Seq:          1,
		},
		seqParser: newAgentTerminalSequenceParser(),
	}

	_, snapshot, _ := session.appendOutputAndSnapshotWritable([]byte("working..."), now)
	if snapshot == nil || snapshot.RuntimeState != "running" {
		got := "<nil>"
		if snapshot != nil {
			got = snapshot.RuntimeState
		}
		t.Fatalf("runtime should become running after output activity, got=%s", got)
	}
}

func TestAgentBrokerSessionOutput_DoesNotDropToIdleOnOutput(t *testing.T) {
	now := time.Unix(200, 0)
	session := &agentBrokerSession{
		record: agentRuntimeSessionRecord{
			SessionID:    "s-1",
			RuntimeState: "running",
			UpdatedAt:    100,
			Seq:          1,
		},
		seqParser: newAgentTerminalSequenceParser(),
	}

	_, snapshot, _ := session.appendOutputAndSnapshotWritable([]byte("\n› prompt\r\n"), now.Add(500*time.Millisecond))
	if snapshot != nil && snapshot.RuntimeState == "idle" {
		t.Fatalf("runtime should not turn idle while output is still arriving")
	}
}

func TestAgentBrokerSessionMarkIdleOnSilence_AfterOutput(t *testing.T) {
	now := time.Unix(300, 0)
	session := &agentBrokerSession{
		record: agentRuntimeSessionRecord{
			SessionID:    "s-1",
			RuntimeState: "running",
			UpdatedAt:    now.Unix() - int64(agentRuntimeIdleAfterSilence/time.Second) - 1,
			Seq:          2,
		},
		lastOutputAt: now.Add(-agentRuntimeIdleAfterSilence - time.Second),
	}

	snapshot := session.markIdleOnSilence(now)
	if snapshot == nil {
		t.Fatalf("snapshot should be emitted when running output is silent")
	}
	if snapshot.RuntimeState != "idle" {
		t.Fatalf("runtime_state=%q, want=idle", snapshot.RuntimeState)
	}
}

func TestAgentBrokerSessionOutput_OSC133D_BecomesWaitingInput(t *testing.T) {
	now := time.Unix(400, 0)
	session := &agentBrokerSession{
		record: agentRuntimeSessionRecord{
			SessionID:    "s-1",
			RuntimeState: "running",
			UpdatedAt:    100,
			Seq:          1,
		},
		seqParser: newAgentTerminalSequenceParser(),
	}

	_, snapshot, _ := session.appendOutputAndSnapshotWritable([]byte("\x1b]133;D;0\x07"), now)
	if snapshot == nil {
		t.Fatalf("snapshot should be emitted for state transition")
	}
	if snapshot.RuntimeState != "waiting_input" {
		t.Fatalf("runtime_state=%q, want=waiting_input", snapshot.RuntimeState)
	}
}

func TestAgentBrokerSessionMarkIdleOnSilence_DoesNotChangeWaitingInput(t *testing.T) {
	now := time.Unix(500, 0)
	session := &agentBrokerSession{
		record: agentRuntimeSessionRecord{
			SessionID:    "s-1",
			RuntimeState: "waiting_input",
			UpdatedAt:    100,
			Seq:          1,
		},
		lastOutputAt: now.Add(-agentRuntimeIdleAfterSilence - time.Second),
	}

	snapshot := session.markIdleOnSilence(now)
	if snapshot != nil {
		t.Fatalf("waiting_input should not be downgraded to idle by silence timeout")
	}
}

func TestAgentBrokerSessionAcquireControl_AllInteractiveClientsCanInput(t *testing.T) {
	session := &agentBrokerSession{
		attachments: map[string]*agentBrokerAttachment{
			"a-1": {clientID: "c-1"},
			"a-2": {clientID: "c-2"},
		},
	}

	inputOK1, resizeOK1 := session.acquireControl("c-1", "interactive")
	if !inputOK1 || resizeOK1 {
		t.Fatalf("first interactive client should be able to input without resize lease")
	}
	inputOK2, resizeOK2 := session.acquireControl("c-2", "interactive")
	if !inputOK2 || resizeOK2 {
		t.Fatalf("second interactive client should also be able to input without resize lease")
	}
}

func TestAgentBrokerSessionAcquireControl_SpectatorModeStillNoResizeLease(t *testing.T) {
	session := &agentBrokerSession{
		attachments: map[string]*agentBrokerAttachment{
			"a-1": {clientID: "c-1"},
		},
	}
	inputOK, resizeOK := session.acquireControl("c-1", "spectator")
	if !inputOK || resizeOK {
		t.Fatalf("spectator mode should keep input enabled and resize disabled")
	}
}

func TestAgentBrokerHandleResizeRequest_DeniedWhenResizeLeaseDisabled(t *testing.T) {
	session := &agentBrokerSession{
		record: agentRuntimeSessionRecord{SessionID: "s-1"},
		attachments: map[string]*agentBrokerAttachment{
			"a-1": {clientID: "c-owner"},
			"a-2": {clientID: "c-other"},
		},
	}
	server := &agentBrokerServer{
		sessions: map[string]*agentBrokerSession{"s-1": session},
	}

	resp := server.handleResizeRequest(agentBrokerRequest{
		SessionID: "s-1",
		ClientID:  "c-other",
		Cols:      120,
		Rows:      40,
	})
	if resp.OK {
		t.Fatalf("resize should be denied when resize lease is disabled")
	}
	if resp.Error != "resize lease denied" {
		t.Fatalf("error=%q, want=resize lease denied", resp.Error)
	}
}

func TestAgentBrokerSessionScreenSnapshot_TracksLatestLines(t *testing.T) {
	now := time.Unix(600, 0)
	session := &agentBrokerSession{
		record:    agentRuntimeSessionRecord{SessionID: "s-1", RuntimeState: "running"},
		seqParser: newAgentTerminalSequenceParser(),
	}

	payload := []byte("line1\nline2\nline3\n")
	_, _, _ = session.appendOutputAndSnapshotWritable(payload, now)
	seq, at, screen := session.screenSnapshot(2)
	if seq == 0 {
		t.Fatalf("screen seq should increase after output")
	}
	if at != now.Unix() {
		t.Fatalf("screen_at=%d, want=%d", at, now.Unix())
	}
	if screen != "line2\nline3" {
		t.Fatalf("screen=%q, want=%q", screen, "line2\nline3")
	}
}

func TestAgentBrokerHandleScreenSnapshotRequest(t *testing.T) {
	session := &agentBrokerSession{
		record:      agentRuntimeSessionRecord{SessionID: "s-1", RuntimeState: "running"},
		screenSeq:   3,
		screenAt:    1000,
		screenLines: []string{"a", "b", "c"},
	}
	server := &agentBrokerServer{
		sessions: map[string]*agentBrokerSession{"s-1": session},
	}

	resp := server.handleScreenSnapshotRequest(agentBrokerRequest{SessionID: "s-1", Rows: 2})
	if !resp.OK {
		t.Fatalf("screen snapshot should succeed: error=%q", resp.Error)
	}
	if resp.ScreenSeq != 3 || resp.ScreenAt != 1000 {
		t.Fatalf("unexpected screen metadata: seq=%d at=%d", resp.ScreenSeq, resp.ScreenAt)
	}
	if resp.Screen != "b\nc" {
		t.Fatalf("screen=%q, want=%q", resp.Screen, "b\nc")
	}
}
