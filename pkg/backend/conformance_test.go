package backend_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"hop.top/wsm/pkg/backend"
	sqlitebackend "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/model"
)

func newBackendForConformance(t *testing.T) backend.Backend {
	t.Helper()
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return b
}

func TestBackendConformance(t *testing.T) {
	ctx := context.Background()
	b := newBackendForConformance(t)

	now := time.Now().UTC()
	ws := model.Workspace{ID: "w1", Name: "alpha", Status: model.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := b.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	got, err := b.GetWorkspace(ctx, "w1")
	if err != nil || got == nil {
		t.Fatalf("get workspace: err=%v got=%v", err, got)
	}

	payload, _ := json.Marshal(model.MessageData{Role: "user", Content: "hello"})
	if _, err := b.AppendEvent(ctx, model.Event{WorkspaceID: ws.ID, Type: model.EventMessageAdded, Data: payload}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	events, err := b.ListEvents(ctx, ws.ID, backend.EventListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if err := b.Migrate(ctx); err != nil {
		t.Fatalf("migrate idempotent: %v", err)
	}

	snap := model.Snapshot{WorkspaceID: ws.ID, AtSeqNum: 1, AtTimestamp: now, Workspace: ws, GeneratedAt: now}
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
		t.Fatalf("expected 0 events after delete, got %d", count)
	}
	sc, err := b.SnapshotCount(ctx, ws.ID)
	if err != nil {
		t.Fatalf("snapshot count: %v", err)
	}
	if sc != 0 {
		t.Fatalf("expected 0 snapshots after delete, got %d", sc)
	}
}
