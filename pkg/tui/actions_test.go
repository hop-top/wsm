package tui

import (
	"context"
	"path/filepath"
	"testing"

	sqlitebackend "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/workspace"
)

func newActionTestManager(t *testing.T) *workspace.Manager {
	t.Helper()
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate backend: %v", err)
	}
	m, err := workspace.NewManager(b)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	return m
}

func TestWorkspaceLifecycleActions(t *testing.T) {
	ctx := context.Background()
	manager := newActionTestManager(t)

	created, err := CreateWorkspace(ctx, manager, "tui-lifecycle")
	if err != nil {
		t.Fatalf("create action failed: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected workspace id from create action")
	}

	nextStatus, err := ToggleArchiveWorkspace(ctx, manager, created)
	if err != nil {
		t.Fatalf("archive toggle failed: %v", err)
	}
	if nextStatus != workspace.StatusArchived {
		t.Fatalf("expected archived status from toggle, got %s", nextStatus)
	}

	archived, err := manager.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get archived workspace: %v", err)
	}
	nextStatus, err = ToggleArchiveWorkspace(ctx, manager, *archived)
	if err != nil {
		t.Fatalf("unarchive toggle failed: %v", err)
	}
	if nextStatus != workspace.StatusActive {
		t.Fatalf("expected active status from unarchive toggle, got %s", nextStatus)
	}

	if _, err := manager.RecordEvent(ctx, created.ID, workspace.EventMessageAdded, workspace.MessageData{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("record event for detail load: %v", err)
	}
	detail, err := LoadWorkspaceDetail(ctx, manager, created)
	if err != nil {
		t.Fatalf("load detail failed: %v", err)
	}
	if detail.EventCount == 0 {
		t.Fatalf("expected detail to include event count > 0")
	}
	if detail.Snapshot == nil {
		t.Fatalf("expected detail to include generated snapshot")
	}

	if err := DeleteWorkspace(ctx, manager, created); err != nil {
		t.Fatalf("delete action failed: %v", err)
	}
	if _, err := manager.Get(ctx, created.ID); !workspace.IsKind(err, workspace.ErrorKindNotFound) {
		t.Fatalf("expected deleted workspace to be not found, got %v", err)
	}
}
