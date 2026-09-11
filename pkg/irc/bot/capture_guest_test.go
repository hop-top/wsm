package bot

import (
	"context"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
)

const guestServer = "wsm://ws-host/irc"

// writeGuestProfile writes another workspace's profile under dataDir the
// way its own enable would have, so the reverse chain reads a real file
// rather than a stub.
func writeGuestProfile(t *testing.T, dataDir, wsID string, p irc.Profile) {
	t.Helper()
	if err := irc.SaveProfile(filepath.Join(dataDir, "irc", wsID, "profile.yaml"), p); err != nil {
		t.Fatalf("save profile %s: %v", wsID, err)
	}
}

// TestCaptureGuestMessageResolvesViaLocalWorkspaces is acceptance point 6:
// a guest speaking in the host's channel is recorded with AgentID set to
// the guest's own profile id.
//
// The resolver is the real chain from provider.ResolverFor rather than a
// stub, because the claim being proven is about that chain: the guest's
// account appears in no member list and in no host profile, so only the
// LocalWorkspaces link — another workspace on this machine enabled against
// the same server — can name it. A stubbed reverse resolver would pass
// this test without that link existing at all.
func TestCaptureGuestMessageResolvesViaLocalWorkspaces(t *testing.T) {
	dataDir := t.TempDir()

	// The guest workspace, enabled against the same server, owns the
	// account carol speaks from.
	writeGuestProfile(t, dataDir, "ws-guest", irc.Profile{
		Mode:     irc.ModeManaged,
		ServerID: guestServer,
		Accounts: map[string]string{"carol": "carol-acct"},
	})

	// The host's own profile knows its members, and nothing about carol.
	host := irc.Profile{
		Mode:     irc.ModeManaged,
		ServerID: guestServer,
		Accounts: map[string]string{"alice": "alice-acct"},
	}
	members := []model.Member{{ProfileID: "alice"}}

	// aps must not be what answers here: with no aps on PATH the local
	// workspace link is the only thing left that can name the guest.
	t.Setenv("APS_BIN", filepath.Join(t.TempDir(), "no-such-aps"))

	_, reverse := provider.ResolverFor(host, members, dataDir)

	rec := &fakeRecorder{}
	c := &Capture{
		Reverse:     reverse,
		ServerID:    guestServer,
		BotAccount:  "wsm-host",
		WorkspaceID: "ws-host",
		WriterID:    "writer-1",
		Recorder:    rec,
	}

	recorded, err := c.Handle(context.Background(), mustParse(
		"@time=2026-09-11T10:22:10.885Z;account=carol-acct;msgid=g1 "+
			":carol!~u@h.irc PRIVMSG #host-ops :guest checking in"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !recorded {
		t.Fatal("the guest's message was skipped, want it recorded")
	}
	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}

	ev := rec.events[0]
	if ev.AgentID != "carol" {
		t.Errorf("AgentID = %q, want carol (the guest's profile id)", ev.AgentID)
	}
	// The event belongs to the host workspace whose channel it was said in,
	// while the agent is the guest: the two must not be conflated.
	if ev.WorkspaceID != "ws-host" {
		t.Errorf("WorkspaceID = %q, want ws-host", ev.WorkspaceID)
	}

	data := decode(t, ev)
	if data.Attribution != AttributionAccount {
		t.Errorf("Attribution = %q, want account", data.Attribution)
	}
	if data.Channel != "#host-ops" {
		t.Errorf("Channel = %q, want #host-ops", data.Channel)
	}
	if data.Content != "guest checking in" {
		t.Errorf("Content = %q", data.Content)
	}

	// The host's own member still resolves through the host profile, which
	// is what shows the guest hit came from the local-workspaces link
	// rather than from a chain that answers everything the same way.
	recorded, err = c.Handle(context.Background(), mustParse(
		"@time=2026-09-11T10:22:11.885Z;account=alice-acct;msgid=g2 "+
			":alice!~u@h.irc PRIVMSG #host-ops :host member here"))
	if err != nil || !recorded {
		t.Fatalf("Handle host member: %v, recorded=%v", err, recorded)
	}
	if got := rec.events[1].AgentID; got != "alice" {
		t.Errorf("host member AgentID = %q, want alice", got)
	}
}

// TestCaptureGuestUnknownAccountStaysUnattributed pins the other half: an
// account no local workspace claims resolves to nobody.
//
// The message is still recorded, and still labelled "account" because the
// server authenticated the sender — wsm simply cannot say who they are.
// Guessing here would misattribute somebody else's traffic to a guest
// permanently.
func TestCaptureGuestUnknownAccountStaysUnattributed(t *testing.T) {
	dataDir := t.TempDir()
	writeGuestProfile(t, dataDir, "ws-guest", irc.Profile{
		Mode:     irc.ModeManaged,
		ServerID: guestServer,
		Accounts: map[string]string{"carol": "carol-acct"},
	})
	t.Setenv("APS_BIN", filepath.Join(t.TempDir(), "no-such-aps"))

	host := irc.Profile{Mode: irc.ModeManaged, ServerID: guestServer}
	_, reverse := provider.ResolverFor(host, nil, dataDir)

	rec := &fakeRecorder{}
	c := &Capture{
		Reverse:     reverse,
		ServerID:    guestServer,
		BotAccount:  "wsm-host",
		WorkspaceID: "ws-host",
		WriterID:    "writer-1",
		Recorder:    rec,
	}

	recorded, err := c.Handle(context.Background(), mustParse(
		"@time=2026-09-11T10:22:10.885Z;account=outsider;msgid=g3 "+
			":outsider!~u@h.irc PRIVMSG #host-ops :who am i"))
	if err != nil || !recorded {
		t.Fatalf("Handle: %v, recorded=%v", err, recorded)
	}
	if got := rec.events[0].AgentID; got != "" {
		t.Errorf("AgentID = %q, want empty for an unclaimed account", got)
	}
	if data := decode(t, rec.events[0]); data.Attribution != AttributionAccount {
		t.Errorf("Attribution = %q, want account", data.Attribution)
	}
}

// TestCaptureGuestOnAnotherServerIsNotBorrowed pins the server scoping: a
// workspace enabled against a different server must not lend its accounts
// to this one. Two servers can hand the same account name to two different
// people, and attributing across that boundary would be a fabrication.
func TestCaptureGuestOnAnotherServerIsNotBorrowed(t *testing.T) {
	dataDir := t.TempDir()
	writeGuestProfile(t, dataDir, "ws-elsewhere", irc.Profile{
		Mode:     irc.ModeManaged,
		ServerID: "wsm://somewhere-else/irc",
		Accounts: map[string]string{"carol": "carol-acct"},
	})
	t.Setenv("APS_BIN", filepath.Join(t.TempDir(), "no-such-aps"))

	host := irc.Profile{Mode: irc.ModeManaged, ServerID: guestServer}
	_, reverse := provider.ResolverFor(host, nil, dataDir)

	rec := &fakeRecorder{}
	c := &Capture{
		Reverse:     reverse,
		ServerID:    guestServer,
		BotAccount:  "wsm-host",
		WorkspaceID: "ws-host",
		WriterID:    "writer-1",
		Recorder:    rec,
	}

	if _, err := c.Handle(context.Background(), mustParse(
		"@time=2026-09-11T10:22:10.885Z;account=carol-acct;msgid=g4 "+
			":carol!~u@h.irc PRIVMSG #host-ops :hello")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := rec.events[0].AgentID; got != "" {
		t.Errorf("AgentID = %q, want empty: another server's account must not resolve", got)
	}
}
