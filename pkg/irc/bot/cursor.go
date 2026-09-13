package bot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// cursorDirMode is the permission for the directory holding a cursor file.
// Cursors sit in the same per-workspace IRC tree as the profile and secrets,
// and are kept as private as the rest of it.
const cursorDirMode os.FileMode = 0o700

// cursorFileMode keeps the cursor file owner-only: it records which channels a
// workspace watches and when it last saw traffic in them.
const cursorFileMode os.FileMode = 0o600

// Cursor is how far capture has got in one channel.
type Cursor struct {
	// LastMsgID is the id of the last message recorded, which a backfill
	// skips when the server replays it.
	LastMsgID string `json:"last_msgid,omitempty" yaml:"last_msgid,omitempty"`
	// LastTime is that message's timestamp, which a backfill resumes from.
	LastTime time.Time `json:"last_time,omitempty" yaml:"last_time,omitempty"`
}

// Cursors is the per-channel capture position, persisted at a path.
//
// It is written after each recorded message and deliberately not fsynced: the
// cost of an fsync per message is real, and the failure it would prevent is
// already handled. A crash loses at most the last few cursor updates, so a
// backfill re-fetches a handful of messages it already holds, and dedupes them
// on MsgID. Duplicate work on crash is the accepted trade; skipped messages
// are not, which is why the cursor only ever moves after a successful record.
//
// The zero value is not usable; build one with NewCursors or LoadCursors. A
// Cursors is safe for concurrent use.
type Cursors struct {
	path string

	mu sync.Mutex
	// byChannel maps channel name to its position.
	byChannel map[string]Cursor
}

// NewCursors returns an empty cursor set that will persist to path. It does
// not touch the filesystem; nothing is written until Save.
func NewCursors(path string) *Cursors {
	return &Cursors{path: path, byChannel: map[string]Cursor{}}
}

// LoadCursors reads the cursor set at path. A missing file yields an empty set
// rather than an error: no cursor file simply means nothing has been captured
// yet, which is the normal state on first run.
func LoadCursors(path string) (*Cursors, error) {
	c := NewCursors(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return nil, fmt.Errorf("irc/bot: read cursors %s: %w", path, err)
	}

	var byChannel map[string]Cursor
	if err := yaml.Unmarshal(data, &byChannel); err != nil {
		return nil, fmt.Errorf("irc/bot: parse cursors %s: %w", path, err)
	}
	for channel, cursor := range byChannel {
		c.byChannel[channel] = cursor
	}
	return c, nil
}

// Path reports where this set persists to.
func (c *Cursors) Path() string { return c.path }

// Get returns the cursor for channel. ok is false when the channel has never
// been captured, which a backfill reads as "no position, start from the
// beginning" rather than as the zero time.
func (c *Cursors) Get(channel string) (Cursor, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cursor, ok := c.byChannel[channel]
	return cursor, ok
}

// Set records the position for channel in memory. Callers persist with Save.
func (c *Cursors) Set(channel string, cursor Cursor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byChannel[channel] = cursor
}

// All returns a copy of every recorded position.
func (c *Cursors) All() map[string]Cursor {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]Cursor, len(c.byChannel))
	for channel, cursor := range c.byChannel {
		out[channel] = cursor
	}
	return out
}

// Save writes the set to its path, atomically: the bytes go to a temporary
// file in the destination directory and are renamed into place, so a crash
// mid-write leaves the previous cursor file intact rather than a truncated one
// that would fail to parse and lose every channel's position at once.
//
// It does not fsync. See the type comment for why that is the right trade.
func (c *Cursors) Save() error {
	if c.path == "" {
		return fmt.Errorf("irc/bot: cursors have no path")
	}

	c.mu.Lock()
	data, err := yaml.Marshal(c.byChannel)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("irc/bot: encode cursors: %w", err)
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, cursorDirMode); err != nil {
		return fmt.Errorf("irc/bot: create cursor dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".cursors-*.yaml")
	if err != nil {
		return fmt.Errorf("irc/bot: create cursor temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(cursorFileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("irc/bot: chmod cursor temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("irc/bot: write cursors: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("irc/bot: close cursor temp file: %w", err)
	}
	if err := os.Rename(tmpName, c.path); err != nil {
		return fmt.Errorf("irc/bot: install cursors %s: %w", c.path, err)
	}
	return nil
}
