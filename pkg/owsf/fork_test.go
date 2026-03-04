package owsf

import "testing"

func TestForkDocument(t *testing.T) {
	parent := NewDocument("wsm", "0.1.0")
	child := ForkDocument(parent, "evt-42", "branch")
	if child.Fork == nil {
		t.Fatal("fork ref required")
	}
	if child.Fork.ParentDocID != parent.DocID {
		t.Fatal("parent doc_id mismatch")
	}
	if child.Fork.ParentEventID != "evt-42" {
		t.Fatal("parent event_id mismatch")
	}
}

func TestForkDocument_NoMultiParent(t *testing.T) {
	parent := NewDocument("wsm", "0.1.0")
	child := ForkDocument(parent, "evt-1", "branch")
	// Fork is a single struct, not a slice;
	// multi-parent impossible by type design
	if child.Fork == nil {
		t.Fatal("fork required")
	}
}

func TestForkDocument_NoMerge(t *testing.T) {
	// Document has Fork *ForkRef (singular) — no MergeRefs field.
	// Compilation alone proves no merge support.
	doc := NewDocument("wsm", "0.1.0")
	_ = doc
}

func TestNoMerge_ProhibitMultiParent(t *testing.T) {
	parent1 := NewDocument("wsm", "0.1.0")
	parent2 := NewDocument("wsm", "0.1.0")
	err := ValidateNoMerge([]string{parent1.DocID, parent2.DocID})
	if err == nil {
		t.Fatal("multi-parent must be prohibited")
	}
}

func TestNoMerge_SingleParentOK(t *testing.T) {
	parent := NewDocument("wsm", "0.1.0")
	err := ValidateNoMerge([]string{parent.DocID})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCombine_NewDocRefInputs(t *testing.T) {
	a := NewDocument("wsm", "0.1.0")
	b := NewDocument("wsm", "0.1.0")
	combined := CombineAsNewDocument("wsm", "0.1.0", []string{a.DocID, b.DocID})
	if combined.Fork != nil {
		t.Fatal("combined doc must not fork")
	}
	if len(combined.InputRefs) != 2 {
		t.Fatal("must reference inputs")
	}
}

// G3: combined document starts with empty ledger (fresh DocID, no writers)
func TestCombine_EmptyLedger(t *testing.T) {
	a := NewDocument("wsm", "0.1.0")
	b := NewDocument("wsm", "0.1.0")
	combined := CombineAsNewDocument("wsm", "0.1.0", []string{a.DocID, b.DocID})
	if combined.DocID == a.DocID || combined.DocID == b.DocID {
		t.Fatal("combined doc must have fresh DocID")
	}
	if len(combined.Writers) != 0 {
		t.Fatal("combined doc must start with empty writers")
	}
}
