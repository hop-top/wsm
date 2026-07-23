package model

import (
	"encoding/json"
	"testing"
)

func TestLayout_Validate(t *testing.T) {
	l := Layout{Kind: "agentcompanies/v1", Path: "org/"}
	if err := l.Validate(); err != nil {
		t.Fatalf("expected valid layout, got %v", err)
	}
}

func TestLayout_Validate_EmptyPathAllowed(t *testing.T) {
	l := Layout{Kind: "none"}
	if err := l.Validate(); err != nil {
		t.Fatalf("expected empty path to be allowed, got %v", err)
	}
}

func TestLayout_Validate_MissingKind(t *testing.T) {
	l := Layout{Path: "org/"}
	if err := l.Validate(); err == nil {
		t.Fatal("expected error for empty kind")
	}
}

func TestWorkspace_Layout_Optional(t *testing.T) {
	ws := Workspace{ID: "w1", Name: "alpha"}
	if ws.Layout != nil {
		t.Fatal("expected nil layout by default")
	}

	blob, err := json.Marshal(ws)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, ok := decodeMap(t, blob)["layout"]; ok {
		t.Fatalf("expected layout omitted from JSON when nil:\n%s", blob)
	}

	ws.Layout = &Layout{Kind: "agentcompanies/v1", Path: "/abs/org"}
	blob, err = json.Marshal(ws)
	if err != nil {
		t.Fatalf("marshal with layout: %v", err)
	}
	raw, ok := decodeMap(t, blob)["layout"]
	if !ok {
		t.Fatalf("expected layout in JSON when set:\n%s", blob)
	}
	var got Layout
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal layout: %v", err)
	}
	if got.Kind != "agentcompanies/v1" || got.Path != "/abs/org" {
		t.Fatalf("layout round-trip mismatch: %+v", got)
	}
}

func decodeMap(t *testing.T, blob []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(blob, &m); err != nil {
		t.Fatalf("decode map: %v", err)
	}
	return m
}
