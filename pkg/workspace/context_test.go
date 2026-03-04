package workspace

import (
	"strings"
	"testing"
	"time"

	"hop.top/wsm/pkg/model"
)

func TestFormatContext(t *testing.T) {
	now := time.Now().UTC()
	snap := &model.Snapshot{
		WorkspaceID: "ws-1",
		AtSeqNum:    42,
		Workspace: model.Workspace{
			Name: "my-project",
		},
		MessageCount: 10,
		ToolCalls:    5,
		FilesChanged: []string{"a.go", "b.go"},
		Devices:      []string{"dev-1"},
		GeneratedAt:  now,
	}
	events := []model.Event{
		{SeqNum: 41, Type: model.EventInteractionMessage, Timestamp: now},
		{SeqNum: 42, Type: model.EventInteractionToolDone, Timestamp: now},
	}
	out := FormatContext(snap, events, ContextOptions{RecentEvents: 5})

	if !strings.Contains(out, "my-project") {
		t.Fatal("expected workspace name in context")
	}
	if !strings.Contains(out, "10 messages") {
		t.Fatal("expected message count")
	}
	if !strings.Contains(out, "a.go") {
		t.Fatal("expected file in context")
	}
	if !strings.Contains(out, "seq 42") {
		t.Fatal("expected event seq in context")
	}
}
