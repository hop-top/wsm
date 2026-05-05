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

	mustRecord(workspace.EventInteractionMessage, workspace.MessageData{Role: "user", Content: "m1"})
	mustRecord(workspace.EventInteractionToolCall, workspace.ToolCallData{ToolName: "grep"})
	mustRecord(workspace.EventMutationFilePatch, workspace.FileChangeData{Path: "a.txt", Operation: "update"})
	mustRecord(workspace.EventMutationFilePatch, workspace.FileChangeData{Path: "b.txt", Operation: "update"})
	mustRecord(workspace.EventMutationFilePatch, workspace.FileChangeData{Path: "a.txt", Operation: "update"})
	mustRecord(workspace.EventDeviceAttached, workspace.DeviceData{DeviceID: "dev-1", DeviceName: "Laptop"})
	mustRecord(workspace.EventDeviceAttached, workspace.DeviceData{DeviceID: "dev-2", DeviceName: "CI"})
	mustRecord(workspace.EventDeviceDetached, workspace.DeviceData{DeviceID: "dev-2", DeviceName: "CI"})

	firstSnapshotRes := r.run("snapshot", "generate", ws.ID, "--format", "json")
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

	mustRecord(workspace.EventInteractionMessage, workspace.MessageData{Role: "assistant", Content: "m2"})
	mustRecord(workspace.EventMutationFilePatch, workspace.FileChangeData{Path: "c.txt", Operation: "create"})

	secondSnapshotRes := r.run("snapshot", "generate", ws.ID, "--format", "json")
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

	inspectRes := r.run("workspace", "show", ws.ID, "--format", "json")
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
	emptySnap, err := mgr.Snapshot(ctx, empty.ID, workspace.SnapshotOptions{})
	if err != nil {
		t.Fatalf("snapshot empty workspace: %v", err)
	}
	if emptySnap.MessageCount != 0 || emptySnap.ToolCalls != 0 {
		t.Fatalf("expected zero counters for empty workspace snapshot, got messages=%d tools=%d", emptySnap.MessageCount, emptySnap.ToolCalls)
	}
	if len(emptySnap.FilesChanged) != 0 || len(emptySnap.Devices) != 0 {
		t.Fatalf("expected empty files/devices for empty workspace snapshot, got files=%v devices=%v", emptySnap.FilesChanged, emptySnap.Devices)
	}

	implicitRes := r.run("snapshot", "generate", "--format", "json")
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
	r.run("event", "add", wsID, "interaction.message", "--content=context-test")

	genRes := r.run("snapshot", "generate", wsID, "--format", "json")
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

func TestSnapshotGenerate_Session(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("snap-session-test")

	r.run("event", "add", wsID, "interaction.message", "--content=m1", "--session-id=sess-1")
	r.run("event", "add", wsID, "interaction.message", "--content=m2", "--session-id=sess-2")
	r.run("event", "add", wsID, "interaction.message", "--content=m3", "--session-id=sess-1")

	allRes := r.run("snapshot", "generate", wsID, "--format", "json")
	requireExitCode(t, allRes, 0)
	var allSnap workspace.Snapshot
	mustUnmarshalJSON(t, allRes.Stdout, &allSnap)
	if allSnap.MessageCount != 3 {
		t.Fatalf("expected 3 messages in full snapshot, got %d", allSnap.MessageCount)
	}
	if len(allSnap.Sessions) != 2 {
		t.Fatalf("expected 2 sessions in full snapshot, got %d", len(allSnap.Sessions))
	}
	if allSnap.ID == "" {
		t.Fatal("expected snapshot to have an ID")
	}

	sess1Res := r.run("snapshot", "generate", wsID, "-s", "sess-1", "--format", "json")
	requireExitCode(t, sess1Res, 0)
	var sess1Snap workspace.Snapshot
	mustUnmarshalJSON(t, sess1Res.Stdout, &sess1Snap)
	if sess1Snap.MessageCount != 2 {
		t.Fatalf("expected 2 messages in sess-1 snapshot, got %d", sess1Snap.MessageCount)
	}
	if len(sess1Snap.Sessions) != 1 || sess1Snap.Sessions[0] != "sess-1" {
		t.Fatalf("expected only sess-1 in sessions, got %v", sess1Snap.Sessions)
	}

	sess2Res := r.run("snapshot", "generate", wsID, "-s", "sess-2", "--format", "json")
	requireExitCode(t, sess2Res, 0)
	var sess2Snap workspace.Snapshot
	mustUnmarshalJSON(t, sess2Res.Stdout, &sess2Snap)
	if sess2Snap.MessageCount != 1 {
		t.Fatalf("expected 1 message in sess-2 snapshot, got %d", sess2Snap.MessageCount)
	}
}

func TestSnapshotGenerate_NameAndSeed(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("snap-name-test")

	r.run("event", "add", wsID, "interaction.message", "--content=m1")

	namedRes := r.run("snapshot", "generate", wsID, "-n", "baseline", "--seed", "--format", "json")
	requireExitCode(t, namedRes, 0)
	var namedSnap workspace.Snapshot
	mustUnmarshalJSON(t, namedRes.Stdout, &namedSnap)
	if namedSnap.Name != "baseline" {
		t.Fatalf("expected name=baseline, got %q", namedSnap.Name)
	}
	if !namedSnap.IsSeed {
		t.Fatal("expected is_seed=true")
	}

	getRes := r.run("snapshot", "generate", wsID, "-n", "baseline", "--format", "json")
	requireExitCode(t, getRes, 0)
	var getSnap workspace.Snapshot
	mustUnmarshalJSON(t, getRes.Stdout, &getSnap)
	if getSnap.ID != namedSnap.ID {
		t.Fatalf("expected same snapshot when using name, got %s vs %s", getSnap.ID, namedSnap.ID)
	}

	listRes := r.run("snapshot", "list", "-w", wsID, "--format", "json")
	requireExitCode(t, listRes, 0)
	var snaps []workspace.Snapshot
	mustUnmarshalJSON(t, listRes.Stdout, &snaps)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].Name != "baseline" {
		t.Fatalf("expected name in list, got %q", snaps[0].Name)
	}
	if !snaps[0].IsSeed {
		t.Fatal("expected is_seed in list")
	}
}

func TestSnapshotUpdate(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("snap-update-test")

	r.run("event", "add", wsID, "interaction.message", "--content=m1")
	genRes := r.run("snapshot", "generate", wsID, "--format", "json")
	requireExitCode(t, genRes, 0)
	var snap workspace.Snapshot
	mustUnmarshalJSON(t, genRes.Stdout, &snap)

	updateRes := r.run("snapshot", "update", snap.ID, "-n", "renamed", "--seed", "--format", "json")
	requireExitCode(t, updateRes, 0)
	var updated workspace.Snapshot
	mustUnmarshalJSON(t, updateRes.Stdout, &updated)
	if updated.Name != "renamed" {
		t.Fatalf("expected name=renamed, got %q", updated.Name)
	}
	if !updated.IsSeed {
		t.Fatal("expected is_seed=true")
	}

	byNameRes := r.run("snapshot", "update", "renamed", "-n", "final", "--format", "json")
	requireExitCode(t, byNameRes, 0)
	var byName workspace.Snapshot
	mustUnmarshalJSON(t, byNameRes.Stdout, &byName)
	if byName.Name != "final" {
		t.Fatalf("expected name=final, got %q", byName.Name)
	}
}

func TestSnapshotRemove(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("snap-remove-test")

	r.run("event", "add", wsID, "interaction.message", "--content=m1")
	genRes := r.run("snapshot", "generate", wsID, "-n", "to-remove", "--format", "json")
	requireExitCode(t, genRes, 0)
	var snap workspace.Snapshot
	mustUnmarshalJSON(t, genRes.Stdout, &snap)

	listBefore := r.run("snapshot", "list", "-w", wsID, "--format", "json")
	requireExitCode(t, listBefore, 0)
	var before []workspace.Snapshot
	mustUnmarshalJSON(t, listBefore.Stdout, &before)
	if len(before) != 1 {
		t.Fatalf("expected 1 snapshot before remove, got %d", len(before))
	}

	rmRes := r.run("snapshot", "delete", snap.ID, "--force")
	requireExitCode(t, rmRes, 0)

	listAfter := r.run("snapshot", "list", "-w", wsID, "--format", "json")
	requireExitCode(t, listAfter, 0)
	var after []workspace.Snapshot
	mustUnmarshalJSON(t, listAfter.Stdout, &after)
	if len(after) != 0 {
		t.Fatalf("expected 0 snapshots after remove, got %d", len(after))
	}

	rmByNameRes := r.run("snapshot", "delete", "nonexistent", "--force")
	requireExitCode(t, rmByNameRes, 3)
}

func TestSnapshotList(t *testing.T) {
	r := newRunner(t)
	ws1 := r.mustCreateWorkspace("snap-list-1")
	ws2 := r.mustCreateWorkspace("snap-list-2")

	r.run("event", "add", ws1, "interaction.message", "--content=ws1-m1", "--session-id=sess-a")
	requireExitCode(t, r.run("snapshot", "generate", ws1, "--format", "json"), 0)

	r.run("event", "add", ws2, "interaction.message", "--content=ws2-m1", "--session-id=sess-b")
	r.run("event", "add", ws2, "interaction.message", "--content=ws2-m2", "--session-id=sess-c")
	requireExitCode(t, r.run("snapshot", "generate", ws2, "--format", "json"), 0)

	r.run("event", "add", ws1, "interaction.message", "--content=ws1-m2", "--session-id=sess-a")
	requireExitCode(t, r.run("snapshot", "generate", ws1, "--format", "json"), 0)

	allRes := r.run("snapshot", "list", "--format", "json")
	requireExitCode(t, allRes, 0)
	var all []workspace.Snapshot
	mustUnmarshalJSON(t, allRes.Stdout, &all)
	if len(all) != 3 {
		t.Fatalf("expected 3 total snapshots, got %d", len(all))
	}

	ws1Res := r.run("snapshot", "list", "-w", ws1, "--format", "json")
	requireExitCode(t, ws1Res, 0)
	var ws1Snaps []workspace.Snapshot
	mustUnmarshalJSON(t, ws1Res.Stdout, &ws1Snaps)
	if len(ws1Snaps) != 2 {
		t.Fatalf("expected 2 snapshots for ws1, got %d", len(ws1Snaps))
	}

	ws2Res := r.run("snapshot", "list", "-w", "snap-list-2", "--format", "json")
	requireExitCode(t, ws2Res, 0)
	var ws2Snaps []workspace.Snapshot
	mustUnmarshalJSON(t, ws2Res.Stdout, &ws2Snaps)
	if len(ws2Snaps) != 1 {
		t.Fatalf("expected 1 snapshot for ws2, got %d", len(ws2Snaps))
	}
	if ws2Snaps[0].MessageCount != 2 {
		t.Fatalf("expected message_count=2, got %d", ws2Snaps[0].MessageCount)
	}
	if len(ws2Snaps[0].Sessions) != 2 {
		t.Fatalf("expected 2 sessions in ws2 snapshot, got %d", len(ws2Snaps[0].Sessions))
	}

	plainRes := r.run("snapshot", "list")
	requireExitCode(t, plainRes, 0)
	if !strings.Contains(plainRes.Stdout, "ID") || !strings.Contains(plainRes.Stdout, "WORKSPACE") {
		t.Fatalf("expected table headers in plain output, got:\n%s", plainRes.Stdout)
	}

	multiRes := r.run("snapshot", "list", "-w", ws1, "-w", "snap-list-2", "--format", "json")
	requireExitCode(t, multiRes, 0)
	var multiSnaps []workspace.Snapshot
	mustUnmarshalJSON(t, multiRes.Stdout, &multiSnaps)
	if len(multiSnaps) != 3 {
		t.Fatalf("expected 3 snapshots for multi-workspace filter, got %d", len(multiSnaps))
	}

	sessRes := r.run("snapshot", "list", "-s", "sess-b", "--format", "json")
	requireExitCode(t, sessRes, 0)
	var sessSnaps []workspace.Snapshot
	mustUnmarshalJSON(t, sessRes.Stdout, &sessSnaps)
	if len(sessSnaps) != 1 {
		t.Fatalf("expected 1 snapshot with sess-b, got %d", len(sessSnaps))
	}
	if sessSnaps[0].WorkspaceID != ws2 {
		t.Fatalf("expected sess-b snapshot to be from ws2")
	}

	sessMultiRes := r.run("snapshot", "list", "-s", "sess-a", "-s", "sess-c", "--format", "json")
	requireExitCode(t, sessMultiRes, 0)
	var sessMultiSnaps []workspace.Snapshot
	mustUnmarshalJSON(t, sessMultiRes.Stdout, &sessMultiSnaps)
	if len(sessMultiSnaps) != 3 {
		t.Fatalf("expected 3 snapshots with sess-a or sess-c, got %d", len(sessMultiSnaps))
	}
}

// TestSnapshotDelete_NoteFlag exercises the --note|-n flag plumbed in
// T-1287. With the flag set the delete succeeds; omitted, it also
// succeeds (no enforcement yet — that's T-1288 with the policy engine).
func TestSnapshotDelete_NoteFlag(t *testing.T) {
	r := newRunner(t)
	ws1 := r.mustCreateWorkspace("snap-note-with")
	ws2 := r.mustCreateWorkspace("snap-note-without")

	r.run("event", "add", ws1, "interaction.message", "--content=m1")
	gen1 := r.run("snapshot", "generate", ws1, "-n", "snap-with-note", "--format", "json")
	requireExitCode(t, gen1, 0)
	var snap1 workspace.Snapshot
	mustUnmarshalJSON(t, gen1.Stdout, &snap1)

	r.run("event", "add", ws2, "interaction.message", "--content=m2")
	gen2 := r.run("snapshot", "generate", ws2, "-n", "snap-no-note", "--format", "json")
	requireExitCode(t, gen2, 0)
	var snap2 workspace.Snapshot
	mustUnmarshalJSON(t, gen2.Stdout, &snap2)

	withNote := r.run("snapshot", "delete", snap1.ID, "--force", "--note", "obsolete")
	requireExitCode(t, withNote, 0)

	withoutNote := r.run("snapshot", "delete", snap2.ID, "--force")
	requireExitCode(t, withoutNote, 0)
}
