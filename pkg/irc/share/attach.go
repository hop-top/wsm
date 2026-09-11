// Package share attaches one workspace's IRC presence to another
// workspace's managed server, and detaches it again.
//
// # The trust boundary
//
// Same-machine sharing is the only shape this package implements. The guest's
// config points at the host's loopback plaintext listener and the host's
// loopback ergo HTTP API, and the host's API token is read at runtime from the
// host's own Paths.APIToken through irc.ReadAPIToken. The token is never copied
// into the guest's config, its secrets store, or its profile.
//
// That is safe for exactly one reason: same machine, same user, same $DataDir.
// A guest process can already read the host's data directory, so handing it a
// path to the token grants nothing it did not have. Reading rather than copying
// is what makes the host rotating its token take effect immediately, and what
// makes purging the host revoke every guest at once.
//
// Cross-machine sharing is deliberately NOT this path. A workspace on another
// machine attaches with plain `wsm irc enable --external` against the host's
// remote listener, using an account the host operator provisions with
// `wsm irc account create <host> --raw <name>`. The host's API token is never
// handed out, because off the machine there is no boundary that would make
// doing so safe.
package share

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// ErrHostNotManaged reports that the named host workspace does not run its own
// server, so it has nothing to share. Attaching to an external workspace would
// point the guest at a server neither workspace owns.
var ErrHostNotManaged = errors.New("irc/share: host workspace is not managed")

// ErrHostNotRunning reports that the host's daemon is not up. Attach needs the
// host's ergo to create the guest's bot account, so a stopped host is a
// prerequisite failure rather than something to queue.
var ErrHostNotRunning = errors.New("irc/share: host workspace is not running")

// ErrBotAccountReserved reports that the guest's bot account already exists on
// the host. The host reserves the whole wsm-* namespace for bots it knows
// about, so an existing one means either a half-finished attach or another
// workspace already holding that name: both are conflicts a human resolves,
// never something to overwrite, because overwriting would hand this guest
// control of an account another workspace is authenticating with.
var ErrBotAccountReserved = errors.New("irc/share: guest bot account already exists on host")

// ErrHostStoppedAtDetach reports that the host was not running when a detach
// needed it. The guest records the detach as pending and the caller reports it,
// so the operator knows to rerun once the host is up.
var ErrHostStoppedAtDetach = errors.New("irc/share: host workspace is not running; detach is pending")

// ErrNotAttached reports a detach of a workspace that is not a guest.
var ErrNotAttached = errors.New("irc/share: workspace is not attached to a host")

// Profiles pairs a workspace's durable IRC state with the paths it lives at.
// Attach and Detach mutate Profile and save it to Paths.Profile, so a caller
// holding one of these after the call sees what was persisted.
type Profiles struct {
	// ID is the workspace id, which is what SharedWith and HostWorkspace
	// record. Never the workspace name: a rename must not strand the link.
	ID string
	// Name is the workspace name, used to derive channel prefixes and the
	// bot account. Presentation only; nothing is keyed by it.
	Name string
	// Paths locates this workspace's IRC files.
	Paths irc.Paths
	// Profile is this workspace's durable state.
	Profile irc.Profile
	// Root is the directory holding .wsm/, where the config override lives.
	Root string
}

// Config is what Attach and Detach need beyond the two workspaces.
type Config struct {
	// Accounts creates the guest's bot account on the HOST's server. The
	// caller opens it against the host's API, which is the only side that
	// may create accounts there.
	Accounts provider.AccountProvider

	// Running reports whether a workspace's daemon is up, given its paths.
	// Injected because liveness is a process probe the CLI already owns and
	// this package must stay testable without spawning anything.
	Running func(paths irc.Paths) bool

	// Signal asks the host's daemon to reconcile, which is what suspends a
	// departed guest's bot and runs the revoke sweep. Injected for the same
	// reason as Running.
	Signal func(paths irc.Paths) error

	// Reconcile runs the guest's final reconcile against the borrowed
	// channels it is giving up, stripping the roles it authored. Nil skips
	// it, which is what a guest that never borrowed anything wants.
	Reconcile func(ctx context.Context) error

	// Now is the clock the profile's timestamps are taken from. Nil means
	// time.Now.
	Now func() time.Time
}

// now reads the configured clock.
func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}

// running probes a workspace's daemon, defaulting to "not running" when no
// prober was supplied: claiming a host is up without checking is the one
// answer that could let an attach proceed against a dead server.
func (c Config) running(paths irc.Paths) bool {
	if c.Running == nil {
		return false
	}
	return c.Running(paths)
}

// BotAccount is the account name a guest workspace's bot holds on its host's
// server: the reserved wsm- prefix plus the guest's name slug.
//
// The host reserves this namespace (irc.BotAccountPrefix), which is what makes
// "is this account another workspace's bot?" answerable from the name alone —
// discovery relies on it to list a guest's channels as peers rather than
// sweeping them as unmanaged.
func BotAccount(guestName string) string {
	slug := irc.Slug(guestName)
	if slug == "" {
		slug = "guest"
	}
	name := irc.BotAccountPrefix + slug
	if len(name) > irc.MaxAccountLength {
		name = name[:irc.MaxAccountLength]
	}
	return name
}

// ChannelPrefix is the namespace a guest's channels sit in on a shared server.
//
// Without it a guest and its host could both want "#dev", and the second one to
// reconcile would find the first one's channel registered to a foreign founder.
// The prefix is derived from the guest's name rather than configured, so two
// workspaces cannot collide by both leaving it unset.
func ChannelPrefix(guestName string) string {
	slug := irc.Slug(guestName)
	if slug == "" {
		slug = "guest"
	}
	return slug + "-"
}

// Attach makes guest a tenant on host's server.
//
// The order is chosen so a failure never leaves a claim nothing backs: the host
// is verified before anything is written, the bot account is created on the
// host before the guest's profile claims to have one, and the host's SharedWith
// gains the guest last, after the guest is actually able to connect. A crash
// between any two steps leaves state a rerun can complete rather than state a
// human has to unpick.
//
// The caller spawns the guest's daemon afterwards; this function never starts
// anything.
func Attach(ctx context.Context, guest, host *Profiles, cfg Config) error {
	if guest == nil || host == nil {
		return fmt.Errorf("irc/share: attach: both guest and host are required")
	}
	if guest.ID == "" || host.ID == "" {
		return fmt.Errorf("irc/share: attach: both workspace ids are required")
	}
	if guest.ID == host.ID {
		return fmt.Errorf("irc/share: attach: a workspace cannot host itself")
	}

	// The host must own its server. An external host is connected to a
	// server it does not control, so it can neither create the guest's
	// account nor promise the loopback listener the guest would be pointed
	// at.
	if host.Profile.Mode != irc.ModeManaged {
		return fmt.Errorf("%w: %s is %s", ErrHostNotManaged, host.Name, host.Profile.Mode)
	}
	if !host.Profile.Ports.Valid() {
		return fmt.Errorf("irc/share: attach: host %s has no allocated port triple", host.Name)
	}
	// Account creation goes through the host's live ergo, so a stopped host
	// cannot be attached to. Reported before any write.
	if !cfg.running(host.Paths) {
		return fmt.Errorf("%w: start it with `wsm irc start %s`", ErrHostNotRunning, host.Name)
	}
	if cfg.Accounts == nil {
		return fmt.Errorf("irc/share: attach: an account provider on the host is required")
	}
	if !cfg.Accounts.Caps().Has(provider.CapCreateAccount) {
		return fmt.Errorf("irc/share: attach: host %s cannot create accounts", host.Name)
	}

	botAccount := BotAccount(guest.Name)

	// The guest's bot password is generated here and stored in the GUEST's
	// secrets. The host never holds it: the host created the account, but
	// only the guest authenticates as it.
	passphrase, err := irc.GeneratePassphrase()
	if err != nil {
		return err
	}

	switch err := cfg.Accounts.CreateAccount(ctx, botAccount, passphrase); {
	case err == nil:
	case errors.Is(err, provider.ErrExists):
		// Not recoverable by taking it over. An existing wsm-* account is
		// either another workspace's live bot or the remains of a previous
		// attach, and resetting its password would silently lock out
		// whoever is using it.
		return fmt.Errorf("%w: %s on %s", ErrBotAccountReserved, botAccount, host.Name)
	default:
		return fmt.Errorf("irc/share: attach: create bot account %s on %s: %w",
			botAccount, host.Name, err)
	}

	secrets, err := irc.LoadSecrets(guest.Paths.Secrets)
	if err != nil {
		return err
	}
	secrets.Set(botAccount, passphrase)
	if err := secrets.Save(guest.Paths.Secrets); err != nil {
		return err
	}

	// The override is what points the guest's daemon at the host: without it
	// the guest would resolve its own (absent) server. Written before the
	// profile claims the attachment so a crash leaves a guest that is not
	// yet attached rather than one that believes it is but cannot connect.
	if err := WriteGuestOverride(guest.Root, host.Profile, guest.Name, botAccount); err != nil {
		return err
	}

	guest.Profile.Mode = irc.ModeExternal
	guest.Profile.Provider = irc.ProviderErgo
	guest.Profile.HostWorkspace = host.ID
	guest.Profile.ServerID = host.Profile.ServerID
	guest.Profile.BotAccount = botAccount
	guest.Profile.PendingDetach = false
	if guest.Profile.Root == "" {
		guest.Profile.Root = guest.Root
	}
	if guest.Profile.CreatedAt.IsZero() {
		guest.Profile.CreatedAt = cfg.now()
	}
	if err := irc.SaveProfile(guest.Paths.Profile, guest.Profile); err != nil {
		return err
	}

	// Last: the host now admits to hosting this guest. Recorded after the
	// guest is fully able to connect, so the host's own guest-refusal guard
	// never fires for an attachment that did not complete.
	host.Profile.SharedWith = addShared(host.Profile.SharedWith, guest.ID)
	return irc.SaveProfile(host.Paths.Profile, host.Profile)
}

// Detach removes guest from host's server.
//
// The order is the whole point of this function, and it is the reverse of
// Attach's for the same reason: each step must run while the thing it depends
// on is still true.
//
//  1. The guest's final reconcile, with nothing desired on the channels it
//     borrowed. This strips the roles the guest itself authored. It has to
//     happen first, while the guest's bot still holds the privilege the host
//     delegated to it — once the delegation is gone the guest can no longer
//     remove its own grants, and they would be stranded on the host as roles
//     nobody claims.
//
//  2. The host's delegate entries naming the guest are removed from its
//     override. This is what makes the guest "departed" in the host's eyes;
//     the host's next reconcile reads the absence and runs its revoke sweep.
//     It must follow step 1 (see above) and precede step 4 (the sweep has to
//     see the entries already gone).
//
//  3. The host drops the guest from SharedWith and saves. This is what makes
//     the host's reconcile treat the guest's bot as departed and suspend it,
//     and what releases the host's own disable/stop guard. It must precede the
//     signal, or the reconcile the signal triggers would read a profile that
//     still lists the guest and suspend nothing.
//
//  4. The host is signalled. Its reconcile suspends the guest's bot and runs
//     the revoke sweep over what steps 2 and 3 just changed. A host that is
//     not running cannot do this, which is the pending case below.
//
//  5. Only now is the guest's HostWorkspace cleared. Doing it earlier would
//     leave a guest that no longer believes it is attached while the host is
//     still granting it privileges — and would strand step 1, which needs the
//     guest to know which host it is reconciling against.
//
// A host that is stopped at step 4 leaves the attachment half-torn-down: steps
// 1-3 have run, so the guest is already excluded, but nothing has suspended its
// bot. That is recorded on the guest as a pending detach and reported as
// ErrHostStoppedAtDetach so the operator reruns once the host is up.
func Detach(ctx context.Context, guest, host *Profiles, cfg Config) error {
	if guest == nil {
		return fmt.Errorf("irc/share: detach: a guest is required")
	}
	if guest.Profile.HostWorkspace == "" {
		return ErrNotAttached
	}

	// Step 1: strip what the guest authored, while it still can.
	if cfg.Reconcile != nil {
		if err := cfg.Reconcile(ctx); err != nil {
			return fmt.Errorf("irc/share: detach: final reconcile: %w", err)
		}
	}

	// A host whose state is gone entirely (purged with --force) leaves
	// nothing to strip, sweep or signal. The guest still has to forget it,
	// which is step 5 and the only step that can still run.
	if host == nil || !hostStateExists(host.Paths) {
		return clearGuestAttachment(guest)
	}

	// Step 2: the host stops delegating to this guest.
	if err := RemoveDelegates(host.Root, guest.ID, guest.Name); err != nil {
		return err
	}

	// Step 3: the host stops listing the guest, which is what its reconcile
	// reads to decide the guest's bot has departed.
	host.Profile.SharedWith = removeShared(host.Profile.SharedWith, guest.ID)
	if err := irc.SaveProfile(host.Paths.Profile, host.Profile); err != nil {
		return err
	}

	// Step 4: the host reconciles, suspending the bot and sweeping the
	// revoked grants. A stopped host cannot, so the guest records the detach
	// as pending and the caller reports it.
	if !cfg.running(host.Paths) {
		guest.Profile.PendingDetach = true
		if err := irc.SaveProfile(guest.Paths.Profile, guest.Profile); err != nil {
			return err
		}
		return fmt.Errorf("%w: rerun `wsm irc disable %s` once `wsm irc start %s` is up",
			ErrHostStoppedAtDetach, guest.Name, host.Name)
	}
	if cfg.Signal != nil {
		if err := cfg.Signal(host.Paths); err != nil {
			return fmt.Errorf("irc/share: detach: signal host %s: %w", host.Name, err)
		}
	}

	// Step 5: the guest forgets its host.
	return clearGuestAttachment(guest)
}

// clearGuestAttachment drops the guest's side of the link and saves it.
func clearGuestAttachment(guest *Profiles) error {
	guest.Profile.HostWorkspace = ""
	guest.Profile.PendingDetach = false
	return irc.SaveProfile(guest.Paths.Profile, guest.Profile)
}

// hostStateExists reports whether the host's profile is still on disk. A host
// purged with --force leaves nothing behind, and a guest disabling afterwards
// must still be able to clear its own side rather than failing on the absence.
func hostStateExists(paths irc.Paths) bool {
	if paths.Profile == "" {
		return false
	}
	_, err := os.Stat(paths.Profile)
	return err == nil
}

// HostState is what a guest reports about the workspace hosting it. It is read
// from the host's own status.json, which is possible only because both live
// under the same $DataDir on the same machine.
type HostState struct {
	// Workspace is the host's workspace id.
	Workspace string `json:"workspace"`
	// State is the host daemon's coarse state, or "gone" when the host's
	// state no longer exists at all (purged).
	State string `json:"state"`
}

// StateGone is the host state a guest reports when the host's IRC state has
// been removed from under it. Distinct from "stopped": a stopped host comes
// back, a purged one does not.
const StateGone = "gone"

// StateStopped is the host state a guest reports when the host's state is
// still on disk but no daemon is serving it.
const StateStopped = "stopped"

// ReadHostState reports the state of the workspace hosting this one.
//
// The host's status.json is read directly, which is possible only because both
// workspaces live under the same $DataDir on the same machine — the same
// property that lets the guest read the host's API token. A host whose profile
// is gone entirely was purged, which is reported as StateGone rather than as a
// missing file: the guest needs to tell "my host is down" from "my host no
// longer exists", because only the second one means its own attachment is
// never coming back.
//
// Failure is not an error. A guest reporting its own state must not fail
// because the host's is unreadable, so an unparseable or absent status yields
// StateStopped: no daemon is demonstrably serving it.
func ReadHostState(hostPaths irc.Paths, statusState func(irc.Paths) (string, bool)) string {
	if !hostStateExists(hostPaths) {
		return StateGone
	}
	if statusState == nil {
		return StateStopped
	}
	state, ok := statusState(hostPaths)
	if !ok || state == "" {
		return StateStopped
	}
	return state
}

// addShared appends id unless it is already listed. The list is a set; a
// duplicate would make the host's guest count wrong and its refusal message
// name the same workspace twice.
func addShared(list []string, id string) []string {
	for _, existing := range list {
		if existing == id {
			return list
		}
	}
	return append(list, id)
}

// removeShared drops every occurrence of id, preserving order.
func removeShared(list []string, id string) []string {
	out := list[:0:0]
	for _, existing := range list {
		if existing != id {
			out = append(out, existing)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hostAPIBase is the loopback URL of the host's ergo HTTP API.
func hostAPIBase(hostProfile irc.Profile) string {
	return "http://" + net.JoinHostPort(
		irc.DefaultBind, strconv.Itoa(hostProfile.Ports.API))
}
