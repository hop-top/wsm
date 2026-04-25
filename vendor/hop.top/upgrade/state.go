package upgrade

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// snoozeRecord is persisted to disk.
type snoozeRecord struct {
	Until time.Time `json:"until"`
}

func writeSnooze(stateDir, binary string, duration time.Duration) error {
	dir := resolvedStateDir(Config{StateDir: stateDir, BinaryName: binary})
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	rec := snoozeRecord{Until: time.Now().Add(duration)}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "snooze.json"), data, 0o600)
}

func isSnoozed(stateDir, binary string) (bool, error) {
	dir := resolvedStateDir(Config{StateDir: stateDir, BinaryName: binary})
	data, err := os.ReadFile(filepath.Join(dir, "snooze.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var rec snoozeRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return false, nil // corrupt → ignore snooze
	}
	return time.Now().Before(rec.Until), nil
}

// cachedResult wraps a Result for disk persistence.
type cachedResult struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest"`
	URL         string    `json:"url"`
	Notes       string    `json:"notes"`
	CheckedAt   time.Time `json:"checked_at"`
	UpdateAvail bool      `json:"update_avail"`
}

func saveCachedResult(stateDir, binary string, r *Result) error {
	dir := resolvedStateDir(Config{StateDir: stateDir, BinaryName: binary})
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	rec := cachedResult{
		Current:     r.Current,
		Latest:      r.Latest,
		URL:         r.URL,
		Notes:       r.Notes,
		CheckedAt:   r.CheckedAt,
		UpdateAvail: r.UpdateAvail,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "cache.json"), data, 0o600)
}

func loadCachedResult(stateDir, binary string) (*Result, error) {
	dir := resolvedStateDir(Config{StateDir: stateDir, BinaryName: binary})
	data, err := os.ReadFile(filepath.Join(dir, "cache.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilerr
	}
	if err != nil {
		return nil, err
	}
	var rec cachedResult
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	return &Result{
		Current:     rec.Current,
		Latest:      rec.Latest,
		URL:         rec.URL,
		Notes:       rec.Notes,
		CheckedAt:   rec.CheckedAt,
		UpdateAvail: rec.UpdateAvail,
	}, nil
}
