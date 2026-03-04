package model

import "testing"

func TestEvent_HasWriterFields(t *testing.T) {
	e := Event{
		WriterID:     "w-1",
		WriterSeq:    1,
		LamportClock: 42,
	}
	if e.WriterID == "" {
		t.Fatal("writer_id required")
	}
	if e.WriterSeq == 0 {
		t.Fatal("writer_seq required")
	}
	if e.LamportClock == 0 {
		t.Fatal("lamport_clock required")
	}
}

func TestEvent_HasSpaceID(t *testing.T) {
	e := Event{SpaceID: "sp-1"}
	if e.SpaceID == "" {
		t.Fatal("space_id required")
	}
}

func TestEvent_HasCausalDeps(t *testing.T) {
	e := Event{CausalDeps: []string{"evt-1", "evt-2"}}
	if len(e.CausalDeps) != 2 {
		t.Fatalf("expected 2 causal deps, got %d", len(e.CausalDeps))
	}
}

func TestArtifactData_Fields(t *testing.T) {
	a := ArtifactData{
		ArtifactID: "art-1",
		Type:       "binary",
		Path:       "/build/output.wasm",
		Hash:       "sha256:abc123",
		Size:       4096,
	}
	if a.ArtifactID == "" {
		t.Fatal("artifact_id required")
	}
	if a.Type == "" {
		t.Fatal("type required")
	}
}

func TestEventTypes_OWSFNamespace(t *testing.T) {
	cases := map[EventType]string{
		EventInteractionMessage:   "interaction.message",
		EventInteractionToolCall:  "interaction.tool_call",
		EventMutationFilePatch:    "mutation.file_patch",
		EventMutationArtifact:     "mutation.artifact",
		EventTopologySpaceCreate:  "topology.space_create",
		EventTopologySpaceDestroy: "topology.space_destroy",
		EventCheckpointSnapshot:   "checkpoint.snapshot",
	}
	for et, want := range cases {
		if string(et) != want {
			t.Errorf("%s != %s", et, want)
		}
	}
}
