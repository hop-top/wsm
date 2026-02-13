package device

import "testing"

func TestCurrent_StableID(t *testing.T) {
	a := Current()
	b := Current()
	if a.ID == "" || b.ID == "" {
		t.Fatal("expected non-empty device ids")
	}
	if a.ID != b.ID {
		t.Fatalf("expected stable id on same machine, got %s vs %s", a.ID, b.ID)
	}
}

func TestCurrent_OSDetection(t *testing.T) {
	d := Current()
	if d.OS == "" || d.Arch == "" {
		t.Fatalf("expected os and arch, got os=%q arch=%q", d.OS, d.Arch)
	}
}
