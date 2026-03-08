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
	if _, err := b.AppendEvent(ctx, model.Event{WorkspaceID: ws.ID, Type: model.EventInteractionMessage, Data: payload}); err != nil {
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

func TestBackend_SpaceCRUD(t *testing.T) {
	ctx := context.Background()
	b := newBackendForConformance(t)

	s := model.Space{
		ID: "sp-1", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/proj",
		Capabilities: []string{"read", "write"},
		TrustLevel:   model.TrustLocal,
		CreatedAt:    time.Now().UTC(),
	}
	if err := b.SaveSpace(ctx, s); err != nil {
		t.Fatal(err)
	}
	got, err := b.GetSpace(ctx, "sp-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected space, got nil")
	}
	if got.Boundary != "/proj" {
		t.Fatalf("boundary: want /proj, got %s", got.Boundary)
	}
	if len(got.Capabilities) != 2 {
		t.Fatalf("capabilities: want 2, got %d", len(got.Capabilities))
	}

	// Save child
	child := model.Space{
		ID: "sp-2", Type: model.SpaceContainer,
		Parent: "sp-1", Boundary: "docker://app",
		Capabilities: []string{"read"},
		TrustLevel:   model.TrustVerified,
		CreatedAt:    time.Now().UTC(),
	}
	if err := b.SaveSpace(ctx, child); err != nil {
		t.Fatal(err)
	}

	// List by parent
	children, err := b.ListSpaces(ctx, "sp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 {
		t.Fatalf("children: want 1, got %d", len(children))
	}

	// List roots
	roots, err := b.ListSpaces(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Fatalf("roots: want 1, got %d", len(roots))
	}

	// Not found
	missing, err := b.GetSpace(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Fatal("expected nil for missing space")
	}

	// Delete without force fails when children exist
	if err := b.DeleteSpace(ctx, "sp-1", false); err == nil {
		t.Fatal("expected error deleting space with children without force")
	}

	// Delete with force cascades children
	if err := b.DeleteSpace(ctx, "sp-1", true); err != nil {
		t.Fatalf("force delete: %v", err)
	}
	if got, _ := b.GetSpace(ctx, "sp-1"); got != nil {
		t.Fatal("expected parent deleted")
	}
	if got, _ := b.GetSpace(ctx, "sp-2"); got != nil {
		t.Fatal("expected child cascaded")
	}

	// Delete not found
	if err := b.DeleteSpace(ctx, "nonexistent", false); err == nil {
		t.Fatal("expected not_found error")
	}
}

func TestBackend_WorkspaceSpaceAssociation(t *testing.T) {
	ctx := context.Background()
	b := newBackendForConformance(t)

	now := time.Now().UTC()
	ws1 := model.Workspace{ID: "w1", Name: "proj-a", Status: model.StatusActive, CreatedAt: now, UpdatedAt: now}
	ws2 := model.Workspace{ID: "w2", Name: "proj-b", Status: model.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := b.CreateWorkspace(ctx, ws1); err != nil {
		t.Fatal(err)
	}
	if err := b.CreateWorkspace(ctx, ws2); err != nil {
		t.Fatal(err)
	}

	sp := model.Space{
		ID: "sp-1", Type: model.SpaceFilesystem,
		Boundary: "/shared", Capabilities: []string{"read"},
		TrustLevel: model.TrustLocal, CreatedAt: now,
	}
	if err := b.SaveSpace(ctx, sp); err != nil {
		t.Fatal(err)
	}

	// Associate with both workspaces
	if err := b.AssociateSpace(ctx, "w1", "sp-1"); err != nil {
		t.Fatalf("associate w1: %v", err)
	}
	if err := b.AssociateSpace(ctx, "w2", "sp-1"); err != nil {
		t.Fatalf("associate w2: %v", err)
	}

	// Idempotent association
	if err := b.AssociateSpace(ctx, "w1", "sp-1"); err != nil {
		t.Fatalf("idempotent associate: %v", err)
	}

	// List spaces for workspace
	spaces, err := b.ListWorkspaceSpaces(ctx, "w1")
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 1 || spaces[0].ID != "sp-1" {
		t.Fatalf("expected 1 space sp-1, got %d", len(spaces))
	}

	// List workspaces for space
	workspaces, err := b.ListSpaceWorkspaces(ctx, "sp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}

	// Dissociate
	if err := b.DissociateSpace(ctx, "w1", "sp-1"); err != nil {
		t.Fatalf("dissociate: %v", err)
	}
	spaces, err = b.ListWorkspaceSpaces(ctx, "w1")
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 0 {
		t.Fatalf("expected 0 spaces after dissociate, got %d", len(spaces))
	}

	// Dissociate not found
	if err := b.DissociateSpace(ctx, "w1", "sp-1"); err == nil {
		t.Fatal("expected error for missing association")
	}

	// Workspace delete cascades associations
	if err := b.DeleteWorkspace(ctx, "w2"); err != nil {
		t.Fatal(err)
	}
	workspaces, err = b.ListSpaceWorkspaces(ctx, "sp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 0 {
		t.Fatalf("expected 0 workspaces after cascade, got %d", len(workspaces))
	}

	// Space delete cascades associations
	sp2 := model.Space{
		ID: "sp-2", Type: model.SpaceAPI,
		Boundary: "https://api.example.com", Capabilities: []string{"read"},
		TrustLevel: model.TrustExternal, CreatedAt: now,
	}
	if err := b.SaveSpace(ctx, sp2); err != nil {
		t.Fatal(err)
	}
	ws3 := model.Workspace{ID: "w3", Name: "proj-c", Status: model.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := b.CreateWorkspace(ctx, ws3); err != nil {
		t.Fatal(err)
	}
	if err := b.AssociateSpace(ctx, "w3", "sp-2"); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteSpace(ctx, "sp-2", false); err != nil {
		t.Fatal(err)
	}
	spaces, err = b.ListWorkspaceSpaces(ctx, "w3")
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 0 {
		t.Fatalf("expected 0 spaces after space delete cascade, got %d", len(spaces))
	}
}
