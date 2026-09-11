package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIRCBorrowed_ChannelAddBorrowedJSON drives `irc channel add --borrowed`
// through to the configuration it writes.
//
// The borrowed entry is how a guest workspace declares a channel that belongs
// to somebody else's server, so the thing worth pinning hardest is the name:
// it is the host's channel verbatim, with no local prefix and no {{ws}}
// expansion. A borrowed name that went through the local naming rules would
// point the bot at a channel that does not exist.
func TestIRCBorrowed_ChannelAddBorrowedJSON(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("borrow-guest")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	add := r.runWithEnv(env, "irc", "channel", "add", "borrow-guest", "#host-ops",
		"--borrowed", "acme-host:#host-ops",
		"--members", "principals",
		"--format", "json")
	requireExitCode(t, add, 0)

	var doc struct {
		Channel string `json:"channel"`
		Action  string `json:"action"`
		Config  string `json:"config"`
		Synced  bool   `json:"synced"`
	}
	mustUnmarshalJSON(t, add.Stdout, &doc)
	if doc.Channel != "#host-ops" || doc.Action != "add" {
		t.Fatalf("add doc = %+v; want the borrowed channel", doc)
	}
	if doc.Synced {
		t.Fatal("no daemon is running, so the edit must report synced=false")
	}

	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, "borrowed:") {
		t.Fatalf("the borrowed entry never reached the config:\n%s", got)
	}
	if !strings.Contains(got, "acme-host") {
		t.Errorf("the host workspace was dropped:\n%s", got)
	}
	// The host named this channel. A local prefix would point the bot at a
	// channel nobody registered.
	if !strings.Contains(got, "#host-ops") {
		t.Errorf("the host channel name was not preserved verbatim:\n%s", got)
	}

	// The channel lists as borrowed rather than as a template wsm owns: the
	// distinction is what stops an operator reading it as a channel this
	// workspace registers and maintains.
	list := r.runWithEnv(env, "irc", "channel", "list", "borrow-guest", "--format", "json")
	requireExitCode(t, list, 0)

	rows := mustUnmarshalJSONArray[struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}](t, list.Stdout)

	var found bool
	for _, row := range rows {
		if row.Name == "#host-ops" {
			found = true
			if row.Source != "borrowed" {
				t.Errorf("source = %q, want borrowed", row.Source)
			}
		}
	}
	if !found {
		t.Fatalf("list must carry the borrowed channel, got %+v", rows)
	}
}

// TestIRCBorrowed_ChannelAddRejectsMalformedBorrowed pins the refusals: the
// flag takes <host-workspace>:<#channel> and nothing else.
//
// A malformed value must not leave a half-declared channel behind. A borrowed
// entry missing its host would otherwise read as an ordinary owned channel on
// the next reconcile, which is the one reading that makes wsm register a name
// the host owns.
func TestIRCBorrowed_ChannelAddRejectsMalformedBorrowed(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("borrow-refuse")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	for _, entry := range []string{"#no-host", "acme-host:", ":#orphan", "acme-host:nohash"} {
		res := r.runWithEnv(env, "irc", "channel", "add", "borrow-refuse", "#nope",
			"--borrowed", entry)
		requireExitCode(t, res, 2)

		raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read config: %v", err)
		}
		if strings.Contains(string(raw), "#nope") {
			t.Fatalf("--borrowed %q was refused but the channel was still written:\n%s",
				entry, string(raw))
		}
	}
}
