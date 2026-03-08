package device

import (
	"os/exec"
	"regexp"
	"strings"
)

var ioUUIDRe = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

// platformMachineID reads the IOPlatformUUID on macOS via ioreg.
func platformMachineID() (string, error) {
	out, err := exec.Command(
		"ioreg", "-rd1", "-c", "IOPlatformExpertDevice",
	).Output()
	if err != nil {
		return "", err
	}
	m := ioUUIDRe.FindSubmatch(out)
	if m == nil {
		return "", nil
	}
	return strings.TrimSpace(string(m[1])), nil
}
