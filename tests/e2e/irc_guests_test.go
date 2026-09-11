package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIRCGuests_ChannelAddGuestJSON drives `irc channel add --guest`
// through to the configuration it writes.
//
// The guest allow list is how a host hands somebody outside the workspace
// a persistent role, so the two things worth pinning are that the entry
// reaches config.yaml at all and that an entry written without a role
// lands as voice rather than op: the flag's default decides what an
// operator gets when they do not think about it.
func TestIRCGuests_ChannelAddGuestJSON(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("guest-host")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	add := r.runWithEnv(env, "irc", "channel", "add", "guest-host", "#host-ops",
		"--members", "principals",
		"--guest", "carol",
		"--guest", "dana:op",
		"--modes", "ntRi",
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
		t.Fatalf("add doc = %+v; want the declared channel", doc)
	}
	if doc.Synced {
		t.Fatal("no daemon is running, so the edit must report synced=false")
	}

	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, "guests:") {
		t.Fatalf("the guest allow list never reached the config:\n%s", got)
	}
	// The role-less entry defaults to voice: a guest is somebody outside
	// this workspace, and the safe default lets them speak rather than
	// reconfigure the channel.
	if !strings.Contains(got, "carol") || !strings.Contains(got, "voice") {
		t.Errorf("a --guest with no role must default to voice:\n%s", got)
	}
	if !strings.Contains(got, "dana") || !strings.Contains(got, "op") {
		t.Errorf("an explicit --guest <p>:op must be written as op:\n%s", got)
	}
	// The invite-only mode is what the guest's voice exists to bypass.
	if !strings.Contains(got, "ntRi") {
		t.Errorf("channel modes were dropped:\n%s", got)
	}

	// The channel shows up as a declared template, guests and all.
	list := r.runWithEnv(env, "irc", "channel", "list", "guest-host", "--format", "json")
	requireExitCode(t, list, 0)

	var sections channelSections
	mustUnmarshalJSON(t, list.Stdout, &sections)

	var found bool
	for _, row := range sections.Managed {
		if row.Name == "#host-ops" && row.Source == "template" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the managed section must carry the channel declared with guests, got %+v", sections)
	}
}

// TestIRCGuests_ChannelAddRejectsBadGuestRole pins the refusal: the only
// roles an allow-list entry may name are voice and op.
//
// Halfop is deliberately absent. It is a delegation rung for another
// workspace's bot, not a privilege to hand a person through the guest
// list, and accepting it here would quietly widen what the flag grants.
func TestIRCGuests_ChannelAddRejectsBadGuestRole(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("guest-refuse")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	for _, entry := range []string{"dave:halfop", "dave:founder", ":voice"} {
		res := r.runWithEnv(env, "irc", "channel", "add", "guest-refuse", "#nope",
			"--guest", entry)
		requireExitCode(t, res, 2)

		// A refused flag must not leave a half-declared channel behind.
		raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read config: %v", err)
		}
		if strings.Contains(string(raw), "#nope") {
			t.Fatalf("--guest %q was refused but the channel was still written:\n%s",
				entry, string(raw))
		}
	}
}
