package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot"
	"hop.top/wsm/pkg/irc/bot/bottest"
	"hop.top/wsm/pkg/irc/managed"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provision"
	"hop.top/wsm/pkg/irc/supervisor"
	"hop.top/wsm/pkg/model"
)

// shutdownStep is one entry in the shutdown order log. The TERM test asserts
// on the sequence, so the steps are recorded rather than merely counted.
type shutdownStep string

const (
	stepBotQuit  shutdownStep = "bot-quit"
	stepBotClose shutdownStep = "bot-close"
	stepErgoStop shutdownStep = "ergo-stop"
)

// orderLog records shutdown steps in the order they happened.
type orderLog struct {
	mu    sync.Mutex
	steps []shutdownStep
}

func (l *orderLog) add(s shutdownStep) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.steps = append(l.steps, s)
}

func (l *orderLog) all() []shutdownStep {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]shutdownStep, len(l.steps))
	copy(out, l.steps)
	return out
}

// fakeSupervisor stands in for the managed ergo child.
type fakeSupervisor struct {
	order *orderLog

	mu       sync.Mutex
	started  int
	stopped  int
	state    supervisor.State
	startErr error
}

func (f *fakeSupervisor) Start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.started++
	f.state = supervisor.State{Phase: supervisor.PhaseRunning, PID: 4242}
	return nil
}

func (f *fakeSupervisor) Stop(context.Context) error {
	if f.order != nil {
		f.order.add(stepErgoStop)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	f.state = supervisor.State{Phase: supervisor.PhaseStopped}
	return nil
}

func (f *fakeSupervisor) Status() supervisor.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *fakeSupervisor) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started, f.stopped
}

// recordingTransport wraps the bottest fake so shutdown steps are observable.
type recordingTransport struct {
	*bottest.FakeTransport
	order *orderLog
}

func (t *recordingTransport) Send(ctx context.Context, raw string) error {
	if strings.HasPrefix(raw, "QUIT") && t.order != nil {
		t.order.add(stepBotQuit)
	}
	return t.FakeTransport.Send(ctx, raw)
}

func (t *recordingTransport) Close() {
	if t.order != nil {
		t.order.add(stepBotClose)
	}
	t.FakeTransport.Close()
}

// fakeRecorder swallows events; capture is not what these tests assert on.
type fakeRecorder struct {
	mu     sync.Mutex
	events []model.Event
}

func (r *fakeRecorder) Record(_ context.Context, ev model.Event) (model.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return ev, nil
}

// nopAccounts and nopRoles are provider stubs with no capabilities: these
// tests exercise the daemon's wiring, not a backend's behaviour.
type nopAccounts struct{}

func (nopAccounts) Caps() provider.Caps { return 0 }
func (nopAccounts) CreateAccount(context.Context, string, string) error {
	return provider.ErrUnsupported
}
func (nopAccounts) SetPassword(context.Context, string, string) error {
	return provider.ErrUnsupported
}
func (nopAccounts) Suspend(context.Context, string) error { return provider.ErrUnsupported }

type nopRoles struct {
	mu   sync.Mutex
	nick string
}

func (r *nopRoles) Caps() provider.Caps { return 0 }
func (r *nopRoles) EnsureChannel(context.Context, provider.Channel) error {
	return provider.ErrUnsupported
}
func (r *nopRoles) ListRoles(context.Context, string) (map[string]irc.Role, error) {
	return map[string]irc.Role{}, nil
}
func (r *nopRoles) ApplyRoles(context.Context, string, []provider.ModeChange, []provider.ModeChange) error {
	return nil
}
func (r *nopRoles) MyRole(context.Context, string) (irc.Role, error) { return "", nil }

// SetNick is the injection point the plain provider exposes. Recording it is
// how the wiring test proves the daemon calls it.
func (r *nopRoles) SetNick(nick string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nick = nick
}

func (r *nopRoles) gotNick() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nick
}

// testEnv is a daemon wired entirely to fakes, plus the fakes themselves.
type testEnv struct {
	daemon    *Daemon
	sup       *fakeSupervisor
	transport *recordingTransport
	roles     *nopRoles
	order     *orderLog
	paths     irc.Paths

	mu             sync.Mutex
	reconciles     int
	reconcileDeps  []provision.Deps
	reconcileDelay time.Duration
}

// newTestEnv builds a daemon for one mode with every seam faked.
func newTestEnv(t *testing.T, mode irc.Mode, mutate func(*Deps)) *testEnv {
	t.Helper()

	root := t.TempDir()
	paths, err := irc.ResolvePaths(irc.PathsInput{
		DataDir:  filepath.Join(root, "data"),
		StateDir: filepath.Join(root, "state"),
	}, "01JWORKSPACE")
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}

	// A workspace root with a local config, so members.yaml resolves under it
	// rather than against the test process's working directory.
	wsRoot := filepath.Join(root, "ws")
	if err := os.MkdirAll(filepath.Join(wsRoot, ".wsm"), 0o700); err != nil {
		t.Fatalf("mkdir ws root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsRoot, ".wsm", "config.yaml"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write ws config: %v", err)
	}

	secrets := irc.Secrets{"wsm-acme": "hunter2"}
	if err := secrets.Save(paths.Secrets); err != nil {
		t.Fatalf("save secrets: %v", err)
	}

	profile := irc.Profile{
		Mode:       mode,
		Provider:   irc.ProviderPlain,
		ServerID:   "irc.example:6697",
		Root:       wsRoot,
		BotAccount: "wsm-acme",
	}
	cfg := irc.Config{
		Bot:    irc.Bot{Account: "wsm-acme"},
		Server: irc.Server{Host: "irc.example", Port: 6697},
	}
	if mode == irc.ModeManaged {
		profile.ServerID = irc.ManagedServerID
		profile.Ports = irc.Ports{Plain: 6700, Remote: 6701, API: 6702}
		if err := irc.SaveAPIToken(paths.APIToken, "token-1"); err != nil {
			t.Fatalf("save api token: %v", err)
		}
	}

	env := &testEnv{
		order: &orderLog{},
		roles: &nopRoles{},
		paths: paths,
	}
	env.sup = &fakeSupervisor{order: env.order}
	env.transport = &recordingTransport{
		FakeTransport: bottest.New("multi-prefix", "server-time"),
		order:         env.order,
	}

	deps := Deps{
		Cfg:       cfg,
		Paths:     paths,
		Profile:   profile,
		Workspace: model.Workspace{ID: "01JWORKSPACE", Name: "acme"},
		DataDir:   filepath.Join(root, "data"),
		StateDir:  filepath.Join(root, "state"),
		DeviceID:  "dev-1",
		Hooks: Hooks{
			NewSupervisor: func(*Daemon) Supervisor { return env.sup },
			NewTransport:  func(bot.Options) Transport { return env.transport },
			OpenProviders: func(d *Daemon, _ provider.Deps) (provider.Providers, error) {
				// Mirror the real wiring: the plain provider learns the bot's
				// nick through SetNick.
				providers := provider.Providers{
					Accounts:  nopAccounts{},
					Roles:     env.roles,
					Directory: nil,
				}
				if setter, ok := providers.Roles.(interface{ SetNick(string) }); ok {
					setter.SetNick(d.botAccount())
				}
				return providers, nil
			},
			Reconcile: func(_ context.Context, deps provision.Deps) (provision.Report, error) {
				env.mu.Lock()
				env.reconciles++
				env.reconcileDeps = append(env.reconcileDeps, deps)
				delay := env.reconcileDelay
				env.mu.Unlock()
				if delay > 0 {
					time.Sleep(delay)
				}
				return provision.Report{ProviderCaps: []string{"none"}}, nil
			},
			NewRecorder: func(*Daemon) bot.Recorder { return &fakeRecorder{} },
			APIStatus: func(context.Context, *provider.APIConfig) (string, error) {
				return "2.19.1", nil
			},
			Signal: func(int, syscall.Signal) error { return nil },
		},
	}
	if mutate != nil {
		mutate(&deps)
	}

	d, err := New(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.daemon = d
	return env
}

func (e *testEnv) reconcileCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reconciles
}

// TestStartManagedWiring checks managed mode starts the child, connects, and
// records the server version.
func TestStartManagedWiring(t *testing.T) {
	env := newTestEnv(t, irc.ModeManaged, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	started, _ := env.sup.counts()
	if started != 1 {
		t.Errorf("supervisor started %d times, want 1", started)
	}
	if env.daemon.ergoVersion != "2.19.1" {
		t.Errorf("ergoVersion = %q, want 2.19.1", env.daemon.ergoVersion)
	}

	status := env.daemon.Status()
	if status.Mode != irc.ModeManaged {
		t.Errorf("status mode = %q, want managed", status.Mode)
	}
	if status.Server.Host != irc.DefaultBind || status.Server.Port != 6700 {
		t.Errorf("server = %+v, want the loopback plaintext listener", status.Server)
	}
	if status.ErgoPID != 4242 {
		t.Errorf("ergo pid = %d, want 4242", status.ErgoPID)
	}
}

// TestStartExternalWiring checks external mode starts no child and describes
// the configured server.
func TestStartExternalWiring(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	started, _ := env.sup.counts()
	if started != 0 {
		t.Errorf("supervisor started %d times in external mode, want 0", started)
	}
	if env.daemon.sup != nil {
		t.Error("external mode built a supervisor")
	}

	status := env.daemon.Status()
	if status.Server.Host != "irc.example" || status.Server.Port != 6697 {
		t.Errorf("server = %+v, want the configured external server", status.Server)
	}
	if status.ErgoPID != 0 {
		t.Errorf("ergo pid = %d, want 0 in external mode", status.ErgoPID)
	}
}

// TestModeBranchesOnProfileNotConfig pins the rule that every branch reads the
// profile.
//
// Config.Mode only seeds the profile at enable. A daemon that branched on the
// config would start supervising an ergo for a workspace that was enabled
// against somebody else's server, purely because the global config was edited.
func TestModeBranchesOnProfileNotConfig(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, func(d *Deps) {
		// The config says managed; the profile says external and wins.
		d.Cfg.Mode = irc.ModeManaged
	})

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	if started, _ := env.sup.counts(); started != 0 {
		t.Errorf("supervisor started %d times: the daemon branched on Config.Mode", started)
	}
}

// TestPlainProviderLearnsBotNick is the SetNick trap.
//
// The plain provider assumes nick equals account and defaults to
// irc.DefaultBotAccount. A deployment whose bot nick differs and whose
// provider was never told cannot find itself in NAMES, so every ApplyRoles
// reports ErrNotOpped and no role is ever applied — silently.
func TestPlainProviderLearnsBotNick(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	if got := env.roles.gotNick(); got != "wsm-acme" {
		t.Errorf("provider nick = %q, want %q (SetNick was not called with the profile's bot account)",
			got, "wsm-acme")
	}
	if got := env.roles.gotNick(); got == irc.DefaultBotAccount {
		t.Errorf("provider nick = %q: left at the package default", got)
	}
}

// TestShutdownQuitsBotBeforeStoppingErgo is the TERM ordering.
//
// Stopping the child first drops the bot's connection underneath it, which is
// indistinguishable from a server fault: the transport starts reconnecting to
// a server that is deliberately going away while the supervisor is being asked
// to stop it. The bot must leave first.
func TestShutdownQuitsBotBeforeStoppingErgo(t *testing.T) {
	env := newTestEnv(t, irc.ModeManaged, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	env.daemon.shutdown()

	steps := env.order.all()
	quit, stop := -1, -1
	for i, s := range steps {
		switch s {
		case stepBotQuit:
			if quit < 0 {
				quit = i
			}
		case stepErgoStop:
			if stop < 0 {
				stop = i
			}
		}
	}

	if quit < 0 {
		t.Fatalf("no QUIT was sent; steps = %v", steps)
	}
	if stop < 0 {
		t.Fatalf("ergo was never stopped; steps = %v", steps)
	}
	if quit > stop {
		t.Errorf("ergo stopped before the bot quit; steps = %v", steps)
	}
}

// TestShutdownClosesTransportBeforeStoppingErgo checks the whole bot teardown,
// not just the QUIT line, precedes the child's stop.
func TestShutdownClosesTransportBeforeStoppingErgo(t *testing.T) {
	env := newTestEnv(t, irc.ModeManaged, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	env.daemon.shutdown()

	steps := env.order.all()
	closed, stop := -1, -1
	for i, s := range steps {
		switch s {
		case stepBotClose:
			if closed < 0 {
				closed = i
			}
		case stepErgoStop:
			if stop < 0 {
				stop = i
			}
		}
	}

	if closed < 0 || stop < 0 {
		t.Fatalf("incomplete shutdown; steps = %v", steps)
	}
	if closed > stop {
		t.Errorf("ergo stopped before the transport closed; steps = %v", steps)
	}
}

// TestReconcileRequestsCoalesce is the SIGHUP burst behaviour.
//
// A storm of HUPs must cost one reconcile, not one each: reconcile talks to
// the server and rewrites the profile, so queueing N of them turns a
// well-meant `wsm irc sync` loop into a self-inflicted flood. A request that
// arrives while one is already pending is dropped, because the pending run
// observes the same world the dropped one would have.
func TestReconcileRequestsCoalesce(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	for range 50 {
		env.daemon.Reconcile()
	}

	if got := len(env.daemon.reconcileCh); got != 1 {
		t.Errorf("queued reconcile requests = %d, want 1: a burst was not coalesced", got)
	}
}

// TestReconcileBurstRunsOnce drives the serve loop and counts actual runs.
func TestReconcileBurstRunsOnce(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	// start() runs one reconcile of its own.
	baseline := env.reconcileCount()

	// A burst arrives while nothing is draining the channel.
	for range 25 {
		env.daemon.Reconcile()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = env.daemon.serve(ctx)
	}()

	// Give the loop time to drain whatever was queued.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if env.reconcileCount() > baseline {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if got := env.reconcileCount() - baseline; got != 1 {
		t.Errorf("reconciles from a burst of 25 = %d, want 1", got)
	}
}

// TestBackfillRequestsCoalesce checks on-demand backfill is coalesced the same
// way, for the same reason.
func TestBackfillRequestsCoalesce(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	for range 30 {
		env.daemon.Backfill()
	}

	if got := len(env.daemon.backfillCh); got != 1 {
		t.Errorf("queued backfill requests = %d, want 1", got)
	}
}

// TestReconcileKeepsStateWhenMembersInvalid checks a broken members file
// degrades rather than wiping roles.
func TestReconcileKeepsStateWhenMembersInvalid(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	before := env.reconcileCount()
	if env.daemon.lastReport == nil {
		t.Fatal("no report from the first reconcile")
	}

	// Break members.yaml, then ask for another reconcile.
	membersPath := filepath.Join(env.daemon.profile.Root, ".wsm", "members.yaml")
	if err := os.WriteFile(membersPath, []byte("members: [[[\n"), 0o600); err != nil {
		t.Fatalf("write members: %v", err)
	}
	env.daemon.reconcile(context.Background())

	if got := env.reconcileCount(); got != before {
		t.Errorf("reconcile ran %d extra times despite an invalid members file, want 0", got-before)
	}
	if env.daemon.lastReport == nil {
		t.Error("the previous report was discarded when members.yaml failed to parse")
	}

	status := env.daemon.Status()
	if status.State != StateDegraded {
		t.Errorf("state = %q, want %q", status.State, StateDegraded)
	}
	found := false
	for _, r := range status.Reasons {
		if strings.HasPrefix(r, "members.yaml invalid:") {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons = %v, want one naming the invalid members file", status.Reasons)
	}
}

// TestStatusTransitions walks running to degraded and back.
func TestStatusTransitions(t *testing.T) {
	env := newTestEnv(t, irc.ModeManaged, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	env.daemon.markConnected()
	if got := env.daemon.Status().State; got != StateRunning {
		t.Errorf("state = %q, want %q", got, StateRunning)
	}

	// The child dies for good.
	env.sup.mu.Lock()
	env.sup.state = supervisor.State{Phase: supervisor.PhaseDegraded, LastErr: "exit 1"}
	env.sup.mu.Unlock()

	if got := env.daemon.Status().State; got != StateDegraded {
		t.Errorf("state after the child degraded = %q, want %q", got, StateDegraded)
	}

	// It comes back.
	env.sup.mu.Lock()
	env.sup.state = supervisor.State{Phase: supervisor.PhaseRunning, PID: 77}
	env.sup.mu.Unlock()

	status := env.daemon.Status()
	if status.State != StateRunning {
		t.Errorf("state after recovery = %q, want %q", status.State, StateRunning)
	}
	if status.ErgoPID != 77 {
		t.Errorf("ergo pid = %d, want 77", status.ErgoPID)
	}
}

// TestStatusFileIsWritten checks the document reaches disk where the CLI
// expects it.
func TestStatusFileIsWritten(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	env.daemon.writeStatus()

	got, err := ReadStatus(env.paths.Status)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if got.Mode != irc.ModeExternal {
		t.Errorf("mode = %q, want external", got.Mode)
	}
	if got.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", got.PID, os.Getpid())
	}
}

// TestReconnectTriggersBackfill checks a recovered session re-fetches the
// history it missed while it was down.
func TestReconnectTriggersBackfill(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	// A cursor exists, so there is a position to resume from.
	env.daemon.cursors.Set("#acme", bot.Cursor{
		LastMsgID: "m1",
		LastTime:  time.Now().Add(-time.Hour),
	})
	env.daemon.mu.Lock()
	env.daemon.channels = []string{"#acme"}
	env.daemon.mu.Unlock()

	env.daemon.statusMu.Lock()
	env.daemon.lastBackfill = nil
	env.daemon.statusMu.Unlock()

	// The transport says it is back.
	env.transport.Reconnect()

	env.daemon.statusMu.Lock()
	summary := env.daemon.lastBackfill
	env.daemon.statusMu.Unlock()

	if summary == nil {
		t.Fatal("a reconnect did not run a backfill")
	}
}

// TestBackfillSkipsChannelsWithoutCursor checks an uncaptured channel is left
// to live capture rather than replayed from the beginning.
func TestBackfillSkipsChannelsWithoutCursor(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	env.daemon.mu.Lock()
	env.daemon.channels = []string{"#never-captured"}
	env.daemon.mu.Unlock()

	env.daemon.backfillAll(context.Background())

	env.daemon.statusMu.Lock()
	summary := env.daemon.lastBackfill
	env.daemon.statusMu.Unlock()

	if summary == nil {
		t.Fatal("no summary")
	}
	if len(summary.Channels) != 0 {
		t.Errorf("channels = %+v, want none: a channel with no cursor has nothing to resume from",
			summary.Channels)
	}
}

// TestDelegateGrantsRecorded is the provenance a revoke depends on.
//
// A revoke strips only the roles wsm can prove the departing bot granted, so
// the observation has to happen here, while the mode change is on the wire.
func TestDelegateGrantsRecorded(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, func(d *Deps) {
		d.Cfg.Channels = []irc.ChannelTemplate{{
			Name:      "#{{ws}}",
			Delegates: []irc.Delegate{{Workspace: "guest", Bot: "wsm-guest", MaxRole: irc.RoleOp}},
		}}
	})

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	channel := irc.ChannelName("", "#{{ws}}", "acme")

	env.daemon.handleMode(bottest.MustParse(
		":wsm-guest!u@h MODE " + channel + " +o alice",
	))

	env.daemon.mu.Lock()
	role, ok := env.daemon.profile.DelegateGrants[channel]["wsm-guest"]["alice"]
	env.daemon.mu.Unlock()

	if !ok {
		t.Fatalf("no grant recorded for alice; grants = %+v", env.daemon.profile.DelegateGrants)
	}
	if role != irc.RoleOp {
		t.Errorf("role = %q, want %q", role, irc.RoleOp)
	}

	// The minus form forgets it again.
	env.daemon.handleMode(bottest.MustParse(
		":wsm-guest!u@h MODE " + channel + " -o alice",
	))

	env.daemon.mu.Lock()
	_, stillThere := env.daemon.profile.DelegateGrants[channel]["wsm-guest"]["alice"]
	env.daemon.mu.Unlock()

	if stillThere {
		t.Error("a revoked grant is still recorded as provenance")
	}
}

// TestDelegateGrantsIgnoreNonDelegates checks a mode set by anyone else is
// never recorded as something wsm may later strip.
func TestDelegateGrantsIgnoreNonDelegates(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, func(d *Deps) {
		d.Cfg.Channels = []irc.ChannelTemplate{{
			Name:      "#{{ws}}",
			Delegates: []irc.Delegate{{Workspace: "guest", Bot: "wsm-guest", MaxRole: irc.RoleOp}},
		}}
	})

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	channel := irc.ChannelName("", "#{{ws}}", "acme")

	env.daemon.handleMode(bottest.MustParse(
		":a-human!u@h MODE " + channel + " +o alice",
	))

	env.daemon.mu.Lock()
	grants := env.daemon.profile.DelegateGrants[channel]
	env.daemon.mu.Unlock()

	if len(grants) != 0 {
		t.Errorf("grants = %+v, want none: a human's mode change is foreign", grants)
	}
}

// TestDelegateGrantsPersist checks the provenance survives a restart, since
// the revoke that consumes it runs in a later process.
func TestDelegateGrantsPersist(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, func(d *Deps) {
		d.Cfg.Channels = []irc.ChannelTemplate{{
			Name:      "#{{ws}}",
			Delegates: []irc.Delegate{{Workspace: "guest", Bot: "wsm-guest", MaxRole: irc.RoleVoice}},
		}}
	})

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	channel := irc.ChannelName("", "#{{ws}}", "acme")
	env.daemon.handleMode(bottest.MustParse(
		":wsm-guest!u@h MODE " + channel + " +v bob",
	))

	saved, err := irc.LoadProfile(env.paths.Profile)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if saved.DelegateGrants[channel]["wsm-guest"]["bob"] != irc.RoleVoice {
		t.Errorf("persisted grants = %+v, want bob at voice", saved.DelegateGrants)
	}
}

// TestParseModeChanges covers the argument alignment, which is what decides
// whether a recorded grant names the right account.
func TestParseModeChanges(t *testing.T) {
	cases := []struct {
		name  string
		modes string
		args  []string
		want  []modeChange
		ok    bool
	}{
		{
			name:  "single grant",
			modes: "+o",
			args:  []string{"alice"},
			want:  []modeChange{{add: true, role: irc.RoleOp, account: "alice"}},
			ok:    true,
		},
		{
			name:  "mixed signs",
			modes: "+o-v",
			args:  []string{"alice", "bob"},
			want: []modeChange{
				{add: true, role: irc.RoleOp, account: "alice"},
				{add: false, role: irc.RoleVoice, account: "bob"},
			},
			ok: true,
		},
		{
			name:  "key consumes its argument before a role",
			modes: "+ko",
			args:  []string{"sekrit", "alice"},
			want:  []modeChange{{add: true, role: irc.RoleOp, account: "alice"}},
			ok:    true,
		},
		{
			name:  "limit takes an argument when set",
			modes: "+lv",
			args:  []string{"50", "bob"},
			want:  []modeChange{{add: true, role: irc.RoleVoice, account: "bob"}},
			ok:    true,
		},
		{
			name:  "limit takes none when cleared",
			modes: "-l+v",
			args:  []string{"bob"},
			want:  []modeChange{{add: true, role: irc.RoleVoice, account: "bob"}},
			ok:    true,
		},
		{
			name:  "argless modes do not consume",
			modes: "+mnt-o",
			args:  []string{"alice"},
			want:  []modeChange{{add: false, role: irc.RoleOp, account: "alice"}},
			ok:    true,
		},
		{
			name:  "halfop",
			modes: "+h",
			args:  []string{"carol"},
			want:  []modeChange{{add: true, role: irc.RoleHalfop, account: "carol"}},
			ok:    true,
		},
		{
			name:  "unknown letter abandons the line",
			modes: "+Zo",
			args:  []string{"whatever", "alice"},
			want:  nil,
			ok:    false,
		},
		{
			name:  "missing argument abandons the line",
			modes: "+o",
			args:  nil,
			want:  nil,
			ok:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseModeChanges(tc.modes, tc.args)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("changes = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("change %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestLockIsExclusive checks a second daemon refuses rather than joining in.
func TestLockIsExclusive(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.acquireLock(); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(env.daemon.releaseLock)

	second := newTestEnv(t, irc.ModeExternal, func(d *Deps) {
		d.Paths = env.paths
	})
	err := second.daemon.acquireLock()
	if err == nil {
		second.daemon.releaseLock()
		t.Fatal("a second daemon acquired the same workspace lock")
	}

	var locked *LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("err = %v, want a LockedError", err)
	}
	if !strings.Contains(locked.Error(), "wsm irc stop") {
		t.Errorf("message = %q, want a hint naming the fix", locked.Error())
	}
}

// TestLockReleasedOnExit checks the lock is reusable once the holder is gone.
func TestLockReleasedOnExit(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.acquireLock(); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	env.daemon.releaseLock()

	second := newTestEnv(t, irc.ModeExternal, func(d *Deps) {
		d.Paths = env.paths
	})
	if err := second.daemon.acquireLock(); err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	second.daemon.releaseLock()
}

// TestPIDFileLifecycle checks the pidfile is written and removed.
func TestPIDFileLifecycle(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.writePID(); err != nil {
		t.Fatalf("writePID: %v", err)
	}
	raw, err := os.ReadFile(env.paths.DaemonPID)
	if err != nil {
		t.Fatalf("read pidfile: %v", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		t.Error("pidfile is empty")
	}

	env.daemon.removePID()
	if _, err := os.Stat(env.paths.DaemonPID); !os.IsNotExist(err) {
		t.Errorf("pidfile still present after removal: %v", err)
	}
}

// TestOnUnauthorizedExternalIsRefused checks wsm does not try to rotate a
// credential on a server it does not own.
func TestOnUnauthorizedExternalIsRefused(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	err := env.daemon.onUnauthorized(context.Background())
	if err == nil {
		t.Fatal("external mode attempted a token rotation")
	}
	if !strings.Contains(err.Error(), "does not own") {
		t.Errorf("err = %v, want it to say wsm does not own the server", err)
	}
}

// TestOnUnauthorizedManagedReloadsOutOfBand is the token rotation sequence.
//
// The API cannot reload its own tokens: every endpoint, /v1/rehash included,
// authenticates against the config that is loaded. So the new token has to be
// written, rendered, and made live from outside the API before any retry is
// worth making.
func TestOnUnauthorizedManagedReloadsOutOfBand(t *testing.T) {
	var (
		mu        sync.Mutex
		signals   []syscall.Signal
		probes    int
		probeFrom string
	)

	// The renderer resolves ergo's translation directory, which otherwise
	// means looking the binary up on PATH. The override is the package's own
	// escape hatch for that, so the real Renderer stays in the test.
	t.Setenv(managed.EnvLanguagesPath, t.TempDir())

	env := newTestEnv(t, irc.ModeManaged, func(d *Deps) {
		d.Hooks.Signal = func(_ int, sig syscall.Signal) error {
			mu.Lock()
			defer mu.Unlock()
			signals = append(signals, sig)
			return nil
		}
		d.Hooks.APIStatus = func(_ context.Context, api *provider.APIConfig) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			probes++
			if api != nil {
				probeFrom = api.Token
			}
			return "2.19.1", nil
		}
	})

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	// A running child, so the reload goes by signal rather than over IRC.
	if err := os.WriteFile(env.paths.ErgoPID, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatalf("write ergo pidfile: %v", err)
	}

	before, err := irc.LoadAPIToken(env.paths.APIToken)
	if err != nil {
		t.Fatalf("read token: %v", err)
	}

	if err := env.daemon.onUnauthorized(context.Background()); err != nil {
		t.Fatalf("onUnauthorized: %v", err)
	}

	after, err := irc.LoadAPIToken(env.paths.APIToken)
	if err != nil {
		t.Fatalf("read rotated token: %v", err)
	}
	if after == before {
		t.Error("the API token was not rotated")
	}

	mu.Lock()
	gotSignals := append([]syscall.Signal(nil), signals...)
	gotProbes := probes
	gotFrom := probeFrom
	mu.Unlock()

	if len(gotSignals) == 0 || gotSignals[0] != syscall.SIGHUP {
		t.Errorf("signals = %v, want a SIGHUP to the child", gotSignals)
	}
	if gotProbes == 0 {
		t.Error("the rotated token was never confirmed against /v1/status")
	}
	if gotFrom != after {
		t.Errorf("status was probed with %q, want the rotated token %q", gotFrom, after)
	}

	// The config must have been re-rendered, since the old one still names
	// the old token.
	rendered, err := os.ReadFile(env.paths.ErgoYAML)
	if err != nil {
		t.Fatalf("read rendered config: %v", err)
	}
	if len(rendered) == 0 {
		t.Error("the ergo config was not re-rendered")
	}
}

// TestNewRejectsUnenabledProfile checks the daemon refuses a workspace that
// was never enabled, rather than inventing a mode.
func TestNewRejectsUnenabledProfile(t *testing.T) {
	_, err := New(Deps{})
	if err == nil {
		t.Fatal("New accepted a profile with no mode")
	}
	if !strings.Contains(err.Error(), "enable") {
		t.Errorf("err = %v, want it to name the fix", err)
	}
}

// TestLivenessMarksDisconnect checks a dropped session starts the degraded
// clock, since the transport offers no disconnect callback of its own.
func TestLivenessMarksDisconnect(t *testing.T) {
	env := newTestEnv(t, irc.ModeExternal, nil)

	if err := env.daemon.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(env.daemon.shutdown)

	env.daemon.markConnected()
	// Nothing else is driving this transport, so assigning the field is safe;
	// the fake has no setter for it.
	env.transport.SendErr = bot.ErrNotConnected
	env.daemon.probeLiveness(context.Background())

	env.daemon.statusMu.Lock()
	connected := env.daemon.connected
	since := env.daemon.disconnectedSince
	env.daemon.statusMu.Unlock()

	if connected {
		t.Error("the session still reads as connected after ErrNotConnected")
	}
	if since.IsZero() {
		t.Error("the disconnection clock was not started")
	}

	// A later probe must not restart the clock, or the grace period could
	// never elapse.
	first := since
	env.daemon.probeLiveness(context.Background())
	env.daemon.statusMu.Lock()
	second := env.daemon.disconnectedSince
	env.daemon.statusMu.Unlock()
	if !second.Equal(first) {
		t.Errorf("disconnection clock restarted: %s then %s", first, second)
	}
}
