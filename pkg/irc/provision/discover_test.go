package provision

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
)

// fakeDirectory is a DirectoryProvider whose every answer is scripted. It
// records what was joined and purged, which is how the tests that matter --
// "a peer is never touched", "a purge re-checks first" -- assert on the
// absence of an action rather than on a return value.
type fakeDirectory struct {
	caps provider.Caps

	listed  []provider.ChannelInfo
	listErr error

	// founders answers per-channel lookups for listings that carry no
	// founder of their own.
	founders   map[string]string
	founderErr error

	joinErr  map[string]error
	purgeErr map[string]error

	joined []string
	purged []string

	// listFn, when set, supersedes listed: it is how a test makes the
	// server's answer change between passes.
	listFn func() []provider.ChannelInfo
}

func (f *fakeDirectory) Caps() provider.Caps { return f.caps }

func (f *fakeDirectory) ListChannels(context.Context) ([]provider.ChannelInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listFn != nil {
		return f.listFn(), nil
	}
	return f.listed, nil
}

func (f *fakeDirectory) Founder(_ context.Context, channel string) (string, error) {
	if f.founderErr != nil {
		return "", f.founderErr
	}
	return f.founders[channel], nil
}

func (f *fakeDirectory) JoinHidden(_ context.Context, channel string) error {
	if err := f.joinErr[channel]; err != nil {
		return err
	}
	f.joined = append(f.joined, channel)
	return nil
}

func (f *fakeDirectory) Purge(_ context.Context, channel string) error {
	if err := f.purgeErr[channel]; err != nil {
		return err
	}
	f.purged = append(f.purged, channel)
	return nil
}

func (f *fakeDirectory) RelayedIdentity(ircmsg.Message) (provider.RelayIdentity, bool) {
	return provider.RelayIdentity{}, false
}

func (f *fakeDirectory) didJoin(channel string) bool {
	for _, c := range f.joined {
		if c == channel {
			return true
		}
	}
	return false
}

func (f *fakeDirectory) didPurge(channel string) bool {
	for _, c := range f.purged {
		if c == channel {
			return true
		}
	}
	return false
}

// fakeTransport records joins and parts for the plain-join and release paths.
type fakeTransport struct {
	provider.Transport

	joined  []string
	parted  []string
	joinErr error
}

func (f *fakeTransport) Join(_ context.Context, ch string) error {
	if f.joinErr != nil {
		return f.joinErr
	}
	f.joined = append(f.joined, ch)
	return nil
}

func (f *fakeTransport) Part(_ context.Context, ch string) error {
	f.parted = append(f.parted, ch)
	return nil
}

func (f *fakeTransport) didPart(ch string) bool {
	for _, c := range f.parted {
		if c == ch {
			return true
		}
	}
	return false
}

// testClock is a clock the test advances by hand, so the ten-minute purge
// grace costs no wall time.
type testClock struct{ t time.Time }

func (c *testClock) Now() time.Time { return c.t }

func (c *testClock) After(time.Duration) <-chan time.Time { return nil }

func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// allDirectoryCaps is every directory capability, which is the normal case:
// the cap-gated paths are exercised by removing one at a time.
const allDirectoryCaps = provider.CapListChannels | provider.CapChannelFounder |
	provider.CapJoinHidden | provider.CapPurgeChannel

// discoverDeps assembles Deps for a discovery pass. The profile is returned
// so a test can inspect the adoption bookkeeping afterwards.
func discoverDeps(dir *fakeDirectory, tr provider.Transport, cfg irc.Config, p *irc.Profile, clock irc.Clock) Deps {
	return Deps{
		Directory: dir,
		Transport: tr,
		Profile:   p,
		Cfg:       cfg,
		Workspace: model.Workspace{ID: "ws-1", Name: "Reed Forge"},
		Clock:     clock,
	}
}

// discoverProfile is a managed-mode profile with this workspace's own bot.
func discoverProfile() *irc.Profile {
	return &irc.Profile{
		Mode:       irc.ModeManaged,
		ServerID:   testServer,
		BotAccount: "wsm-reed",
	}
}

// namespaceState is a desired state granting one creator a namespace, spelled
// the way Desired spells it.
func namespaceState(templates []string, account, glob string) DesiredState {
	st := DesiredState{Namespaces: map[string]string{}}
	for _, name := range templates {
		st.Channels = append(st.Channels, Channel{
			Channel: provider.Channel{Name: name},
		})
	}
	if account != "" {
		st.Namespaces[account] = glob
	}
	return st
}

// TestDiscoverClassification is the classification table. Each case is one
// listed channel and the class the rules must land on, in the documented
// order: template -> peer -> adopted -> requested -> candidate -> unmanaged.
func TestDiscoverClassification(t *testing.T) {
	const glob = "#reed-forge-sami-*"

	tests := []struct {
		name string
		info provider.ChannelInfo
		// adopted seeds Profile.Discovered.
		adopted map[string]irc.Discovered
		// requested seeds the irc.adopted override.
		requested []irc.AdoptedChannel
		// wantDiscovered is whether the channel ends up adopted.
		wantDiscovered bool
		wantPeer       bool
		wantUnmanaged  bool
		wantOwner      string
	}{
		{
			// A template channel is reconcile's; discovery must not adopt
			// it even though its founder is the workspace's own bot.
			name: "template channel is left to reconcile",
			info: provider.ChannelInfo{
				Name: "#reed-forge", Registered: true, Founder: "wsm-reed",
			},
		},
		{
			// The peer rule outranks everything that could act. This
			// channel also sits inside sami's namespace, and must still
			// be a peer.
			name: "peer bot channel inside a namespace is still a peer",
			info: provider.ChannelInfo{
				Name: "#reed-forge-sami-shared", Registered: true, Founder: "wsm-blue",
			},
			wantPeer: true,
		},
		{
			name: "any wsm- account is a peer",
			info: provider.ChannelInfo{
				Name: "#something-else", Registered: true, Founder: "wsm-other",
			},
			wantPeer: true,
		},
		{
			name: "already adopted stays adopted",
			info: provider.ChannelInfo{
				Name: "#reed-forge-sami-ops", Registered: true, Founder: "sami-acct",
			},
			adopted: map[string]irc.Discovered{
				"#reed-forge-sami-ops": {Owner: "sami-acct", Joined: true},
			},
			wantDiscovered: true,
			wantOwner:      "sami-acct",
		},
		{
			// An operator request adopts a channel that matches no
			// namespace at all.
			name: "requested channel is adopted outside any namespace",
			info: provider.ChannelInfo{
				Name: "#legacy-room", Registered: true, Founder: "someone",
			},
			requested:      []irc.AdoptedChannel{{Name: "#legacy-room", Owner: "someone"}},
			wantDiscovered: true,
			wantOwner:      "someone",
		},
		{
			name: "candidate: in namespace and founded by the creator",
			info: provider.ChannelInfo{
				Name: "#reed-forge-sami-ops", Registered: true, Founder: "sami-acct",
			},
			wantDiscovered: true,
			wantOwner:      "sami-acct",
		},
		{
			// The name matches but the founder does not, so the channel
			// is NOT the creator's. This is the shared-server case: it
			// falls through to unmanaged rather than being adopted.
			name: "in namespace but foreign founder is unmanaged",
			info: provider.ChannelInfo{
				Name: "#reed-forge-sami-ops", Registered: true, Founder: "stranger",
			},
			wantUnmanaged: true,
		},
		{
			// A stem match is not a namespace match: "#...-samizdat" is
			// not inside the "sami-" namespace.
			name: "stem collision is not in the namespace",
			info: provider.ChannelInfo{
				Name: "#reed-forge-samizdat", Registered: true, Founder: "sami-acct",
			},
			wantUnmanaged: true,
		},
		{
			name: "registered channel matching nothing is unmanaged",
			info: provider.ChannelInfo{
				Name: "#random", Registered: true, Founder: "nobody",
			},
			wantUnmanaged: true,
		},
		{
			// An unregistered channel is transient occupancy. wsm has no
			// standing to drop a registration that does not exist.
			name: "unregistered channel is out of scope",
			info: provider.ChannelInfo{Name: "#hallway", Registered: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &fakeDirectory{caps: allDirectoryCaps, listed: []provider.ChannelInfo{tt.info}}
			p := discoverProfile()
			p.Discovered = tt.adopted

			cfg := irc.Config{
				Mode:              irc.ModeManaged,
				UnmanagedChannels: irc.UnmanagedReport,
				Adopted:           tt.requested,
			}
			desired := namespaceState([]string{"#reed-forge"}, "sami-acct", glob)

			got, err := Discover(context.Background(),
				discoverDeps(dir, &fakeTransport{}, cfg, p, &testClock{t: time.Now()}), desired)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}

			if gotDiscovered := len(got.Discovered) > 0; gotDiscovered != tt.wantDiscovered {
				t.Errorf("discovered = %v, want %v (%+v)", gotDiscovered, tt.wantDiscovered, got.Discovered)
			}
			if gotPeer := len(got.Peer) > 0; gotPeer != tt.wantPeer {
				t.Errorf("peer = %v, want %v (%+v)", gotPeer, tt.wantPeer, got.Peer)
			}
			if gotUnmanaged := len(got.Unmanaged) > 0; gotUnmanaged != tt.wantUnmanaged {
				t.Errorf("unmanaged = %v, want %v (%+v)", gotUnmanaged, tt.wantUnmanaged, got.Unmanaged)
			}
			if tt.wantOwner != "" && len(got.Discovered) > 0 {
				if got.Discovered[0].Owner != tt.wantOwner {
					t.Errorf("owner = %q, want %q", got.Discovered[0].Owner, tt.wantOwner)
				}
			}
		})
	}
}

// TestDiscoverPeerNeverTouched is the rule that protects another workspace's
// channels. A peer channel sitting inside this workspace's creator namespace,
// under a purge policy, past the grace, must still never be joined or purged.
//
// Mutation check: remove the peer branch from classify and this fails -- the
// channel becomes a candidate (adopted and joined) or unmanaged (purged).
func TestDiscoverPeerNeverTouched(t *testing.T) {
	const peerChannel = "#reed-forge-sami-secret"

	dir := &fakeDirectory{
		caps: allDirectoryCaps,
		listed: []provider.ChannelInfo{
			{Name: peerChannel, Registered: true, Founder: "wsm-blue"},
		},
	}
	tr := &fakeTransport{}
	p := discoverProfile()
	clock := &testClock{t: time.Now()}

	cfg := irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedPurge}
	desired := namespaceState(nil, "sami-acct", "#reed-forge-sami-*")

	deps := discoverDeps(dir, tr, cfg, p, clock)

	// First pass, then well past the grace, then again: a peer must survive
	// every one of them.
	if _, err := Discover(context.Background(), deps, desired); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	clock.advance(PurgeGrace * 3)
	got, err := Discover(context.Background(), deps, desired)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if dir.didPurge(peerChannel) {
		t.Error("a peer workspace's channel was purged")
	}
	if dir.didJoin(peerChannel) || len(tr.joined) > 0 {
		t.Error("a peer workspace's channel was joined")
	}
	if _, ok := p.Discovered[peerChannel]; ok {
		t.Error("a peer workspace's channel was adopted")
	}
	if len(got.Unmanaged) != 0 {
		t.Errorf("peer channel reported unmanaged: %+v", got.Unmanaged)
	}
	if len(got.Peer) != 1 || got.Peer[0].Name != peerChannel {
		t.Errorf("peer = %+v, want the one peer channel", got.Peer)
	}
}

// TestDiscoverPeerByLocalBotAccount pins the third peer source: a neighbour
// workspace whose bot account does not carry the wsm- prefix. Only the local
// profile scan can recognise it.
func TestDiscoverPeerByLocalBotAccount(t *testing.T) {
	dataDir := t.TempDir()
	writeNeighbourProfile(t, dataDir, "ws-blue", irc.Profile{
		ServerID: testServer, BotAccount: "bluebot",
	})

	dir := &fakeDirectory{
		caps: allDirectoryCaps,
		listed: []provider.ChannelInfo{
			{Name: "#reed-forge-sami-x", Registered: true, Founder: "bluebot"},
		},
	}
	p := discoverProfile()

	deps := discoverDeps(dir, &fakeTransport{},
		irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedPurge},
		p, &testClock{t: time.Now()})
	deps.DataDir = dataDir

	got, err := Discover(context.Background(), deps,
		namespaceState(nil, "sami-acct", "#reed-forge-sami-*"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(got.Peer) != 1 {
		t.Fatalf("peer = %+v, want the neighbour's channel", got.Peer)
	}
	if len(got.Unmanaged) != 0 {
		t.Errorf("neighbour bot's channel reported unmanaged: %+v", got.Unmanaged)
	}
}

// TestDiscoverPurgeGrace pins the ten-minute wait: an unmanaged channel is
// reported immediately but only purged once the grace has elapsed.
func TestDiscoverPurgeGrace(t *testing.T) {
	const channel = "#squatter"

	dir := &fakeDirectory{
		caps:   allDirectoryCaps,
		listed: []provider.ChannelInfo{{Name: channel, Registered: true, Founder: "stranger"}},
	}
	p := discoverProfile()
	clock := &testClock{t: time.Now()}
	deps := discoverDeps(dir, &fakeTransport{},
		irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedPurge},
		p, clock)

	got, err := Discover(context.Background(), deps, DesiredState{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if dir.didPurge(channel) {
		t.Fatal("purged on first sight, before the grace elapsed")
	}
	if len(got.Unmanaged) != 1 {
		t.Fatalf("unmanaged = %+v, want the one channel", got.Unmanaged)
	}
	if p.UnmanagedSeen[channel].IsZero() {
		t.Error("grace start was not recorded in the profile")
	}

	// Just short of the grace: still untouched.
	clock.advance(PurgeGrace - time.Second)
	if _, err := Discover(context.Background(), deps, DesiredState{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if dir.didPurge(channel) {
		t.Fatal("purged one second before the grace elapsed")
	}

	// Past it: dropped.
	clock.advance(2 * time.Second)
	got, err = Discover(context.Background(), deps, DesiredState{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !dir.didPurge(channel) {
		t.Error("not purged after the grace elapsed")
	}
	if got.Purged != 1 {
		t.Errorf("purged count = %d, want 1", got.Purged)
	}
}

// TestDiscoverGraceResets pins the reset. A channel that sat unmanaged for
// most of the grace and then became a template, an adoption or a peer must
// start from zero if it ever goes unmanaged again -- carrying the old clock
// forward would purge it almost immediately.
func TestDiscoverGraceResets(t *testing.T) {
	const channel = "#reed-forge-sami-ops"

	for _, tt := range []struct {
		name string
		// rescue is the desired state that reclassifies the channel.
		rescue  DesiredState
		founder string
	}{
		{
			name:    "becomes a template channel",
			rescue:  namespaceState([]string{channel}, "", ""),
			founder: "stranger",
		},
		{
			name:    "becomes a candidate for adoption",
			rescue:  namespaceState(nil, "sami-acct", "#reed-forge-sami-*"),
			founder: "sami-acct",
		},
		{
			name:    "becomes a peer's channel",
			rescue:  DesiredState{},
			founder: "wsm-blue",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := provider.ChannelInfo{Name: channel, Registered: true, Founder: "stranger"}
			dir := &fakeDirectory{caps: allDirectoryCaps, listed: []provider.ChannelInfo{info}}
			p := discoverProfile()
			clock := &testClock{t: time.Now()}
			deps := discoverDeps(dir, &fakeTransport{},
				irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedPurge},
				p, clock)

			// Unmanaged, and almost out of grace.
			if _, err := Discover(context.Background(), deps, DesiredState{}); err != nil {
				t.Fatalf("Discover: %v", err)
			}
			clock.advance(PurgeGrace - time.Second)

			// Rescued: the grace must be forgotten.
			dir.listed = []provider.ChannelInfo{
				{Name: channel, Registered: true, Founder: tt.founder},
			}
			if _, err := Discover(context.Background(), deps, tt.rescue); err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if _, ok := p.UnmanagedSeen[channel]; ok {
				t.Fatal("grace survived the reclassification")
			}

			// Unmanaged again, one second later. With a reset clock this is
			// far from the grace; without one it would purge at once.
			clock.advance(2 * time.Second)
			dir.listed = []provider.ChannelInfo{info}
			delete(p.Discovered, channel)
			if _, err := Discover(context.Background(), deps, DesiredState{}); err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if dir.didPurge(channel) {
				t.Error("purged using the pre-reset grace clock")
			}
		})
	}
}

// TestDiscoverReclassifiesBeforePurge is the second guard against destroying
// another workspace's channel. A channel that has been unmanaged for the whole
// grace, but is a peer's by the time the purge would happen, must not be
// dropped on the strength of the older decision.
//
// Mutation check: make sweep purge without re-running classify -- e.g. by
// purging straight from the recorded grace -- and this fails.
func TestDiscoverReclassifiesBeforePurge(t *testing.T) {
	const channel = "#contested"

	info := provider.ChannelInfo{Name: channel, Registered: true, Founder: "stranger"}
	dir := &fakeDirectory{caps: allDirectoryCaps, listed: []provider.ChannelInfo{info}}
	p := discoverProfile()
	clock := &testClock{t: time.Now()}
	deps := discoverDeps(dir, &fakeTransport{},
		irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedPurge},
		p, clock)

	if _, err := Discover(context.Background(), deps, DesiredState{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if p.UnmanagedSeen[channel].IsZero() {
		t.Fatal("grace was not started")
	}

	// The grace elapses, but the channel has changed hands to a wsm bot.
	clock.advance(PurgeGrace + time.Minute)
	dir.listed = []provider.ChannelInfo{
		{Name: channel, Registered: true, Founder: "wsm-blue"},
	}

	got, err := Discover(context.Background(), deps, DesiredState{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if dir.didPurge(channel) {
		t.Error("purged a channel that was a peer's by the time the purge ran")
	}
	if got.Purged != 0 {
		t.Errorf("purged count = %d, want 0", got.Purged)
	}
	if len(got.Peer) != 1 {
		t.Errorf("peer = %+v, want the channel reclassified as a peer", got.Peer)
	}
}

// TestDiscoverJoinRefusedRetries pins that a refused join keeps the adoption:
// Joined is false with a reason, and the next pass tries again.
func TestDiscoverJoinRefusedRetries(t *testing.T) {
	const channel = "#reed-forge-sami-ops"

	dir := &fakeDirectory{
		caps:     allDirectoryCaps,
		listed:   []provider.ChannelInfo{{Name: channel, Registered: true, Founder: "sami-acct"}},
		joinErr:  map[string]error{channel: errors.New("473 invite only")},
		founders: map[string]string{},
	}
	tr := &fakeTransport{joinErr: errors.New("473 invite only")}
	p := discoverProfile()
	deps := discoverDeps(dir, tr,
		irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedReport},
		p, &testClock{t: time.Now()})
	desired := namespaceState(nil, "sami-acct", "#reed-forge-sami-*")

	got, err := Discover(context.Background(), deps, desired)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	rec, ok := p.Discovered[channel]
	if !ok {
		t.Fatal("a refused join dropped the adoption")
	}
	if rec.Joined {
		t.Error("Joined = true after a refused join")
	}
	if rec.Reason == "" {
		t.Error("no reason recorded for the refused join")
	}
	if got.Retrying != 1 {
		t.Errorf("retrying = %d, want 1", got.Retrying)
	}

	// The next pass retries, and succeeds once the channel lets the bot in.
	delete(dir.joinErr, channel)
	if _, err := Discover(context.Background(), deps, desired); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !dir.didJoin(channel) {
		t.Error("the refused join was not retried on the next pass")
	}
	if !p.Discovered[channel].Joined {
		t.Error("Joined stayed false after a successful retry")
	}
}

// TestDiscoverReleases pins the three ways an adoption ends: the channel is
// gone from the listing, its founder changed, or the creator flag was cleared
// so the namespace no longer covers it.
func TestDiscoverReleases(t *testing.T) {
	const channel = "#reed-forge-sami-ops"

	tests := []struct {
		name string
		// next is the listing on the second pass.
		next []provider.ChannelInfo
		// desired is the state on the second pass.
		desired DesiredState
		// wantReleased is whether the adoption should be dropped.
		wantReleased bool
		// wantUnmanaged is whether it then falls under the sweeper.
		wantUnmanaged bool
	}{
		{
			name:         "channel vanishes from the listing",
			next:         nil,
			desired:      namespaceState(nil, "sami-acct", "#reed-forge-sami-*"),
			wantReleased: true,
		},
		{
			name: "founder changes hands",
			next: []provider.ChannelInfo{
				{Name: channel, Registered: true, Founder: "someone-else"},
			},
			desired:       namespaceState(nil, "sami-acct", "#reed-forge-sami-*"),
			wantReleased:  true,
			wantUnmanaged: true,
		},
		{
			// The creator flag is cleared, so Desired exports no namespace.
			// The ex-creator's channel falls under the unmanaged policy.
			name: "creator flag cleared",
			next: []provider.ChannelInfo{
				{Name: channel, Registered: true, Founder: "sami-acct"},
			},
			desired:       DesiredState{},
			wantReleased:  true,
			wantUnmanaged: true,
		},
		{
			name: "still a candidate: kept",
			next: []provider.ChannelInfo{
				{Name: channel, Registered: true, Founder: "sami-acct"},
			},
			desired:      namespaceState(nil, "sami-acct", "#reed-forge-sami-*"),
			wantReleased: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &fakeDirectory{
				caps:   allDirectoryCaps,
				listed: []provider.ChannelInfo{{Name: channel, Registered: true, Founder: "sami-acct"}},
			}
			tr := &fakeTransport{}
			p := discoverProfile()
			deps := discoverDeps(dir, tr,
				irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedReport},
				p, &testClock{t: time.Now()})

			first := namespaceState(nil, "sami-acct", "#reed-forge-sami-*")
			if _, err := Discover(context.Background(), deps, first); err != nil {
				t.Fatalf("first Discover: %v", err)
			}
			if _, ok := p.Discovered[channel]; !ok {
				t.Fatal("the channel was not adopted on the first pass")
			}

			dir.listed = tt.next
			got, err := Discover(context.Background(), deps, tt.desired)
			if err != nil {
				t.Fatalf("second Discover: %v", err)
			}

			_, stillAdopted := p.Discovered[channel]
			if stillAdopted == tt.wantReleased {
				t.Errorf("adopted = %v, want released = %v", stillAdopted, tt.wantReleased)
			}
			if tt.wantReleased && got.Released != 1 {
				t.Errorf("released count = %d, want 1", got.Released)
			}
			if gotUnmanaged := len(got.Unmanaged) > 0; gotUnmanaged != tt.wantUnmanaged {
				t.Errorf("unmanaged = %v, want %v", gotUnmanaged, tt.wantUnmanaged)
			}
			if tt.wantReleased && len(tt.next) > 0 && !tr.didPart(channel) {
				t.Error("a released channel was not parted")
			}
		})
	}
}

// TestDiscoverExternalScoping pins the prefix restriction on a shared server:
// with a prefix only channels under it are swept, and with none the sweeper
// judges nothing beyond templates and namespaces, saying why.
func TestDiscoverExternalScoping(t *testing.T) {
	// The creator's channel is listed under both spellings, because the
	// namespace it belongs to depends on the configured prefix: with
	// "acme-" the prefixed name is the creator's, with no prefix the bare
	// one is. A channel literally named "#acme-..." is not inside an
	// unprefixed workspace's namespace, and must not be treated as it.
	listing := []provider.ChannelInfo{
		{Name: "#acme-squatter", Registered: true, Founder: "stranger"},
		{Name: "#otherco-private", Registered: true, Founder: "stranger"},
		{Name: "#acme-reed-forge-sami-ops", Registered: true, Founder: "sami-acct"},
		{Name: "#reed-forge-sami-ops", Registered: true, Founder: "sami-acct"},
	}

	tests := []struct {
		name   string
		prefix string
		// wantUnmanaged names the channels the sweep may judge.
		wantUnmanaged []string
		wantWarning   bool
		// wantAdopted is the creator channel, which is classified either way.
		wantAdopted bool
	}{
		{
			// The unprefixed creator channel is outside this tenant's
			// prefix, so it is not swept either -- it is simply not this
			// workspace's business.
			name:          "prefix scopes the sweep to this tenant",
			prefix:        "acme-",
			wantUnmanaged: []string{"#acme-squatter"},
			wantAdopted:   true,
		},
		{
			// Without a prefix nothing may be swept: every classification
			// would reach across tenants. The creator channel is still
			// adopted, because a namespace match is this workspace's own.
			name:          "no prefix judges nothing, and warns",
			prefix:        "",
			wantUnmanaged: nil,
			wantWarning:   true,
			wantAdopted:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := &fakeDirectory{caps: allDirectoryCaps, listed: listing}
			p := discoverProfile()
			p.Mode = irc.ModeExternal

			cfg := irc.Config{
				Mode:              irc.ModeExternal,
				ChannelPrefix:     tt.prefix,
				UnmanagedChannels: irc.UnmanagedReport,
			}
			// The glob is spelled as Desired would spell it for this prefix.
			glob := irc.CreatorGlob(tt.prefix, "Reed Forge", "sami")
			desired := namespaceState(nil, "sami-acct", glob)

			got, err := Discover(context.Background(),
				discoverDeps(dir, &fakeTransport{}, cfg, p, &testClock{t: time.Now()}), desired)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}

			var names []string
			for _, u := range got.Unmanaged {
				names = append(names, u.Name)
			}
			if !equalStrings(names, tt.wantUnmanaged) {
				t.Errorf("unmanaged = %v, want %v", names, tt.wantUnmanaged)
			}

			// The other tenant's channel is never in scope.
			for _, n := range names {
				if n == "#otherco-private" {
					t.Error("swept another tenant's channel")
				}
			}

			hasWarning := len(got.Warnings) > 0
			if hasWarning != tt.wantWarning {
				t.Errorf("warnings = %v, want a warning = %v", got.Warnings, tt.wantWarning)
			}

			if gotAdopted := len(got.Discovered) > 0; gotAdopted != tt.wantAdopted {
				t.Errorf("discovered = %v, want %v", gotAdopted, tt.wantAdopted)
			}
		})
	}
}

// TestDiscoverUnsupportedIsReported pins that a provider which cannot do a
// step reports it and carries on. A missing capability must never fail a pass
// or, worse, look like "nothing to do".
func TestDiscoverUnsupportedIsReported(t *testing.T) {
	t.Run("cannot list", func(t *testing.T) {
		dir := &fakeDirectory{caps: 0}
		got, err := Discover(context.Background(),
			discoverDeps(dir, &fakeTransport{}, irc.Config{Mode: irc.ModeManaged},
				discoverProfile(), &testClock{t: time.Now()}), DesiredState{})
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if got.Skipped != "cap:list_channels" {
			t.Errorf("skipped = %q, want cap:list_channels", got.Skipped)
		}
	})

	t.Run("list returns ErrUnsupported", func(t *testing.T) {
		dir := &fakeDirectory{caps: allDirectoryCaps, listErr: provider.ErrUnsupported}
		got, err := Discover(context.Background(),
			discoverDeps(dir, &fakeTransport{}, irc.Config{Mode: irc.ModeManaged},
				discoverProfile(), &testClock{t: time.Now()}), DesiredState{})
		if err != nil {
			t.Fatalf("ErrUnsupported must not fail the pass: %v", err)
		}
		if got.Skipped == "" {
			t.Error("an unsupported listing was not reported as skipped")
		}
	})

	t.Run("cannot purge", func(t *testing.T) {
		const channel = "#squatter"
		dir := &fakeDirectory{
			caps:   provider.CapListChannels | provider.CapChannelFounder,
			listed: []provider.ChannelInfo{{Name: channel, Registered: true, Founder: "stranger"}},
		}
		p := discoverProfile()
		clock := &testClock{t: time.Now()}
		deps := discoverDeps(dir, &fakeTransport{},
			irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedPurge},
			p, clock)

		if _, err := Discover(context.Background(), deps, DesiredState{}); err != nil {
			t.Fatalf("Discover: %v", err)
		}
		clock.advance(PurgeGrace + time.Minute)

		got, err := Discover(context.Background(), deps, DesiredState{})
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if got.Purged != 0 {
			t.Errorf("purged = %d on a provider that cannot purge", got.Purged)
		}
		if len(got.Unmanaged) != 1 {
			t.Errorf("unmanaged = %+v, want the channel still reported", got.Unmanaged)
		}
		var found bool
		for _, w := range got.Warnings {
			if strings.Contains(w, "purge_channel") {
				found = true
			}
		}
		if !found {
			t.Errorf("warnings = %v, want the missing purge capability", got.Warnings)
		}
	})
}

// TestDiscoverIgnorePolicy pins that the ignore policy records nothing at all:
// no report entry, and no grace clock started behind the operator's back.
func TestDiscoverIgnorePolicy(t *testing.T) {
	const channel = "#squatter"

	dir := &fakeDirectory{
		caps:   allDirectoryCaps,
		listed: []provider.ChannelInfo{{Name: channel, Registered: true, Founder: "stranger"}},
	}
	p := discoverProfile()

	got, err := Discover(context.Background(),
		discoverDeps(dir, &fakeTransport{},
			irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedIgnore},
			p, &testClock{t: time.Now()}), DesiredState{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(got.Unmanaged) != 0 {
		t.Errorf("unmanaged = %+v, want none under the ignore policy", got.Unmanaged)
	}
	if len(p.UnmanagedSeen) != 0 {
		t.Errorf("grace recorded under the ignore policy: %v", p.UnmanagedSeen)
	}
}

// TestDiscoverOperatorRelease pins that an irc.released entry drops an
// adoption the daemon would otherwise keep.
func TestDiscoverOperatorRelease(t *testing.T) {
	const channel = "#reed-forge-sami-ops"

	dir := &fakeDirectory{
		caps:   allDirectoryCaps,
		listed: []provider.ChannelInfo{{Name: channel, Registered: true, Founder: "sami-acct"}},
	}
	tr := &fakeTransport{}
	p := discoverProfile()
	desired := namespaceState(nil, "sami-acct", "#reed-forge-sami-*")

	deps := discoverDeps(dir, tr,
		irc.Config{Mode: irc.ModeManaged, UnmanagedChannels: irc.UnmanagedReport},
		p, &testClock{t: time.Now()})

	if _, err := Discover(context.Background(), deps, desired); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if _, ok := p.Discovered[channel]; !ok {
		t.Fatal("not adopted on the first pass")
	}

	deps.Cfg.Released = []string{channel}
	got, err := Discover(context.Background(), deps, desired)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if _, ok := p.Discovered[channel]; ok {
		t.Error("a released channel is still adopted")
	}
	if got.Released != 1 {
		t.Errorf("released = %d, want 1", got.Released)
	}
	if !tr.didPart(channel) {
		t.Error("a released channel was not parted")
	}
}
