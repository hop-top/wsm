package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/device"
	"hop.top/wsm/pkg/model"
)

func openTestBackend(t *testing.T) *Backend {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wsm.db")
	b, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return b
}

func TestSQLiteBackend_Conformance(t *testing.T) {
	ctx := context.Background()
	b := openTestBackend(t)
	if err := b.Migrate(ctx); err != nil {
		t.Fatalf("migrate idempotent: %v", err)
	}

	now := time.Now().UTC()
	ws := model.Workspace{ID: "w1", Name: "demo", Status: model.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := b.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := b.GetWorkspace(ctx, ws.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v got=%v", err, got)
	}
	if got.Name != ws.Name {
		t.Fatalf("unexpected name: %s", got.Name)
	}

	d, _ := json.Marshal(model.MessageData{Role: "user", Content: "hello"})
	e1, err := b.AppendEvent(ctx, model.Event{WorkspaceID: ws.ID, Type: model.EventMessageAdded, Data: d})
	if err != nil {
		t.Fatalf("append1: %v", err)
	}
	e2, err := b.AppendEvent(ctx, model.Event{WorkspaceID: ws.ID, Type: model.EventMessageAdded, Data: d})
	if err != nil {
		t.Fatalf("append2: %v", err)
	}
	if e1.SeqNum != 1 || e2.SeqNum != 2 {
		t.Fatalf("expected seq 1,2 got %d,%d", e1.SeqNum, e2.SeqNum)
	}

	events, err := b.ListEvents(ctx, ws.ID, backend.EventListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	snap := model.Snapshot{WorkspaceID: ws.ID, AtSeqNum: 2, AtTimestamp: now, Workspace: ws, GeneratedAt: now}
	if err := b.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if err := b.DeleteWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	count, err := b.EventCount(ctx, ws.ID)
	if err != nil {
		t.Fatalf("event count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected cascaded event delete, got %d", count)
	}
	sc, err := b.SnapshotCount(ctx, ws.ID)
	if err != nil {
		t.Fatalf("snapshot count: %v", err)
	}
	if sc != 0 {
		t.Fatalf("expected cascaded snapshot delete, got %d", sc)
	}
}

func TestSQLiteBackend_GetEvent(t *testing.T) {
	ctx := context.Background()
	b := openTestBackend(t)

	now := time.Now().UTC()
	ws := model.Workspace{ID: "w-get-ev", Name: "get-event", Status: model.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := b.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	d, _ := json.Marshal(model.MessageData{Role: "user", Content: "hello"})
	e1, err := b.AppendEvent(ctx, model.Event{WorkspaceID: ws.ID, Type: model.EventMessageAdded, Data: d})
	if err != nil {
		t.Fatalf("append event: %v", err)
	}

	// Lookup by seq_num (decimal string)
	bySeq, err := b.GetEvent(ctx, ws.ID, "1")
	if err != nil {
		t.Fatalf("get event by seq_num: %v", err)
	}
	if bySeq == nil {
		t.Fatal("expected event by seq_num, got nil")
	}
	if bySeq.ID != e1.ID {
		t.Fatalf("expected event id %s, got %s", e1.ID, bySeq.ID)
	}

	// Lookup by ULID string
	byID, err := b.GetEvent(ctx, ws.ID, e1.ID)
	if err != nil {
		t.Fatalf("get event by ULID: %v", err)
	}
	if byID == nil {
		t.Fatal("expected event by ULID, got nil")
	}
	if byID.SeqNum != e1.SeqNum {
		t.Fatalf("expected seq_num %d, got %d", e1.SeqNum, byID.SeqNum)
	}

	// Not found returns nil, nil
	missing, err := b.GetEvent(ctx, ws.ID, "99999")
	if err != nil {
		t.Fatalf("get missing event: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing event, got %+v", missing)
	}

	missingULID, err := b.GetEvent(ctx, ws.ID, "01NOTAREALULID000000000000")
	if err != nil {
		t.Fatalf("get missing ULID event: %v", err)
	}
	if missingULID != nil {
		t.Fatalf("expected nil for missing ULID event, got %+v", missingULID)
	}
}

func TestSQLite_ListDeviceSessions(t *testing.T) {
	ctx := context.Background()
	b := openTestBackend(t)

	now := time.Now().UTC()
	ws := model.Workspace{
		ID: "w-ds", Name: "device-sessions",
		Status: model.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := b.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	dev := device.Device{
		ID: "dev-1", Name: "laptop", Hostname: "laptop",
		OS: "darwin", Arch: "arm64",
		FirstSeen: now, LastSeen: now,
	}
	if err := b.SaveDevice(ctx, dev); err != nil {
		t.Fatalf("save device: %v", err)
	}

	d, _ := json.Marshal(model.MessageData{Role: "user", Content: "hello"})
	for i := 0; i < 3; i++ {
		_, err := b.AppendEvent(ctx, model.Event{
			WorkspaceID: ws.ID,
			Type:        model.EventMessageAdded,
			DeviceID:    dev.ID,
			SessionID:   "sess-1",
			AgentID:     "claude",
			Data:        d,
		})
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	// second session on same device
	_, err := b.AppendEvent(ctx, model.Event{
		WorkspaceID: ws.ID,
		Type:        model.EventMessageAdded,
		DeviceID:    dev.ID,
		SessionID:   "sess-2",
		AgentID:     "user",
		Data:        d,
	})
	if err != nil {
		t.Fatalf("append sess-2 event: %v", err)
	}

	sessions, err := b.ListDeviceSessions(ctx, dev.ID)
	if err != nil {
		t.Fatalf("ListDeviceSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Most recent session first (sess-2)
	if sessions[0].SessionID != "sess-2" {
		t.Fatalf("expected first session sess-2, got %q", sessions[0].SessionID)
	}
	if sessions[0].EventCount != 1 {
		t.Fatalf("expected 1 event in sess-2, got %d", sessions[0].EventCount)
	}
	if sessions[1].SessionID != "sess-1" {
		t.Fatalf("expected second session sess-1, got %q", sessions[1].SessionID)
	}
	if sessions[1].EventCount != 3 {
		t.Fatalf("expected 3 events in sess-1, got %d", sessions[1].EventCount)
	}
	if sessions[0].WorkspaceName != "device-sessions" {
		t.Fatalf("expected workspace name device-sessions, got %q", sessions[0].WorkspaceName)
	}

	// Unknown device returns empty slice
	empty, err := b.ListDeviceSessions(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListDeviceSessions unknown: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 sessions for unknown device, got %d", len(empty))
	}
}
