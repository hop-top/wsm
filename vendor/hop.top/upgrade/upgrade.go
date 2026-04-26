// Package upgrade provides standardized self-upgrade logic for hop family CLIs.
//
// Features:
//   - Version check against GitHub releases (or custom URL)
//   - XDG-compliant state (snooze, cache)
//   - Safe binary self-replacement
//   - Multi-interface: CLI, TUI (Bubble Tea), REPL, SKILL preamble
package upgrade

import (
	"context"
	"time"
)

// Checker performs version checks and drives upgrade flows.
type Checker struct {
	cfg Config
}

// New returns a Checker configured with the given options.
func New(opts ...Option) *Checker {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return &Checker{cfg: cfg}
}

// Check fetches the latest release info, applies snooze/cache rules, and
// returns a Result. Always non-nil; errors are embedded in Result.Err.
func (c *Checker) Check(ctx context.Context) *Result {
	// respect cache TTL
	cached, err := loadCachedResult(c.cfg.StateDir, c.cfg.BinaryName)
	if err == nil && cached != nil && time.Since(cached.CheckedAt) < c.cfg.CacheTTL {
		return cached
	}

	latest, err := fetchLatest(ctx, c.cfg)
	if err != nil {
		return &Result{Err: err, CheckedAt: time.Now()}
	}

	r := &Result{
		Current:    c.cfg.CurrentVersion,
		Latest:     latest.Version,
		URL:        latest.URL,
		Notes:      latest.Notes,
		CheckedAt:  time.Now(),
		UpdateAvail: isNewer(c.cfg.CurrentVersion, latest.Version),
	}

	_ = saveCachedResult(c.cfg.StateDir, c.cfg.BinaryName, r)
	return r
}

// ShouldNotify returns true when an update is available AND the user has not
// snoozed (or the snooze has expired).
func (c *Checker) ShouldNotify(ctx context.Context) bool {
	r := c.Check(ctx)
	if !r.UpdateAvail {
		return false
	}
	snoozed, err := isSnoozed(c.cfg.StateDir, c.cfg.BinaryName)
	if err != nil {
		return true // fail open
	}
	return !snoozed
}

// Upgrade downloads and installs the latest binary in-place.
// It replaces the running binary atomically.
func (c *Checker) Upgrade(ctx context.Context) error {
	r := c.Check(ctx)
	if r.Err != nil {
		return r.Err
	}
	if !r.UpdateAvail {
		return nil
	}
	return replaceBinary(ctx, c.cfg, r.URL)
}

// Snooze records a snooze for the configured duration.
func (c *Checker) Snooze() error {
	return writeSnooze(c.cfg.StateDir, c.cfg.BinaryName, c.cfg.SnoozeDuration)
}

// WhatsNew returns the release notes for the latest version.
func (c *Checker) WhatsNew(ctx context.Context) string {
	r := c.Check(ctx)
	if r.Err != nil || r.Notes == "" {
		return ""
	}
	return r.Notes
}
