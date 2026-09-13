package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/device"
	"hop.top/wsm/pkg/irc"
	ircdaemon "hop.top/wsm/pkg/irc/daemon"
	"hop.top/wsm/pkg/workspace"
)

// newIRCDaemonCmd builds the hidden `wsm irc daemon <ws>` command.
//
// It is hidden because it is not an operator-facing verb: `wsm irc start`
// spawns it detached, and running it by hand is a debugging affordance rather
// than part of the documented surface. It runs in the foreground and logs to
// the workspace's daemon log, so a hand invocation behaves exactly like the
// spawned one.
func newIRCDaemonCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon <workspace>",
		Short: "Run the IRC daemon for a workspace (internal)",
		Long: `Run the workspace's IRC daemon in the foreground.

Managed mode supervises an ergo child; both modes run the bot, reconcile on
start and on SIGHUP, capture channel traffic into the event log, and maintain
status.json. SIGUSR1 runs a backfill. SIGTERM and SIGINT quit the bot, stop the
child, and exit.

One daemon may hold a workspace at a time, enforced by an exclusive lock for
the daemon's lifetime. This command is spawned by 'wsm irc start'; run it by
hand only to watch a workspace's IRC session directly.`,
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCDaemon(ctx, cmd, app, args[0])
			})
		},
	}

	// Layer-A: a long-running process that owns a server, a session and the
	// event log. Re-running it against a workspace that already has a daemon
	// is refused by the lock rather than duplicated, so the honest
	// idempotency tier is conditional.
	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	setExitCodes(cmd, ClassOK, ClassUsage, ClassNotFound, ClassNotConfigured, ClassPrerequisite, ClassBackend)
	return cmd
}

// runIRCDaemon resolves the workspace, builds the daemon and serves until a
// terminating signal.
func runIRCDaemon(ctx context.Context, cmd *cobra.Command, app *appContext, ref string) error {
	ws, err := app.manager.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return err
	}

	paths, err := irc.ResolvePaths(irc.PathsInput{
		DataDir:  app.cfg.DataDir,
		StateDir: app.cfg.StateDir,
	}, ws.ID)
	if err != nil {
		return workspace.NewErrorNotConfigured(err.Error())
	}

	profile, err := irc.LoadProfile(paths.Profile)
	if err != nil {
		if os.IsNotExist(err) {
			return workspace.NewErrorNotConfigured(fmt.Sprintf(
				"IRC is not enabled for %q: run `wsm irc enable %s` first", ws.Name, ws.Name,
			))
		}
		return workspace.NewErrorBackend("read irc profile", err)
	}

	// The config is the global file merged with the workspace's own override,
	// found under the root the profile recorded at enable: the daemon runs
	// from an arbitrary working directory and cannot discover it otherwise.
	cfg, err := irc.LoadWorkspace(app.cfg.ConfigFile, profile.Root)
	if err != nil {
		return workspace.NewErrorBackend("load irc config", err)
	}

	logFile, log, err := openDaemonLog(paths)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	d, err := ircdaemon.New(ircdaemon.Deps{
		Cfg:              cfg,
		Paths:            paths,
		Profile:          profile,
		Workspace:        *ws,
		Backend:          app.backend,
		DataDir:          app.cfg.DataDir,
		StateDir:         app.cfg.StateDir,
		DeviceID:         device.CurrentWithDataDir(app.cfg.DataDir).ID,
		GlobalConfigPath: app.cfg.ConfigFile,
		Log:              log,
	})
	if err != nil {
		// Both of New's refusals are statements about wsm's own
		// recorded state — a profile with no mode, paths that were
		// never resolved — not about the environment around it.
		return workspace.NewErrorNotConfigured(err.Error())
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	watchSignals(runCtx, stop, d, log)

	// Capture runs alongside the signal loop: it is driven by the transport's
	// delivery goroutine and ends when the context does.
	captureDone := make(chan struct{})
	go func() {
		defer close(captureDone)
		if err := d.RunCapture(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("irc capture stopped", "error", err)
		}
	}()

	err = d.Run(runCtx)
	<-captureDone

	var locked *ircdaemon.LockedError
	if errors.As(err, &locked) {
		// Exit 1 with the hint: another daemon already holding the workspace
		// is a runtime condition the operator resolves, not a usage mistake.
		return &output.Error{
			Code:         output.CodeGeneric,
			Message:      locked.Error(),
			SuggestedFix: "stop the running daemon, or inspect it with `wsm irc status`",
			ExitCode:     1,
		}
	}
	return err
}

// watchSignals maps signals onto daemon actions.
//
// SIGHUP reconciles, SIGUSR1 backfills, and both are coalesced by the daemon
// rather than queued here: a burst of either must cost one run, not one each.
// SIGTERM and SIGINT cancel the run context, which is what starts the ordered
// shutdown.
func watchSignals(ctx context.Context, stop context.CancelFunc, d *ircdaemon.Daemon, log *slog.Logger) {
	ch := make(chan os.Signal, 8)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-ch:
				switch sig {
				case syscall.SIGHUP:
					log.Info("irc daemon reconciling on signal")
					d.Reconcile()
				case syscall.SIGUSR1:
					log.Info("irc daemon backfilling on signal")
					d.Backfill()
				case syscall.SIGTERM, syscall.SIGINT:
					log.Info("irc daemon stopping on signal", "signal", sig.String())
					stop()
					return
				}
			}
		}
	}()
}

// openDaemonLog opens the workspace's daemon log for appending.
//
// The log is appended to rather than truncated: a restart must not destroy the
// output explaining why the previous run ended.
func openDaemonLog(paths irc.Paths) (*os.File, *slog.Logger, error) {
	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return nil, nil, workspace.NewErrorBackend("create state dir", err)
	}
	f, err := os.OpenFile(paths.DaemonLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, workspace.NewErrorBackend("open daemon log", err)
	}
	return f, slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})), nil
}
