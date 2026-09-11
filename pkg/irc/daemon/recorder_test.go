package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/wsm/pkg/backend"
	sqlitebackend "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/model"
)

// newBackend opens a real sqlite backend in a temp dir.
//
// The Seen paging walk depends on exactly how ListEvents orders and bounds its
// results, so a hand-rolled fake would only re-assert this test's own beliefs
// about the backend. Running against the real implementation is what makes the
// paging assertions evidence rather than tautology.
func newBackend(t *testing.T) backend.Backend {
	t.Helper()
	b, err := sqlitebackend.Open(filepath.Join(t.TempDir(), "wsm.db"))
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return b
}

// newWorkspace creates the workspace row AppendEvent requires.
func newWorkspace(t *testing.T, b backend.Backend, id string) {
	t.Helper()
	now := time.Now().UTC()
	ws := model.Workspace{ID: id, Name: id, Status: model.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := b.CreateWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
}

// messageEvent builds a capture-shaped message event.
func messageEvent(wsID, msgid string, ts time.Time) model.Event {
	data, err := json.Marshal(model.MessageData{
		Role:        "member",
		Content:     "hello " + msgid,
		Format:      "text/irc",
		Channel:     "#ws",
		Nick:        "someone",
		MsgID:       msgid,
		Attribution: "account",
	})
	if err != nil {
		panic(err)
	}
	return model.Event{
		WorkspaceID: wsID,
		Type:        model.EventInteractionMessage,
		Timestamp:   ts,
		Data:        data,
	}
}

// TestRecorderSetsWriterIDBeforeAlign pins the ordering that makes the "irc:"
// writer id durable.
//
// owsf.DefaultAndAlign fills an EMPTY WriterID with its own OASF-prefixed
// default, so a recorder that aligned first and assigned afterwards would
// either be overwritten or have to fight the aligner. The recorded event must
// carry the irc form, and must not carry the OASF default.
func TestRecorderSetsWriterIDBeforeAlign(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	newWorkspace(t, b, "w1")

	rec := NewRecorder(b, "w1", "dev-1")
	stored, err := rec.Record(ctx, messageEvent("w1", "m1", time.Now().UTC()))
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	want := WriterID("w1")
	if stored.WriterID != want {
		t.Errorf("WriterID = %q, want %q", stored.WriterID, want)
	}
	if strings.Contains(stored.WriterID, "wsm-internal") {
		t.Errorf("WriterID = %q: the OASF default overwrote the irc writer id", stored.WriterID)
	}
	if stored.DeviceID != "dev-1" {
		t.Errorf("DeviceID = %q, want %q", stored.DeviceID, "dev-1")
	}
}

// TestRecorderPreservesTimestampAndAgent checks the two fields the capture
// path resolves per message and that Manager.RecordEvent would have clobbered.
func TestRecorderPreservesTimestampAndAgent(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	newWorkspace(t, b, "w1")

	ts := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	ev := messageEvent("w1", "m1", ts)
	ev.AgentID = "profile-alice"

	rec := NewRecorder(b, "w1", "dev-1")
	stored, err := rec.Record(ctx, ev)
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	if !stored.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %s, want %s", stored.Timestamp, ts)
	}
	if stored.AgentID != "profile-alice" {
		t.Errorf("AgentID = %q, want %q", stored.AgentID, "profile-alice")
	}
}

// TestSeenFindsNewestBeyondOnePage is the paging boundary.
//
// ListEvents orders ascending and applies Limit afterwards, so a limit-based
// walk returns the OLDEST page of the log and never reaches recent messages.
// With more events than one page holds, the message a backfill is most likely
// to be deduping against is the newest one — which is exactly the one such a
// walk misses.
func TestSeenFindsNewestBeyondOnePage(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	newWorkspace(t, b, "w1")

	rec := NewRecorder(b, "w1", "dev-1")
	base := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	const total = 25
	for i := range total {
		ev := messageEvent("w1", fmt.Sprintf("m%02d", i), base.Add(time.Duration(i)*time.Minute))
		if _, err := rec.Record(ctx, ev); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	seen := NewSeen(b, "w1")
	seen.pageSize = 5 // several windows, so the walk has to page

	// The newest message: unreachable by an oldest-first limited walk.
	found, err := seen.HasMsgID(ctx, "m24", time.Time{})
	if err != nil {
		t.Fatalf("HasMsgID newest: %v", err)
	}
	if !found {
		t.Error("HasMsgID(newest) = false, want true: the walk never reached the end of the log")
	}

	// A message in the middle, to show the walk does not merely check the
	// last window either.
	found, err = seen.HasMsgID(ctx, "m12", time.Time{})
	if err != nil {
		t.Fatalf("HasMsgID middle: %v", err)
	}
	if !found {
		t.Error("HasMsgID(middle) = false, want true")
	}

	// The oldest message, which every walk reaches.
	found, err = seen.HasMsgID(ctx, "m00", time.Time{})
	if err != nil {
		t.Fatalf("HasMsgID oldest: %v", err)
	}
	if !found {
		t.Error("HasMsgID(oldest) = false, want true")
	}
}

// TestSeenUnknownMsgID checks the negative answer terminates rather than
// looping over the window walk forever.
func TestSeenUnknownMsgID(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	newWorkspace(t, b, "w1")

	rec := NewRecorder(b, "w1", "dev-1")
	base := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	for i := range 12 {
		ev := messageEvent("w1", fmt.Sprintf("m%02d", i), base.Add(time.Duration(i)*time.Minute))
		if _, err := rec.Record(ctx, ev); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	seen := NewSeen(b, "w1")
	seen.pageSize = 4

	found, err := seen.HasMsgID(ctx, "nope", time.Time{})
	if err != nil {
		t.Fatalf("HasMsgID: %v", err)
	}
	if found {
		t.Error("HasMsgID(unknown) = true, want false")
	}
}

// TestSeenStopsAtSince checks the walk stops once a window predates the
// backfill's horizon, instead of scanning the whole log.
func TestSeenStopsAtSince(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	newWorkspace(t, b, "w1")

	rec := NewRecorder(b, "w1", "dev-1")
	base := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	for i := range 20 {
		ev := messageEvent("w1", fmt.Sprintf("m%02d", i), base.Add(time.Duration(i)*time.Minute))
		if _, err := rec.Record(ctx, ev); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	seen := NewSeen(b, "w1")
	seen.pageSize = 5

	// m00 is older than the horizon, so the bounded walk must report it as
	// not seen rather than paging all the way back to it.
	since := base.Add(15 * time.Minute)
	found, err := seen.HasMsgID(ctx, "m00", since)
	if err != nil {
		t.Fatalf("HasMsgID: %v", err)
	}
	if found {
		t.Error("HasMsgID(older than since) = true, want false: the walk ignored its horizon")
	}

	// Something inside the horizon is still found.
	found, err = seen.HasMsgID(ctx, "m18", since)
	if err != nil {
		t.Fatalf("HasMsgID recent: %v", err)
	}
	if !found {
		t.Error("HasMsgID(within since) = false, want true")
	}
}

// TestSeenEmptyMsgID checks an absent id is never a hit: treating it as one
// would silently drop a real message from a backfill.
func TestSeenEmptyMsgID(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	newWorkspace(t, b, "w1")

	seen := NewSeen(b, "w1")
	found, err := seen.HasMsgID(ctx, "", time.Time{})
	if err != nil {
		t.Fatalf("HasMsgID: %v", err)
	}
	if found {
		t.Error("HasMsgID(\"\") = true, want false")
	}
}
