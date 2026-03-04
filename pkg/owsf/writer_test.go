package owsf

import "testing"

func TestWriterRegistry_Add(t *testing.T) {
	reg := NewWriterRegistry()
	w := reg.Register("writer-1")
	if w.ID == "" {
		t.Fatal("writer ID required")
	}
	if w.Seq != 0 {
		t.Fatal("initial seq must be 0")
	}
}

func TestWriterRegistry_NextSeq(t *testing.T) {
	reg := NewWriterRegistry()
	w := reg.Register("writer-1")
	s1, err := reg.NextSeq(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := reg.NextSeq(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s1 != 1 || s2 != 2 {
		t.Fatalf("monotonic seq: got %d, %d", s1, s2)
	}
}

func TestWriterRegistry_NextSeq_UnregisteredPanicsNot(t *testing.T) {
	reg := NewWriterRegistry()
	_, err := reg.NextSeq("unknown")
	if err == nil {
		t.Fatal("expected error for unregistered writer")
	}
	if err != ErrWriterNotFound {
		t.Fatalf("expected ErrWriterNotFound, got %v", err)
	}
}

func TestWriterRegistry_All(t *testing.T) {
	reg := NewWriterRegistry()
	reg.Register("w1")
	reg.Register("w2")
	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 writers, got %d", len(all))
	}
}
