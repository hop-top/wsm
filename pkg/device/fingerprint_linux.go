package device

import (
	"os"
	"strings"
)

// platformMachineID reads /etc/machine-id or /var/lib/dbus/machine-id on Linux.
func platformMachineID() (string, error) {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}
	return "", nil
}
