package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	sqlitebackend "hop.top/wsm/pkg/backend/sqlite"
)

// newKitTopicTestManager builds a Manager wired to an in-memory bus and
// returns both. Used to assert that domain.Service[Workspace] /
// domain.Service[Snapshot] now publish kit.runtime.entity.* pre/post
// events under the bus, in addition to the wsm-prefixed aliases.
func newKitTopicTestManager(t *testing.T) (*Manager, bus.Bus) {
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

// TestService_KitPreEventsFireOnCreate proves that wiring
// domain.Service[Workspace] inside Manager.Create now publishes the
// canonical kit pre_validated and pre_persisted topics — the seam the
// kit/runtime/policy engine subscribes to in T-1288.
func TestService_KitPreEventsFireOnCreate(t *testing.T) {
	m, b := newKitTopicTestManager(t)
	ctx := context.Background()

	var (
		mu     sync.Mutex
		topics []string
	)
	b.Subscribe("kit.runtime.entity.#", func(_ context.Context, e bus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		topics = append(topics, string(e.Topic))
		return nil
	})

	if _, err := m.Create(ctx, "kit-events-create", CreateOptions{}); err != nil {
		t.Fatalf("create: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"kit.runtime.entity.pre_validated",
		"kit.runtime.entity.pre_persisted",
		"kit.runtime.entity.created",
	}
	if len(topics) != len(want) {
		t.Fatalf("expected %d kit topics, got %d (%+v)", len(want), len(topics), topics)
	}
	for i, w := range want {
		if topics[i] != w {
			t.Fatalf("topic[%d]=%q, want %q", i, topics[i], w)
		}
	}
}

// TestService_KitPreEventsFireOnArchiveUnarchiveDelete proves the
// service seams fire across the full workspace lifecycle, not just on
// Create. Each lifecycle call should emit pre_validated, pre_persisted,
// and the corresponding post topic (updated for archive/unarchive,
// deleted for delete).
func TestService_KitPreEventsFireOnArchiveUnarchiveDelete(t *testing.T) {
	m, b := newKitTopicTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "kit-events-lifecycle", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Subscribe AFTER create so the subsequent ops are isolated.
	var (
		mu     sync.Mutex
		topics []string
	)
	b.Subscribe("kit.runtime.entity.#", func(_ context.Context, e bus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		topics = append(topics, string(e.Topic))
		return nil
	})

	if err := m.Archive(ctx, ws.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := m.Unarchive(ctx, ws.ID); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if err := m.Delete(ctx, ws.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{
		// Archive (Update verb)
		"kit.runtime.entity.pre_validated",
		"kit.runtime.entity.pre_persisted",
		"kit.runtime.entity.updated",
		// Unarchive (Update verb)
		"kit.runtime.entity.pre_validated",
		"kit.runtime.entity.pre_persisted",
		"kit.runtime.entity.updated",
		// Delete
		"kit.runtime.entity.pre_validated",
		"kit.runtime.entity.pre_persisted",
		"kit.runtime.entity.deleted",
	}
	if len(topics) != len(want) {
		t.Fatalf("expected %d kit topics, got %d (%+v)", len(want), len(topics), topics)
	}
	for i, w := range want {
		if topics[i] != w {
			t.Fatalf("topic[%d]=%q, want %q", i, topics[i], w)
		}
	}
}

// TestService_PreValidatedVetoBlocksCreate proves that a synchronous
// subscriber returning an error on the kit pre_validated topic
// vetoes the entire Create operation. This is the foundational seam
// kit/runtime/policy rides on top of (T-1288).
func TestService_PreValidatedVetoBlocksCreate(t *testing.T) {
	m, b := newKitTopicTestManager(t)
	ctx := context.Background()

	vetoErr := errors.New("policy: blocked")
	b.Subscribe("kit.runtime.entity.pre_validated", func(_ context.Context, e bus.Event) error {
		// Veto only when the create payload comes through.
		if p, ok := e.Payload.(domain.PreEntityPayload); ok && p.Op == domain.OpCreate {
			return vetoErr
		}
		return nil
	})

	_, err := m.Create(ctx, "kit-veto-create", CreateOptions{})
	if err == nil {
		t.Fatalf("expected create to be vetoed, got nil error")
	}
	if !errors.Is(err, vetoErr) {
		t.Fatalf("expected error wrapping vetoErr, got %v", err)
	}

	// Also confirm the workspace was NOT persisted.
	if _, err := m.Get(ctx, "kit-veto-create"); !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected workspace to be absent after veto, got %v", err)
	}
}

// TestService_PrePersistedVetoBlocksDelete proves the post-validation,
// pre-write veto seam fires on Delete. This seam carries the validated
// EntityID (delete has no body) so a policy can require --note,
// status==DONE, etc., before letting the row vanish.
func TestService_PrePersistedVetoBlocksDelete(t *testing.T) {
	m, b := newKitTopicTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "kit-veto-delete", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	vetoErr := errors.New("policy: --note required")
	b.Subscribe("kit.runtime.entity.pre_persisted", func(_ context.Context, e bus.Event) error {
		if p, ok := e.Payload.(domain.PreEntityPayload); ok && p.Op == domain.OpDelete {
			return vetoErr
		}
		return nil
	})

	if err := m.Delete(ctx, ws.ID); err == nil {
		t.Fatalf("expected delete to be vetoed, got nil error")
	} else if !errors.Is(err, vetoErr) {
		t.Fatalf("expected error wrapping vetoErr, got %v", err)
	}

	// Workspace must still exist.
	if _, err := m.Get(ctx, ws.ID); err != nil {
		t.Fatalf("expected workspace to still exist after veto, got %v", err)
	}
}

// TestService_SnapshotKitPreEventsFire confirms the snapshot
// service publishes kit pre/post events on Create / Update / Delete.
// The wsm.runtime.snapshot.deleted alias was previously missing
// (ADR-0008 §6); this refactor closes the gap.
func TestService_SnapshotKitPreEventsFire(t *testing.T) {
	m, b := newKitTopicTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "kit-snap-events", CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	var (
		mu     sync.Mutex
		topics []string
	)
	b.Subscribe("kit.runtime.entity.#", func(_ context.Context, e bus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		topics = append(topics, string(e.Topic))
		return nil
	})

	snap, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{Name: "s1"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	newName := "s1-renamed"
	if _, err := m.UpdateSnapshot(ctx, snap.ID, UpdateSnapshotOptions{Name: &newName}); err != nil {
		t.Fatalf("update snapshot: %v", err)
	}
	if err := m.DeleteSnapshot(ctx, snap.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"kit.runtime.entity.pre_validated",
		"kit.runtime.entity.pre_persisted",
		"kit.runtime.entity.created",
		"kit.runtime.entity.pre_validated",
		"kit.runtime.entity.pre_persisted",
		"kit.runtime.entity.updated",
		"kit.runtime.entity.pre_validated",
		"kit.runtime.entity.pre_persisted",
		"kit.runtime.entity.deleted",
	}
	if len(topics) != len(want) {
		t.Fatalf("expected %d kit topics, got %d (%+v)", len(want), len(topics), topics)
	}
	for i, w := range want {
		if topics[i] != w {
			t.Fatalf("topic[%d]=%q, want %q", i, topics[i], w)
		}
	}
}

// TestService_SnapshotDeleteAliasFires proves the new
// wsm.runtime.snapshot.deleted alias is wired (the gap noted in
// ADR-0008 §6 / Critical risk #3 of T-1286).
func TestService_SnapshotDeleteAliasFires(t *testing.T) {
	m, b := newKitTopicTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "snap-delete-alias", CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	snap, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{Name: "to-delete"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	var (
		mu  sync.Mutex
		got []string
	)
	b.Subscribe("wsm.runtime.snapshot.#", func(_ context.Context, e bus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, string(e.Topic))
		return nil
	})

	if err := m.DeleteSnapshot(ctx, snap.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "wsm.runtime.snapshot.deleted" {
		t.Fatalf("expected wsm.runtime.snapshot.deleted alias, got %+v", got)
	}
}
