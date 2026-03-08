package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateFromLegacy_NoLegacy(t *testing.T) {
	root := t.TempDir()
	cfg := Config{DataDir: filepath.Join(root, "xdg", "wsm")}

	// No legacy dir exists; should be a no-op.
	if err := MigrateFromLegacy(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Target dir should not have been created.
	if _, err := os.Stat(cfg.DataDir); !os.IsNotExist(err) {
		t.Fatal("expected data dir to not exist when no legacy present")
	}
}

func TestMigrateFromLegacy_MovesDB(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)

	// Set up legacy dir with a DB file.
	legacyDir := filepath.Join(root, ".wsm")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacyDB := filepath.Join(legacyDir, "wsm.db")
	if err := os.WriteFile(legacyDB, []byte("fake-db"), 0o644); err != nil {
		t.Fatalf("write legacy db: %v", err)
	}

	xdgData := filepath.Join(root, ".local", "share", "wsm")
	cfg := Config{DataDir: xdgData}

	if err := MigrateFromLegacy(cfg); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Legacy DB should be gone.
	if _, err := os.Stat(legacyDB); !os.IsNotExist(err) {
		t.Fatal("expected legacy DB to be removed after migration")
	}

	// Target DB should exist with correct content.
	data, err := os.ReadFile(filepath.Join(xdgData, "wsm.db"))
	if err != nil {
		t.Fatalf("read target db: %v", err)
	}
	if string(data) != "fake-db" {
		t.Fatalf("expected migrated content, got %q", string(data))
	}
}

func TestMigrateFromLegacy_TargetExists(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)

	// Set up legacy dir with a DB file.
	legacyDir := filepath.Join(root, ".wsm")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "wsm.db"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write legacy db: %v", err)
	}

	// Set up target dir with existing DB.
	xdgData := filepath.Join(root, ".local", "share", "wsm")
	if err := os.MkdirAll(xdgData, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(xdgData, "wsm.db"), []byte("current"), 0o644); err != nil {
		t.Fatalf("write target db: %v", err)
	}

	cfg := Config{DataDir: xdgData}

	if err := MigrateFromLegacy(cfg); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Target should be untouched.
	data, err := os.ReadFile(filepath.Join(xdgData, "wsm.db"))
	if err != nil {
		t.Fatalf("read target db: %v", err)
	}
	if string(data) != "current" {
		t.Fatalf("expected target unchanged, got %q", string(data))
	}

	// Legacy should still exist.
	if _, err := os.Stat(filepath.Join(legacyDir, "wsm.db")); err != nil {
		t.Fatal("expected legacy DB to remain when target exists")
	}
}
