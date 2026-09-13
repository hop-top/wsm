//go:build darwin || linux

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/device"
	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot"
	ircdaemon "hop.top/wsm/pkg/irc/daemon"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/workspace"
)

// backfillWait bounds how long the CLI waits for a running daemon to
// report a finished backfill.
//
// Sixty seconds covers a thousand-message page per channel against a
// loaded server. Past that the request is not abandoned — the daemon
// keeps going and records the result in status.json — but the command
// stops claiming to know the outcome, which is the honest answer to a
// script waiting on its exit code.
const backfillWait = 60 * time.Second

// backfillPoll is how often the wait re-reads status.json.
const backfillPoll = 250 * time.Millisecond

// backfillRequestName is the file a CLI-initiated backfill leaves for
// the daemon, beside the rest of the workspace's volatile state.
const backfillRequestName = "backfill.request"

// newIRCBackfillCmd builds `wsm irc backfill`.
func newIRCBackfillCmd(opts *globalOptions) *cobra.Command {
	var channels []string

	cmd := &cobra.Command{
		Use:   "backfill <workspace>",
		Short: "Replay channel history into the event log",
		Long: `Fetch the messages wsm missed and record them.

This never opens a second IRC session beside the daemon. With a daemon
running the request is filed and the daemon executes it, reporting
through status.json; with the daemon stopped this command takes the same
exclusive lock the daemon holds while alive and runs the backfill under
it. Either way exactly one process is ever writing cursors, which is
what keeps a replay from recording the same messages twice.

Without --channel every channel with a cursor is replayed. A channel
that has never been captured has no position to resume from, so live
capture starts it rather than a backfill.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCBackfill(ctx, cmd, app, opts, args[0], channels)
			})
		},
	}

	cmd.Flags().StringArrayVar(&channels, "channel", nil, "Replay only this channel (repeatable)")

	cli.SetSideEffect(cmd, cli.SideEffectWriteShared)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	setExitCodes(cmd, ClassOK, ClassUsage, ClassNotFound, ClassUnauthorized, ClassNotConfigured, ClassPrerequisite, ClassBackend)
	return cmd
}

// ircBackfillDoc is the `wsm irc backfill --json` document.
type ircBackfillDoc struct {
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
	// RequestID correlates this run with the record left for the daemon.
	RequestID string `json:"request_id"`
	// Mode reports which path ran: "daemon" or "offline".
	Mode string `json:"mode"`
	// Channels holds one report per channel replayed.
	Channels []ircdaemon.BackfillChannel `json:"channels,omitempty"`
	// At is when the reported run finished.
	At *time.Time `json:"at,omitempty"`
	// Err is a run-level failure the daemon reported.
	Err string `json:"err,omitempty"`
}

// backfillRequest is the record a CLI-initiated backfill leaves behind.
//
// It is written before the signal and never deleted by this command: the
// daemon owns consuming it. Writing it is what makes a requested
// backfill auditable — the id in status.json's report can be traced to
// the invocation that asked for it, and a request that no daemon ever
// consumed is visible as a file rather than as silence.
type backfillRequest struct {
	// ID identifies this request.
	ID string `json:"id"`
	// Channels names what to replay; empty means every channel with a
	// cursor.
	Channels []string `json:"channels,omitempty"`
	// At is when the request was filed.
	At time.Time `json:"at"`
}

// runIRCBackfill replays history through whichever path owns the lock.
func runIRCBackfill(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	channels []string,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}
	// A replay writes messages into the workspace's event log, which is
	// the same shared state a reconcile touches: both are work asked of
	// the daemon rather than a report from it.
	if err := rt.gate.Require(workspace.CapIRCSync); err != nil {
		return err
	}

	channels = normalizeChannels(channels)
	requestID := newBackfillID()

	if done, derr := previewDryRun(cmd,
		map[string]any{
			"workspace": rt.ws.Name,
			"channels":  channels,
		},
		cli.Effect{
			Kind:       "update",
			Target:     "irc-events:" + rt.ws.ID,
			Reversible: false,
			Detail:     backfillDetail(channels),
		},
	); done {
		return derr
	}

	doc := ircBackfillDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		RequestID: requestID,
	}

	status, running := daemonState(rt.paths)
	if running {
		doc.Mode = "daemon"
		if err := requestDaemonBackfill(rt, status, requestID, channels, &doc); err != nil {
			return err
		}
	} else {
		doc.Mode = "offline"
		if err := runOfflineBackfill(ctx, rt, app, profile, channels, &doc); err != nil {
			return err
		}
	}

	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}
	return renderIRCBackfillResult(doc)
}

// requestDaemonBackfill files a request, signals the daemon and waits
// for it to report.
//
// The daemon is the lock holder, so it is the only process that may
// touch cursors. Asking it and waiting is therefore not a convenience —
// it is the only correct way to backfill a live workspace.
func requestDaemonBackfill(
	rt *ircRuntime,
	status ircdaemon.Status,
	requestID string,
	channels []string,
	doc *ircBackfillDoc,
) error {
	// The baseline is read before the signal: the daemon reports a
	// backfill by replacing last_backfill, and without a "before" there
	// is no way to tell its answer from the previous run's.
	baseline := lastBackfillAt(status)

	if err := writeBackfillRequest(rt.paths, requestID, channels); err != nil {
		return err
	}
	if err := signalDaemon(status.PID, syscall.SIGUSR1); err != nil {
		return &output.Error{
			Code:     output.CodeGeneric,
			Message:  fmt.Sprintf("signal irc daemon %d: %v", status.PID, err),
			ExitCode: 1,
		}
	}

	summary, err := awaitBackfill(rt.paths, baseline)
	if err != nil {
		return err
	}
	doc.Channels = summary.Channels
	doc.Err = summary.Err
	at := summary.At
	doc.At = &at
	return nil
}

// writeBackfillRequest records what was asked for, atomically.
//
// The rename is what makes it atomic: the daemon may read this file the
// instant the signal lands, and a partially written request would be
// indistinguishable from a malformed one.
func writeBackfillRequest(paths irc.Paths, requestID string, channels []string) error {
	req := backfillRequest{ID: requestID, Channels: channels, At: time.Now().UTC()}
	raw, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return workspace.NewErrorBackend("encode backfill request", err)
	}

	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return workspace.NewErrorBackend("create state dir", err)
	}
	path := filepath.Join(paths.StateDir, backfillRequestName)

	tmp, err := os.CreateTemp(paths.StateDir, ".backfill-*.json")
	if err != nil {
		return workspace.NewErrorBackend("create backfill request", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return workspace.NewErrorBackend("write backfill request", err)
	}
	if err := tmp.Close(); err != nil {
		return workspace.NewErrorBackend("close backfill request", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return workspace.NewErrorBackend("install backfill request", err)
	}
	return nil
}

// awaitBackfill waits for the daemon to publish a report newer than the
// baseline.
//
// A timeout is exit 1 rather than a silent success: the backfill may
// still be running, and a script that read exit 0 would move on as
// though the history were already recorded.
func awaitBackfill(paths irc.Paths, baseline time.Time) (ircdaemon.BackfillSummary, error) {
	deadline := time.Now().Add(backfillWait)
	for {
		status, err := ircdaemon.ReadStatus(paths.Status)
		if err == nil && status.LastBackfill != nil {
			if status.LastBackfill.At.After(baseline) {
				return *status.LastBackfill, nil
			}
		}
		if time.Now().After(deadline) {
			return ircdaemon.BackfillSummary{}, &output.Error{
				Code: output.CodeGeneric,
				Message: fmt.Sprintf(
					"irc daemon did not report a backfill within %s: it may still be running; check `wsm irc status`",
					backfillWait),
				ExitCode: 1,
			}
		}
		time.Sleep(backfillPoll)
	}
}

// lastBackfillAt reports when the daemon last finished a backfill.
func lastBackfillAt(status ircdaemon.Status) time.Time {
	if status.LastBackfill == nil {
		return time.Time{}
	}
	return status.LastBackfill.At
}

// runOfflineBackfill replays history with no daemon running.
//
// The exclusive lock is the whole point. It is the same file the daemon
// takes for its lifetime, so holding it here means a daemon cannot start
// underneath this run and no second CLI backfill can race it. Cursors
// are written only by whoever holds this lock, which is what makes
// "exactly one writer" true rather than hoped for.
func runOfflineBackfill(
	ctx context.Context,
	rt *ircRuntime,
	app *appContext,
	profile irc.Profile,
	channels []string,
	doc *ircBackfillDoc,
) error {
	lock, err := acquireWorkspaceLock(rt.paths)
	if err != nil {
		return err
	}
	defer lock.release()

	cursors, err := bot.LoadCursors(rt.paths.Cursors)
	if err != nil {
		return workspace.NewErrorBackend("read cursors", err)
	}

	targets := channels
	if len(targets) == 0 {
		targets = sortedKeys(cursors.All())
	}
	if len(targets) == 0 {
		// Nothing has ever been captured, so there is no position to
		// resume from. Not an error: live capture is what starts a
		// channel, and reporting zero channels says exactly that.
		doc.At = nowPtr()
		return nil
	}

	session, err := openIRCSession(ctx, rt, profile)
	if err != nil {
		return err
	}
	defer session.Close()

	members, _ := loadWorkspaceMembers(profile)
	_, reverse := provider.ResolverFor(profile, members, app.cfg.DataDir)

	capture := &bot.Capture{
		Transport:   session.transport,
		Directory:   session.providers.Directory,
		Reverse:     reverse,
		ServerID:    serverIDOf(rt, profile),
		BotAccount:  botAccountOf(rt, profile),
		WorkspaceID: rt.ws.ID,
		WriterID:    ircdaemon.WriterID(rt.ws.ID),
		Recorder: ircdaemon.NewRecorder(
			app.backend, rt.ws.ID, device.CurrentWithDataDir(app.cfg.DataDir).ID),
		Cursors: cursors,
	}

	opts := bot.BackfillOptions{
		Limit: bot.DefaultBackfillLimit,
		Seen:  ircdaemon.NewSeen(app.backend, rt.ws.ID),
	}

	for _, channel := range targets {
		cursor, known := cursors.Get(channel)
		if !known {
			// An explicitly named channel with no cursor has no
			// position to resume from; report it rather than inventing
			// one, which would replay the channel's entire history.
			doc.Channels = append(doc.Channels, ircdaemon.BackfillChannel{Channel: channel})
			continue
		}
		report, berr := capture.Backfill(ctx, channel, cursor, opts)
		if berr != nil {
			doc.Err = berr.Error()
			continue
		}
		doc.Channels = append(doc.Channels, ircdaemon.BackfillChannel{
			Channel:     report.Channel,
			Fetched:     report.Fetched,
			Recorded:    report.Recorded,
			Skipped:     report.Skipped,
			Gap:         report.Gap,
			Unsupported: report.Unsupported,
		})
	}

	// Saved while the lock is still held, so the cursor file is never
	// written by a process that has given up its claim to it.
	if serr := cursors.Save(); serr != nil {
		return workspace.NewErrorBackend("save cursors", serr)
	}
	doc.At = nowPtr()
	return nil
}

// workspaceLock is an exclusive flock held for one offline backfill.
type workspaceLock struct {
	file *os.File
}

// release drops the lock and closes its file.
func (l *workspaceLock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
}

// acquireWorkspaceLock takes the workspace's exclusive lock, or reports
// who holds it.
//
// Non-blocking, matching the daemon: a held lock means another process
// is already driving this workspace's session, and waiting for it would
// turn a clear refusal into a hang. The lock path is the daemon's own,
// read from the daemon package rather than rebuilt here, so the two can
// never drift onto different files and both believe they are alone.
func acquireWorkspaceLock(paths irc.Paths) (*workspaceLock, error) {
	path := ircdaemon.LockPath(paths)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, workspace.NewErrorBackend("create state dir", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, workspace.NewErrorBackend(fmt.Sprintf("open %s", path), err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, &output.Error{
			Code: output.CodeGeneric,
			Message: fmt.Sprintf(
				"another process holds %s: an irc daemon may have started; "+
					"cursors are only ever written by the lock holder", path),
			SuggestedFix: "re-run the backfill, which will file a request with the running daemon",
			ExitCode:     1,
		}
	}
	return &workspaceLock{file: f}, nil
}

// normalizeChannels trims, de-duplicates and orders the requested
// channels, so a request records what was asked for once each.
func normalizeChannels(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, ch := range raw {
		ch = strings.TrimSpace(ch)
		if ch == "" || seen[ch] {
			continue
		}
		seen[ch] = true
		out = append(out, ch)
	}
	sort.Strings(out)
	return out
}

// newBackfillID mints an identifier for one backfill request.
//
// The clock is the source: a request is correlated by the operator
// reading a log beside a status document, and a timestamp is what makes
// two requests orderable at a glance. Uniqueness within a nanosecond is
// not needed, because a second request in the same nanosecond would
// coalesce into the same daemon run anyway.
func newBackfillID() string {
	return "bf-" + time.Now().UTC().Format("20060102T150405.000000000")
}

// nowPtr returns a pointer to the current instant, for the report.
func nowPtr() *time.Time {
	now := time.Now().UTC()
	return &now
}

// backfillDetail describes the run for a dry-run plan.
func backfillDetail(channels []string) string {
	if len(channels) == 0 {
		return "replay every channel with a cursor into the event log"
	}
	return "replay " + strings.Join(channels, ", ") + " into the event log"
}

// ircBackfillRow is one channel's outcome in the table rendering.
type ircBackfillRow struct {
	Channel     string `table:"CHANNEL,priority=9"  json:"channel"`
	Fetched     int    `table:"FETCHED,priority=8"  json:"fetched"`
	Recorded    int    `table:"RECORDED,priority=8" json:"recorded"`
	Skipped     int    `table:"SKIPPED,priority=6"  json:"skipped"`
	Gap         bool   `table:"GAP,priority=7"      json:"gap"`
	Unsupported bool   `table:"UNSUPPORTED,priority=5" json:"unsupported"`
}

// renderIRCBackfillResult renders the human-readable outcome.
func renderIRCBackfillResult(doc ircBackfillDoc) error {
	rows := make([]ircBackfillRow, 0, len(doc.Channels))
	for _, ch := range doc.Channels {
		rows = append(rows, ircBackfillRow{
			Channel:     ch.Channel,
			Fetched:     ch.Fetched,
			Recorded:    ch.Recorded,
			Skipped:     ch.Skipped,
			Gap:         ch.Gap,
			Unsupported: ch.Unsupported,
		})
	}
	if len(rows) > 0 {
		if err := output.Render(os.Stdout, output.Table, rows); err != nil {
			return err
		}
	}

	// §6.3: diagnostics are non-data → stderr.
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "No channel had a cursor to resume from.")
	}
	for _, ch := range doc.Channels {
		if ch.Gap {
			fmt.Fprintf(os.Stderr,
				"gap: %s lost messages to the server's retention window\n", ch.Channel)
		}
		if ch.Unsupported {
			fmt.Fprintf(os.Stderr,
				"unsupported: %s — the server granted no chathistory\n", ch.Channel)
		}
	}
	if doc.Err != "" {
		fmt.Fprintf(os.Stderr, "backfill reported: %s\n", doc.Err)
	}
	return nil
}
