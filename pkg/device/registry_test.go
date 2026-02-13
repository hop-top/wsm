package device

import (
	"testing"
	"time"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	d := Device{ID: "id-1", Name: "dev1", FirstSeen: time.Now(), LastSeen: time.Now()}
	if err := r.Register(d); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := r.Get("id-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != d.ID {
		t.Fatalf("expected id %s, got %s", d.ID, got.ID)
	}
}

func TestRegistry_UpdateLastSeen(t *testing.T) {
	r := NewRegistry()
	first := time.Now().Add(-time.Hour)
	if err := r.Register(Device{ID: "id-1", Name: "dev1", FirstSeen: first, LastSeen: first}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	latest := time.Now()
	if err := r.Register(Device{ID: "id-1", Name: "dev1", FirstSeen: latest, LastSeen: latest}); err != nil {
		t.Fatalf("register second: %v", err)
	}
	got, err := r.Get("id-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.FirstSeen.Equal(first) {
		t.Fatalf("expected first_seen preserved: %v != %v", got.FirstSeen, first)
	}
	if !got.LastSeen.Equal(latest) {
		t.Fatalf("expected last_seen updated: %v != %v", got.LastSeen, latest)
	}
}
