package provision

import (
	"context"
	"fmt"
	"io"
	"strings"

	"hop.top/kit/go/runtime/bus"

	"hop.top/wsm/pkg/irc"
)

// TopicPattern is the bus pattern the lifecycle hook subscribes to. The
// trailing `#` matches every wsm.runtime.workspace.* action, so a topic
// added later reaches the handler rather than being silently dropped by a
// list of exact subscriptions that nobody remembers to extend.
const TopicPattern = "wsm.runtime.workspace.#"

// Lifecycle is the IRC side of a workspace lifecycle transition. It is
// implemented by the `wsm irc` verbs: Enable provisions and starts,
// Start/Stop move the daemon, Purge removes the workspace's IRC state.
//
// The interface lives here rather than in the CLI so this package stays
// the consumer's dependency and the hook is testable with a fake. Every
// method takes the workspace ULID, never a name: a rename must not strand
// the state a previous event created.
type Lifecycle interface {
	// Enable provisions IRC for a workspace and starts its daemon.
	Enable(ctx context.Context, wsID string) error
	// Start starts the daemon of an already-enabled workspace.
	Start(ctx context.Context, wsID string) error
	// Stop stops the workspace's daemon, leaving its state on disk.
	Stop(ctx context.Context, wsID string) error
	// Purge removes the workspace's IRC data and state.
	Purge(ctx context.Context, wsID string) error
}

// ProfileLoader reads the durable profile of one workspace. The hook needs
// it for the host guard: a bus event carries only a workspace id, and
// whether that workspace hosts guests is recorded in Profile.SharedWith.
//
// A workspace with no profile yet (never enabled) must report the zero
// Profile and a nil error, not a failure: "not enabled" is the normal
// state of every workspace at created time, and the guard has nothing to
// refuse there.
type ProfileLoader func(wsID string) (irc.Profile, error)

// FileProfileLoader reads profiles from the standard on-disk layout.
// A missing profile yields the zero Profile so a not-yet-enabled
// workspace reads as "no guests" rather than as an error.
func FileProfileLoader(dataDir, stateDir string) ProfileLoader {
	return func(wsID string) (irc.Profile, error) {
		paths, err := irc.ResolvePaths(irc.PathsInput{
			DataDir:  dataDir,
			StateDir: stateDir,
		}, wsID)
		if err != nil {
			return irc.Profile{}, err
		}
		profile, err := irc.LoadProfile(paths.Profile)
		if err != nil {
			return irc.Profile{}, nil
		}
		return profile, nil
	}
}

// HookOptions carries the hook's collaborators.
type HookOptions struct {
	// Lifecycle performs the IRC-side transitions. Required.
	Lifecycle Lifecycle
	// Profiles reads Profile.SharedWith for the host guard. When nil the
	// guard cannot consult anything and every teardown is refused, which
	// fails safe: refusing to stop a server is recoverable by hand,
	// stranding somebody else's guests is not.
	Profiles ProfileLoader
	// Errs receives refusals and failures. Callers pass os.Stderr; a nil
	// writer discards them.
	Errs io.Writer
}

// Subscribe wires workspace lifecycle events to the IRC lifecycle, and
// reports the unsubscribe the caller must run at shutdown.
//
// The subscription only exists when the operator asked for it: managed
// mode with auto_provision on. External mode is somebody else's server —
// wsm has no authority to start or stop it from a workspace event — and
// auto_provision off means the operator drives `wsm irc` by hand. In
// either case nothing is subscribed at all, so no handler can fire and
// no event is inspected; the returned unsubscribe is a no-op.
//
// UX consequence worth stating plainly: wiring this inline makes
// `wsm workspace add` do the IRC provisioning before it exits, so the
// command can block for up to the daemon readiness timeout (~15s) while
// the new workspace's daemon comes up. That is the cost of having a
// workspace that is usable the moment the command returns, rather than
// one whose chat arrives some indeterminate time later.
func Subscribe(b bus.Bus, cfg irc.Config, opts HookOptions) bus.Unsubscribe {
	noop := func() {}
	if b == nil || opts.Lifecycle == nil {
		return noop
	}
	if !cfg.AutoProvision || cfg.Mode != irc.ModeManaged {
		return noop
	}

	h := &hook{opts: opts}
	return b.Subscribe(TopicPattern, h.handle)
}

// hook holds the handler's collaborators.
type hook struct {
	opts HookOptions
}

// handle routes one workspace event to the IRC lifecycle.
//
// The handler always reports nil. The Manager ignores the error Publish
// returns (lifecycle correctness must not depend on a subscriber), so a
// returned error would be swallowed and the operator would see nothing
// at all. Failures are written to Errs with the command that retries
// them by hand instead.
func (h *hook) handle(ctx context.Context, e bus.Event) error {
	wsID := e.WorkspaceID
	if wsID == "" {
		return nil
	}

	switch e.Topic {
	case "wsm.runtime.workspace.created":
		h.run(ctx, "enable", wsID, h.opts.Lifecycle.Enable)
	case "wsm.runtime.workspace.unarchived":
		h.run(ctx, "start", wsID, h.opts.Lifecycle.Start)
	case "wsm.runtime.workspace.archived":
		if h.hostGuarded(wsID, "stop") {
			return nil
		}
		h.run(ctx, "stop", wsID, h.opts.Lifecycle.Stop)
	case "wsm.runtime.workspace.deleted":
		if h.hostGuarded(wsID, "disable --purge") {
			return nil
		}
		// No second confirmation here. `wsm workspace delete` already
		// asked, and a prompt from a bus handler would either hang a
		// non-interactive caller or ask the same operator twice for the
		// same decision.
		h.run(ctx, "stop", wsID, h.opts.Lifecycle.Stop)
		h.run(ctx, "disable --purge", wsID, h.opts.Lifecycle.Purge)
	default:
		// updated, and any topic added later: nothing about IRC changes
		// when a workspace's metadata does.
	}
	return nil
}

// hostGuarded reports whether wsID hosts guest workspaces, and logs the
// refusal when it does.
//
// Other workspaces' accounts and channels live on a host's server. Tearing
// it down from a bus event would cut them off without anyone asking, so the
// hook declines and names the manual command that carries --force — the
// operator who does want the teardown says so explicitly.
func (h *hook) hostGuarded(wsID, verb string) bool {
	if h.opts.Profiles == nil {
		h.logf("irc %s skipped for workspace %s: no profile loader to check hosted guests; "+
			"run `wsm irc %s %s --force` by hand if the teardown is intended",
			verb, wsID, verb, wsID)
		return true
	}
	profile, err := h.opts.Profiles(wsID)
	if err != nil {
		h.logf("irc %s skipped for workspace %s: profile unreadable (%v); "+
			"run `wsm irc %s %s --force` by hand if the teardown is intended",
			verb, wsID, err, verb, wsID)
		return true
	}
	if len(profile.SharedWith) == 0 {
		return false
	}
	h.logf("irc %s skipped: workspace %s hosts %d guest workspace(s) on its IRC server (%s); "+
		"stopping it would cut them off; run `wsm irc %s %s --force` to proceed anyway",
		verb, wsID, len(profile.SharedWith), strings.Join(profile.SharedWith, ", "), verb, wsID)
	return true
}

// run performs one lifecycle step and reports a failure with its retry.
func (h *hook) run(ctx context.Context, verb, wsID string, fn func(context.Context, string) error) {
	if err := fn(ctx, wsID); err != nil {
		h.logf("irc %s failed for workspace %s: %v; retry with `wsm irc %s %s`",
			verb, wsID, err, verb, wsID)
	}
}

// logf writes one line to Errs. A nil writer discards it.
func (h *hook) logf(format string, args ...any) {
	if h.opts.Errs == nil {
		return
	}
	fmt.Fprintln(h.opts.Errs, fmt.Sprintf(format, args...))
}
