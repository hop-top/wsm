package workspace

import "hop.top/wsm/pkg/model"

// GenerateSnapshot replays events up to maxSeqNum (0 means all events).
func GenerateSnapshot(ws Workspace, events []Event, maxSeqNum uint64) Snapshot {
	return model.GenerateSnapshot(ws, events, maxSeqNum)
}
