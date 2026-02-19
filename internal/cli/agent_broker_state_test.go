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
