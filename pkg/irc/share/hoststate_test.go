package share

import (
	"os"
	"testing"

	"hop.top/wsm/pkg/irc"
)

// TestReadHostState pins what a guest reports about the workspace hosting it.
//
// The distinction that matters is "gone" versus "stopped": a stopped host comes
// back with everything the guest depends on, while a purged one never does, and
// a guest that could not tell them apart would wait forever for a server that
// no longer exists.
func TestReadHostState(t *testing.T) {
	w := newWorld(t)

	t.Run("running host reports its own state", func(t *testing.T) {
		got := ReadHostState(w.host.Paths, func(irc.Paths) (string, bool) {
			return "running", true
		})
		if got != "running" {
			t.Fatalf("host state = %q; want running", got)
		}
	})

	t.Run("host with no live daemon reads stopped", func(t *testing.T) {
		got := ReadHostState(w.host.Paths, func(irc.Paths) (string, bool) {
			return "", false
		})
		if got != StateStopped {
			t.Fatalf("host state = %q; want %q", got, StateStopped)
		}
	})

	t.Run("unreadable host status does not fail the guest", func(t *testing.T) {
		if got := ReadHostState(w.host.Paths, nil); got != StateStopped {
			t.Fatalf("host state = %q; want %q: a guest must still be able to "+
				"report its own state", got, StateStopped)
		}
	})

	t.Run("purged host reads gone", func(t *testing.T) {
		if err := os.RemoveAll(w.host.Paths.DataDir); err != nil {
			t.Fatalf("purge host: %v", err)
		}
		got := ReadHostState(w.host.Paths, func(irc.Paths) (string, bool) {
			return "running", true
		})
		if got != StateGone {
			t.Fatalf("host state = %q; want %q: a purged host is not coming back",
				got, StateGone)
		}
	})
}
