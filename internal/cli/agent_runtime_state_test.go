package cli

import (
	"slices"
	"testing"
	"time"
)

func TestPruneExitedRuntimeSessions_KeepsLatestAndDropsExpired(t *testing.T) {
	now := time.Now().Unix()
	rows := []agentRuntimeSessionRecord{
		{SessionID: "s-running", WorkspaceID: "WS-1", RuntimeState: "running", UpdatedAt: now - 5},
		{SessionID: "s-e1", WorkspaceID: "WS-1", RuntimeState: "exited", UpdatedAt: now - 10, storagePath: "/tmp/e1.json"},
		{SessionID: "s-e2", WorkspaceID: "WS-1", RuntimeState: "exited", UpdatedAt: now - 20, storagePath: "/tmp/e2.json"},
		{SessionID: "s-e3", WorkspaceID: "WS-1", RuntimeState: "exited", UpdatedAt: now - 30, storagePath: "/tmp/e3.json"},
		{SessionID: "s-e4", WorkspaceID: "WS-1", RuntimeState: "exited", UpdatedAt: now - 40, storagePath: "/tmp/e4.json"},
		{
			SessionID:    "s-old",
			WorkspaceID:  "WS-1",
			RuntimeState: "exited",
			UpdatedAt:    now - int64((agentRuntimeExitedRetention/time.Second)+60),
			storagePath:  "/tmp/old.json",
		},
	}

	trimmed, stalePaths := pruneExitedRuntimeSessions(rows, now)
	if len(trimmed) != 4 {
		t.Fatalf("trimmed len=%d, want=4 (running + 3 exited)", len(trimmed))
	}
	keptIDs := make([]string, 0, len(trimmed))
	for _, r := range trimmed {
		keptIDs = append(keptIDs, r.SessionID)
	}
	if !slices.Contains(keptIDs, "s-running") {
		t.Fatalf("running session should be kept: %v", keptIDs)
	}
	if !slices.Contains(keptIDs, "s-e1") || !slices.Contains(keptIDs, "s-e2") || !slices.Contains(keptIDs, "s-e3") {
		t.Fatalf("latest 3 exited sessions should be kept: %v", keptIDs)
	}
	if slices.Contains(keptIDs, "s-e4") || slices.Contains(keptIDs, "s-old") {
		t.Fatalf("stale exited sessions should be removed: %v", keptIDs)
	}

	if len(stalePaths) != 2 {
		t.Fatalf("stale path count=%d, want=2", len(stalePaths))
	}
	if !slices.Contains(stalePaths, "/tmp/e4.json") || !slices.Contains(stalePaths, "/tmp/old.json") {
		t.Fatalf("unexpected stale paths: %v", stalePaths)
	}
}

func TestMergeRuntimeSessionRows_LiveOverridesPersisted(t *testing.T) {
	fileRows := []agentRuntimeSessionRecord{
		{SessionID: "s-1", WorkspaceID: "WS-1", RuntimeState: "idle", UpdatedAt: 100},
		{SessionID: "s-2", WorkspaceID: "WS-1", RuntimeState: "exited", UpdatedAt: 90},
	}
	liveRows := []agentRuntimeSessionRecord{
		{SessionID: "s-1", WorkspaceID: "WS-1", RuntimeState: "running", UpdatedAt: 110},
		{SessionID: "s-3", WorkspaceID: "WS-2", RuntimeState: "running", UpdatedAt: 120},
	}
	merged := mergeRuntimeSessionRows(liveRows, fileRows)
	if len(merged) != 3 {
		t.Fatalf("merged len=%d, want=3", len(merged))
	}
	byID := map[string]agentRuntimeSessionRecord{}
	for _, row := range merged {
		byID[row.SessionID] = row
	}
	if byID["s-1"].RuntimeState != "running" {
		t.Fatalf("live state should override file state: %+v", byID["s-1"])
	}
	if byID["s-2"].RuntimeState != "exited" {
		t.Fatalf("file-only exited row should remain: %+v", byID["s-2"])
	}
	if byID["s-3"].RuntimeState != "running" {
		t.Fatalf("live-only row should be included: %+v", byID["s-3"])
	}
}
