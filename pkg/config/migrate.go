package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// LegacyDir returns the legacy ~/.wsm directory path.
// Returns empty string if HOME cannot be resolved.
func LegacyDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".wsm")
}

// MigrateFromLegacy checks for a legacy ~/.wsm/wsm.db and moves it
// to the XDG-resolved DataDir when the destination does not yet exist.
//
// This is best-effort: if no legacy path exists or the XDG target
// already has a DB, it is a no-op. Returns an error only when a
// filesystem operation fails mid-migration.
func MigrateFromLegacy(cfg Config) error {
	legacyDir := LegacyDir()
	if legacyDir == "" {
		return nil
	}

	legacyDB := filepath.Join(legacyDir, "wsm.db")
	if _, err := os.Stat(legacyDB); os.IsNotExist(err) {
		return nil // nothing to migrate
	}

	targetDB := filepath.Join(cfg.DataDir, "wsm.db")
	if _, err := os.Stat(targetDB); err == nil {
		return nil // target already exists; do not overwrite
	}

	// Ensure target directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir %s: %w", cfg.DataDir, err)
	}

	if err := os.Rename(legacyDB, targetDB); err != nil {
		return fmt.Errorf("move %s -> %s: %w", legacyDB, targetDB, err)
	}

	return nil
}
