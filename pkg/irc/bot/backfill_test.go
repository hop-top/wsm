package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc/bot/bottest"
)

// historyServer scripts CHATHISTORY replies in request order.
//
// The fake transport matches scripted replies by command prefix, which cannot
// distinguish one page from the next: successive pages differ only in the
// timestamp selector, which the code under test chooses. Answering in order
// instead lets a test lay out the exact sequence of pages a server would
// return, and afterwards assert on the commands that asked for them.
type historyServer struct {
	*bottest.FakeTransport

	// pages are returned one per CHATHISTORY request, in order. A request
	// past the end yields an empty page, which is what a server says when
	// the history is exhausted.
	pages [][]ircmsg.Message
	// oldest answers the retention probe (the epoch-selector request) when
	// non-nil, ahead of pages.
	oldest []ircmsg.Message

	// requests logs every CHATHISTORY line issued, in order.
	requests []string
	// page counts how many paging requests have been answered.
	page int

	// onSend runs for each CHATHISTORY line before the reply is returned,
	// which is how a test drives the out-of-band batch delivery a server
	// without labeled-response uses.
	onSend func(raw string)
	// delivered guards onSend against emitting the same page twice.
	delivered bool
}

// newHistoryServer builds a transport granting the capabilities a healthy
// session negotiates, including draft/chathistory.
func newHistoryServer(pages ...[]ircmsg.Message) *historyServer {
	return &historyServer{
		FakeTransport: bottest.New("message-tags", "account-tag", "server-time",
			"batch", CapChatHistory, CapLabeledResponse),
		pages: pages,
	}
}

// SendLabeled answers CHATHISTORY from the script and refuses anything else, so
// a stray command fails the test where it was issued.
func (h *historyServer) SendLabeled(ctx context.Context, raw string) ([]ircmsg.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(raw, "CHATHISTORY ") {
		return nil, fmt.Errorf("unexpected command %q", raw)
	}
	h.requests = append(h.requests, raw)

	if h.onSend != nil {
		h.onSend(raw)
	}

	// The retention probe is the epoch selector; it never consumes a page.
	if strings.Contains(raw, "timestamp="+epochSelector) {
		return h.oldest, nil
	}

	if h.page >= len(h.pages) {
		return nil, nil
	}
	page := h.pages[h.page]
	h.page++
	return page, nil
}

// pagingRequests returns the issued lines with the retention probe removed, so
// a test can assert on paging alone.
func (h *historyServer) pagingRequests() []string {
	var out []string
	for _, raw := range h.requests {
		if !strings.Contains(raw, "timestamp="+epochSelector) {
			out = append(out, raw)
		}
	}
	return out
}

// histLine builds a history PRIVMSG from alice carrying a msgid and time, the
// shape probes/chathistory-paging.txt records.
func histLine(msgid string, at time.Time, content string) ircmsg.Message {
	return mustParse(fmt.Sprintf("@time=%s;account=alice;msgid=%s :alice!~u@h.irc PRIVMSG #hist :%s",
		at.UTC().Format(historyTimeFormat), msgid, content))
}

// histPage builds n messages one second apart starting at base, named by
// prefix, which is how a test lays out a page of a given size.
func histPage(prefix string, base time.Time, n int) []ircmsg.Message {
	out := make([]ircmsg.Message, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%d", prefix, i)
		out = append(out, histLine(id, base.Add(time.Duration(i)*time.Second), id))
	}
	return out
}

// baseTime anchors every scripted timestamp.
var baseTime = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

// newBackfillCapture wires a Capture to a recorder and transport, reusing the
// attribution table the capture tests share.
func newBackfillCapture(rec Recorder, tr *historyServer) *Capture {
	c := newCapture(rec, nil)
	c.Transport = tr
	return c
}

// contentsOf lists the recorded message bodies in order.
func contentsOf(t *testing.T, rec *fakeRecorder) []string {
	t.Helper()
	var out []string
	for _, ev := range rec.events {
		out = append(out, decode(t, ev).Content)
	}
	return out
}

// TestBackfillPagesUntilShortPage walks two full pages and a partial one, the
// sequence probes/chathistory-paging.txt recorded, and pins that every message
// lands exactly once in order.
func TestBackfillPagesUntilShortPage(t *testing.T) {
	srv := newHistoryServer(
		histPage("a", baseTime, 3),
		histPage("b", baseTime.Add(10*time.Second), 3),
		histPage("c", baseTime.Add(20*time.Second), 2),
	)
	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	report, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Fetched != 8 || report.Recorded != 8 || report.Skipped != 0 {
		t.Errorf("report = %+v, want 8 fetched, 8 recorded, 0 skipped", report)
	}
	if report.Gap || report.Unsupported {
		t.Errorf("report = %+v, want neither gap nor unsupported", report)
	}

	want := []string{"a-0", "a-1", "a-2", "b-0", "b-1", "b-2", "c-0", "c-1"}
	got := contentsOf(t, rec)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("recorded %v, want %v", got, want)
	}

	// Three pages asked for, and no fourth: the third came back short.
	if n := len(srv.pagingRequests()); n != 3 {
		t.Errorf("issued %d paging requests, want 3: %v", n, srv.pagingRequests())
	}
}

// TestBackfillFullFinalPageStillProbes is finding 1 stated as a test: a final
// page whose size EQUALS the limit is not proof of exhaustion, so the loop must
// issue one more request and only stop on the empty page that follows.
//
// This is the case that silently truncates a backfill when paging stops on a
// full page, and it is invisible in every history that does not divide evenly
// by the page size.
func TestBackfillFullFinalPageStillProbes(t *testing.T) {
	srv := newHistoryServer(
		histPage("a", baseTime, 3),
		histPage("b", baseTime.Add(10*time.Second), 3),
		// Exhausted: the next request returns an empty page.
	)
	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	report, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Recorded != 6 {
		t.Errorf("Recorded = %d, want 6", report.Recorded)
	}

	// Two pages of content plus the request that proved exhaustion. Stopping
	// on the full second page would issue only two.
	requests := srv.pagingRequests()
	if len(requests) != 3 {
		t.Fatalf("issued %d paging requests, want 3 (the last proving exhaustion): %v",
			len(requests), requests)
	}

	// The confirming request must resume past the last message, not repeat
	// the previous window.
	last := baseTime.Add(10*time.Second + 2*time.Second)
	wantSelector := "timestamp=" + last.Add(time.Millisecond).UTC().Format(historyTimeFormat)
	if !strings.Contains(requests[2], wantSelector) {
		t.Errorf("final request = %q, want selector %s", requests[2], wantSelector)
	}
}

// TestBackfillEmptyChannel pins that a channel with no history records nothing,
// reports no gap, and stops after one request.
func TestBackfillEmptyChannel(t *testing.T) {
	srv := newHistoryServer()
	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	report, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Fetched != 0 || report.Recorded != 0 {
		t.Errorf("report = %+v, want nothing fetched or recorded", report)
	}
	if report.Gap {
		t.Error("empty channel reported a gap")
	}
	if n := len(srv.pagingRequests()); n != 1 {
		t.Errorf("issued %d paging requests, want 1", n)
	}
}

// TestBackfillUnsupportedIssuesNoCommands pins the degraded path: without
// draft/chathistory nothing may go on the wire at all, and a cursor that
// existed means the messages since it are unreachable — a gap.
func TestBackfillUnsupportedIssuesNoCommands(t *testing.T) {
	cases := []struct {
		name    string
		cursor  Cursor
		wantGap bool
	}{
		{
			name:    "with a cursor, the messages since it are lost",
			cursor:  Cursor{LastMsgID: "m-1", LastTime: baseTime},
			wantGap: true,
		},
		{
			// Nothing was ever captured, so there is no position to have
			// lost anything from.
			name:    "without a cursor there is nothing to lose",
			cursor:  Cursor{},
			wantGap: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newHistoryServer(histPage("a", baseTime, 3))
			// Everything but chathistory.
			srv.SetCaps("message-tags", "account-tag", "server-time", "batch", CapLabeledResponse)

			rec := &fakeRecorder{}
			c := newBackfillCapture(rec, srv)

			report, err := c.Backfill(context.Background(), "#hist", tc.cursor, BackfillOptions{})
			if err != nil {
				t.Fatalf("Backfill: %v", err)
			}

			if !report.Unsupported {
				t.Error("Unsupported = false, want true")
			}
			if report.Gap != tc.wantGap {
				t.Errorf("Gap = %v, want %v", report.Gap, tc.wantGap)
			}
			if len(srv.requests) != 0 {
				t.Errorf("issued %v, want no commands at all", srv.requests)
			}
			if rec.calls != 0 {
				t.Errorf("recorder called %d times, want 0", rec.calls)
			}
		})
	}
}

// TestBackfillGapDetection covers the retention rule from both sides. The
// quiet-channel case is the one that matters: an old cursor whose history is
// still fully retained is emphatically not a gap, however long ago it was
// written.
func TestBackfillGapDetection(t *testing.T) {
	cursorAt := baseTime.Add(time.Hour)

	cases := []struct {
		name    string
		oldest  []ircmsg.Message
		cursor  Cursor
		wantGap bool
	}{
		{
			// Retention rolled past the cursor: the oldest message the
			// server still holds is NEWER than where capture stopped, so
			// everything between was dropped before it could be fetched.
			name:    "retention rolled past the cursor",
			oldest:  []ircmsg.Message{histLine("old", cursorAt.Add(time.Hour), "oldest retained")},
			cursor:  Cursor{LastMsgID: "m-1", LastTime: cursorAt},
			wantGap: true,
		},
		{
			// A quiet channel: the cursor is ancient in wall-clock terms
			// but the window still reaches back before it, so nothing was
			// lost. A time-based heuristic cries wolf here.
			name:    "quiet channel whose window still covers the cursor",
			oldest:  []ircmsg.Message{histLine("old", cursorAt.Add(-24*time.Hour), "oldest retained")},
			cursor:  Cursor{LastMsgID: "m-1", LastTime: cursorAt},
			wantGap: false,
		},
		{
			// The cursor's own message is still the oldest retained one:
			// continuity, not loss.
			name:    "oldest retained message is the cursor itself",
			oldest:  []ircmsg.Message{histLine("m-1", cursorAt, "the cursor's message")},
			cursor:  Cursor{LastMsgID: "m-1", LastTime: cursorAt},
			wantGap: false,
		},
		{
			name:    "empty channel retains nothing to prove loss",
			oldest:  nil,
			cursor:  Cursor{LastMsgID: "m-1", LastTime: cursorAt},
			wantGap: false,
		},
		{
			name:    "no cursor means no position to lose",
			oldest:  []ircmsg.Message{histLine("old", cursorAt.Add(time.Hour), "oldest retained")},
			cursor:  Cursor{},
			wantGap: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newHistoryServer()
			srv.oldest = tc.oldest

			rec := &fakeRecorder{}
			c := newBackfillCapture(rec, srv)

			report, err := c.Backfill(context.Background(), "#hist", tc.cursor, BackfillOptions{Limit: 3})
			if err != nil {
				t.Fatalf("Backfill: %v", err)
			}
			if report.Gap != tc.wantGap {
				t.Errorf("Gap = %v, want %v", report.Gap, tc.wantGap)
			}
		})
	}
}

// TestBackfillGapProbeUsesEpochSelector pins that the retention probe asks for
// the single oldest retained message, which is what makes the rule a
// measurement rather than a guess.
func TestBackfillGapProbeUsesEpochSelector(t *testing.T) {
	srv := newHistoryServer()
	srv.oldest = []ircmsg.Message{histLine("old", baseTime, "oldest retained")}

	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	if _, err := c.Backfill(context.Background(), "#hist",
		Cursor{LastMsgID: "m-1", LastTime: baseTime}, BackfillOptions{Limit: 3}); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if len(srv.requests) == 0 {
		t.Fatal("issued no commands")
	}
	want := "CHATHISTORY AFTER #hist timestamp=" + epochSelector + " 1"
	if srv.requests[0] != want {
		t.Errorf("first request = %q, want %q", srv.requests[0], want)
	}
}

// TestBackfillSkipsCursorMsgID pins that the message the cursor names is not
// recorded a second time when the server replays it.
func TestBackfillSkipsCursorMsgID(t *testing.T) {
	page := histPage("a", baseTime, 3)
	srv := newHistoryServer(page)
	srv.oldest = []ircmsg.Message{page[0]}

	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	report, err := c.Backfill(context.Background(), "#hist",
		Cursor{LastMsgID: "a-0", LastTime: baseTime}, BackfillOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Skipped != 1 || report.Recorded != 2 {
		t.Errorf("report = %+v, want 1 skipped and 2 recorded", report)
	}
	for _, content := range contentsOf(t, rec) {
		if content == "a-0" {
			t.Error("recorded the message the cursor already names")
		}
	}
}

// seenSet answers HasMsgID from a fixed table, standing in for the daemon's
// backend lookup.
type seenSet struct {
	ids   map[string]bool
	err   error
	since []time.Time
}

func (s *seenSet) HasMsgID(_ context.Context, msgid string, since time.Time) (bool, error) {
	s.since = append(s.since, since)
	if s.err != nil {
		return false, s.err
	}
	return s.ids[msgid], nil
}

// TestBackfillDedupesAgainstSeen pins that messages the event log already holds
// are skipped, which is what keeps a re-run from duplicating history.
func TestBackfillDedupesAgainstSeen(t *testing.T) {
	srv := newHistoryServer(histPage("a", baseTime, 3))
	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	seen := &seenSet{ids: map[string]bool{"a-0": true, "a-2": true}}
	report, err := c.Backfill(context.Background(), "#hist", Cursor{},
		BackfillOptions{Limit: 3, Seen: seen})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Fetched != 3 || report.Recorded != 1 || report.Skipped != 2 {
		t.Errorf("report = %+v, want 3 fetched, 1 recorded, 2 skipped", report)
	}
	if got := contentsOf(t, rec); len(got) != 1 || got[0] != "a-1" {
		t.Errorf("recorded %v, want only a-1", got)
	}
}

// TestBackfillSeenErrorPropagates pins that a failing dedupe lookup stops the
// run rather than recording a message that may already exist: a duplicate in
// the event log cannot be taken back.
func TestBackfillSeenErrorPropagates(t *testing.T) {
	srv := newHistoryServer(histPage("a", baseTime, 3))
	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	wantErr := errors.New("backend down")
	_, err := c.Backfill(context.Background(), "#hist", Cursor{},
		BackfillOptions{Limit: 3, Seen: &seenSet{err: wantErr}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Backfill error = %v, want it to wrap %v", err, wantErr)
	}
	if rec.calls != 0 {
		t.Errorf("recorder called %d times, want 0", rec.calls)
	}
}

// TestBackfillOverlappingPageRecordsOnce pins that a message re-served across
// pages lands once. Paging resumes by timestamp, so a server can legitimately
// repeat the boundary message.
func TestBackfillOverlappingPageRecordsOnce(t *testing.T) {
	first := histPage("a", baseTime, 3)
	// The second page repeats the last message of the first, then adds one.
	second := []ircmsg.Message{
		first[2],
		histLine("a-3", baseTime.Add(3*time.Second), "a-3"),
	}
	srv := newHistoryServer(first, second)

	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	report, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Recorded != 4 || report.Skipped != 1 {
		t.Errorf("report = %+v, want 4 recorded and 1 skipped", report)
	}
	want := []string{"a-0", "a-1", "a-2", "a-3"}
	if got := contentsOf(t, rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("recorded %v, want %v", got, want)
	}
}

// TestBackfillSkipsHistServSynthetics pins that backfill inherits the capture
// path's HistServ skip rather than re-implementing it: recording one would put
// "joined the channel" in the log as something alice said.
func TestBackfillSkipsHistServSynthetics(t *testing.T) {
	page := []ircmsg.Message{
		mustParse("@batch=1;msgid=_j1;time=2026-09-11T10:00:00.000Z :HistServ!HistServ@localhost PRIVMSG #hist :alice [account: alice] joined the channel"),
		histLine("a-0", baseTime.Add(time.Second), "a real message"),
	}
	srv := newHistoryServer(page)

	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	report, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Fetched != 2 || report.Recorded != 1 || report.Skipped != 1 {
		t.Errorf("report = %+v, want 2 fetched, 1 recorded, 1 skipped", report)
	}
	for _, content := range contentsOf(t, rec) {
		if strings.Contains(content, "joined the channel") {
			t.Errorf("recorded a HistServ synthetic: %q", content)
		}
	}
}

// TestBackfillAdvancesCursorPerMessage pins that the cursor tracks the last
// recorded message, so an interrupted run resumes from what actually landed.
func TestBackfillAdvancesCursorPerMessage(t *testing.T) {
	srv := newHistoryServer(histPage("a", baseTime, 2))
	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)
	c.Cursors = NewCursors(t.TempDir() + "/cursors.yaml")

	if _, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 2}); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	cur, ok := c.Cursors.Get("#hist")
	if !ok {
		t.Fatal("no cursor recorded for #hist")
	}
	if cur.LastMsgID != "a-1" {
		t.Errorf("LastMsgID = %q, want a-1", cur.LastMsgID)
	}
	if want := baseTime.Add(time.Second); !cur.LastTime.Equal(want) {
		t.Errorf("LastTime = %v, want %v", cur.LastTime, want)
	}
}

// TestBackfillRecorderErrorStopsRun pins that a failed record aborts rather
// than pressing on: the cursor must not advance past a message that was never
// stored, or the next run skips it forever.
func TestBackfillRecorderErrorStopsRun(t *testing.T) {
	srv := newHistoryServer(histPage("a", baseTime, 3))
	wantErr := errors.New("recorder down")
	rec := &fakeRecorder{err: wantErr}
	c := newBackfillCapture(rec, srv)
	c.Cursors = NewCursors(t.TempDir() + "/cursors.yaml")

	_, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 3})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Backfill error = %v, want it to wrap %v", err, wantErr)
	}
	if _, ok := c.Cursors.Get("#hist"); ok {
		t.Error("cursor advanced despite the record failing")
	}
}

// TestBackfillDefaultSinceOverlapsCursor pins that the default resume point sits
// before the cursor. The cursor is written without fsync, so resuming exactly at
// it can skip messages a crash lost; the overlap is re-fetched and deduped.
func TestBackfillDefaultSinceOverlapsCursor(t *testing.T) {
	cursorAt := baseTime.Add(2 * time.Hour)
	srv := newHistoryServer()
	srv.oldest = []ircmsg.Message{histLine("old", baseTime, "oldest retained")}

	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	if _, err := c.Backfill(context.Background(), "#hist",
		Cursor{LastMsgID: "m-1", LastTime: cursorAt}, BackfillOptions{}); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	requests := srv.pagingRequests()
	if len(requests) == 0 {
		t.Fatal("issued no paging request")
	}
	want := fmt.Sprintf("CHATHISTORY AFTER #hist timestamp=%s %d",
		cursorAt.Add(-defaultSinceSlack).UTC().Format(historyTimeFormat), DefaultBackfillLimit)
	if requests[0] != want {
		t.Errorf("first paging request = %q, want %q", requests[0], want)
	}
}

// TestBackfillSinceOverride pins that a caller-supplied Since chooses the
// resume point, which is what lets an operator force a deeper re-fetch.
func TestBackfillSinceOverride(t *testing.T) {
	srv := newHistoryServer()
	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	from := baseTime.Add(-72 * time.Hour)
	if _, err := c.Backfill(context.Background(), "#hist", Cursor{},
		BackfillOptions{Limit: 5, Since: func(Cursor) time.Time { return from }}); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	requests := srv.pagingRequests()
	if len(requests) == 0 {
		t.Fatal("issued no paging request")
	}
	want := fmt.Sprintf("CHATHISTORY AFTER #hist timestamp=%s 5",
		from.UTC().Format(historyTimeFormat))
	if requests[0] != want {
		t.Errorf("first paging request = %q, want %q", requests[0], want)
	}
}

// TestBackfillCollectsUnlabeledBatch pins the fallback path: on a server
// without labeled-response the reply arrives through SubscribeBatch instead of
// the send's return, and paging must work either way.
func TestBackfillCollectsUnlabeledBatch(t *testing.T) {
	srv := newHistoryServer()
	// No labeled-response: SendLabeled returns nothing to correlate.
	srv.SetCaps("message-tags", "account-tag", "server-time", "batch", CapChatHistory)

	rec := &fakeRecorder{}
	c := newBackfillCapture(rec, srv)

	// Deliver the page out-of-band as the transport would, once the
	// subscription is registered.
	srv.onSend = func(raw string) {
		if strings.Contains(raw, "timestamp="+epochSelector) {
			return
		}
		if srv.delivered {
			return
		}
		srv.delivered = true
		srv.EmitBatch(chatHistoryBatch, histPage("a", baseTime, 2))
	}

	report, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if report.Recorded != 2 {
		t.Errorf("Recorded = %d, want 2", report.Recorded)
	}
	want := []string{"a-0", "a-1"}
	if got := contentsOf(t, rec); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("recorded %v, want %v", got, want)
	}
}

// TestBackfillNoTransport pins that a Capture without a transport reports the
// misconfiguration rather than panicking.
func TestBackfillNoTransport(t *testing.T) {
	c := newCapture(&fakeRecorder{}, nil)
	if _, err := c.Backfill(context.Background(), "#hist", Cursor{}, BackfillOptions{}); err == nil {
		t.Fatal("Backfill succeeded without a transport, want an error")
	}
}
