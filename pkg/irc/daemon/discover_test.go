package daemon

import (
	"context"
	"sync"
	"testing"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provision"
)

// stubDirectory is a directory provider that reports no capability. The
// daemon tests care only that a non-nil Directory reaches discovery: what it
// answers is provision's business, exhaustively covered there.
type stubDirectory struct{}

func (stubDirectory) Caps() provider.Caps { return 0 }

func (stubDirectory) ListChannels(context.Context) ([]provider.ChannelInfo, error) {
	return nil, provider.ErrUnsupported
}

func (stubDirectory) Founder(context.Context, string) (string, error) {
	return "", provider.ErrUnsupported
}

func (stubDirectory) JoinHidden(context.Context, string) error { return provider.ErrUnsupported }

func (stubDirectory) Purge(context.Context, string) error { return provider.ErrUnsupported }

func (stubDirectory) RelayedIdentity(ircmsg.Message) (provider.RelayIdentity, bool) {
	return provider.RelayIdentity{}, false
}

// discoverSpy counts discovery passes and records what each was handed.
type discoverSpy struct {
	mu      sync.Mutex
	passes  int
	desired []provision.DesiredState
	report  provision.DiscoverReport
}

func (s *discoverSpy) hook(
	_ context.Context,
	_ provision.Deps,
	desired provision.DesiredState,
) (provision.DiscoverReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.passes++
	s.desired = append(s.desired, desired)
	return s.report, nil
}

func (s *discoverSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.passes
}

// withDiscovery wires a spy and a non-nil directory into a test daemon.
//
// The stock harness supplies a nil Directory, which discovery correctly
// declines to run against; a test that left it nil would pass while proving
// nothing.
func withDiscovery(spy *discoverSpy) func(*Deps) {
	return func(d *Deps) {
		inner := d.Hooks.OpenProviders
		d.Hooks.OpenProviders = func(dm *Daemon, pd provider.Deps) (provider.Providers, error) {
			providers, err := inner(dm, pd)
			if err != nil {
				return providers, err
			}
			providers.Directory = stubDirectory{}
			return providers, nil
		}
		d.Hooks.Discover = spy.hook
	}
}

// TestDiscoverRunsOnReconcile pins that a reconcile carries a discovery pass
// with it. Discovery classifies against the template set the reconcile just
// computed, so the two must happen together rather than drift apart.
func TestDiscoverRunsOnReconcile(t *testing.T) {
	spy := &discoverSpy{}
	env := newTestEnv(t, irc.ModeManaged, withDiscovery(spy))

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	// start runs the first reconcile, which must have carried discovery.
	if got := spy.count(); got == 0 {
		t.Fatal("no discovery pass ran on start")
	}

	before := spy.count()
	env.daemon.reconcile(context.Background())
	if got := spy.count(); got <= before {
		t.Errorf("discovery passes = %d, want more than %d after a reconcile", got, before)
	}
}

// TestDiscoverPeriodicSweep pins the standalone sweep. A creator's new channel
// and an unmanaged registration are events nothing in wsm triggers, so
// discovery has to run on its own clock rather than only alongside reconcile.
func TestDiscoverPeriodicSweep(t *testing.T) {
	spy := &discoverSpy{}
	env := newTestEnv(t, irc.ModeManaged, withDiscovery(spy))

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	before := spy.count()

	// The ticker fires this directly. Waiting out DiscoverInterval would
	// trade a minute of wall time for no extra confidence in the wiring.
	env.daemon.discover(context.Background())

	if got := spy.count(); got != before+1 {
		t.Errorf("discovery passes = %d, want exactly one more than %d", got, before)
	}
}

// TestDiscoverReportReachesStatus pins that what discovery found is what
// status.json publishes: the CLI reads these three lists from the document.
func TestDiscoverReportReachesStatus(t *testing.T) {
	spy := &discoverSpy{
		report: provision.DiscoverReport{
			Discovered: []provision.DiscoveredChannel{
				{Name: "#acme-sami-ops", Owner: "sami-acct", Joined: true},
			},
			Peer: []provision.PeerChannel{
				{Name: "#blue-dev", Founder: "wsm-blue"},
			},
			Unmanaged: []provision.UnmanagedChannel{
				{Name: "#squatter", Founder: "stranger"},
			},
		},
	}
	env := newTestEnv(t, irc.ModeManaged, withDiscovery(spy))

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	status := env.daemon.Status()

	if len(status.Discovered) != 1 || status.Discovered[0].Name != "#acme-sami-ops" {
		t.Errorf("status discovered = %+v, want the adopted channel", status.Discovered)
	}
	if len(status.Peer) != 1 || status.Peer[0].Founder != "wsm-blue" {
		t.Errorf("status peer = %+v, want the peer channel", status.Peer)
	}
	if len(status.Unmanaged) != 1 || status.Unmanaged[0].Name != "#squatter" {
		t.Errorf("status unmanaged = %+v, want the unmanaged channel", status.Unmanaged)
	}
	if status.LastDiscover == nil {
		t.Error("status carries no discovery report")
	}

	// The document must survive a round trip: the CLI reads it back from
	// disk, not from this struct.
	if err := WriteStatus(env.paths.Status, status); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	got, err := ReadStatus(env.paths.Status)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if len(got.Discovered) != 1 || len(got.Peer) != 1 || len(got.Unmanaged) != 1 {
		t.Errorf("round-tripped status lost discovery: %+v", got)
	}
	// The fields that were already in the document must still decode, or the
	// addition was not additive.
	if got.Mode != status.Mode || got.State != status.State || got.PID != status.PID {
		t.Errorf("round trip changed pre-existing fields: %+v", got)
	}
}

// TestDiscoverSkippedWithoutDirectory pins that a provider with no directory
// half is simply not discovered against. It must not panic, and must not
// invent an empty result that looks like "the server has no channels".
func TestDiscoverSkippedWithoutDirectory(t *testing.T) {
	spy := &discoverSpy{}
	env := newTestEnv(t, irc.ModeManaged, func(d *Deps) {
		// Directory stays nil, as the stock harness leaves it.
		d.Hooks.Discover = spy.hook
	})

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	env.daemon.discover(context.Background())

	if got := spy.count(); got != 0 {
		t.Errorf("discovery ran %d times with no directory provider, want 0", got)
	}
	if status := env.daemon.Status(); status.LastDiscover != nil {
		t.Errorf("status carries a discovery report with no directory: %+v", status.LastDiscover)
	}
}
