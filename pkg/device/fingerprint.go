package device

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hop.top/kit/go/core/xdg"
)

const machineIDFile = "machine-id"

// MachineFingerprint returns a stable, deterministic machine identifier.
// Resolution order:
//  1. Persisted ID from dataDir/machine-id
//  2. Platform-specific source (macOS IOPlatformUUID, Linux /etc/machine-id)
//  3. Random UUID fallback
//
// The resolved ID is persisted for future stability.
func MachineFingerprint(dataDir string) (string, error) {
	if dataDir == "" {
		dataDir = defaultDataDir()
	}

	// 1. Try persisted ID.
	if id, err := ReadPersistedID(dataDir); err == nil && id != "" {
		return id, nil
	}

	// 2. Try platform-specific source.
	id, err := platformMachineID()
	if err != nil || id == "" {
		// 3. Fallback: generate random UUID.
		id, err = generateUUID()
		if err != nil {
			return "", fmt.Errorf("generate fallback uuid: %w", err)
		}
	}

	// Persist for future calls.
	if perr := PersistMachineID(dataDir, id); perr != nil {
		return id, fmt.Errorf("persist machine id: %w", perr)
	}
	return id, nil
}

// ReadPersistedID reads a previously persisted machine ID from dataDir.
func ReadPersistedID(dataDir string) (string, error) {
	p := filepath.Join(dataDir, machineIDFile)
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("empty machine id in %s", p)
	}
	return id, nil
}

// PersistMachineID writes the machine ID to dataDir/machine-id.
func PersistMachineID(dataDir, id string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	p := filepath.Join(dataDir, machineIDFile)
	return os.WriteFile(p, []byte(id+"\n"), 0o600)
}

// defaultDataDir returns $XDG_DATA_HOME/wsm via kit/xdg.
func defaultDataDir() string {
	if p, err := xdg.DataDir("wsm"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "wsm")
}

// generateUUID produces a random v4 UUID string.
func generateUUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	// Set version 4 and variant bits.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
