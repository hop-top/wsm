package daemon

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot"
	"hop.top/wsm/pkg/irc/managed"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provider/ergo"
	"hop.top/wsm/pkg/irc/provision"
	"hop.top/wsm/pkg/irc/supervisor"
	"hop.top/wsm/pkg/model"

	// The plain provider registers itself in init and is reached through
	// provider.Open, so the import is for the registration rather than for a
	// symbol. ergo is imported for NewWithAccount and registers itself too.
	_ "hop.top/wsm/pkg/irc/provider/plain"
)

// DelegateReassertInterval is how often the daemon re-runs reconcile while any
// channel template carries a delegate.
//
// A delegate bot cannot escalate its own privilege, but it can act laterally:
// devoice a member, kick them, set +m or +k or +b. None of those show up as a
// role the diff would notice, so the host re-asserts what it authored on a
// timer as well as on every observed mode change.
const DelegateReassertInterval = 5 * time.Minute

// ReadyTimeout bounds the wait for a freshly spawned ergo to serve its API.
const ReadyTimeout = 10 * time.Second

// readyPoll is how often readiness is re-probed inside ReadyTimeout.
const readyPoll = 200 * time.Millisecond

// ReloadTimeout bounds the wait for a rehashed ergo to accept a rotated API
// token. Five seconds is the budget the error-handling contract allows before
// the recovery is declared failed.
const ReloadTimeout = 5 * time.Second

// reloadPoll is how often /v1/status is re-probed inside ReloadTimeout.
const reloadPoll = 250 * time.Millisecond

// livenessInterval is how often the daemon checks that its bot session is
// still up.
//
// The transport reconnects on its own and exposes no disconnect callback, so
// the only way to learn the session dropped is to ask it something. A cheap
// periodic probe gives the degraded clock a real signal instead of waiting for
// an unrelated command to fail at an arbitrary later time.
const livenessInterval = 5 * time.Second

// Deps is everything the daemon needs from its caller.
//
// The caller owns loading and opening: config, profile, paths, the backend and
// the device identity all arrive resolved, so every one of them is
// substitutable in a test and this package opens nothing implicitly.
type Deps struct {
	// Cfg is the resolved irc config block for this workspace.
	Cfg irc.Config
	// Paths locates every file the daemon owns.
	Paths irc.Paths
	// Profile is the durable per-workspace IRC state. Required: every mode
	// branch reads Profile.Mode, never Cfg.Mode.
	Profile irc.Profile
	// Workspace is the workspace being served.
	Workspace model.Workspace
	// Backend persists captured events.
	Backend backend.Backend
	// DataDir is the wsm data root, used to resolve sibling workspaces and
	// the device fingerprint.
	DataDir string
	// StateDir is the wsm volatile state root. It is carried explicitly
	// rather than unwound from Paths.StateDir, which is already joined with
	// irc/<workspace-id>: recovering the root by walking back up that join
	// would silently break the moment the layout changed.
	StateDir string
	// DeviceID stamps every recorded event.
	DeviceID string
	// GlobalConfigPath is the wsm config file the irc block is merged from.
	GlobalConfigPath string
	// Log receives daemon diagnostics. Nil means discard.
	Log *slog.Logger

	// Hooks replaces the process-facing pieces in a test: spawning ergo,
	// dialling the server, building providers. Zero value means the real
	// implementations.
	Hooks Hooks
}

// Hooks are the seams a test substitutes.
//
// Everything here touches a process, a socket or a clock, which is exactly
// what a daemon test cannot afford to do for real. They are gathered in one
// struct rather than scattered as optional fields so the set of things that
// are faked is visible in one place.
type Hooks struct {
	// NewSupervisor builds the managed child's supervisor.
	NewSupervisor func(d *Daemon) Supervisor
	// NewTransport builds the bot session.
	NewTransport func(opts bot.Options) Transport
	// OpenProviders builds the provider trio.
	OpenProviders func(d *Daemon, deps provider.Deps) (provider.Providers, error)
	// Reconcile runs one reconciliation.
	Reconcile func(ctx context.Context, deps provision.Deps) (provision.Report, error)
	// NewRecorder builds the event recorder.
	NewRecorder func(d *Daemon) bot.Recorder
	// APIStatus probes the ergo HTTP API, returning its version.
	APIStatus func(ctx context.Context, api *provider.APIConfig) (string, error)
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
	// Signal delivers a signal to a pid, for the out-of-band reload.
	Signal func(pid int, sig syscall.Signal) error
}

// Supervisor is the managed child's lifecycle, as the daemon uses it.
//
// It mirrors the concrete supervisor.Supervisor rather than embedding it, so a
// test can drive start, stop and status without spawning a process.
type Supervisor interface {
	// Start spawns the child and blocks until it is ready.
	Start(ctx context.Context) error
	// Stop terminates the child and removes its pidfile.
	Stop(ctx context.Context) error
	// Status snapshots the child.
	Status() supervisor.State
}

// Transport is the bot session, as the daemon uses it. It is provider's
// Transport plus the teardown the daemon performs on its way out.
type Transport interface {
	provider.Transport
}

// Daemon serves one workspace's IRC presence.
type Daemon struct {
	deps Deps
	log  *slog.Logger

	lock *lockFile

	sup       Supervisor
	transport Transport
	providers provider.Providers
	capture   *bot.Capture
	cursors   *bot.Cursors
	recorder  bot.Recorder

	api         *provider.APIConfig
	ergoVersion string

	// mu guards everything a reconcile and the mode observer both touch:
	// the profile, the last report, and the members error. Reconcile mutates
	// Profile.Accounts and Profile.AuthoredRoles and saves the file, while
	// the observer mutates Profile.DelegateGrants and saves it too, so the
	// two must never run at once.
	mu         sync.Mutex
	profile    irc.Profile
	lastReport *provision.Report
	membersErr string

	// statusMu guards the fields the status writer reads.
	statusMu          sync.Mutex
	connected         bool
	disconnectedSince time.Time
	lastBackfill      *BackfillSummary
	negotiated        []string

	// reconcileCh carries reconcile requests. It has capacity one, which is
	// what coalesces a burst: a request arriving while one is already
	// pending is dropped, because the pending run will observe the same
	// world the second request would have asked about.
	reconcileCh chan struct{}
	// backfillCh carries on-demand backfill requests, coalesced the same way.
	backfillCh chan struct{}

	channels []string
}

// New builds a daemon from its dependencies.
func New(deps Deps) (*Daemon, error) {
	if deps.Profile.Mode == "" {
		return nil, fmt.Errorf("irc/daemon: profile has no mode: enable the workspace first")
	}
	if deps.Paths.Status == "" || deps.Paths.DaemonPID == "" {
		return nil, fmt.Errorf("irc/daemon: paths are not resolved")
	}

	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	return &Daemon{
		deps:        deps,
		log:         log,
		profile:     deps.Profile,
		reconcileCh: make(chan struct{}, 1),
		backfillCh:  make(chan struct{}, 1),
	}, nil
}

// now reads the clock.
func (d *Daemon) now() time.Time {
	if d.deps.Hooks.Now != nil {
		return d.deps.Hooks.Now()
	}
	return time.Now()
}

// Run serves the workspace until ctx is cancelled or a terminating signal
// arrives, then shuts down and returns.
//
// Shutdown order is the one thing here that is not interchangeable: the bot
// leaves before the managed child is stopped. Killing ergo first would make
// the bot's own disconnect look like a server fault, which the supervisor
// would then try to recover from while the daemon is trying to exit.
func (d *Daemon) Run(ctx context.Context) error {
	if err := d.acquireLock(); err != nil {
		return err
	}
	defer d.releaseLock()

	if err := d.writePID(); err != nil {
		return err
	}
	defer d.removePID()

	if err := d.start(ctx); err != nil {
		// A failed start still leaves a status document behind, because the
		// operator's next move is `wsm irc status` and an empty file would
		// tell them nothing about why the daemon is gone.
		d.setMembersErr(err.Error())
		d.writeStatus()
		d.shutdown()
		return err
	}

	err := d.serve(ctx)
	d.shutdown()
	return err
}

// start brings the workspace up: the managed child, the bot session, the
// providers, the first reconcile, the first backfill and the capture loop.
func (d *Daemon) start(ctx context.Context) error {
	if d.profile.Mode == irc.ModeManaged {
		if err := d.startManaged(ctx); err != nil {
			return err
		}
	}

	if err := d.connect(ctx); err != nil {
		return err
	}
	if err := d.openProviders(); err != nil {
		return err
	}

	d.startCapture()

	// A reconcile before the first backfill: backfill reads the channel list
	// the templates describe, and joining them is reconcile's job.
	d.reconcile(ctx)
	d.backfillAll(ctx)

	// The transport re-establishes authentication, capabilities and oper
	// before running these, so a reconnect is the right moment to re-fetch
	// whatever history was missed while the session was down.
	d.transport.OnReconnect(func() {
		d.markConnected()
		d.backfillAll(context.WithoutCancel(ctx))
	})

	if d.hasDelegates() {
		d.watchModes()
	}

	return nil
}

// serve runs until the context is done, handling signals and timers.
func (d *Daemon) serve(ctx context.Context) error {
	reassert := d.delegateTicker()
	defer reassert.Stop()

	liveness := time.NewTicker(livenessInterval)
	defer liveness.Stop()

	status := time.NewTicker(time.Second)
	defer status.Stop()

	d.markConnected()
	d.writeStatus()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-d.reconcileCh:
			// Requests are coalesced by the channel's capacity of one, so a
			// storm of SIGHUPs costs one reconcile rather than one each.
			d.reconcile(ctx)
			d.writeStatus()

		case <-d.backfillCh:
			d.backfillAll(ctx)
			d.writeStatus()

		case <-reassert.C:
			// Re-assert authored roles a delegate may have undone laterally.
			d.reconcile(ctx)
			d.writeStatus()

		case <-liveness.C:
			d.probeLiveness(ctx)

		case <-status.C:
			d.writeStatus()
		}
	}
}

// Reconcile asks for a reconciliation. It never blocks: a request arriving
// while one is already queued is dropped, because the queued run will see the
// same world.
func (d *Daemon) Reconcile() {
	select {
	case d.reconcileCh <- struct{}{}:
	default:
	}
}

// Backfill asks for an on-demand backfill, coalesced like Reconcile.
func (d *Daemon) Backfill() {
	select {
	case d.backfillCh <- struct{}{}:
	default:
	}
}

// delegateTicker returns the re-assertion ticker, stopped when no template
// carries a delegate: there is nothing to re-assert against.
func (d *Daemon) delegateTicker() *time.Ticker {
	t := time.NewTicker(DelegateReassertInterval)
	if !d.hasDelegates() {
		t.Stop()
	}
	return t
}

// hasDelegates reports whether any channel template delegates to another
// workspace's bot.
func (d *Daemon) hasDelegates() bool {
	for _, ch := range d.deps.Cfg.Channels {
		if len(ch.Delegates) > 0 {
			return true
		}
	}
	return false
}

// startManaged renders the ergo config, then starts and awaits the child.
func (d *Daemon) startManaged(ctx context.Context) error {
	api, err := d.managedAPI()
	if err != nil {
		return err
	}
	d.api = api

	d.sup = d.newSupervisor()
	if err := d.sup.Start(ctx); err != nil {
		return fmt.Errorf("irc/daemon: start ergo: %w", err)
	}

	if version, err := d.apiStatus(ctx, d.api); err == nil {
		d.ergoVersion = version
	}
	return nil
}

// newSupervisor builds the managed child's supervisor.
func (d *Daemon) newSupervisor() Supervisor {
	if d.deps.Hooks.NewSupervisor != nil {
		return d.deps.Hooks.NewSupervisor(d)
	}

	managed.Binary = d.deps.Cfg.Binary
	return &supervisor.Supervisor{
		Binary:   d.deps.Cfg.Binary,
		ConfPath: d.deps.Paths.ErgoYAML,
		PIDFile:  d.deps.Paths.ErgoPID,
		LogPath:  d.deps.Paths.ErgoLog,
		Ready:    d.readyProbe,
		Backoff:  irc.NewBackoff(),
	}
}

// readyProbe polls the ergo HTTP API until it answers or the budget runs out.
//
// A clean spawn is not evidence the server is serving: a misconfigured
// api.listener is a soft failure in ergo, which logs the error and keeps
// running without an API. Readiness therefore has to ask the API itself.
func (d *Daemon) readyProbe(ctx context.Context) error {
	deadline := d.now().Add(ReadyTimeout)
	var lastErr error

	for d.now().Before(deadline) {
		version, err := d.apiStatus(ctx, d.api)
		if err == nil {
			d.ergoVersion = version
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyPoll):
		}
	}
	return fmt.Errorf("irc/daemon: ergo API did not answer within %s: %w", ReadyTimeout, lastErr)
}

// apiStatus asks the ergo API for its version.
func (d *Daemon) apiStatus(ctx context.Context, api *provider.APIConfig) (string, error) {
	if d.deps.Hooks.APIStatus != nil {
		return d.deps.Hooks.APIStatus(ctx, api)
	}
	if api == nil {
		return "", fmt.Errorf("irc/daemon: no API configured")
	}
	info, err := ergo.NewClient(api).Status(ctx)
	if err != nil {
		return "", err
	}
	return info.Version, nil
}

// managedAPI addresses the loopback HTTP API of the server wsm runs, or of the
// host workspace's server when this workspace is a guest on it.
func (d *Daemon) managedAPI() (*provider.APIConfig, error) {
	hostPaths, err := d.hostPaths()
	if err != nil {
		return nil, err
	}

	token, err := irc.ReadAPIToken(d.deps.Cfg, d.deps.Paths, hostPaths)
	if err != nil {
		return nil, fmt.Errorf("irc/daemon: read api token: %w", err)
	}

	port := d.profile.Ports.API
	if hostPaths != nil {
		// A guest talks to the host's API, whose port lives in the host's
		// own profile rather than in this workspace's.
		hostProfile, err := irc.LoadProfile(hostPaths.Profile)
		if err != nil {
			return nil, fmt.Errorf("irc/daemon: read host profile: %w", err)
		}
		port = hostProfile.Ports.API
	}

	return &provider.APIConfig{
		BaseURL: "http://" + net.JoinHostPort(irc.DefaultBind, strconv.Itoa(port)),
		Token:   token,
	}, nil
}

// hostPaths resolves the hosting workspace's paths when this workspace is a
// guest, and returns nil when it is not.
func (d *Daemon) hostPaths() (*irc.Paths, error) {
	if d.profile.HostWorkspace == "" {
		return nil, nil
	}
	p, err := irc.ResolvePaths(irc.PathsInput{
		DataDir:  d.deps.DataDir,
		StateDir: d.deps.StateDir,
	}, d.profile.HostWorkspace)
	if err != nil {
		return nil, fmt.Errorf("irc/daemon: resolve host paths: %w", err)
	}
	return &p, nil
}

// connect dials the server and negotiates the session.
func (d *Daemon) connect(ctx context.Context) error {
	opts, err := d.transportOptions()
	if err != nil {
		return err
	}

	d.transport = d.newTransport(opts)
	if err := d.transport.Connect(ctx); err != nil {
		return fmt.Errorf("irc/daemon: connect: %w", err)
	}

	d.statusMu.Lock()
	d.negotiated = d.transport.Negotiated().Names()
	d.statusMu.Unlock()
	return nil
}

// newTransport builds the bot session.
func (d *Daemon) newTransport(opts bot.Options) Transport {
	if d.deps.Hooks.NewTransport != nil {
		return d.deps.Hooks.NewTransport(opts)
	}
	return bot.New(opts)
}

// transportOptions builds the session options for this workspace's mode.
//
// Managed connects to its own loopback plaintext listener and opers up: the
// listener never leaves 127.0.0.1, so plaintext there is not a downgrade, and
// the oper block the rendered config carries is keyed by the bot's account.
// External connects to the configured server, with TLS unless the operator
// turned it off.
func (d *Daemon) transportOptions() (bot.Options, error) {
	secrets, err := irc.LoadSecrets(d.deps.Paths.Secrets)
	if err != nil {
		return bot.Options{}, fmt.Errorf("irc/daemon: load secrets: %w", err)
	}
	password, err := irc.ReadBotPassword(d.deps.Cfg, secrets)
	if err != nil {
		return bot.Options{}, fmt.Errorf("irc/daemon: read bot password: %w", err)
	}

	account := d.botAccount()
	opts := bot.Options{
		Account:  account,
		Password: password,
		Nick:     account,
		Backoff:  irc.NewBackoff(),
		Log:      d.log,
	}

	if d.profile.Mode == irc.ModeManaged {
		opts.Addr = net.JoinHostPort(irc.DefaultBind, strconv.Itoa(d.profile.Ports.Plain))
		// The rendered opers block is keyed by the bot account, and its
		// password is the bot's own secret, bcrypt-hashed.
		opts.OperName = account
		opts.OperPass = password
		return opts, nil
	}

	opts.Addr = net.JoinHostPort(d.deps.Cfg.Server.Host, strconv.Itoa(d.deps.Cfg.Server.Port))
	if d.deps.Cfg.Server.TLSEnabled() {
		opts.TLS = tlsConfig(d.deps.Cfg.Server)
	}
	return opts, nil
}

// botAccount is the account the bot authenticates as: what the profile
// recorded at enable, falling back to the configured value.
func (d *Daemon) botAccount() string {
	if d.profile.BotAccount != "" {
		return d.profile.BotAccount
	}
	if d.deps.Cfg.Bot.Account != "" {
		return d.deps.Cfg.Bot.Account
	}
	return irc.DefaultBotAccount
}

// openProviders builds the provider trio for this workspace's backend.
//
// Both concrete providers need the bot's own identity, which provider.Deps
// does not carry, and each takes it a different way. Getting either wrong
// fails quietly rather than loudly: ergo's MyRole matches nobody and reports
// no role, and plain cannot find itself in NAMES so every ApplyRoles returns
// ErrNotOpped.
func (d *Daemon) openProviders() error {
	deps := provider.Deps{
		Transport: d.transport,
		API:       d.providerAPI(),
		Log:       d.log,
	}

	providers, err := d.openProviderTrio(deps)
	if err != nil {
		return fmt.Errorf("irc/daemon: open provider %q: %w", d.profile.Provider, err)
	}
	d.providers = providers
	return nil
}

// openProviderTrio builds the trio, injecting the bot's identity the way each
// provider requires.
func (d *Daemon) openProviderTrio(deps provider.Deps) (provider.Providers, error) {
	if d.deps.Hooks.OpenProviders != nil {
		return d.deps.Hooks.OpenProviders(d, deps)
	}

	account := d.botAccount()

	if d.profile.Provider == irc.ProviderErgo {
		// The registry factory leaves the nick empty, which disables the
		// live-MODE fallback and CS TRANSFER. A daemon wants both, so the
		// trio is constructed directly.
		return ergo.NewWithAccount(deps, account)
	}

	providers, err := provider.Open(string(d.profile.Provider), deps)
	if err != nil {
		return provider.Providers{}, err
	}

	// The plain provider assumes nick equals account and defaults to
	// irc.DefaultBotAccount. On a deployment whose bot nick differs, a
	// provider that was never told cannot find itself in NAMES, so every
	// role application reports ErrNotOpped and nothing is ever granted.
	if setter, ok := providers.Roles.(interface{ SetNick(string) }); ok {
		setter.SetNick(account)
	}
	return providers, nil
}

// providerAPI is the API config the providers get: the loopback API in managed
// mode, an external ergo's API when one is configured, and nil otherwise.
func (d *Daemon) providerAPI() *provider.APIConfig {
	if d.profile.Mode == irc.ModeManaged {
		return d.api
	}
	if strings.TrimSpace(d.deps.Cfg.Server.ErgoAPI) == "" {
		return nil
	}
	token, err := irc.ReadAPIToken(d.deps.Cfg, d.deps.Paths, nil)
	if err != nil {
		d.log.Warn("irc external api token unavailable", "error", err)
		return nil
	}
	return &provider.APIConfig{
		BaseURL: strings.TrimSuffix(d.deps.Cfg.Server.ErgoAPI, "/"),
		Token:   token,
	}
}

// startCapture wires the capture path and runs it until the daemon stops.
func (d *Daemon) startCapture() {
	cursors, err := bot.LoadCursors(d.deps.Paths.Cursors)
	if err != nil {
		d.log.Warn("irc cursors unreadable, starting empty", "error", err)
		cursors = bot.NewCursors(d.deps.Paths.Cursors)
	}
	d.cursors = cursors
	d.recorder = d.newRecorder()

	members, _ := d.loadMembers()
	_, reverse := provider.ResolverFor(d.profile, members, d.deps.DataDir)

	d.capture = &bot.Capture{
		Transport:   d.transport,
		Directory:   d.providers.Directory,
		Reverse:     reverse,
		ServerID:    d.profile.ServerID,
		BotAccount:  d.botAccount(),
		WorkspaceID: d.deps.Workspace.ID,
		WriterID:    WriterID(d.deps.Workspace.ID),
		Recorder:    d.recorder,
		Cursors:     d.cursors,
	}
}

// newRecorder builds the event recorder.
func (d *Daemon) newRecorder() bot.Recorder {
	if d.deps.Hooks.NewRecorder != nil {
		return d.deps.Hooks.NewRecorder(d)
	}
	return NewRecorder(d.deps.Backend, d.deps.Workspace.ID, d.deps.DeviceID)
}

// RunCapture runs the live capture loop until ctx is done. It is exported so
// the command can run it alongside Run's signal loop.
func (d *Daemon) RunCapture(ctx context.Context) error {
	if d.capture == nil {
		return nil
	}
	return d.capture.Run(ctx)
}

// loadMembers reads the workspace's member list.
//
// An unreadable or invalid members.yaml is reported rather than fatal: the
// daemon keeps whatever state it already has and degrades with the reason.
// Wiping roles because a file failed to parse would turn a typo into an
// outage.
func (d *Daemon) loadMembers() ([]model.Member, error) {
	localPath := irc.WorkspaceConfigPath(d.profile.Root)
	if localPath == "" {
		// No override file exists. The notional path still has to sit under
		// the workspace root, because LoadMemberConfig reads members.yaml
		// from the file's own directory and a relative path would resolve
		// against the daemon's working directory instead.
		localPath = filepath.Join(d.profile.Root, ".wsm", "config.yaml")
	}

	mc, err := config.LoadMemberConfig(localPath)
	if err != nil {
		return nil, err
	}
	return mc.Members, nil
}

// reconcile runs one reconciliation and records its outcome.
//
// It holds the profile mutex for the whole run: reconcile mutates the profile
// and saves it, and so does the delegate mode observer, so the two must never
// interleave.
func (d *Daemon) reconcile(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()

	members, err := d.loadMembers()
	if err != nil {
		// Keep the previous state. The reason surfaces in status.json and the
		// roles that are already applied stay applied.
		d.membersErr = err.Error()
		d.log.Error("irc members config invalid, keeping previous state", "error", err)
		return
	}
	d.membersErr = ""

	secrets, err := irc.LoadSecrets(d.deps.Paths.Secrets)
	if err != nil {
		d.log.Error("irc secrets unreadable", "error", err)
		return
	}

	resolver, _ := provider.ResolverFor(d.profile, members, d.deps.DataDir)

	deps := provision.Deps{
		Accounts:       d.providers.Accounts,
		Roles:          d.providers.Roles,
		Directory:      d.providers.Directory,
		Transport:      d.transport,
		Resolver:       resolver,
		Secrets:        secrets,
		SecretsPath:    d.deps.Paths.Secrets,
		Profile:        &d.profile,
		ProfilePath:    d.deps.Paths.Profile,
		Cfg:            d.deps.Cfg,
		Workspace:      d.deps.Workspace,
		Members:        members,
		DataDir:        d.deps.DataDir,
		OnUnauthorized: d.onUnauthorized,
	}

	report, err := d.runReconcile(ctx, deps)
	if err != nil {
		d.log.Error("irc reconcile failed", "error", err)
		return
	}

	d.lastReport = &report
	d.channels = channelNames(report)
}

// runReconcile calls the reconciler.
func (d *Daemon) runReconcile(ctx context.Context, deps provision.Deps) (provision.Report, error) {
	if d.deps.Hooks.Reconcile != nil {
		return d.deps.Hooks.Reconcile(ctx, deps)
	}
	return provision.Reconcile(ctx, deps)
}

// channelNames lists the channels a report covers, in report order.
func channelNames(r provision.Report) []string {
	out := make([]string, 0, len(r.Channels))
	for _, ch := range r.Channels {
		out = append(out, ch.Name)
	}
	return out
}

// onUnauthorized recovers from an API token the server no longer accepts.
//
// The ergo API cannot reload its own tokens: every endpoint, /v1/rehash
// included, authenticates against the config that is currently loaded. So a
// rotated token has to be written, rendered into the config, and then made
// live by a reload from OUTSIDE the API — a SIGHUP to the child, or an oper
// REHASH over IRC when the daemon does not hold the child. Only after the
// server has actually picked the new token up is a retry worth making.
func (d *Daemon) onUnauthorized(ctx context.Context) error {
	if d.profile.Mode != irc.ModeManaged {
		// External mode owns its own credentials; wsm has nothing to rotate.
		return fmt.Errorf("irc/daemon: api rejected the token and wsm does not own this server")
	}

	token, err := irc.GeneratePassphrase()
	if err != nil {
		return fmt.Errorf("irc/daemon: generate api token: %w", err)
	}
	if err := irc.SaveAPIToken(d.deps.Paths.APIToken, token); err != nil {
		return fmt.Errorf("irc/daemon: save api token: %w", err)
	}

	secrets, err := irc.LoadSecrets(d.deps.Paths.Secrets)
	if err != nil {
		return fmt.Errorf("irc/daemon: load secrets: %w", err)
	}

	// Renderer rather than the package-level RenderConfig: irc.Profile
	// carries no directory roots, so the free function can only reach
	// absolute paths through package state the daemon would have to set and
	// keep in step. Passing the paths makes the dependency explicit.
	renderer := managed.Renderer{Paths: d.deps.Paths}
	rendered, err := renderer.RenderConfig(d.deps.Cfg, d.profile, secrets)
	if err != nil {
		return fmt.Errorf("irc/daemon: render ergo config: %w", err)
	}
	if err := os.WriteFile(d.deps.Paths.ErgoYAML, rendered, 0o600); err != nil {
		return fmt.Errorf("irc/daemon: write ergo config: %w", err)
	}

	if err := d.reloadOutOfBand(ctx); err != nil {
		return err
	}

	d.api = &provider.APIConfig{
		BaseURL: "http://" + net.JoinHostPort(irc.DefaultBind, strconv.Itoa(d.profile.Ports.API)),
		Token:   token,
	}
	return d.awaitToken(ctx, d.api)
}

// reloadOutOfBand makes the re-rendered config live without going through the
// API, which would authenticate against the very config being replaced.
func (d *Daemon) reloadOutOfBand(ctx context.Context) error {
	pid, running, err := supervisor.ReadPID(d.deps.Paths.ErgoPID)
	if err == nil && running && pid > 0 {
		if err := d.signal(pid, syscall.SIGHUP); err == nil {
			return nil
		}
	}

	// No child handle: ask the server to rehash over IRC instead. The bot's
	// oper class carries the rehash capability for exactly this.
	if d.transport != nil && d.transport.IsOper() {
		if err := d.transport.Send(ctx, "REHASH"); err != nil {
			return fmt.Errorf("irc/daemon: rehash over irc: %w", err)
		}
		return nil
	}
	return fmt.Errorf("irc/daemon: no way to reload ergo: no running child and the bot is not oper")
}

// signal delivers sig to pid.
func (d *Daemon) signal(pid int, sig syscall.Signal) error {
	if d.deps.Hooks.Signal != nil {
		return d.deps.Hooks.Signal(pid, sig)
	}
	return syscall.Kill(pid, sig)
}

// awaitToken waits for the server to accept the rotated token.
func (d *Daemon) awaitToken(ctx context.Context, api *provider.APIConfig) error {
	deadline := d.now().Add(ReloadTimeout)
	var lastErr error

	for d.now().Before(deadline) {
		version, err := d.apiStatus(ctx, api)
		if err == nil {
			d.ergoVersion = version
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(reloadPoll):
		}
	}
	return fmt.Errorf("irc/daemon: rotated api token was not accepted within %s: %w", ReloadTimeout, lastErr)
}

// backfillAll replays history for every channel that has a cursor.
//
// A channel with no cursor has never been captured, so there is no position to
// resume from and nothing to reconcile against: live capture starts it.
func (d *Daemon) backfillAll(ctx context.Context) {
	if d.capture == nil || d.cursors == nil {
		return
	}

	summary := &BackfillSummary{At: d.now()}
	opts := bot.BackfillOptions{
		Limit: bot.DefaultBackfillLimit,
		Seen:  NewSeen(d.deps.Backend, d.deps.Workspace.ID),
	}

	for _, channel := range d.backfillChannels() {
		cur, ok := d.cursors.Get(channel)
		if !ok {
			continue
		}
		report, err := d.capture.Backfill(ctx, channel, cur, opts)
		if err != nil {
			summary.Err = err.Error()
			d.log.Error("irc backfill failed", "channel", channel, "error", err)
			continue
		}
		summary.Channels = append(summary.Channels, backfillChannel(report))
	}

	if err := d.cursors.Save(); err != nil {
		d.log.Warn("irc cursors not saved", "error", err)
	}

	d.statusMu.Lock()
	d.lastBackfill = summary
	d.statusMu.Unlock()
}

// backfillChannels lists the channels to replay: the ones the last reconcile
// covered, or every channel with a cursor before the first reconcile.
func (d *Daemon) backfillChannels() []string {
	d.mu.Lock()
	channels := make([]string, len(d.channels))
	copy(channels, d.channels)
	d.mu.Unlock()

	if len(channels) > 0 {
		return channels
	}

	for channel := range d.cursors.All() {
		channels = append(channels, channel)
	}
	sort.Strings(channels)
	return channels
}

// watchModes records the roles delegate bots hand out, and re-asserts what wsm
// authored whenever anyone changes a mode on a host channel.
//
// This is the provenance a delegate revoke depends on. A revoke strips only
// the roles wsm can prove the departing bot granted, so a grant nobody
// observed is left alone as unverified rather than removed on suspicion — and
// the only place that observation can happen is here, while the mode change is
// on the wire.
func (d *Daemon) watchModes() {
	d.transport.Subscribe("MODE", func(msg ircmsg.Message) {
		d.handleMode(msg)
	})
}

// handleMode records delegate grants and re-asserts authored roles.
func (d *Daemon) handleMode(msg ircmsg.Message) {
	setter := msg.Nick()
	if setter == "" || len(msg.Params) < 2 {
		return
	}
	channel := msg.Params[0]
	if !strings.HasPrefix(channel, "#") {
		return
	}

	// Only a bot named as a delegate on this channel can produce provenance.
	// Anyone else's mode change is foreign and is re-asserted against, never
	// recorded as something wsm may later strip.
	if d.isDelegate(channel, setter) {
		d.recordDelegateGrants(channel, setter, msg)
	}

	// A delegate cannot escalate, but it can devoice, kick or lock a channel.
	// None of that shows up as a role the diff would notice, so the authored
	// state is re-asserted rather than compared.
	d.Reconcile()
}

// isDelegate reports whether bot is a configured delegate on channel.
func (d *Daemon) isDelegate(channel, bot string) bool {
	prefix := d.deps.Cfg.ChannelPrefix
	for _, tmpl := range d.deps.Cfg.Channels {
		if len(tmpl.Delegates) == 0 {
			continue
		}
		if irc.ChannelName(prefix, tmpl.Name, d.deps.Workspace.Name) != channel {
			continue
		}
		for _, del := range tmpl.Delegates {
			if del.Bot == bot {
				return true
			}
		}
	}
	return false
}

// recordDelegateGrants writes the roles a delegate bot set, and forgets the
// ones it removed.
func (d *Daemon) recordDelegateGrants(channel, bot string, msg ircmsg.Message) {
	changes, ok := parseModeChanges(msg.Params[1], msg.Params[2:])
	if !ok || len(changes) == 0 {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.profile.DelegateGrants == nil {
		d.profile.DelegateGrants = map[string]map[string]map[string]irc.Role{}
	}
	if d.profile.DelegateGrants[channel] == nil {
		d.profile.DelegateGrants[channel] = map[string]map[string]irc.Role{}
	}
	if d.profile.DelegateGrants[channel][bot] == nil {
		d.profile.DelegateGrants[channel][bot] = map[string]irc.Role{}
	}
	grants := d.profile.DelegateGrants[channel][bot]

	for _, ch := range changes {
		if ch.add {
			grants[ch.account] = ch.role
			continue
		}
		delete(grants, ch.account)
	}

	// An empty record is pruned so a bot that granted and then revoked
	// everything does not linger in the profile as an empty map.
	if len(grants) == 0 {
		delete(d.profile.DelegateGrants[channel], bot)
	}
	if len(d.profile.DelegateGrants[channel]) == 0 {
		delete(d.profile.DelegateGrants, channel)
	}

	if err := irc.SaveProfile(d.deps.Paths.Profile, d.profile); err != nil {
		d.log.Error("irc profile not saved after delegate grant", "error", err)
	}
}

// modeChange is one observed role grant or revoke.
type modeChange struct {
	add     bool
	role    irc.Role
	account string
}

// parseModeChanges decodes the role changes in one MODE line.
//
// Only the three privilege modes are interesting, but the others cannot simply
// be ignored: several of them consume an argument, and miscounting would pair
// a role with the wrong nick and record a grant that never happened. A letter
// whose arity this does not know therefore abandons the whole line rather than
// guessing, because a wrong provenance record is worse than none — it is what
// a later revoke would strip a role on the strength of.
func parseModeChanges(modes string, args []string) ([]modeChange, bool) {
	var out []modeChange
	add := true
	next := 0

	take := func() (string, bool) {
		if next >= len(args) {
			return "", false
		}
		arg := args[next]
		next++
		return arg, true
	}

	for _, letter := range modes {
		switch letter {
		case '+':
			add = true
		case '-':
			add = false

		case 'o', 'h', 'v':
			account, ok := take()
			if !ok {
				return nil, false
			}
			out = append(out, modeChange{add: add, role: roleOfMode(letter), account: account})

		// Argument-taking modes whose argument is not a role holder. They are
		// consumed so the ones that follow stay aligned.
		case 'b', 'e', 'I', 'k', 'f', 'j':
			if _, ok := take(); !ok {
				return nil, false
			}
		case 'l':
			// A limit carries an argument when set and none when cleared.
			if add {
				if _, ok := take(); !ok {
					return nil, false
				}
			}

		// Argument-less modes.
		case 'm', 'n', 't', 'i', 's', 'p', 'r', 'R', 'C', 'M', 'S', 'u', 'z', 'Q', 'A', 'O':

		default:
			// An unknown letter may or may not consume an argument. Either
			// assumption can misalign the rest of the line, so the whole line
			// is abandoned.
			return nil, false
		}
	}
	return out, true
}

// roleOfMode maps a privilege mode letter to its role. It mirrors the plain
// provider's modeLetter, in the other direction.
func roleOfMode(letter rune) irc.Role {
	switch letter {
	case 'o':
		return irc.RoleOp
	case 'h':
		return irc.RoleHalfop
	default:
		return irc.RoleVoice
	}
}

// probeLiveness checks the bot session is still up.
func (d *Daemon) probeLiveness(ctx context.Context) {
	if d.transport == nil {
		return
	}
	err := d.transport.Send(ctx, "PING :wsm")
	if errors.Is(err, bot.ErrNotConnected) {
		d.markDisconnected()
		return
	}
	if err == nil {
		d.markConnected()
	}
}

// markConnected records a live session.
func (d *Daemon) markConnected() {
	d.statusMu.Lock()
	defer d.statusMu.Unlock()
	d.connected = true
	d.disconnectedSince = time.Time{}
}

// markDisconnected starts the disconnection clock, leaving an existing one
// alone so the elapsed time keeps growing across probes.
func (d *Daemon) markDisconnected() {
	d.statusMu.Lock()
	defer d.statusMu.Unlock()
	d.connected = false
	if d.disconnectedSince.IsZero() {
		d.disconnectedSince = d.now()
	}
}

// setMembersErr records a startup failure as the degraded reason.
func (d *Daemon) setMembersErr(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.membersErr = msg
}

// Status builds the current status document.
func (d *Daemon) Status() Status {
	d.mu.Lock()
	report := d.lastReport
	membersErr := d.membersErr
	profile := d.profile
	d.mu.Unlock()

	d.statusMu.Lock()
	connected := d.connected
	since := d.disconnectedSince
	lastBackfill := d.lastBackfill
	negotiated := append([]string(nil), d.negotiated...)
	d.statusMu.Unlock()

	var supState *supervisor.State
	if d.sup != nil {
		s := d.sup.Status()
		supState = &s
	}

	state, reasons := assess(health{
		supervised:        supState,
		connected:         connected,
		disconnectedSince: since,
		report:            report,
		membersErr:        membersErr,
		now:               d.now(),
	})

	status := Status{
		Mode:          profile.Mode,
		Provider:      profile.Provider,
		State:         state,
		PID:           os.Getpid(),
		Since:         d.now(),
		Reasons:       reasons,
		Server:        serverStatus(d.deps.Cfg, profile),
		Caps:          Caps{Negotiated: negotiated},
		Warnings:      warnings(d.deps.Cfg, profile),
		ErgoVersion:   d.ergoVersion,
		LastReconcile: report,
		LastBackfill:  lastBackfill,
	}

	if report != nil {
		status.Caps.Provider = report.ProviderCaps
	}
	if supState != nil {
		status.ErgoPID = supState.PID
		status.Restarts = supState.Restarts
	}
	if d.cursors != nil {
		status.Cursors = d.cursors.All()
	}
	return status
}

// writeStatus persists the current status document.
func (d *Daemon) writeStatus() {
	if err := WriteStatus(d.deps.Paths.Status, d.Status()); err != nil {
		d.log.Error("irc status not written", "error", err)
	}
}

// shutdown tears the workspace down in the one order that is safe.
//
// The bot QUITs first. Stopping the managed child first would drop the bot's
// connection underneath it, which looks exactly like a server fault: the
// transport would start reconnecting to a server that is deliberately going
// away, and the supervisor would be asked to recover a child that is
// deliberately being stopped.
func (d *Daemon) shutdown() {
	if d.transport != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = d.transport.Send(ctx, "QUIT :wsm shutting down")
		cancel()
		d.transport.Close()
	}

	if d.sup != nil {
		ctx, cancel := context.WithTimeout(context.Background(), supervisor.StopGrace+time.Second)
		if err := d.sup.Stop(ctx); err != nil {
			d.log.Error("irc ergo not stopped cleanly", "error", err)
		}
		cancel()
	}

	if d.cursors != nil {
		if err := d.cursors.Save(); err != nil {
			d.log.Warn("irc cursors not saved on shutdown", "error", err)
		}
	}
}

// writePID records the daemon's own pid.
func (d *Daemon) writePID() error {
	if err := os.MkdirAll(filepath.Dir(d.deps.Paths.DaemonPID), 0o700); err != nil {
		return fmt.Errorf("irc/daemon: create state dir: %w", err)
	}
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(d.deps.Paths.DaemonPID, []byte(pid+"\n"), 0o600); err != nil {
		return fmt.Errorf("irc/daemon: write pidfile: %w", err)
	}
	return nil
}

// removePID drops the pidfile on the way out.
func (d *Daemon) removePID() {
	if err := os.Remove(d.deps.Paths.DaemonPID); err != nil && !os.IsNotExist(err) {
		d.log.Warn("irc pidfile not removed", "error", err)
	}
}

// LockPath is the file whose exclusive lock one daemon holds for its lifetime.
func LockPath(paths irc.Paths) string {
	return filepath.Join(paths.StateDir, "irc.lock")
}

// lockFile is an exclusive flock held for the daemon's lifetime.
type lockFile struct {
	file *os.File
}

// acquireLock takes the workspace's exclusive lock.
//
// The lock is what makes "one daemon per workspace" true rather than hoped
// for: two daemons would both drive the bot, both write cursors and both
// reconcile, and the resulting interleaving is not recoverable by retrying.
// A held lock is therefore a refusal with a hint, never a wait.
func (d *Daemon) acquireLock() error {
	path := LockPath(d.deps.Paths)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("irc/daemon: create state dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("irc/daemon: open lock: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return &LockedError{Path: path, Workspace: d.deps.Workspace.Name}
	}

	d.lock = &lockFile{file: f}
	return nil
}

// releaseLock drops the lock and closes its file.
func (d *Daemon) releaseLock() {
	if d.lock == nil {
		return
	}
	_ = syscall.Flock(int(d.lock.file.Fd()), syscall.LOCK_UN)
	_ = d.lock.file.Close()
	d.lock = nil
}

// LockedError reports that another daemon already holds this workspace.
type LockedError struct {
	// Path is the lock file.
	Path string
	// Workspace is the workspace name, for the hint.
	Workspace string
}

// Error states the problem and the fix, because this is the message an
// operator sees when a second daemon refuses to start.
func (e *LockedError) Error() string {
	ws := e.Workspace
	if ws == "" {
		ws = "<workspace>"
	}
	return fmt.Sprintf(
		"another wsm irc daemon already holds %s: run `wsm irc status %s` to see it, or `wsm irc stop %s` to stop it",
		e.Path, ws, ws,
	)
}

// tlsConfig builds the client TLS configuration for an external server.
//
// InsecureSkipVerify is honoured because an operator may legitimately be
// running a server with a private CA, but it is never chosen here: it comes
// from the config, and the daemon reports it as a warning in status.json for
// as long as it is in force.
func tlsConfig(s irc.Server) *tls.Config {
	return &tls.Config{
		ServerName:         s.Host,
		InsecureSkipVerify: s.InsecureSkipVerify, //nolint:gosec // operator-configured, surfaced as a status warning
		MinVersion:         tls.VersionTLS12,
	}
}
