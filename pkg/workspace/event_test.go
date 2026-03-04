package workspace

import (
	"encoding/json"
	"testing"
	"time"
)

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

func TestEvent_Serialization(t *testing.T) {
	ev := Event{
		ID:          "01HXYZTESTEVENT",
		WorkspaceID: "01HXYZWORKSPACE",
		Type:        EventInteractionMessage,
		Timestamp:   time.Now().UTC(),
		Data:        mustMarshal(t, MessageData{Role: "user", Content: "hello"}),
		SeqNum:      2,
	}

	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var decoded Event
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if decoded.Type != EventInteractionMessage || decoded.SeqNum != 2 {
		t.Fatalf("unexpected event after round-trip: %+v", decoded)
	}
}

func TestEvent_PayloadRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		target  any
	}{
		{
			name:    "message",
			payload: MessageData{Role: "assistant", Content: "hi"},
			target:  &MessageData{},
		},
		{
			name:    "tool",
			payload: ToolCallData{ToolName: "grep", Status: "completed", Duration: 42},
			target:  &ToolCallData{},
		},
		{
			name:    "file",
			payload: FileChangeData{Path: "a.txt", Operation: "modify"},
			target:  &FileChangeData{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := mustMarshal(t, tc.payload)
			if err := json.Unmarshal(data, tc.target); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
		})
	}
}

func TestEventPayload_TypeSafety(t *testing.T) {
	raw := mustMarshal(t, MessageData{Role: "user", Content: "strict"})

	var asMessage MessageData
	if err := json.Unmarshal(raw, &asMessage); err != nil {
		t.Fatalf("decode as message: %v", err)
	}
	if asMessage.Role != "user" || asMessage.Content != "strict" {
		t.Fatalf("unexpected decoded message payload: %+v", asMessage)
	}

	var asTool ToolCallData
	if err := json.Unmarshal(raw, &asTool); err != nil {
		t.Fatalf("decode as tool should still be valid json: %v", err)
	}
	if asTool.ToolName != "" {
		t.Fatalf("expected unrelated struct fields to remain zero-values, got %+v", asTool)
	}
}

func TestEvent_SessionAgentRoundTrip(t *testing.T) {
	ev := Event{
		ID:          "01HXYZTESTEVENT2",
		WorkspaceID: "01HXYZWORKSPACE",
		Type:        EventInteractionMessage,
		SessionID:   "sess-round-trip",
		AgentID:     "agent-round-trip",
		Timestamp:   time.Now().UTC(),
		Data:        mustMarshal(t, MessageData{Role: "user", Content: "hello"}),
		SeqNum:      3,
	}

	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var decoded Event
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if decoded.SessionID != "sess-round-trip" {
		t.Fatalf("expected session_id=sess-round-trip, got %q", decoded.SessionID)
	}
	if decoded.AgentID != "agent-round-trip" {
		t.Fatalf("expected agent_id=agent-round-trip, got %q", decoded.AgentID)
	}
}
