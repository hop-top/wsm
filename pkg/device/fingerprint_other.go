//go:build !darwin && !linux

package device

// platformMachineID is a no-op on unsupported platforms; triggers UUID fallback.
func platformMachineID() (string, error) {
	return "", nil
}
