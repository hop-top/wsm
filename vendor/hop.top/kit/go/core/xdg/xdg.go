// Package xdg resolves per-user directories following the XDG Base Directory
// Specification, with OS-native fallbacks.
//
// Each function checks the corresponding XDG environment variable first
// (XDG_CONFIG_HOME, XDG_DATA_HOME, XDG_CACHE_HOME, XDG_STATE_HOME). When the
// variable is unset or empty it falls back to the platform-native directory:
//
//   - macOS:   ~/Library/Application Support/<tool>  (data/state)
//     os.UserConfigDir / os.UserCacheDir    (config/cache)
//   - Windows: %LocalAppData%\<tool>
//   - Linux:   ~/.config, ~/.local/share, ~/.cache, ~/.local/state
//
// Use MustEnsure to create a directory returned by any of these functions;
// it panics on error rather than propagating it, which is appropriate for
// startup-time path resolution.
package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir returns the configuration directory for the named tool.
// It checks $XDG_CONFIG_HOME first, then falls back to os.UserConfigDir.
// The returned path is not guaranteed to exist; call MustEnsure to create it.
func ConfigDir(tool string) (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, tool), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, tool), nil
}

// DataDir returns the data directory for the named tool.
// It checks $XDG_DATA_HOME first. When unset, the fallback is platform-native:
// ~/Library/Application Support/<tool> on macOS, %LocalAppData%\<tool> on
// Windows, ~/.local/share/<tool> on all other systems.
// Returns an error if the home directory or %LocalAppData% cannot be resolved.
func DataDir(tool string) (string, error) {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, tool), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", tool), nil
	case "windows":
		if local := os.Getenv("LocalAppData"); local != "" {
			return filepath.Join(local, tool), nil
		}
		return "", fmt.Errorf("%%LocalAppData%% not set")
	default:
		return filepath.Join(home, ".local", "share", tool), nil
	}
}

// CacheDir returns the cache directory for the named tool.
// It checks $XDG_CACHE_HOME first, then falls back to os.UserCacheDir.
// The returned path is not guaranteed to exist; call MustEnsure to create it.
func CacheDir(tool string) (string, error) {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, tool), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache dir: %w", err)
	}
	return filepath.Join(dir, tool), nil
}

// StateDir returns the state directory for the named tool.
// It checks $XDG_STATE_HOME first. When unset, the fallback is platform-native:
// ~/Library/Application Support/<tool>/state on macOS,
// %LocalAppData%\<tool>\state on Windows,
// ~/.local/state/<tool> on all other systems.
// Note the /state suffix appended on macOS and Windows to avoid colliding with
// DataDir, which uses the same root directory on those platforms.
func StateDir(tool string) (string, error) {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, tool), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", tool, "state"), nil
	case "windows":
		if local := os.Getenv("LocalAppData"); local != "" {
			return filepath.Join(local, tool, "state"), nil
		}
		return "", fmt.Errorf("%%LocalAppData%% not set")
	default:
		return filepath.Join(home, ".local", "state", tool), nil
	}
}

// MustEnsure creates dir (and any parents) with mode 0750, then returns dir.
// It panics if the input error is non-nil or if os.MkdirAll fails.
// Intended for use with the directory functions in this package at program
// startup, where a missing directory is unrecoverable:
//
//	dir := xdg.MustEnsure(xdg.DataDir("mytool"))
func MustEnsure(dir string, err error) string {
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		panic(err)
	}
	return dir
}
