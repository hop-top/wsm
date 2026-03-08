package device

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strings"
	"time"
)

// Device describes event-producing host identity.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Hostname  string    `json:"hostname"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// Current returns a deterministic local-machine identity payload.
// It uses the default data directory for fingerprint persistence.
func Current() Device {
	return CurrentWithDataDir("")
}

// CurrentWithDataDir returns a device identity using the given data directory
// for fingerprint persistence. An empty dataDir uses the default location.
func CurrentWithDataDir(dataDir string) Device {
	host, _ := os.Hostname()
	now := time.Now().UTC()

	id, err := MachineFingerprint(dataDir)
	if err != nil || id == "" {
		// Fallback to legacy hostname-based hash if fingerprint fails.
		id = legacyID(host)
	}

	if host == "" {
		host = "unknown"
	}
	return Device{
		ID:        id,
		Name:      host,
		Hostname:  host,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		FirstSeen: now,
		LastSeen:  now,
	}
}

// legacyID produces the original hostname-based device ID for backward compat.
func legacyID(hostname string) string {
	machineKey := strings.Join([]string{runtime.GOOS, runtime.GOARCH, hostname}, "|")
	digest := sha256.Sum256([]byte(machineKey))
	return hex.EncodeToString(digest[:16])
}
