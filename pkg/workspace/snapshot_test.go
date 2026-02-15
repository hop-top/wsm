package workspace

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"
)

func mustRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestGenerateSnapshot(t *testing.T) {
	now := time.Now().UTC()
	ws := Workspace{ID: "w1", Name: "demo", Status: StatusActive}
	events := []Event{
		{SeqNum: 1, Type: EventMessageAdded, Timestamp: now, Data: mustRawJSON(t, MessageData{Role: "user", Content: "hi"})},
		{SeqNum: 2, Type: EventToolCallStarted, Timestamp: now.Add(time.Second), Data: mustRawJSON(t, ToolCallData{ToolName: "grep"})},
		{SeqNum: 3, Type: EventFileChanged, Timestamp: now.Add(2 * time.Second), Data: mustRawJSON(t, FileChangeData{Path: "a.txt", Operation: "modify"})},
		{SeqNum: 4, Type: EventFileChanged, Timestamp: now.Add(3 * time.Second), Data: mustRawJSON(t, FileChangeData{Path: "b.txt", Operation: "modify"})},
		{SeqNum: 5, Type: EventFileChanged, Timestamp: now.Add(4 * time.Second), Data: mustRawJSON(t, FileChangeData{Path: "a.txt", Operation: "modify"})},
		{SeqNum: 6, Type: EventDeviceAttached, Timestamp: now.Add(5 * time.Second), Data: mustRawJSON(t, DeviceData{DeviceID: "dev-1"})},
		{SeqNum: 7, Type: EventDeviceAttached, Timestamp: now.Add(6 * time.Second), Data: mustRawJSON(t, DeviceData{DeviceID: "dev-2"})},
		{SeqNum: 8, Type: EventDeviceDetached, Timestamp: now.Add(7 * time.Second), Data: mustRawJSON(t, DeviceData{DeviceID: "dev-2"})},
	}

	s := GenerateSnapshot(ws, events, 0)
	if s.MessageCount != 1 || s.ToolCalls != 1 {
		t.Fatalf("unexpected counters: %+v", s)
	}
	if s.AtSeqNum != 8 {
		t.Fatalf("expected snapshot sequence 8, got %d", s.AtSeqNum)
	}
	if !reflect.DeepEqual(s.FilesChanged, []string{"a.txt", "b.txt"}) {
		t.Fatalf("expected deduplicated file list, got %v", s.FilesChanged)
	}
	if !reflect.DeepEqual(s.Devices, []string{"dev-1"}) {
		t.Fatalf("expected attached device set [dev-1], got %v", s.Devices)
	}
}

func TestGenerateSnapshot_Incremental(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	ws, err := m.Create(ctx, "snapshot-incremental", CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "user", Content: "m1"}); err != nil {
		t.Fatalf("record first message: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventFileChanged, FileChangeData{Path: "a.txt", Operation: "modify"}); err != nil {
		t.Fatalf("record first file: %v", err)
	}

	snap1, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{})
	if err != nil {
		t.Fatalf("first snapshot: %v", err)
	}

	if _, err := m.RecordEvent(ctx, ws.ID, EventMessageAdded, MessageData{Role: "assistant", Content: "m2"}); err != nil {
		t.Fatalf("record second message: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventFileChanged, FileChangeData{Path: "b.txt", Operation: "create"}); err != nil {
		t.Fatalf("record second file: %v", err)
	}
	if _, err := m.RecordEvent(ctx, ws.ID, EventFileChanged, FileChangeData{Path: "a.txt", Operation: "modify"}); err != nil {
		t.Fatalf("record duplicate file: %v", err)
	}

	snap2, err := m.Snapshot(ctx, ws.ID, SnapshotOptions{})
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if snap2.AtSeqNum <= snap1.AtSeqNum {
		t.Fatalf("expected incremental snapshot sequence to advance: %d <= %d", snap2.AtSeqNum, snap1.AtSeqNum)
	}
	if snap2.MessageCount != snap1.MessageCount+1 {
		t.Fatalf("expected message count to increment by one, got first=%d second=%d", snap1.MessageCount, snap2.MessageCount)
	}
	files := append([]string(nil), snap2.FilesChanged...)
	sort.Strings(files)
	if !reflect.DeepEqual(files, []string{"a.txt", "b.txt"}) {
		t.Fatalf("unexpected deduplicated files in incremental snapshot: %v", snap2.FilesChanged)
	}
}

func TestGenerateSnapshot_Empty(t *testing.T) {
	ws := Workspace{ID: "w-empty", Name: "empty", Status: StatusActive}
	s := GenerateSnapshot(ws, nil, 0)
	if s.MessageCount != 0 || s.ToolCalls != 0 {
		t.Fatalf("expected zero counters, got messages=%d tools=%d", s.MessageCount, s.ToolCalls)
	}
	if len(s.FilesChanged) != 0 || len(s.Devices) != 0 {
		t.Fatalf("expected empty files/devices, got files=%v devices=%v", s.FilesChanged, s.Devices)
	}
	if s.AtSeqNum != 0 {
		t.Fatalf("expected empty snapshot seq number 0, got %d", s.AtSeqNum)
	}
}
