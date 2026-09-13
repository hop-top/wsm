package bot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCursorsRoundTrip pins that a saved set reloads identically, including
// the timestamps a backfill resumes from.
func TestCursorsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cursors.yaml")

	saved := NewCursors(path)
	when := time.Date(2026, 9, 11, 10, 22, 10, 885000000, time.UTC)
	saved.Set("#dev", Cursor{LastMsgID: "abc", LastTime: when})
	saved.Set("#ops", Cursor{LastMsgID: "def", LastTime: when.Add(time.Minute)})

	if err := saved.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadCursors(path)
	if err != nil {
		t.Fatalf("LoadCursors: %v", err)
	}

	dev, ok := loaded.Get("#dev")
	if !ok {
		t.Fatal("#dev missing after reload")
	}
	if dev.LastMsgID != "abc" || !dev.LastTime.Equal(when) {
		t.Errorf("#dev = %+v, want msgid abc at %s", dev, when)
	}
	if got := len(loaded.All()); got != 2 {
		t.Errorf("reloaded %d channels, want 2", got)
	}
}

// TestLoadCursorsMissingFile pins that a first run is not an error.
func TestLoadCursorsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", "cursors.yaml")

	c, err := LoadCursors(path)
	if err != nil {
		t.Fatalf("LoadCursors on a missing file: %v", err)
	}
	if got := len(c.All()); got != 0 {
		t.Errorf("loaded %d channels from a missing file, want 0", got)
	}
	if _, ok := c.Get("#dev"); ok {
		t.Error("Get reported a position that was never recorded")
	}
}

// TestLoadCursorsMalformed pins that a corrupt file is reported rather than
// silently treated as empty, which would restart every backfill from zero.
func TestLoadCursorsMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cursors.yaml")
	if err := os.WriteFile(path, []byte("::not a mapping::\n\t- broken"), cursorFileMode); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := LoadCursors(path); err == nil {
		t.Fatal("LoadCursors on a malformed file returned nil, want an error")
	}
}

// TestCursorsSaveIsAtomic pins that the write leaves no temporary file behind
// and lands at owner-only permissions.
func TestCursorsSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cursors.yaml")

	c := NewCursors(path)
	c.Set("#dev", Cursor{LastMsgID: "abc", LastTime: time.Now().UTC()})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "cursors.yaml" {
		t.Errorf("directory holds %d entries, want only cursors.yaml", len(entries))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != cursorFileMode {
		t.Errorf("cursor file mode = %o, want %o", perm, cursorFileMode)
	}
}

// TestCursorsSaveCreatesParentDir pins that the first save works before
// anything else has made the workspace's IRC directory.
func TestCursorsSaveCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "irc", "ws-1", "cursors.yaml")

	c := NewCursors(path)
	c.Set("#dev", Cursor{LastMsgID: "abc"})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat after save: %v", err)
	}
}

// TestCaptureAdvancesCursor pins that a recorded message moves the channel's
// position, which is what stops a backfill re-fetching it.
func TestCaptureAdvancesCursor(t *testing.T) {
	rec := &fakeRecorder{}
	c := newCapture(rec, nil)
	c.Cursors = NewCursors(filepath.Join(t.TempDir(), "cursors.yaml"))

	if _, err := c.Handle(context.Background(),
		mustParse("@time=2026-09-11T10:22:10.885Z;account=alice;msgid=pxsd :alice!~u@h.irc PRIVMSG #tags :hi")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got, ok := c.Cursors.Get("#tags")
	if !ok {
		t.Fatal("#tags has no cursor after a recorded message")
	}
	if got.LastMsgID != "pxsd" {
		t.Errorf("LastMsgID = %q, want the recorded msgid", got.LastMsgID)
	}
	want := time.Date(2026, 9, 11, 10, 22, 10, 885000000, time.UTC)
	if !got.LastTime.Equal(want) {
		t.Errorf("LastTime = %s, want %s", got.LastTime, want)
	}
}

// TestCaptureRecorderErrorLeavesCursorUnchanged is the guard that keeps a
// failed write recoverable. If the cursor advanced past a message the recorder
// rejected, the next backfill would resume after it and that message would be
// lost from the log permanently, with nothing to indicate it ever existed.
func TestCaptureRecorderErrorLeavesCursorUnchanged(t *testing.T) {
	rec := &fakeRecorder{}
	c := newCapture(rec, nil)
	c.Cursors = NewCursors(filepath.Join(t.TempDir(), "cursors.yaml"))

	// A first message succeeds, establishing a known position.
	if _, err := c.Handle(context.Background(),
		mustParse("@time=2026-09-11T10:22:10.885Z;account=alice;msgid=first :alice!~u@h.irc PRIVMSG #tags :one")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	before, ok := c.Cursors.Get("#tags")
	if !ok {
		t.Fatal("no cursor after the first message")
	}

	// The next record fails.
	rec.err = errors.New("backend unavailable")
	recorded, err := c.Handle(context.Background(),
		mustParse("@time=2026-09-11T10:22:20.885Z;account=alice;msgid=second :alice!~u@h.irc PRIVMSG #tags :two"))
	if err == nil {
		t.Fatal("Handle returned nil on a failing recorder, want an error")
	}
	if recorded {
		t.Error("Handle reported the message recorded despite the failure")
	}

	after, ok := c.Cursors.Get("#tags")
	if !ok {
		t.Fatal("the cursor disappeared after a failed record")
	}
	if after != before {
		t.Errorf("cursor moved to %+v on a failed record, want it left at %+v", after, before)
	}
}

// TestCaptureSkippedMessageLeavesCursorUnchanged pins that a skipped message
// (here a HistServ join) does not move the position either.
func TestCaptureSkippedMessageLeavesCursorUnchanged(t *testing.T) {
	rec := &fakeRecorder{}
	c := newCapture(rec, nil)
	c.Cursors = NewCursors(filepath.Join(t.TempDir(), "cursors.yaml"))

	if _, err := c.Handle(context.Background(),
		mustParse(":HistServ!HistServ@localhost PRIVMSG #tags :alice [account: alice] joined the channel")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if _, ok := c.Cursors.Get("#tags"); ok {
		t.Error("a skipped message moved the cursor")
	}
}

// TestCaptureNilCursorsIsSafe pins the nil-safety the field documents.
func TestCaptureNilCursorsIsSafe(t *testing.T) {
	rec := &fakeRecorder{}
	c := newCapture(rec, nil)
	c.Cursors = nil

	recorded, err := c.Handle(context.Background(),
		mustParse("@time=2026-09-11T10:22:10.885Z;account=alice;msgid=pxsd :alice!~u@h.irc PRIVMSG #tags :hi"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !recorded {
		t.Error("message was skipped, want it recorded without cursors")
	}
}
