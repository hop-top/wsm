package model

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/oklog/ulid/v2"
)

// GenerateSnapshot replays events up to maxSeqNum (or all if maxSeqNum == 0).
func GenerateSnapshot(ws Workspace, events []Event, maxSeqNum uint64) Snapshot {
	snap := Snapshot{
		ID:           ulid.Make().String(),
		WorkspaceID:  ws.ID,
		Workspace:    ws,
		FilesChanged: make([]string, 0),
		Devices:      make([]string, 0),
		Sessions:     make([]string, 0),
		GeneratedAt:  time.Now().UTC(),
	}

	files := make(map[string]struct{})
	attached := make(map[string]struct{})
	sessions := make(map[string]struct{})
	var last Event
	var seen bool

	for _, ev := range events {
		if maxSeqNum > 0 && ev.SeqNum > maxSeqNum {
			continue
		}
		seen = true
		last = ev

		if ev.SessionID != "" {
			sessions[ev.SessionID] = struct{}{}
		}

		switch ev.Type {
		case EventInteractionMessage:
			snap.MessageCount++
		case EventInteractionToolCall:
			snap.ToolCalls++
		case EventMutationFilePatch:
			var p FileChangeData
			if json.Unmarshal(ev.Data, &p) == nil && p.Path != "" {
				files[p.Path] = struct{}{}
			}
		case EventMutationArtifact:
			var p ArtifactData
			if json.Unmarshal(ev.Data, &p) == nil && p.Path != "" {
				files[p.Path] = struct{}{}
			}
		case EventTopologySpaceCreate, EventTopologySpaceDestroy:
			// No direct snapshot field impact yet beyond incrementing some counters
			// if we decide to add topology stats to snapshots.
		case EventDeviceAttached:
			var p DeviceData
			if json.Unmarshal(ev.Data, &p) == nil && p.DeviceID != "" {
				attached[p.DeviceID] = struct{}{}
			}
		case EventDeviceDetached:
			var p DeviceData
			if json.Unmarshal(ev.Data, &p) == nil && p.DeviceID != "" {
				delete(attached, p.DeviceID)
			}
		case EventWorkspaceArchived:
			t := ev.Timestamp
			snap.Workspace.Status = StatusArchived
			snap.Workspace.ArchivedAt = &t
		case EventWorkspaceUnarchived:
			snap.Workspace.Status = StatusActive
			snap.Workspace.ArchivedAt = nil
		}
	}

	if seen {
		snap.AtSeqNum = last.SeqNum
		snap.AtTimestamp = last.Timestamp
	}

	for p := range files {
		snap.FilesChanged = append(snap.FilesChanged, p)
	}
	sort.Strings(snap.FilesChanged)
	for d := range attached {
		snap.Devices = append(snap.Devices, d)
	}
	sort.Strings(snap.Devices)
	for s := range sessions {
		snap.Sessions = append(snap.Sessions, s)
	}
	sort.Strings(snap.Sessions)

	return snap
}
