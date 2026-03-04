package owsf

import (
	"testing"

	"hop.top/wsm/pkg/model"
)

func TestCanonicalOrder(t *testing.T) {
	events := []model.Event{
		{ID: "e3", WriterID: "w2", WriterSeq: 1, LamportClock: 2},
		{ID: "e1", WriterID: "w1", WriterSeq: 1, LamportClock: 1},
		{ID: "e2", WriterID: "w1", WriterSeq: 2, LamportClock: 2},
	}
	sorted := CanonicalOrder(events)
	// 1. lamport_clock asc
	// 2. tie-break writer_id
	// 3. tie-break writer_seq
	// 4. tie-break event_id
	if sorted[0].ID != "e1" {
		t.Fatalf("first: want e1, got %s", sorted[0].ID)
	}
	if sorted[1].ID != "e2" {
		t.Fatalf("second: want e2, got %s", sorted[1].ID)
	}
	if sorted[2].ID != "e3" {
		t.Fatalf("third: want e3, got %s", sorted[2].ID)
	}
}

func TestCanonicalOrder_Deterministic(t *testing.T) {
	events := []model.Event{
		{ID: "e2", WriterID: "w1", WriterSeq: 1, LamportClock: 5},
		{ID: "e1", WriterID: "w1", WriterSeq: 2, LamportClock: 5},
	}
	a := CanonicalOrder(events)
	b := CanonicalOrder(events)
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("non-deterministic at %d", i)
		}
	}
}

func TestCanonicalOrder_DoesNotMutateInput(t *testing.T) {
	events := []model.Event{
		{ID: "e2", LamportClock: 2},
		{ID: "e1", LamportClock: 1},
	}
	_ = CanonicalOrder(events)
	if events[0].ID != "e2" {
		t.Fatal("input was mutated")
	}
}
