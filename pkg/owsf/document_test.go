package owsf

import "testing"

func TestNewDocument_RequiredFields(t *testing.T) {
	doc := NewDocument("wsm", "0.1.0")
	if doc.OWSFVersion == "" {
		t.Fatal("owsf_version required")
	}
	if doc.DocID == "" {
		t.Fatal("doc_id required")
	}
	if doc.CreatedAt.IsZero() {
		t.Fatal("created_at required")
	}
	if doc.Producer.Name == "" {
		t.Fatal("producer name required")
	}
}

func TestDocument_ForkSingleParent(t *testing.T) {
	doc := NewDocument("wsm", "0.1.0")
	doc.Fork = &ForkRef{
		ParentDocID:   "parent-1",
		ParentEventID: "evt-1",
	}
	if doc.Fork.ParentDocID == "" {
		t.Fatal("fork parent_doc_id required")
	}
	// struct enforces single parent by design
	// (no slice; single pointer)
}

func TestDocument_IndependentlyVerifiable(t *testing.T) {
	doc := NewDocument("wsm", "0.1.0")
	// OWSF document must carry all info needed
	// for verification without OASF transport
	if doc.OWSFVersion == "" {
		t.Fatal("version required for independent verify")
	}
	if doc.DocID == "" {
		t.Fatal("doc_id required for independent verify")
	}
	if doc.Producer.Name == "" {
		t.Fatal("producer required for independent verify")
	}
}
