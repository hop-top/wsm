package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/backend"
	sqlitebackend "hop.top/wsm/pkg/backend/sqlite"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	m, err := NewManager(b)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func TestManager_Create(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "my-project", CreateOptions{Description: "desc", Tags: []string{"one", "two"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ws.ID == "" {
		t.Fatalf("expected generated workspace id, got empty")
	}
	if ws.Status != StatusActive {
		t.Fatalf("expected active status, got %s", ws.Status)
	}
	if ws.Description != "desc" {
		t.Fatalf("expected description to round-trip, got %q", ws.Description)
	}
	if len(ws.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(ws.Tags))
	}
}

func TestManager_Create_Duplicate(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "my-project", CreateOptions{}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := m.Create(ctx, "my-project", CreateOptions{}); !IsKind(err, ErrorKindConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestManager_Create_UniqueNameConflict(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "same-name", CreateOptions{}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := m.Create(ctx, "same-name", CreateOptions{}); !IsKind(err, ErrorKindConflict) {
		t.Fatalf("expected unique-name conflict, got %v", err)
	}
}

func TestManager_List(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, "bravo", CreateOptions{}); err != nil {
		t.Fatalf("create bravo: %v", err)
	}
	if _, err := m.Create(ctx, "alpha", CreateOptions{}); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	items, err := m.List(ctx, backend.ListOptions{SortBy: "name", SortDir: "asc"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "alpha" || items[1].Name != "bravo" {
		t.Fatalf("expected alphabetical ordering, got %q then %q", items[0].Name, items[1].Name)
	}
}

func TestManager_List_FilterStatus(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	active, err := m.Create(ctx, "active-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create active: %v", err)
	}
	archived, err := m.Create(ctx, "archived-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create archived: %v", err)
	}
	if err := m.Archive(ctx, archived.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	activeStatus := StatusActive
	activeItems, err := m.List(ctx, backend.ListOptions{Status: &activeStatus})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(activeItems) != 1 || activeItems[0].ID != active.ID {
		t.Fatalf("unexpected active filter output: %+v", activeItems)
	}

	archivedStatus := StatusArchived
	archivedItems, err := m.List(ctx, backend.ListOptions{Status: &archivedStatus})
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(archivedItems) != 1 || archivedItems[0].ID != archived.ID {
		t.Fatalf("unexpected archived filter output: %+v", archivedItems)
	}
}

func TestManager_Get(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	created, err := m.Create(ctx, "my-project", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := m.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != created.ID || got.Name != "my-project" {
		t.Fatalf("unexpected workspace from get: %+v", got)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Get(context.Background(), "does-not-exist")
	if !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestManager_Archive(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	created, err := m.Create(ctx, "to-archive", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Archive(ctx, created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	archived, err := m.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if archived.Status != StatusArchived {
		t.Fatalf("expected archived status, got %s", archived.Status)
	}
}

func TestManager_Archive_AlreadyArchived(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	created, err := m.Create(ctx, "to-archive", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Archive(ctx, created.ID); err != nil {
		t.Fatalf("archive first: %v", err)
	}
	if err := m.Archive(ctx, created.ID); !IsKind(err, ErrorKindConflict) {
		t.Fatalf("expected conflict on second archive, got %v", err)
	}
}

func TestManager_Unarchive(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	created, err := m.Create(ctx, "to-unarchive", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Archive(ctx, created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := m.Unarchive(ctx, created.ID); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	active, err := m.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if active.Status != StatusActive {
		t.Fatalf("expected active status, got %s", active.Status)
	}
}

func TestManager_Delete(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "delete-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "x"}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if _, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	if err := m.Delete(ctx, ws.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := m.Get(ctx, ws.ID); !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected deleted workspace to be not found, got %v", err)
	}
}

func TestManager_Delete_NotFound(t *testing.T) {
	m := newTestManager(t)
	err := m.Delete(context.Background(), "does-not-exist")
	if !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected not-found delete error, got %v", err)
	}
}

func TestManager_ResolveRef(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "my-project", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	byID, err := m.ResolveWorkspaceRef(ctx, ws.ID)
	if err != nil {
		t.Fatalf("resolve by id: %v", err)
	}
	if byID.ID != ws.ID {
		t.Fatalf("resolved wrong workspace by id: %s", byID.ID)
	}
	byName, err := m.ResolveWorkspaceRef(ctx, ws.Name)
	if err != nil {
		t.Fatalf("resolve by name: %v", err)
	}
	if byName.ID != ws.ID {
		t.Fatalf("resolved wrong workspace by name: %s", byName.ID)
	}
}

func TestManager_RecordEvent(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "event-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ev, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("record event: %v", err)
	}
	if ev.ID == "" {
		t.Fatalf("expected ulid event id, got empty")
	}
	if ev.SeqNum == 0 {
		t.Fatalf("expected non-zero sequence number")
	}
	if ev.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}

func TestManager_RecordEvent_SeqNum(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "seq-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var seqs []uint64
	for i := 0; i < 5; i++ {
		ev, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: fmt.Sprintf("msg-%d", i)})
		if err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
		seqs = append(seqs, ev.SeqNum)
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			t.Fatalf("expected monotonic seq_num increments, got %d then %d", seqs[i-1], seqs[i])
		}
	}
}

func TestManager_Events_Filtering(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "my-project", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("record message: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventToolCallStarted, ToolCallData{ToolName: "grep"}); err != nil {
		t.Fatalf("record tool: %v", err)
	}

	events, err := m.Events(ctx, ws.ID, backend.EventListOptions{Types: []EventType{EventMessageAdded}})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventMessageAdded {
		t.Fatalf("unexpected filtered events: %+v", events)
	}
}

func TestManager_RecordEvent_Archived(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "my-project", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Archive(ctx, ws.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "x"}); !IsKind(err, ErrorKindConflict) {
		t.Fatalf("expected conflict recording archived workspace event, got %v", err)
	}
}

func TestManager_EventIntegrity_Sequential(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "integrity-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 250; i++ {
		if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: fmt.Sprintf("m-%d", i)}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	events, err := m.Events(ctx, ws.ID, backend.EventListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for i := 1; i < len(events); i++ {
		if events[i].SeqNum != events[i-1].SeqNum+1 {
			t.Fatalf("expected contiguous seq numbers, got %d then %d", events[i-1].SeqNum, events[i].SeqNum)
		}
	}
}

func TestManager_EventIntegrity_Filter(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "integrity-filter-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: fmt.Sprintf("m-%d", i)}); err != nil {
			t.Fatalf("record message: %v", err)
		}
		if _, err := m.RecordEvent(ctx, ws.ID, EventToolCallStarted, ToolCallData{ToolName: fmt.Sprintf("tool-%d", i)}); err != nil {
			t.Fatalf("record tool: %v", err)
		}
	}
	events, err := m.Events(ctx, ws.ID, backend.EventListOptions{Types: []EventType{EventMessageAdded}})
	if err != nil {
		t.Fatalf("filtered events: %v", err)
	}
	if len(events) != 10 {
		t.Fatalf("expected 10 filtered message events, got %d", len(events))
	}
	for _, ev := range events {
		if ev.Type != EventMessageAdded {
			t.Fatalf("unexpected filtered type: %s", ev.Type)
		}
	}
}

func TestManager_FullLifecycle(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "full-lifecycle", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "first"}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if _, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{}); err != nil {
		t.Fatalf("snapshot: %v", err)
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
	if _, err := m.Get(ctx, ws.ID); !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected deleted workspace to be absent, got %v", err)
	}
}

func TestManager_APIContract(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "api-contract", CreateOptions{Metadata: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := m.ResolveWorkspaceRef(ctx, ws.Name); err != nil {
		t.Fatalf("resolve by name: %v", err)
	}
	if _, err := m.ResolveWorkspaceRef(ctx, ws.ID); err != nil {
		t.Fatalf("resolve by id: %v", err)
	}

	if _, err := m.RecordEvent(ctx, ws.ID, EventFileChanged, FileChangeData{Path: "a.txt", Operation: "modify"}); err != nil {
		t.Fatalf("record event: %v", err)
	}

	if _, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if count, err := m.EventCount(ctx, ws.ID); err != nil || count == 0 {
		t.Fatalf("event count contract failed: count=%d err=%v", count, err)
	}
	if snaps, err := m.SnapshotCount(ctx, ws.ID); err != nil || snaps == 0 {
		t.Fatalf("snapshot count contract failed: count=%d err=%v", snaps, err)
	}
}

func newTestManagerWithOpts(t *testing.T, opts ...ManagerOption) *Manager {
	t.Helper()
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	m, err := NewManager(b, opts...)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func newTestManagerWithOptsAndBackend(t *testing.T, b backend.Backend, opts ...ManagerOption) *Manager {
	t.Helper()
	m, err := NewManager(b, opts...)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func TestManager_RecordEvent_SessionAgentID(t *testing.T) {
	m := newTestManagerWithOpts(t, WithSessionID("sess-1"), WithAgentID("agent-1"))
	ctx := context.Background()

	ws, err := m.Create(ctx, "session-agent-work", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ev, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("record event: %v", err)
	}
	if ev.SessionID != "sess-1" {
		t.Fatalf("expected session_id=sess-1, got %q", ev.SessionID)
	}
	if ev.AgentID != "agent-1" {
		t.Fatalf("expected agent_id=agent-1, got %q", ev.AgentID)
	}

	// Verify persistence via listing
	events, err := m.Events(ctx, ws.ID, backend.EventListOptions{Types: []EventType{EventMessageAdded}})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 message event, got %d", len(events))
	}
	if events[0].SessionID != "sess-1" || events[0].AgentID != "agent-1" {
		t.Fatalf("persisted event lost session/agent IDs: session=%q agent=%q", events[0].SessionID, events[0].AgentID)
	}
}

func TestManager_DeviceAgentSessionAttribution(t *testing.T) {
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	m1 := newTestManagerWithOptsAndBackend(t, b, WithSessionID("sess-a"), WithAgentID("bot-1"))
	m2 := newTestManagerWithOptsAndBackend(t, b, WithSessionID("sess-b"), WithAgentID("bot-1"))
	ctx := context.Background()

	ws, err := m1.Create(ctx, "multi-session", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := m1.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "from-a"}); err != nil {
		t.Fatalf("m1 record: %v", err)
	}
	if _, err := m2.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "from-b"}); err != nil {
		t.Fatalf("m2 record: %v", err)
	}

	events, err := m1.Events(ctx, ws.ID, backend.EventListOptions{Types: []EventType{EventMessageAdded}})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 message events, got %d", len(events))
	}

	// Both share the same device and agent, but different sessions
	if events[0].AgentID != "bot-1" || events[1].AgentID != "bot-1" {
		t.Fatalf("expected both events to have agent_id=bot-1, got %q and %q", events[0].AgentID, events[1].AgentID)
	}
	if events[0].DeviceID != events[1].DeviceID {
		t.Fatalf("expected same device_id, got %q and %q", events[0].DeviceID, events[1].DeviceID)
	}
	if events[0].SessionID != "sess-a" || events[1].SessionID != "sess-b" {
		t.Fatalf("expected different session_ids (sess-a, sess-b), got %q and %q", events[0].SessionID, events[1].SessionID)
	}
}

func TestManager_CrossSessionAttribution(t *testing.T) {
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mA := newTestManagerWithOptsAndBackend(t, b, WithSessionID("ide-a"), WithAgentID("vscode"))
	mB := newTestManagerWithOptsAndBackend(t, b, WithSessionID("ide-b"), WithAgentID("cursor"))
	ctx := context.Background()

	ws, err := mA.Create(ctx, "cross-session", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := mA.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: fmt.Sprintf("a-%d", i)}); err != nil {
			t.Fatalf("mA record %d: %v", i, err)
		}
		if _, err := mB.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: fmt.Sprintf("b-%d", i)}); err != nil {
			t.Fatalf("mB record %d: %v", i, err)
		}
	}

	events, err := mA.Events(ctx, ws.ID, backend.EventListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	// Verify monotonic seq_num across both sessions
	for i := 1; i < len(events); i++ {
		if events[i].SeqNum <= events[i-1].SeqNum {
			t.Fatalf("expected monotonic seq_num, got %d then %d", events[i-1].SeqNum, events[i].SeqNum)
		}
	}

	// Verify both session IDs are present
	sessions := map[string]int{}
	for _, ev := range events {
		sessions[ev.SessionID]++
	}
	if sessions["ide-a"] == 0 || sessions["ide-b"] == 0 {
		t.Fatalf("expected events from both sessions, got distribution: %v", sessions)
	}
}

func TestManager_ManagerOptionsContract(t *testing.T) {
	// Zero-value options
	m0 := newTestManagerWithOpts(t)
	ctx := context.Background()
	ws, err := m0.Create(ctx, "opts-zero", CreateOptions{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ev, err := m0.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "x"})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if ev.SessionID != "" || ev.AgentID != "" {
		t.Fatalf("expected empty session/agent IDs with zero options, got session=%q agent=%q", ev.SessionID, ev.AgentID)
	}

	// With options
	m1 := newTestManagerWithOpts(t, WithSessionID("s"), WithAgentID("a"))
	ws2, err := m1.Create(ctx, "opts-set", CreateOptions{})
	if err != nil {
		t.Fatalf("create with opts: %v", err)
	}
	ev2, err := m1.RecordEvent(ctx, ws2.ID, EventMessageAdded, MessageData{Role: "user", Content: "y"})
	if err != nil {
		t.Fatalf("record with opts: %v", err)
	}
	if ev2.SessionID != "s" || ev2.AgentID != "a" {
		t.Fatalf("expected session=s agent=a, got session=%q agent=%q", ev2.SessionID, ev2.AgentID)
	}
}
