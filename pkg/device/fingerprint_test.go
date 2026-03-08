package device

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestPersistAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := "test-machine-id-abc123"

	if err := PersistMachineID(dir, want); err != nil {
		t.Fatalf("persist: %v", err)
	}
	got, err := ReadPersistedID(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip: got %q, want %q", got, want)
	}
}

func TestPersistCreatesDir(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "sub", "dir")

	if err := PersistMachineID(nested, "id-value"); err != nil {
		t.Fatalf("persist to nested dir: %v", err)
	}
	got, err := ReadPersistedID(nested)
	if err != nil {
		t.Fatalf("read from nested dir: %v", err)
	}
	if got != "id-value" {
		t.Fatalf("got %q, want %q", got, "id-value")
	}
}

func TestReadPersistedID_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadPersistedID(dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadPersistedID_Empty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, machineIDFile), []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadPersistedID(dir)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestMachineFingerprint_FallbackAndPersist(t *testing.T) {
	dir := t.TempDir()
	// Use a subdir that doesn't exist yet to also test dir creation.
	dataDir := filepath.Join(dir, "wsm-test")

	id, err := MachineFingerprint(dataDir)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty fingerprint")
	}

	// Should be persisted.
	persisted, err := ReadPersistedID(dataDir)
	if err != nil {
		t.Fatalf("read persisted: %v", err)
	}
	if persisted != id {
		t.Fatalf("persisted %q != returned %q", persisted, id)
	}
}

func TestMachineFingerprint_Stability(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "wsm-test")

	id1, err := MachineFingerprint(dataDir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	id2, err := MachineFingerprint(dataDir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("not stable: %q vs %q", id1, id2)
	}
}

// uuidRe matches UUID v4 format: 8-4-4-4-12 hex.
var uuidRe = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

func TestGenerateUUID_Format(t *testing.T) {
	id, err := generateUUID()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !uuidRe.MatchString(id) {
		t.Fatalf("not a valid v4 UUID: %q", id)
	}
}

func TestGenerateUUID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := generateUUID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate uuid: %s", id)
		}
		seen[id] = true
	}
}
