package workspace

import (
	"context"
	"path/filepath"
	"testing"

	sqlitebackend "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/model"
)

func newTestManagerForSpace(t *testing.T) *Manager {
	t.Helper()
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	m, err := NewManager(b,
		WithWriterID("oasf:agent:test-runner"),
		WithSpaceID("sp-test-root"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func TestManager_CreateSpace_OWSFAlignment(t *testing.T) {
	m := newTestManagerForSpace(t)
	ctx := context.Background()

	ws, _ := m.Create(ctx, "test-ws", CreateOptions{})

	s := Space{
		ID:           "sp-1",
		Type:         SpaceFilesystem,
		Boundary:     "/home/user/project",
		Capabilities: []string{"oasf:cap:fs.read"},
		TrustLevel:   TrustLocal,
	}

	created, err := m.CreateSpace(ctx, ws.ID, s)
	if err != nil {
		t.Fatalf("CreateSpace failed: %v", err)
	}

	if created.WriterID != "oasf:agent:test-runner" {
		t.Errorf("expected WriterID to be inherited from manager, got %q", created.WriterID)
	}

	if len(created.Observables) == 0 {
		t.Fatal("expected Observables to be heuristically populated from Boundary")
	}

	if created.Observables[0].Type != model.ObservableFile {
		t.Errorf("expected ObservableType 'file', got %v", created.Observables[0].Type)
	}
}

func TestManager_CreateSpace_InvalidCapabilities(t *testing.T) {
	m := newTestManagerForSpace(t)
	ctx := context.Background()
	ws, _ := m.Create(ctx, "test-ws", CreateOptions{})

	s := Space{
		ID:           "sp-bad",
		Type:         SpaceFilesystem,
		Boundary:     "/tmp",
		Capabilities: []string{"admin"}, // Missing oasf:cap: prefix
		TrustLevel:   TrustLocal,
	}

	_, err := m.CreateSpace(ctx, ws.ID, s)
	if err == nil {
		t.Fatal("expected error for non-OASF capability taxonomy")
	}
}

func TestManager_RecordEvent_OCSFDefaulting(t *testing.T) {
	// Manager without explicit OWSF fields
	b, _ := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	_ = b.Migrate(context.Background())
	m, _ := NewManager(b)
	ctx := context.Background()
	ws, _ := m.Create(ctx, "test-ws", CreateOptions{})

	// Record an interaction message - should have default writer applied
	ev, err := m.RecordEvent(ctx, ws.ID, EventInteractionMessage, MessageData{Role: "user", Content: "hi"})
	if err != nil {
		t.Fatalf("RecordEvent failed with defaulting: %v", err)
	}

	if ev.WriterID == "" {
		t.Error("expected default WriterID to be applied")
	}
}

func TestManager_CreateSpace_ExistingIDConflicts(t *testing.T) {
	m := newTestManagerForSpace(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "ws-a", CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, "ws-b", CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	mk := func(boundary string) Space {
		return Space{
			ID: "shared-api", Type: SpaceAPI, Boundary: boundary,
			Capabilities: []string{"oasf:cap:api:read"}, TrustLevel: TrustVerified,
		}
	}
	if _, err := m.AddSpaceToWorkspace(ctx, "ws-a", mk("https://api.example/a")); err != nil {
		t.Fatal(err)
	}

	for _, ref := range []string{"ws-b", "ws-a"} {
		_, err := m.AddSpaceToWorkspace(ctx, ref, mk("https://api.example/b"))
		if !IsKind(err, ErrorKindConflict) {
			t.Fatalf("add to %s: got %v, want conflict", ref, err)
		}
	}

	got, err := m.GetSpace(ctx, "shared-api")
	if err != nil {
		t.Fatal(err)
	}
	if got.Boundary != "https://api.example/a" {
		t.Fatalf("boundary overwritten: %q", got.Boundary)
	}
	wsB, err := m.ListWorkspaceSpaces(ctx, "ws-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(wsB) != 0 {
		t.Fatalf("failed add associated space with ws-b: %v", wsB)
	}
}
