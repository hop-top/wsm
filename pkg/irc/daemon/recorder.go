// Package daemon runs one workspace's IRC presence: the managed ergo child
// when wsm owns the server, the bot session in both modes, reconciliation on
// start and on demand, message capture, and the status file the CLI reads.
//
// Every mode branch keys on irc.Profile.Mode rather than irc.Config.Mode.
// Profile records the mode the workspace was actually enabled in; Config only
// seeds it, so editing the global config must never re-point a running
// workspace at a different kind of server.
//
// This package holds no UI dependency (CI enforces `make verify-no-ui-leak`):
// presentation belongs to the command in cmd/, which is the only caller.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/model"
	"hop.top/wsm/pkg/owsf"
)

// WriterPrefix labels events this daemon writes. The capture path records a
// per-message AgentID, so the writer id identifies the producer rather than
// the author: "irc:<workspace-id>".
const WriterPrefix = "irc:"

// WriterID returns the writer id for one workspace's IRC capture.
func WriterID(workspaceID string) string { return WriterPrefix + workspaceID }

// Recorder appends captured events straight to the backend.
//
// It deliberately bypasses workspace.Manager.RecordEvent, which stamps its own
// AgentID and leaves Timestamp to the backend. Capture resolves a different
// AgentID per message and carries the server's own timestamp, so both have to
// survive the write; Manager would overwrite the first and discard the second.
type Recorder struct {
	backend     backend.Backend
	workspaceID string
	deviceID    string
}

// NewRecorder builds a Recorder over backend for one workspace.
//
// deviceID comes from device.CurrentWithDataDir at the call site rather than
// being resolved here, so the daemon and the CLI's offline backfill record the
// same device without this package depending on how it was discovered.
func NewRecorder(b backend.Backend, workspaceID, deviceID string) *Recorder {
	return &Recorder{backend: b, workspaceID: workspaceID, deviceID: deviceID}
}

// Record appends ev and returns it as stored, satisfying bot.Recorder.
//
// WriterID is set before owsf.DefaultAndAlign, never after: DefaultAndAlign
// fills an empty WriterID with an OASF-prefixed default, so assigning it
// afterwards would be silently replaced on the next event that arrives with
// the field already populated, and assigning it before is what makes the
// "irc:" form durable.
func (r *Recorder) Record(ctx context.Context, ev model.Event) (model.Event, error) {
	if ev.WorkspaceID == "" {
		ev.WorkspaceID = r.workspaceID
	}
	if ev.DeviceID == "" {
		ev.DeviceID = r.deviceID
	}
	if ev.WriterID == "" {
		ev.WriterID = WriterID(r.workspaceID)
	}

	if err := owsf.DefaultAndAlign(&ev); err != nil {
		return model.Event{}, fmt.Errorf("irc/daemon: align event: %w", err)
	}

	stored, err := r.backend.AppendEvent(ctx, ev)
	if err != nil {
		return model.Event{}, fmt.Errorf("irc/daemon: append event: %w", err)
	}
	return stored, nil
}

// seenPageSize is how many events one ListEvents call walks back over. The
// backend has no time filter, so paging is by sequence number and the page
// size is purely a round-trip versus memory trade.
const seenPageSize = 500

// Seen answers "is this message already recorded" for a backfill, by walking
// the workspace's events backwards.
//
// The backend filters by sequence number and never by time, so the only way to
// bound the walk is to page backwards until an event older than the caller's
// `since` appears. That is exactly what the bot.Seen contract asks for: since
// is the point past which nothing the backfill could return can exist.
//
// Paging is a descending window rather than a limit, because ListEvents orders
// ascending and applies Limit afterwards: asking for N rows returns the OLDEST
// N of the range, never the newest. A limit-based walk would therefore start at
// the beginning of the log and never reach the messages a backfill is actually
// deduping against, silently re-recording every one of them. Sequence numbers
// are dense (AppendEvent assigns MAX+1 per workspace and nothing deletes an
// event), so a fixed-width window over seq_num bounds each page instead.
type Seen struct {
	backend     backend.Backend
	workspaceID string
	pageSize    int
}

// NewSeen builds a Seen over backend for one workspace.
func NewSeen(b backend.Backend, workspaceID string) *Seen {
	return &Seen{backend: b, workspaceID: workspaceID, pageSize: seenPageSize}
}

// HasMsgID reports whether msgid is already recorded, looking no further back
// than since.
//
// An empty msgid is never "seen": the capture path always supplies an id (a
// content hash when the server sent none), so an empty one means the caller
// has nothing to match on and treating it as a hit would drop a real message.
func (s *Seen) HasMsgID(ctx context.Context, msgid string, since time.Time) (bool, error) {
	if msgid == "" {
		return false, nil
	}

	// The event count is an upper bound on the highest sequence number: it
	// counts every event, while the walk below filters to messages. Starting
	// one past it makes the first window include the newest event there is.
	count, err := s.backend.EventCount(ctx, s.workspaceID)
	if err != nil {
		return false, fmt.Errorf("irc/daemon: count events: %w", err)
	}
	if count == 0 {
		return false, nil
	}

	page := uint64(s.page())
	high := count + 1 // exclusive upper bound

	for high > 1 {
		low := uint64(0) // exclusive lower bound; 0 disables the filter
		if high > page+1 {
			low = high - page - 1
		}

		events, err := s.backend.ListEvents(ctx, s.workspaceID, backend.EventListOptions{
			Types:        []model.EventType{model.EventInteractionMessage},
			AfterSeqNum:  low,
			BeforeSeqNum: high,
		})
		if err != nil {
			return false, fmt.Errorf("irc/daemon: list events: %w", err)
		}

		for _, ev := range events {
			if msgIDOf(ev) == msgid {
				return true, nil
			}
		}

		// Events come back ascending, so the first is the oldest in the
		// window. Once that predates everything the backfill could return,
		// nothing further back can match either. A window holding no
		// messages at all says nothing about age, so the walk continues.
		if len(events) > 0 && !since.IsZero() && events[0].Timestamp.Before(since) {
			return false, nil
		}

		high = low + 1
	}
	return false, nil
}

// page resolves the page size, so a zero-valued Seen still walks.
func (s *Seen) page() int {
	if s.pageSize > 0 {
		return s.pageSize
	}
	return seenPageSize
}

// msgIDOf reads the transport message id out of an event's payload. An event
// whose data is not a message payload simply has no id, which is not an error:
// the event log holds many types and only messages carry one.
func msgIDOf(ev model.Event) string {
	if len(ev.Data) == 0 {
		return ""
	}
	var data model.MessageData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		return ""
	}
	return data.MsgID
}
