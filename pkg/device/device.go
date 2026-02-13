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
func Current() Device {
	host, _ := os.Hostname()
	now := time.Now().UTC()
	machineKey := strings.Join([]string{runtime.GOOS, runtime.GOARCH, host}, "|")
	digest := sha256.Sum256([]byte(machineKey))
	id := hex.EncodeToString(digest[:16])
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
