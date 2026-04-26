package upgrade

import (
	"os"
	"path/filepath"
	"time"
)

// Config holds all upgrade checker settings.
type Config struct {
	// BinaryName is used for state file namespacing (e.g. "tlc", "aps").
	BinaryName string
	// CurrentVersion is the running binary's semver string (e.g. "1.2.3").
	CurrentVersion string
	// GitHubRepo is "owner/repo" used to query the GitHub releases API.
	// Mutually exclusive with ReleaseURL.
	GitHubRepo string
	// ReleaseURL overrides the GitHub API with a custom JSON endpoint.
	// Response must match {version: string, url: string, notes?: string}.
	ReleaseURL string
	// StateDir is an XDG-compliant directory for snooze + cache files.
	// Defaults to $XDG_STATE_HOME/<BinaryName> or ~/.local/state/<BinaryName>.
	StateDir string
	// CacheTTL controls how long a successful check result is reused.
	CacheTTL time.Duration
	// SnoozeDuration controls how long a snooze lasts.
	SnoozeDuration time.Duration
	// Timeout for HTTP requests.
	Timeout time.Duration
}

// Option is a functional option for Config.
type Option func(*Config)

func defaultConfig() Config {
	return Config{
		CacheTTL:       4 * time.Hour,
		SnoozeDuration: 24 * time.Hour,
		Timeout:        10 * time.Second,
	}
}

// WithBinary sets the binary name (required).
func WithBinary(name, currentVersion string) Option {
	return func(c *Config) {
		c.BinaryName = name
		c.CurrentVersion = currentVersion
	}
}

// WithGitHub sets the GitHub owner/repo for release checks.
func WithGitHub(repo string) Option {
	return func(c *Config) { c.GitHubRepo = repo }
}

// WithReleaseURL sets a custom release endpoint.
func WithReleaseURL(url string) Option {
	return func(c *Config) { c.ReleaseURL = url }
}

// WithCacheTTL overrides the default cache TTL.
func WithCacheTTL(d time.Duration) Option {
	return func(c *Config) { c.CacheTTL = d }
}

// WithSnoozeDuration overrides the default snooze duration.
func WithSnoozeDuration(d time.Duration) Option {
	return func(c *Config) { c.SnoozeDuration = d }
}

// WithTimeout overrides the HTTP request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) { c.Timeout = d }
}

// WithStateDir overrides the XDG state directory.
func WithStateDir(dir string) Option {
	return func(c *Config) { c.StateDir = dir }
}

// resolvedStateDir returns StateDir, falling back to XDG standard.
func resolvedStateDir(cfg Config) string {
	if cfg.StateDir != "" {
		return cfg.StateDir
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, cfg.BinaryName, "upgrade")
}
