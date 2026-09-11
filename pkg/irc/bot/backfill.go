package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/model"
)

// DefaultBackfillLimit is how many messages one CHATHISTORY page asks for. It
// matches the ceiling wsm renders into a managed ergo's
// chathistory-maxmessages, so a page is never trimmed by the server to a size
// the paging loop would misread.
const DefaultBackfillLimit = 1000

// defaultSinceSlack is how far before the cursor a backfill restarts when the
// caller supplies no Since. The overlap is deliberate: the cursor is written
// without fsync, so a crash can leave it fractionally ahead of what was
// actually recorded. Re-fetching an hour and deduping on MsgID is cheap;
// resuming exactly at the cursor and being wrong loses messages permanently.
const defaultSinceSlack = time.Hour

// chatHistoryBatch is the BATCH type ergo tags a CHATHISTORY replay with.
const chatHistoryBatch = "chathistory"

// historyTimeFormat is the timestamp spelling CHATHISTORY selectors take:
// RFC3339 with exactly milliseconds, which is what ergo emits and accepts.
const historyTimeFormat = "2006-01-02T15:04:05.000Z"

// epochSelector is the "older than anything" timestamp used to fetch the
// single oldest message the server still retains. It is what makes gap
// detection a retention measurement rather than a guess about elapsed time.
const epochSelector = "1970-01-01T00:00:00.000Z"

// Seen reports whether a message is already in the event log. The daemon
// implements it against the backend; a nil Seen simply means the only dedupe
// available is the cursor's own last id.
type Seen interface {
	// HasMsgID reports whether msgid is already recorded. since bounds how
	// far back the implementation must look, so it can stop paging the
	// backend once it is older than anything this backfill could return.
	HasMsgID(ctx context.Context, msgid string, since time.Time) (bool, error)
}

// BackfillOptions tunes one backfill run. The zero value is usable and yields
// the documented defaults.
type BackfillOptions struct {
	// Limit is how many messages one page requests. Zero means
	// DefaultBackfillLimit.
	Limit int
	// Since derives the instant to resume from. Nil means one hour before
	// the cursor's LastTime; see defaultSinceSlack for why it overlaps.
	Since func(Cursor) time.Time
	// Seen dedupes against messages already recorded. Optional.
	Seen Seen
}

// limit resolves the page size.
func (o BackfillOptions) limit() int {
	if o.Limit > 0 {
		return o.Limit
	}
	return DefaultBackfillLimit
}

// since resolves the instant to resume from.
func (o BackfillOptions) since(cur Cursor) time.Time {
	if o.Since != nil {
		return o.Since(cur)
	}
	return cur.LastTime.Add(-defaultSinceSlack)
}

// Report is the outcome of one channel's backfill.
type Report struct {
	// Channel is the channel backfilled.
	Channel string
	// Fetched counts the messages the server returned, including ones that
	// were skipped.
	Fetched int
	// Recorded counts the messages that reached the event log.
	Recorded int
	// Skipped counts messages deliberately not recorded: duplicates the
	// cursor or Seen already knows, and everything the capture path drops
	// (HistServ synthetics, the bot's own echo, private messages).
	Skipped int
	// Gap reports that the server's retention window rolled past the
	// cursor, so messages between the cursor and the oldest retained
	// message are lost. See Backfill for why this is never a time
	// heuristic.
	Gap bool
	// Unsupported reports that the server never granted draft/chathistory,
	// so no history could be requested at all.
	Unsupported bool
}

// Backfill replays a channel's history into the event log, resuming from cur.
//
// Everything it records goes through Capture's own path, so a backfilled
// message is attributed, deduped and degraded exactly as a live one is — and
// the HistServ synthetics ergo injects into a history batch are skipped there
// rather than re-checked here.
//
// Without draft/chathistory it issues no commands at all and reports
// Unsupported. That is not the same as "nothing to do": if a cursor existed,
// the messages since it are unreachable, so the run is also a gap.
//
// # Gap detection
//
// A gap is a retention measurement, never a time heuristic. Before paging,
// Backfill asks for the single oldest message the server still holds. The
// window rolled past the cursor only when that message is newer than the
// cursor itself — which is precisely the condition under which messages
// between the two were dropped by the server before anyone could fetch them.
//
// Deriving it from elapsed time instead ("the cursor is older than an hour, so
// assume loss") reports a gap on every channel that was merely quiet, which
// trains the operator to ignore the flag. A quiet channel is not a gap: its
// history is intact and simply short. An empty channel is not a gap either,
// because there is no retained message that could prove loss.
//
// # Paging
//
// Paging stops on a short or empty page, never on a full one. A page whose
// size equals the limit says nothing about exhaustion — it is exactly what a
// server returns when more history remains, and also what it returns for the
// final page that happens to divide evenly. Only a page that comes back short
// proves the end, so an extra request confirming it is the cost of not
// silently truncating a backfill. Verified on ergo 2.19.1; see
// probes/chathistory-paging.txt.
func (c *Capture) Backfill(ctx context.Context, channel string, cur Cursor, opts BackfillOptions) (Report, error) {
	report := Report{Channel: channel}

	// A cursor with neither an id nor a time has never been written: this is
	// a first run, so there is no position for retention to have rolled past
	// and nothing to dedupe against.
	cursorExisted := cur.LastMsgID != "" || !cur.LastTime.IsZero()

	if c.Transport == nil {
		return report, fmt.Errorf("irc/bot: backfill has no transport")
	}
	if !c.Transport.Negotiated().Has(CapChatHistory) {
		// No history to ask for. If a cursor existed, whatever was said
		// since it is unreachable, which is a gap by any definition.
		report.Unsupported = true
		report.Gap = cursorExisted
		return report, nil
	}

	// Collect batches delivered out-of-band, for a server without
	// labeled-response where SendLabeled cannot correlate the reply itself.
	var collector batchCollector
	cancel := c.Transport.SubscribeBatch(chatHistoryBatch, collector.add)
	defer cancel()

	gap, err := c.detectGap(ctx, channel, cur, cursorExisted, &collector)
	if err != nil {
		return report, err
	}
	report.Gap = gap

	if err := c.page(ctx, channel, cur, opts, &collector, &report); err != nil {
		return report, err
	}
	return report, nil
}

// detectGap reports whether the retention window rolled past the cursor.
//
// It fetches the single oldest message the server still retains and compares
// it to the cursor. Without a cursor there is no position to lose, and an
// empty channel yields no message to compare, so both are "no gap" rather than
// an unknown.
func (c *Capture) detectGap(ctx context.Context, channel string, cur Cursor, cursorExisted bool, collector *batchCollector) (bool, error) {
	if !cursorExisted {
		return false, nil
	}

	oldest, err := c.fetchPage(ctx, channel, epochSelector, 1, collector)
	if err != nil {
		return false, err
	}
	if len(oldest) == 0 {
		// Nothing retained at all: an empty channel cannot have lost
		// anything.
		return false, nil
	}

	// Strictly newer: a retained message at exactly the cursor's instant is
	// the cursor's own message still in the window, which is continuity
	// rather than loss.
	return messageTime(oldest[0]).After(cur.LastTime), nil
}

// page walks the history forward from the resume point, recording as it goes.
func (c *Capture) page(ctx context.Context, channel string, cur Cursor, opts BackfillOptions, collector *batchCollector, report *Report) error {
	var (
		limit = opts.limit()
		from  = opts.since(cur)
		// seen guards against a server that re-serves a message inside the
		// same run: paging advances by timestamp, so messages sharing the
		// final instant of a page reappear in the next one.
		seen = map[string]bool{}
	)

	for {
		msgs, err := c.fetchPage(ctx, channel, from.UTC().Format(historyTimeFormat), limit, collector)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			return nil
		}
		report.Fetched += len(msgs)

		// The page's own last instant is where the next one resumes,
		// captured before recording so a skipped trailing message still
		// advances the walk.
		last := messageTime(msgs[len(msgs)-1])

		for _, msg := range msgs {
			if err := c.backfillOne(ctx, msg, cur, opts, seen, report); err != nil {
				return err
			}
		}

		// A full page proves nothing about exhaustion; only a short one
		// does. Stopping here on len(msgs) == limit would silently truncate
		// every history that divides evenly by the page size.
		if len(msgs) < limit {
			return nil
		}

		// Advance past the last message's instant. Without the nudge a
		// page whose messages all share one timestamp would request the
		// same window forever.
		next := last.Add(time.Millisecond)
		if !next.After(from) {
			// The server is not moving forward, so neither can we.
			return nil
		}
		from = next
	}
}

// backfillOne records a single history message unless it is already held.
//
// Dedupe happens before recording, which is why it goes through Capture.Event
// rather than Handle: the MsgID a message would be stored under is only
// knowable once the event is built.
func (c *Capture) backfillOne(ctx context.Context, msg ircmsg.Message, cur Cursor, opts BackfillOptions, seen map[string]bool, report *Report) error {
	ev, ok := c.Event(ctx, msg)
	if !ok {
		// The capture path declined it: a HistServ synthetic, the bot's
		// own echo, or not a channel message at all.
		report.Skipped++
		return nil
	}

	msgid := eventMsgID(ev)
	duplicate, err := c.isDuplicate(ctx, msgid, cur, opts, seen)
	if err != nil {
		return err
	}
	if duplicate {
		report.Skipped++
		return nil
	}

	if c.Recorder == nil {
		return fmt.Errorf("irc/bot: backfill has no recorder")
	}
	stored, err := c.Recorder.Record(ctx, ev)
	if err != nil {
		return fmt.Errorf("irc/bot: record backfilled message: %w", err)
	}
	report.Recorded++
	if msgid != "" {
		seen[msgid] = true
	}

	// Only now, with the event durably stored, may the cursor move — the
	// same ordering Handle keeps, for the same reason.
	if c.Cursors != nil {
		c.Cursors.Set(channelOf(stored), Cursor{
			LastMsgID: eventMsgID(stored),
			LastTime:  stored.Timestamp,
		})
	}
	return nil
}

// isDuplicate reports whether this message is already recorded, checking the
// cheap local signals before asking the backend.
func (c *Capture) isDuplicate(ctx context.Context, msgid string, cur Cursor, opts BackfillOptions, seen map[string]bool) (bool, error) {
	if msgid == "" {
		return false, nil
	}
	if msgid == cur.LastMsgID || seen[msgid] {
		return true, nil
	}
	if opts.Seen == nil {
		return false, nil
	}

	held, err := opts.Seen.HasMsgID(ctx, msgid, opts.since(cur))
	if err != nil {
		return false, fmt.Errorf("irc/bot: check recorded msgid %q: %w", msgid, err)
	}
	return held, nil
}

// fetchPage issues one CHATHISTORY AFTER request and returns the messages it
// produced.
//
// The reply may arrive two ways. With labeled-response the transport
// correlates it and hands it back directly; without it the transport can only
// deliver the batch to a subscriber, so the collector is drained as a
// fallback. Taking whichever arrived keeps one paging loop working on both
// kinds of server.
func (c *Capture) fetchPage(ctx context.Context, channel, selector string, limit int, collector *batchCollector) ([]ircmsg.Message, error) {
	collector.reset()

	raw := fmt.Sprintf("CHATHISTORY AFTER %s timestamp=%s %d", channel, selector, limit)
	replies, err := c.Transport.SendLabeled(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("irc/bot: %s: %w", raw, err)
	}

	if msgs := historyMessages(replies); len(msgs) > 0 {
		return msgs, nil
	}
	return historyMessages(collector.take()), nil
}

// historyMessages keeps the lines of a history reply that carry content,
// dropping the BATCH framing and any numeric the server wrapped around it.
func historyMessages(msgs []ircmsg.Message) []ircmsg.Message {
	var out []ircmsg.Message
	for _, msg := range msgs {
		switch msg.Command {
		case "PRIVMSG", "NOTICE":
			out = append(out, msg)
		}
	}
	return out
}

// batchCollector accumulates batches the transport delivers out-of-band.
//
// SubscribeBatch callbacks run on the transport's delivery goroutine, so the
// collector is only touched between requests — reset before a send, taken
// after it — and needs no lock of its own beyond that discipline.
type batchCollector struct {
	msgs []ircmsg.Message
}

// add appends a delivered batch.
func (b *batchCollector) add(msgs []ircmsg.Message) {
	b.msgs = append(b.msgs, msgs...)
}

// reset drops anything collected so far.
func (b *batchCollector) reset() { b.msgs = nil }

// take returns what was collected and clears it.
func (b *batchCollector) take() []ircmsg.Message {
	out := b.msgs
	b.msgs = nil
	return out
}

// eventMsgID reads the MsgID out of a built event's payload. An unreadable
// payload yields an empty id, which dedupe treats as "cannot tell" and records
// rather than dropping.
func eventMsgID(ev model.Event) string {
	var data model.MessageData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		return ""
	}
	return data.MsgID
}

// channelOf reads the channel out of a stored event's payload.
func channelOf(ev model.Event) string {
	var data model.MessageData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		return ""
	}
	return data.Channel
}
