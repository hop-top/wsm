package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// BrowserChannel identifies which browser to launch.
type BrowserChannel string

const (
	BrowserBrave  BrowserChannel = "brave"
	BrowserChrome BrowserChannel = "chrome"
	BrowserArc    BrowserChannel = "arc"
	BrowserEdge   BrowserChannel = "edge"
)

// ValidateBrowserChannel returns an error if c is unknown.
func ValidateBrowserChannel(c BrowserChannel) error {
	switch c {
	case BrowserBrave, BrowserChrome, BrowserArc, BrowserEdge:
		return nil
	default:
		return fmt.Errorf(
			"invalid browser channel %q: must be brave|chrome|arc|edge", c,
		)
	}
}

// BrowserProfile configures browser provisioning for a workspace member.
type BrowserProfile struct {
	// ProfileDir is the browser user-data directory.
	ProfileDir string `json:"profile_dir" yaml:"profile_dir"`

	// CDPPort is the Chrome DevTools Protocol debugging port.
	CDPPort int `json:"cdp_port" yaml:"cdp_port"`

	// Channel selects which browser binary to launch.
	Channel BrowserChannel `json:"channel" yaml:"channel"`
}

// Validate checks required fields.
func (bp *BrowserProfile) Validate() error {
	if bp.ProfileDir == "" {
		return fmt.Errorf("browser_profile: profile_dir is required")
	}
	if bp.CDPPort < 1024 || bp.CDPPort > 65535 {
		return fmt.Errorf(
			"browser_profile: cdp_port must be 1024-65535, got %d",
			bp.CDPPort,
		)
	}
	return ValidateBrowserChannel(bp.Channel)
}

// Provision creates the browser profile directory if it doesn't exist.
// Returns the absolute path to the profile directory.
func (bp *BrowserProfile) Provision() (string, error) {
	if err := bp.Validate(); err != nil {
		return "", err
	}

	absDir, err := filepath.Abs(bp.ProfileDir)
	if err != nil {
		return "", fmt.Errorf("resolve profile dir: %w", err)
	}

	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return "", fmt.Errorf("create browser profile dir: %w", err)
	}

	return absDir, nil
}

// Teardown removes the browser profile directory.
func (bp *BrowserProfile) Teardown() error {
	if bp.ProfileDir == "" {
		return nil
	}
	absDir, err := filepath.Abs(bp.ProfileDir)
	if err != nil {
		return fmt.Errorf("resolve profile dir: %w", err)
	}
	return os.RemoveAll(absDir)
}

// EnvVars returns environment variables to inject for the browser session.
func (bp *BrowserProfile) EnvVars() map[string]string {
	return map[string]string{
		"BROWSER_CHANNEL":  string(bp.Channel),
		"BROWSER_HEADLESS": "true",
		"BROWSER_ARGS": fmt.Sprintf(
			"--user-data-dir=%s --remote-debugging-port=%d",
			bp.ProfileDir, bp.CDPPort,
		),
	}
}
