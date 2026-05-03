package workspace

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"
	sqlitebackend "hop.top/wsm/pkg/backend/sqlite"
)

// newBusTestManager builds a Manager wired to an in-memory bus and
// returns both so tests can assert published topics.
func newBusTestManager(t *testing.T) (*Manager, bus.Bus) {
	t.Helper()
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	eb := bus.New()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = eb.Close(ctx)
	})
	m, err := NewManager(b,
		WithWriterID("oasf:agent:test-runner"),
		WithSpaceID("sp-test-root"),
		WithBus(eb),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m, eb
}

type capturedEvent struct {
	topic       string
	workspaceID string
}

func subscribeAll(t *testing.T, b bus.Bus) (collect func() []capturedEvent) {
	t.Helper()
	var (
		mu     sync.Mutex
		events []capturedEvent
	)
	b.Subscribe("wsm.runtime.#", func(_ context.Context, e bus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, capturedEvent{topic: string(e.Topic), workspaceID: e.WorkspaceID})
		return nil
	})
	return func() []capturedEvent {
		mu.Lock()
		defer mu.Unlock()
		out := make([]capturedEvent, len(events))
		copy(out, events)
		return out
	}
}

func TestManager_PublishesWorkspaceCreated(t *testing.T) {
	m, b := newBusTestManager(t)
	collect := subscribeAll(t, b)
	ctx := context.Background()

	ws, err := m.Create(ctx, "bus-create", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got := collect()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d (%+v)", len(got), got)
	}
	if got[0].topic != "wsm.runtime.workspace.created" {
		t.Fatalf("topic = %q, want wsm.runtime.workspace.created", got[0].topic)
	}
	if got[0].workspaceID != ws.ID {
		t.Fatalf("workspace_id = %q, want %q", got[0].workspaceID, ws.ID)
	}
}

func TestManager_PublishesArchiveLifecycle(t *testing.T) {
	m, b := newBusTestManager(t)
	collect := subscribeAll(t, b)
	ctx := context.Background()

	ws, err := m.Create(ctx, "bus-archive", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Archive(ctx, ws.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := m.Unarchive(ctx, ws.ID); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if err := m.Delete(ctx, ws.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	want := []string{
		"wsm.runtime.workspace.created",
		"wsm.runtime.workspace.archived",
		"wsm.runtime.workspace.unarchived",
		"wsm.runtime.workspace.deleted",
	}
	got := collect()
	if len(got) != len(want) {
		t.Fatalf("expected %d events, got %d (%+v)", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].topic != w {
			t.Fatalf("event[%d] topic = %q, want %q", i, got[i].topic, w)
		}
	}
}

func TestManager_PublishesSnapshotCreated(t *testing.T) {
	m, b := newBusTestManager(t)
	collect := subscribeAll(t, b)
	ctx := context.Background()

	ws, err := m.Create(ctx, "bus-snapshot", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{Name: "s1"}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	got := collect()
	var sawSnapshot bool
	for _, ev := range got {
		if ev.topic == "wsm.runtime.snapshot.created" {
			sawSnapshot = true
			if ev.workspaceID != ws.ID {
				t.Fatalf("snapshot event workspace_id = %q, want %q", ev.workspaceID, ws.ID)
			}
		}
	}
	if !sawSnapshot {
		t.Fatalf("expected wsm.runtime.snapshot.created in %+v", got)
	}
}
