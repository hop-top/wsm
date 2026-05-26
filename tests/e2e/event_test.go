package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"hop.top/wsm/pkg/workspace"
)

func TestEventRecording(t *testing.T) {
	r := newRunner(t)
	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()

	ws, err := mgr.Create(ctx, "event-recording", workspace.CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	addMessage := r.run("event", "add", ws.ID, "interaction.message", "--content=hello", "--format", "json")
	requireExitCode(t, addMessage, 0)
	var messageEvent workspace.Event
	mustUnmarshalJSON(t, addMessage.Stdout, &messageEvent)
	if messageEvent.ID == "" || messageEvent.SeqNum == 0 {
		t.Fatalf("expected message event id/seq_num to be set: %+v", messageEvent)
	}

	addTool := r.run(
		"event", "add", ws.ID, "interaction.tool_call",
		"--tool-name=grep",
		"--input={\"pattern\":\"foo\"}",
		"--format", "json",
	)
	requireExitCode(t, addTool, 0)
	var toolEvent workspace.Event
	mustUnmarshalJSON(t, addTool.Stdout, &toolEvent)
	if toolEvent.SeqNum <= messageEvent.SeqNum {
		t.Fatalf("expected monotonic seq_num from event add, got message=%d tool=%d", messageEvent.SeqNum, toolEvent.SeqNum)
	}

	addFile := r.run(
		"event", "add", ws.ID, "mutation.file_patch",
		"--path=README.md",
		"--operation=modify",
		"--format", "json",
	)
	requireExitCode(t, addFile, 0)
	var fileEvent workspace.Event
	mustUnmarshalJSON(t, addFile.Stdout, &fileEvent)
	if fileEvent.SeqNum <= toolEvent.SeqNum {
		t.Fatalf("expected increasing seq_num for file event, got tool=%d file=%d", toolEvent.SeqNum, fileEvent.SeqNum)
	}

	res := r.run("event", "list", ws.ID, "--format", "json")
	requireExitCode(t, res, 0)
	events := mustUnmarshalJSONArray[workspace.Event](t, res.Stdout)
	if len(events) < 2 {
		t.Fatalf("expected at least workspace.created + interaction.message events, got %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].SeqNum <= events[i-1].SeqNum {
			t.Fatalf("expected strict seq_num ordering, got %d then %d", events[i-1].SeqNum, events[i].SeqNum)
		}
	}

	if err := mgr.Archive(ctx, ws.ID); err != nil {
		t.Fatalf("archive workspace: %v", err)
	}
	blocked := r.run("event", "add", ws.ID, "interaction.message", "--content=blocked")
	requireExitCode(t, blocked, 4)
}

func TestEventListing(t *testing.T) {
	r := newRunner(t)
	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()

	ws, err := mgr.Create(ctx, "event-listing", workspace.CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := mgr.RecordEvent(ctx, ws.ID, workspace.EventInteractionMessage, workspace.MessageData{Role: "user", Content: "one"}); err != nil {
		t.Fatalf("record message event: %v", err)
	}
	if _, err := mgr.RecordEvent(ctx, ws.ID, workspace.EventInteractionToolCall, workspace.ToolCallData{ToolName: "grep"}); err != nil {
		t.Fatalf("record tool event: %v", err)
	}
	if _, err := mgr.RecordEvent(ctx, ws.ID, workspace.EventInteractionMessage, workspace.MessageData{Role: "assistant", Content: "two"}); err != nil {
		t.Fatalf("record second message event: %v", err)
	}

	allRes := r.run("event", "list", ws.ID, "--format", "json")
	requireExitCode(t, allRes, 0)
	all := mustUnmarshalJSONArray[workspace.Event](t, allRes.Stdout)
	if len(all) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].SeqNum <= all[i-1].SeqNum {
			t.Fatalf("events are not sorted by seq_num: %d then %d", all[i-1].SeqNum, all[i].SeqNum)
		}
	}

	filteredRes := r.run("event", "list", ws.ID, "--type=interaction.message", "--format", "json")
	requireExitCode(t, filteredRes, 0)
	filtered := mustUnmarshalJSONArray[workspace.Event](t, filteredRes.Stdout)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered interaction.message events, got %d", len(filtered))
	}
	for _, ev := range filtered {
		if ev.Type != workspace.EventInteractionMessage {
			t.Fatalf("unexpected event type in filtered list: %s", ev.Type)
		}
	}
}

func TestEventFilteringCLI(t *testing.T) {
	r := newRunner(t)
	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()

	ws, err := mgr.Create(ctx, "event-filtering", workspace.CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for i := 1; i <= 100; i++ {
		if i%2 == 0 {
			if _, err := mgr.RecordEvent(ctx, ws.ID, workspace.EventInteractionMessage, workspace.MessageData{Role: "user", Content: fmt.Sprintf("m-%d", i)}); err != nil {
				t.Fatalf("record message event %d: %v", i, err)
			}
			continue
		}
		if _, err := mgr.RecordEvent(ctx, ws.ID, workspace.EventInteractionToolCall, workspace.ToolCallData{ToolName: fmt.Sprintf("tool-%d", i)}); err != nil {
			t.Fatalf("record tool event %d: %v", i, err)
		}
	}

	res := r.run(
		"event", "list", ws.ID,
		"--type=interaction.message",
		"--after-seq=20",
		"--before-seq=40",
		"--limit=25",
		"--format", "json",
	)
	requireExitCode(t, res, 0)
	filtered := mustUnmarshalJSONArray[workspace.Event](t, res.Stdout)
	if len(filtered) == 0 || len(filtered) > 25 {
		t.Fatalf("expected 1..25 events from filtered query, got %d", len(filtered))
	}
	var prev uint64
	for i, ev := range filtered {
		if ev.Type != workspace.EventInteractionMessage {
			t.Fatalf("unexpected filtered event type: %s", ev.Type)
		}
		if ev.SeqNum <= 20 || ev.SeqNum >= 40 {
			t.Fatalf("event seq out of expected range (20,40): %d", ev.SeqNum)
		}
		if i > 0 && ev.SeqNum <= prev {
			t.Fatalf("event ordering not preserved: %d then %d", prev, ev.SeqNum)
		}
		prev = ev.SeqNum
	}

	badType := r.run("event", "list", ws.ID, "--type=not.a.real.type")
	requireExitCode(t, badType, 2)

	badRange := r.run("event", "list", ws.ID, "--after-seq=40", "--before-seq=20")
	requireExitCode(t, badRange, 2)
}

func TestEventIntegrity_LargeVolume(t *testing.T) {
	r := newRunner(t)
	mgr, cleanup := r.openManager()
	defer cleanup()
	ctx := context.Background()

	ws, err := mgr.Create(ctx, "event-integrity", workspace.CreateOptions{})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for i := 1; i <= 1000; i++ {
		if _, err := mgr.RecordEvent(ctx, ws.ID, workspace.EventInteractionMessage, workspace.MessageData{Role: "user", Content: fmt.Sprintf("msg-%d", i)}); err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
	}

	res := r.run("event", "list", ws.ID, "--type=interaction.message", "--format", "json")
	requireExitCode(t, res, 0)
	events := mustUnmarshalJSONArray[workspace.Event](t, res.Stdout)
	if len(events) != 1000 {
		t.Fatalf("expected 1000 message events, got %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].SeqNum != events[i-1].SeqNum+1 {
			t.Fatalf("expected contiguous seq numbers; got %d then %d", events[i-1].SeqNum, events[i].SeqNum)
		}
	}

	var payload workspace.MessageData
	if err := json.Unmarshal(events[0].Data, &payload); err != nil {
		t.Fatalf("decode first event payload: %v", err)
	}
	if payload.Content == "" {
		t.Fatalf("expected non-empty decoded payload: %+v", payload)
	}
}

func TestEventRecording_SessionAgentID(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("session-agent-e2e")

	addRes := r.run(
		"event", "add", wsID, "interaction.message",
		"--content=hello-session",
		"--session-id=e2e-sess-1",
		"--agent-id=e2e-agent-1",
		"--format", "json",
	)
	requireExitCode(t, addRes, 0)

	var ev workspace.Event
	mustUnmarshalJSON(t, addRes.Stdout, &ev)
	if ev.SessionID != "e2e-sess-1" {
		t.Fatalf("expected session_id=e2e-sess-1, got %q", ev.SessionID)
	}
	if ev.AgentID != "e2e-agent-1" {
		t.Fatalf("expected agent_id=e2e-agent-1, got %q", ev.AgentID)
	}

	listRes := r.run("event", "list", wsID, "--format", "json")
	requireExitCode(t, listRes, 0)
	events := mustUnmarshalJSONArray[workspace.Event](t, listRes.Stdout)

	var found bool
	for _, e := range events {
		if e.SessionID == "e2e-sess-1" && e.AgentID == "e2e-agent-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find event with session_id=e2e-sess-1 agent_id=e2e-agent-1 in listed events")
	}
}

func TestEventShow(t *testing.T) {
	r := newRunner(t)
	wsID := r.mustCreateWorkspace("event-show-e2e")

	addRes := r.run("event", "add", wsID, "interaction.message", "--content=show-me", "--format", "json")
	requireExitCode(t, addRes, 0)
	var added workspace.Event
	mustUnmarshalJSON(t, addRes.Stdout, &added)

	bySeq := r.run("event", "show", wsID, fmt.Sprintf("%d", added.SeqNum), "--format", "json")
	requireExitCode(t, bySeq, 0)
	var showedBySeq workspace.Event
	mustUnmarshalJSON(t, bySeq.Stdout, &showedBySeq)
	if showedBySeq.ID != added.ID {
		t.Fatalf("expected event id %s from show by seq, got %s", added.ID, showedBySeq.ID)
	}

	byID := r.run("event", "show", wsID, added.ID, "--format", "json")
	requireExitCode(t, byID, 0)
	var showedByID workspace.Event
	mustUnmarshalJSON(t, byID.Stdout, &showedByID)
	if showedByID.SeqNum != added.SeqNum {
		t.Fatalf("expected seq_num %d from show by ULID, got %d", added.SeqNum, showedByID.SeqNum)
	}

	notFound := r.run("event", "show", wsID, "99999")
	requireExitCode(t, notFound, 3)
}
