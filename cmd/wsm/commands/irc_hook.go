package commands

import (
	"context"
	"fmt"
	"os"

	"hop.top/kit/go/runtime/bus"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provision"
	"hop.top/wsm/pkg/workspace"
)

// wireIRCToBus subscribes the IRC lifecycle to workspace events and
// reports the unsubscribe appContext.Close must run.
//
// The decision to subscribe at all belongs to provision.Subscribe, which
// wires nothing outside managed mode with auto_provision on. This
// function only supplies the adapter that turns a workspace id into the
// `wsm irc` work.
func wireIRCToBus(b bus.Bus, cfg irc.Config, app *appContext) func() {
	unsub := provision.Subscribe(b, cfg, provision.HookOptions{
		Lifecycle: &ircBusLifecycle{app: app},
		Profiles:  provision.FileProfileLoader(app.cfg.DataDir, app.cfg.StateDir),
		Errs:      os.Stderr,
	})
	return func() { unsub() }
}

// ircBusLifecycle drives the IRC verbs from a bus event.
//
// It deliberately reuses the helpers the CLI verbs are built from
// (resolveIRCRuntime, startDaemon, stopDaemon) rather than the run*
// functions themselves. Those take a *cobra.Command for their dry-run
// preview and their --confirm gate, neither of which a bus handler has:
// `wsm workspace delete` already confirmed the destruction, and
// gatePurge off a TTY refuses outright, so routing through it would turn
// every auto-provisioned teardown into an exit-5 refusal.
type ircBusLifecycle struct {
	app *appContext
}

// Enable declines, with the reason, rather than guessing a workspace root.
//
// `wsm irc enable` records Profile.Root from --dir or the process working
// directory, and the daemon reads members.yaml from that path for the rest
// of the workspace's life. A bus handler has neither: the event carries an
// id, model.Workspace has no root field, and the CWD of whoever ran
// `wsm workspace add` is not the new workspace's root in any reliable
// sense. Recording it would produce a daemon reconciling an empty member
// list, which fails later and further from the cause than saying so here.
//
// Auto-provision therefore covers the transitions of an already-enabled
// workspace (start, stop, purge). Turning IRC on for the first time stays
// an explicit `wsm irc enable`, where the root is stated rather than
// inferred.
func (l *ircBusLifecycle) Enable(_ context.Context, wsID string) error {
	return fmt.Errorf(
		"auto-provision cannot infer the workspace root; enable from the workspace directory with `wsm irc enable %s`",
		wsID)
}

// Start starts the daemon of an already-enabled workspace.
func (l *ircBusLifecycle) Start(ctx context.Context, wsID string) error {
	rt, err := l.runtime(ctx, wsID)
	if err != nil {
		return err
	}
	if _, lerr := irc.LoadProfile(rt.paths.Profile); lerr != nil {
		return fmt.Errorf("workspace is not irc-enabled")
	}
	_, _, err = startDaemon(rt)
	return err
}

// Stop stops the workspace's daemon, leaving its state on disk.
func (l *ircBusLifecycle) Stop(ctx context.Context, wsID string) error {
	rt, err := l.runtime(ctx, wsID)
	if err != nil {
		return err
	}
	_, err = stopDaemon(rt)
	return err
}

// Purge removes the workspace's IRC data and state.
//
// No confirmation is asked here. The workspace delete that produced this
// event already carried the destructive gate, and a second prompt from a
// bus handler would either hang a non-interactive caller or ask the same
// operator twice for one decision.
func (l *ircBusLifecycle) Purge(ctx context.Context, wsID string) error {
	if err := requireCapability(l.app, workspace.CapIRCPurge); err != nil {
		return err
	}
	paths, err := irc.ResolvePaths(irc.PathsInput{
		DataDir:  l.app.cfg.DataDir,
		StateDir: l.app.cfg.StateDir,
	}, wsID)
	if err != nil {
		return err
	}
	for _, dir := range []string{paths.DataDir, paths.StateDir} {
		if rerr := os.RemoveAll(dir); rerr != nil {
			return fmt.Errorf("remove %s: %w", dir, rerr)
		}
	}
	return nil
}

// runtime resolves the shared IRC runtime for a workspace id.
//
// A deleted workspace is resolvable only until its record is gone; Purge
// therefore works from paths alone and never comes through here.
func (l *ircBusLifecycle) runtime(ctx context.Context, wsID string) (*ircRuntime, error) {
	return resolveIRCRuntime(ctx, l.app, wsID)
}
