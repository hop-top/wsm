package e2e_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"hop.top/wsm/pkg/workspace"
)

func TestSnapshotGeneration(t *testing.T) {
	r := newRunner(t)
	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()

	ws, err := mgr.Create(ctx, "snapshot-work", workspace.CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	mustRecord := func(eventType workspace.EventType, payload any) {
		t.Helper()
		if _, err := mgr.RecordEvent(ctx, ws.ID, eventType, payload); err != nil {
			t.Fatalf("record %s event: %v", eventType, err)
		}
	}

	mustRecord(workspace.EventMessageAdded, workspace.MessageData{Role: "user", Content: "m1"})
	mustRecord(workspace.EventToolCallStarted, workspace.ToolCallData{ToolName: "grep"})
	mustRecord(workspace.EventFileChanged, workspace.FileChangeData{Path: "a.txt", Operation: "update"})
	mustRecord(workspace.EventFileChanged, workspace.FileChangeData{Path: "b.txt", Operation: "update"})
	mustRecord(workspace.EventFileChanged, workspace.FileChangeData{Path: "a.txt", Operation: "update"})
	mustRecord(workspace.EventDeviceAttached, workspace.DeviceData{DeviceID: "dev-1", DeviceName: "Laptop"})
	mustRecord(workspace.EventDeviceAttached, workspace.DeviceData{DeviceID: "dev-2", DeviceName: "CI"})
	mustRecord(workspace.EventDeviceDetached, workspace.DeviceData{DeviceID: "dev-2", DeviceName: "CI"})

	firstSnapshotRes := r.run("snapshot", "generate", ws.ID, "--json")
	requireExitCode(t, firstSnapshotRes, 0)
	var snap1 workspace.Snapshot
	mustUnmarshalJSON(t, firstSnapshotRes.Stdout, &snap1)
	if snap1.MessageCount != 1 || snap1.ToolCalls != 1 {
		t.Fatalf("unexpected snapshot counters: messages=%d tool_calls=%d", snap1.MessageCount, snap1.ToolCalls)
	}
	if len(snap1.FilesChanged) != 2 {
		t.Fatalf("expected deduplicated files_changed length 2, got %d (%v)", len(snap1.FilesChanged), snap1.FilesChanged)
	}
	sort.Strings(snap1.FilesChanged)
	if snap1.FilesChanged[0] != "a.txt" || snap1.FilesChanged[1] != "b.txt" {
		t.Fatalf("unexpected files_changed contents: %v", snap1.FilesChanged)
	}
	if len(snap1.Devices) != 1 || snap1.Devices[0] != "dev-1" {
		t.Fatalf("unexpected attached devices in snapshot: %v", snap1.Devices)
	}

	mustRecord(workspace.EventMessageAdded, workspace.MessageData{Role: "assistant", Content: "m2"})
	mustRecord(workspace.EventFileChanged, workspace.FileChangeData{Path: "c.txt", Operation: "create"})

	secondSnapshotRes := r.run("snapshot", "generate", ws.ID, "--json")
	requireExitCode(t, secondSnapshotRes, 0)
	var snap2 workspace.Snapshot
	mustUnmarshalJSON(t, secondSnapshotRes.Stdout, &snap2)
	if snap2.AtSeqNum <= snap1.AtSeqNum {
		t.Fatalf("expected incremental snapshot seq to advance, got %d <= %d", snap2.AtSeqNum, snap1.AtSeqNum)
	}
	if snap2.MessageCount != 2 || snap2.ToolCalls != 1 {
		t.Fatalf("unexpected incremental snapshot counters: messages=%d tool_calls=%d", snap2.MessageCount, snap2.ToolCalls)
	}
	sort.Strings(snap2.FilesChanged)
	if len(snap2.FilesChanged) != 3 {
		t.Fatalf("expected 3 deduplicated files in incremental snapshot, got %v", snap2.FilesChanged)
	}

	inspectRes := r.run("inspect", ws.ID, "--json")
	requireExitCode(t, inspectRes, 0)
	var inspectPayload inspectJSON
	mustUnmarshalJSON(t, inspectRes.Stdout, &inspectPayload)
	if inspectPayload.Snapshot.WorkspaceID != ws.ID {
		t.Fatalf("inspect snapshot points to wrong workspace: %s", inspectPayload.Snapshot.WorkspaceID)
	}
	if inspectPayload.Snapshot.MessageCount != 2 {
		t.Fatalf("inspect snapshot message_count mismatch: %d", inspectPayload.Snapshot.MessageCount)
	}

	empty, err := mgr.Create(ctx, "snapshot-empty", workspace.CreateOptions{})
	if err != nil {
		t.Fatalf("create empty workspace: %v", err)
	}
	emptySnap, err := mgr.Snapshot(ctx, empty.ID)
	if err != nil {
		t.Fatalf("snapshot empty workspace: %v", err)
	}
	if emptySnap.MessageCount != 0 || emptySnap.ToolCalls != 0 {
		t.Fatalf("expected zero counters for empty workspace snapshot, got messages=%d tools=%d", emptySnap.MessageCount, emptySnap.ToolCalls)
	}
	if len(emptySnap.FilesChanged) != 0 || len(emptySnap.Devices) != 0 {
		t.Fatalf("expected empty files/devices for empty workspace snapshot, got files=%v devices=%v", emptySnap.FilesChanged, emptySnap.Devices)
	}

	implicitRes := r.run("snapshot", "generate", "--json")
	requireExitCode(t, implicitRes, 0)
	var implicit workspace.Snapshot
	mustUnmarshalJSON(t, implicitRes.Stdout, &implicit)
	if implicit.WorkspaceID != empty.ID {
		t.Fatalf("expected implicit snapshot target to be most-recent workspace %s, got %s", empty.ID, implicit.WorkspaceID)
	}
}

func TestSnapshotContext(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("snap-context-test")
	r.run("events", "add", wsID, "message.added", "--content=context-test")

	genRes := r.run("snapshot", "generate", wsID, "--json")
	requireExitCode(t, genRes, 0)

	ctxRes := r.run("snapshot", "context", wsID)
	requireExitCode(t, ctxRes, 0)
	if !strings.Contains(ctxRes.Stdout, "snap-context-test") {
		t.Fatalf("expected workspace name in context, got: %s", ctxRes.Stdout)
	}
	if !strings.Contains(ctxRes.Stdout, "## Summary") {
		t.Fatalf("expected Summary header in context")
	}
}
