package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"hop.top/kit/go/core/xdg"
)

// defaultPoliciesYAML is the bundled wsm policy. It enforces the
// minimum guard that motivates kit/runtime/policy adoption (T-1288):
// destructive deletes and archives must carry a non-empty `--note`.
// Non-destructive transitions stay open by default; adopters add
// custom rules in their own $XDG_CONFIG_HOME/wsm/policies.yaml.
//
//go:embed policies_default.yaml
var defaultPoliciesYAML []byte

// DefaultPoliciesYAML returns the bundled policy bytes. Callers use
// these when no user-supplied policies.yaml exists, and to seed the
// user file on first boot so adopters can edit it in place.
func DefaultPoliciesYAML() []byte {
	out := make([]byte, len(defaultPoliciesYAML))
	copy(out, defaultPoliciesYAML)
	return out
}

// PoliciesPath returns the user-level policies.yaml path, alongside
// the existing config.yaml under $XDG_CONFIG_HOME/wsm.
func PoliciesPath() (string, error) {
	dir, err := xdg.ConfigDir("wsm")
	if err != nil {
		return "", fmt.Errorf("resolve wsm config dir: %w", err)
	}
	return filepath.Join(dir, "policies.yaml"), nil
}

// EnsureDefaultPoliciesFile writes DefaultPoliciesYAML() to the user
// policies path when the file does not exist or is empty (size 0).
// Returns the resolved path. Existing non-empty user files are left
// untouched — never clobber custom rules an adopter may have authored.
func EnsureDefaultPoliciesFile() (string, error) {
	path, err := PoliciesPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("create config directory for policies: %w", err)
	}
	st, err := os.Stat(path)
	switch {
	case err == nil && st.Size() > 0:
		// User file present and non-empty — keep as-is.
		return path, nil
	case err != nil && !os.IsNotExist(err):
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	// File missing or empty: write the bundled default.
	if err := os.WriteFile(path, DefaultPoliciesYAML(), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}
