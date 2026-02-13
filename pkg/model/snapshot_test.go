package model

import (
	"encoding/json"
	"testing"
	"time"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestGenerateSnapshot_Empty(t *testing.T) {
	ws := Workspace{ID: "w1", Name: "demo", Status: StatusActive}
	s := GenerateSnapshot(ws, nil, 0)
	if s.MessageCount != 0 || s.ToolCalls != 0 || len(s.FilesChanged) != 0 || len(s.Devices) != 0 {
		t.Fatalf("expected empty snapshot, got %+v", s)
	}
}

func TestGenerateSnapshot(t *testing.T) {
	now := time.Now().UTC()
	ws := Workspace{ID: "w1", Name: "demo", Status: StatusActive}
	events := []Event{
		{SeqNum: 1, Type: EventMessageAdded, Timestamp: now, Data: mustJSON(t, MessageData{Role: "user", Content: "hi"})},
		{SeqNum: 2, Type: EventToolCallStarted, Timestamp: now.Add(time.Second), Data: mustJSON(t, ToolCallData{ToolName: "grep"})},
		{SeqNum: 3, Type: EventFileChanged, Timestamp: now.Add(2 * time.Second), Data: mustJSON(t, FileChangeData{Path: "a.txt", Operation: "modify"})},
		{SeqNum: 4, Type: EventFileChanged, Timestamp: now.Add(3 * time.Second), Data: mustJSON(t, FileChangeData{Path: "a.txt", Operation: "modify"})},
		{SeqNum: 5, Type: EventDeviceAttached, Timestamp: now.Add(4 * time.Second), Data: mustJSON(t, DeviceData{DeviceID: "dev-1"})},
		{SeqNum: 6, Type: EventDeviceDetached, Timestamp: now.Add(5 * time.Second), Data: mustJSON(t, DeviceData{DeviceID: "dev-1"})},
	}
	s := GenerateSnapshot(ws, events, 0)
	if s.MessageCount != 1 || s.ToolCalls != 1 {
		t.Fatalf("unexpected counters: %+v", s)
	}
	if len(s.FilesChanged) != 1 || s.FilesChanged[0] != "a.txt" {
		t.Fatalf("expected deduped files, got %+v", s.FilesChanged)
	}
	if len(s.Devices) != 0 {
		t.Fatalf("expected no attached devices, got %+v", s.Devices)
	}
	if s.AtSeqNum != 6 {
		t.Fatalf("expected at_seq_num 6, got %d", s.AtSeqNum)
	}
}
