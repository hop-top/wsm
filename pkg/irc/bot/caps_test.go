package bot

import (
	"reflect"
	"testing"
)

func TestCapSetHasAndNames(t *testing.T) {
	set := newCapSet(map[string]string{
		"sasl":             "PLAIN,EXTERNAL",
		"multi-prefix":     "",
		"labeled-response": "",
	})

	if !set.Has("sasl") {
		t.Error("sasl should be present")
	}
	if set.Has("draft/chathistory") {
		t.Error("draft/chathistory was never acked")
	}

	want := []string{"labeled-response", "multi-prefix", "sasl"}
	if got := set.Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestCapSetValue(t *testing.T) {
	set := newCapSet(map[string]string{"sasl": "PLAIN,EXTERNAL", "batch": ""})

	value, ok := set.Value("sasl")
	if !ok || value != "PLAIN,EXTERNAL" {
		t.Errorf("Value(sasl) = (%q, %v), want (PLAIN,EXTERNAL, true)", value, ok)
	}

	// A valueless cap is still granted; the empty string is not absence.
	value, ok = set.Value("batch")
	if !ok || value != "" {
		t.Errorf("Value(batch) = (%q, %v), want (\"\", true)", value, ok)
	}

	if _, ok := set.Value("multi-prefix"); ok {
		t.Error("multi-prefix was never acked")
	}
}

func TestCapSetZeroValueIsUsable(t *testing.T) {
	// The zero Transport reports an empty set before Connect, so Has must
	// tolerate a nil map rather than panicking.
	var set capSet
	if set.Has("sasl") {
		t.Error("empty set should have nothing")
	}
	if got := set.Names(); len(got) != 0 {
		t.Errorf("Names() = %v, want empty", got)
	}
}

func TestNewCapSetCopies(t *testing.T) {
	source := map[string]string{"sasl": "PLAIN"}
	set := newCapSet(source)

	// ircevent documents AcknowledgedCaps as shared storage, so the set
	// must not alias it.
	delete(source, "sasl")

	if !set.Has("sasl") {
		t.Error("capSet aliased the caller's map")
	}
}

func TestDefaultCapsCoverTheDesign(t *testing.T) {
	want := []string{
		"sasl", "message-tags", "account-tag", "server-time",
		"echo-message", "batch", "draft/chathistory",
		"labeled-response", "multi-prefix",
	}
	if !reflect.DeepEqual(DefaultCaps, want) {
		t.Errorf("DefaultCaps = %v, want %v", DefaultCaps, want)
	}
}
