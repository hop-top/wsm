// Package supervisor owns the lifecycle of the managed ergo child process.
//
// One Supervisor runs inside the wsm IRC daemon in managed mode: it spawns
// `ergo run --conf <path>`, appends the child's stdout and stderr to a log
// file, records the pid, and waits for the caller's readiness probe. If ergo
// exits without having been asked to, the Supervisor restarts it on the shared
// irc.Backoff curve. Once the attempt budget is exhausted it reports Degraded
// and stays alive: the daemon outlives an ergo it cannot resurrect, because a
// degraded daemon still serves status and can be repaired without losing the
// rest of the workspace's state.
//
// This package holds no UI dependency (CI enforces `make verify-no-ui-leak`)
// and no policy of its own: the readiness probe, the backoff curve and the
// clock all arrive from the caller.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"hop.top/wsm/pkg/irc"
)

// StopGrace is how long Stop waits for a child to leave after SIGTERM before
// escalating to SIGKILL.
const StopGrace = 5 * time.Second

// DefaultStableAfter is how long a restarted child must stay up before the
// supervisor accepts it as recovered. Anything shorter is treated as a
// continuation of the same crash loop, so a binary that forks and dies
// immediately still exhausts the restart budget and degrades rather than
// being respawned forever.
const DefaultStableAfter = 10 * time.Second

// logMode is the permission for a newly created ergo log file. The log can
// carry connection metadata, so it stays owner-only like the rest of the IRC
// state tree.
const logMode os.FileMode = 0o600

// pidMode is the permission for the pidfile.
const pidMode os.FileMode = 0o600

// Phase is the coarse lifecycle state of the supervised child, mirroring the
// `state` field of `wsm irc status --json`.
type Phase string

const (
	// PhaseStopped means no child is running and none is wanted: either
	// Start was never called, or Stop completed.
	PhaseStopped Phase = "stopped"
	// PhaseRunning means a child is alive and last passed its readiness
	// probe.
	PhaseRunning Phase = "running"
	// PhaseDegraded means the child is gone and the restart budget is
	// spent. The supervisor, and the daemon around it, stay up.
	PhaseDegraded Phase = "degraded"
)

// State is a point-in-time snapshot of the supervised child, safe to render
// into status.json.
type State struct {
	// Phase is the lifecycle state.
	Phase Phase `json:"phase"`
	// PID is the live child's process id, or 0 when nothing is running.
	PID int `json:"pid"`
	// Restarts counts unexpected exits that the supervisor responded to
	// since Start. It is cumulative for the life of the Supervisor and is
	// never reset by a successful restart, so it reads as a health signal
	// rather than as the remaining budget. The budget itself is internal:
	// it resets once a restarted child proves it can stay up.
	Restarts int `json:"restarts"`
	// LastErr is the most recent failure: the exit error, a failed
	// readiness probe, or the error that exhausted the budget. Empty when
	// nothing has gone wrong.
	LastErr string `json:"last_err,omitempty"`
}

// Supervisor manages one ergo child process.
//
// The zero value is not usable: Binary and ConfPath are required. A zero
// Backoff falls back to irc.NewBackoff, so a caller that does not care about
// the curve can leave it unset.
type Supervisor struct {
	// Binary is the ergo executable, a PATH lookup or an absolute path.
	Binary string
	// ConfPath is the rendered ergo.yaml passed as --conf.
	ConfPath string
	// PIDFile receives the live child's pid; removed by Stop.
	PIDFile string
	// LogPath collects the child's stdout and stderr. It is always
	// appended to, never truncated, so a restart does not destroy the
	// output that explains why the previous child died.
	LogPath string

	// Ready probes the freshly spawned child and returns nil once it is
	// serving. The policy lives with the caller: the daemon passes an HTTP
	// status poll on a 200ms interval capped at 10s. A nil Ready means the
	// child is considered ready as soon as it has been spawned.
	Ready func(ctx context.Context) error

	// Backoff is the restart curve. Zero means irc.NewBackoff().
	Backoff irc.Backoff

	// StableAfter is how long a restarted child must stay up before the
	// restart budget is restored. Zero means DefaultStableAfter.
	StableAfter time.Duration

	// Clock is the time source used to measure how long a child lived and
	// to sleep between restarts. Nil means the Backoff's clock, falling
	// back to irc.SystemClock.
	Clock irc.Clock

	// mu guards every field below it.
	mu sync.Mutex
	// cmd is the live child, nil when nothing is running.
	cmd *exec.Cmd
	// state is the snapshot Status returns.
	state State
	// stopping is closed by Stop to break the supervise loop out of a
	// backoff sleep and to mark an expected exit.
	stopping chan struct{}
	// exited is closed by the supervise loop when it returns, so Stop can
	// wait for the loop to finish rather than racing it.
	exited chan struct{}
	// running reports whether a supervise loop is active, so a second
	// Start is rejected rather than orphaning the first child.
	running bool
}

// Start spawns ergo and blocks until the readiness probe passes, then leaves a
// supervise loop running in the background to restart the child if it dies.
//
// A failure to spawn, or a readiness probe that never passes, is a failed
// start: the child is killed, the pidfile is removed and the error is
// returned. No restart is attempted, because a child that never became ready
// once will not become ready by being run again — that is a configuration
// problem for the caller to surface.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("irc supervisor: already started")
	}
	if s.Binary == "" {
		s.mu.Unlock()
		return fmt.Errorf("irc supervisor: binary is required")
	}
	if s.ConfPath == "" {
		s.mu.Unlock()
		return fmt.Errorf("irc supervisor: conf path is required")
	}
	s.running = true
	s.stopping = make(chan struct{})
	s.exited = make(chan struct{})
	s.state = State{Phase: PhaseStopped}
	stopping := s.stopping
	exited := s.exited
	s.mu.Unlock()

	// nil stopping: Stop cannot be in flight before Start has returned.
	if err := s.spawnAndWaitReady(ctx, nil); err != nil {
		s.mu.Lock()
		s.running = false
		s.state.Phase = PhaseStopped
		s.state.PID = 0
		s.state.LastErr = err.Error()
		s.mu.Unlock()
		close(exited)
		s.removePIDFile()
		return err
	}

	go s.supervise(stopping, exited)
	return nil
}

// spawnAndWaitReady launches the child, records it, and runs the readiness
// probe. On any failure it tears the child back down so no process is left
// behind.
//
// stopping makes the spawn race-free against Stop. Without it a restart that
// begins just after Stop has taken its snapshot of the current child would
// leave the freshly spawned process untracked, and it would outlive the
// supervisor as an orphan. A nil stopping channel means "no Stop can be in
// flight", which is the case for the initial spawn inside Start.
func (s *Supervisor) spawnAndWaitReady(ctx context.Context, stopping <-chan struct{}) error {
	// Never fork when teardown has already begun.
	if stopped(stopping) {
		return fmt.Errorf("irc supervisor: stopping")
	}

	cmd, err := s.spawn()
	if err != nil {
		return err
	}

	// If Stop closed the channel while the fork was in flight, this child
	// is already unwanted and nothing else will reap it.
	if stopped(stopping) {
		s.killNow(cmd)
		return fmt.Errorf("irc supervisor: stopping")
	}

	s.mu.Lock()
	s.cmd = cmd
	s.state.Phase = PhaseRunning
	s.state.PID = cmd.Process.Pid
	s.mu.Unlock()

	if err := s.writePIDFile(cmd.Process.Pid); err != nil {
		s.killNow(cmd)
		return err
	}

	if s.Ready != nil {
		if err := s.Ready(ctx); err != nil {
			s.killNow(cmd)
			return fmt.Errorf("irc supervisor: ergo did not become ready: %w", err)
		}
	}

	// A Stop that landed during the readiness probe must not leave the
	// child running either.
	if stopped(stopping) {
		s.killNow(cmd)
		return fmt.Errorf("irc supervisor: stopping")
	}

	return nil
}

// stopped reports whether ch has been closed. A nil channel is never closed.
func stopped(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// spawn starts `ergo run --conf <path>` with both output streams appended to
// LogPath.
func (s *Supervisor) spawn() (*exec.Cmd, error) {
	logFile, err := s.openLog()
	if err != nil {
		return nil, err
	}

	// Deliberately not exec.CommandContext: the child's lifetime is owned
	// by Stop, which must SIGTERM and escalate on its own schedule rather
	// than have the context kill the process out from under the loop.
	cmd := exec.Command(s.Binary, "run", "--conf", s.ConfPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, fmt.Errorf("irc supervisor: start %s: %w", s.Binary, err)
	}

	// The child holds its own descriptor for the log now; the parent's copy
	// is no longer needed and must not leak across restarts.
	if logFile != nil {
		_ = logFile.Close()
	}

	return cmd, nil
}

// openLog opens LogPath for appending, creating it and its directory if
// needed. An empty LogPath discards the child's output.
func (s *Supervisor) openLog() (*os.File, error) {
	if s.LogPath == "" {
		return nil, nil
	}
	if dir := filepath.Dir(s.LogPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("irc supervisor: create log dir %s: %w", dir, err)
		}
	}
	// O_APPEND, never O_TRUNC: a restart must add to the record of why the
	// previous child died, not erase it.
	f, err := os.OpenFile(s.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logMode)
	if err != nil {
		return nil, fmt.Errorf("irc supervisor: open log %s: %w", s.LogPath, err)
	}
	return f, nil
}

// supervise waits on the current child and restarts it on the backoff curve
// until the budget is spent or Stop is called.
func (s *Supervisor) supervise(stopping chan struct{}, exited chan struct{}) {
	defer close(exited)

	attempt := 0
	backoff := s.backoff()
	clock := s.clock()

	// When the child currently being supervised was spawned. Start has just
	// produced it, so the first measurement runs from now.
	startedAt := clock.Now()

	for {
		s.mu.Lock()
		cmd := s.cmd
		s.mu.Unlock()

		if cmd == nil {
			// Nothing to supervise: Start never produced a child, or
			// Stop cleared it. Either way there is no exit to wait for.
			return
		}

		waitErr := cmd.Wait()

		// An exit that Stop asked for is not a failure and never triggers
		// a restart.
		select {
		case <-stopping:
			return
		default:
		}

		// How long this child managed to stay up decides whether the
		// restart that produced it counts as a recovery. A child that
		// dies immediately, over and over, is a crash loop: it must keep
		// burning the budget until the supervisor degrades, never reset
		// it, or ergo would be respawned forever.
		lived := clock.Now().Sub(startedAt)

		s.mu.Lock()
		s.state.Restarts++
		s.state.PID = 0
		s.state.LastErr = exitReason(waitErr)
		s.cmd = nil
		s.mu.Unlock()

		if lived >= s.stableAfter() {
			// The child had settled before it died, so this is a fresh
			// failure rather than the continuation of a streak: give it
			// the full budget again.
			attempt = 0
		}

		// The child is down. Retry it on the curve until one comes back
		// and the budget survives. Both an unexpected exit and a restart
		// that fails to spawn or never becomes ready consume one attempt,
		// so the two kinds of failure burn the budget at the same rate.
		reason := exitReason(waitErr)

		for {
			attempt++

			if backoff.Done(attempt) {
				s.markDegraded(fmt.Sprintf(
					"ergo failed %d times (%s); restart budget exhausted",
					attempt-1, reason,
				))
				return
			}

			// Wait reports false when stopping fired, which means Stop
			// won the race against the sleep.
			if !backoff.Wait(attempt, stopping) {
				return
			}

			spawnedAt := clock.Now()
			err := s.spawnAndWaitReady(context.Background(), stopping)
			if err == nil {
				startedAt = spawnedAt
				break
			}

			s.mu.Lock()
			s.state.PID = 0
			s.state.LastErr = err.Error()
			s.mu.Unlock()
			reason = err.Error()
		}
	}
}

// stableAfter is how long a restarted child must stay up before the
// supervisor treats it as recovered and restores the full restart budget.
// Below this, a child that exits straight back out is still part of the same
// crash loop.
func (s *Supervisor) stableAfter() time.Duration {
	if s.StableAfter > 0 {
		return s.StableAfter
	}
	return DefaultStableAfter
}

// markDegraded records the terminal state. The supervisor stays alive: the
// daemon must keep serving status even when ergo cannot be revived.
func (s *Supervisor) markDegraded(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Phase = PhaseDegraded
	s.state.PID = 0
	s.state.LastErr = reason
	s.cmd = nil
	s.running = false
	s.removePIDFileLocked()
}

// Stop terminates the child with SIGTERM, escalates to SIGKILL after
// StopGrace, and removes the pidfile. It is safe to call when nothing is
// running and safe to call twice.
//
// A ctx that is already done, or that expires before the grace period, cuts
// the wait short and escalates immediately: a caller that has run out of time
// wants the process gone, not a polite request that may never be answered.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.stopping != nil {
		select {
		case <-s.stopping:
			// Already stopping; fall through to the wait below.
		default:
			close(s.stopping)
		}
	}
	cmd := s.cmd
	exited := s.exited
	s.mu.Unlock()

	var err error
	if cmd != nil && cmd.Process != nil {
		err = s.terminate(ctx, cmd)
	}

	// Let the supervise loop observe the closed stopping channel and
	// return, so no restart races this teardown.
	if exited != nil {
		select {
		case <-exited:
		case <-ctx.Done():
		}
	}

	// The loop may have spawned a replacement between the snapshot above
	// and noticing that it should stop. Re-check, so that child is killed
	// too rather than orphaned.
	s.mu.Lock()
	late := s.cmd
	s.mu.Unlock()
	if late != nil && late != cmd && late.Process != nil {
		if lateErr := s.terminate(ctx, late); lateErr != nil && err == nil {
			err = lateErr
		}
	}

	s.mu.Lock()
	s.cmd = nil
	s.running = false
	// A degraded supervisor that is explicitly stopped becomes stopped:
	// the operator asked for it to be down, which is no longer a fault.
	s.state.Phase = PhaseStopped
	s.state.PID = 0
	s.removePIDFileLocked()
	s.mu.Unlock()

	return err
}

// terminate sends SIGTERM, waits up to StopGrace for the child to leave, then
// sends SIGKILL.
func (s *Supervisor) terminate(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Already gone is success: the goal is a dead child.
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("irc supervisor: signal ergo: %w", err)
	}

	// cmd.Wait belongs to the supervise loop when one is running, so the
	// liveness of the process is polled instead of waited on. Polling is
	// what keeps Stop correct whether or not a loop owns the child.
	deadline := time.Now().Add(StopGrace)
	for time.Now().Before(deadline) {
		if !alive(cmd.Process.Pid) {
			return nil
		}
		select {
		case <-ctx.Done():
			// Out of time: escalate rather than keep waiting.
			return s.kill(cmd)
		case <-time.After(20 * time.Millisecond):
		}
	}

	return s.kill(cmd)
}

// kill sends SIGKILL and reports success if the process is already gone.
func (s *Supervisor) kill(cmd *exec.Cmd) error {
	if err := cmd.Process.Kill(); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("irc supervisor: kill ergo: %w", err)
	}
	return nil
}

// killNow tears a child down during a failed start, where no supervise loop
// exists to reap it. It waits so the process is reaped rather than left a
// zombie.
func (s *Supervisor) killNow(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	s.mu.Lock()
	if s.cmd == cmd {
		s.cmd = nil
	}
	s.state.PID = 0
	s.mu.Unlock()

	s.removePIDFile()
}

// Status returns a snapshot of the supervised child.
func (s *Supervisor) Status() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// clock resolves the time source: the explicit Clock, else the one carried by
// the Backoff, else the system clock.
func (s *Supervisor) clock() irc.Clock {
	if s.Clock != nil {
		return s.Clock
	}
	if s.Backoff.Clock != nil {
		return s.Backoff.Clock
	}
	return irc.SystemClock
}

// backoff resolves the configured curve, falling back to the shared default.
func (s *Supervisor) backoff() irc.Backoff {
	b := s.Backoff
	if b.Min <= 0 && b.Max <= 0 && b.Attempts == 0 {
		return irc.NewBackoff()
	}
	return b
}

// writePIDFile records pid at PIDFile. An empty PIDFile is a no-op.
func (s *Supervisor) writePIDFile(pid int) error {
	if s.PIDFile == "" {
		return nil
	}
	if dir := filepath.Dir(s.PIDFile); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("irc supervisor: create pid dir %s: %w", dir, err)
		}
	}
	data := []byte(strconv.Itoa(pid) + "\n")
	if err := os.WriteFile(s.PIDFile, data, pidMode); err != nil {
		return fmt.Errorf("irc supervisor: write pidfile %s: %w", s.PIDFile, err)
	}
	return nil
}

// removePIDFile deletes the pidfile, ignoring a missing one.
func (s *Supervisor) removePIDFile() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removePIDFileLocked()
}

// removePIDFileLocked is removePIDFile for callers already holding mu.
func (s *Supervisor) removePIDFileLocked() {
	if s.PIDFile == "" {
		return
	}
	_ = os.Remove(s.PIDFile)
}

// ReadPID returns the pid recorded at path and whether that process is
// currently alive.
//
// A pidfile is only evidence that a process was started, never that one is
// still running: wsm crashes, machines reboot, and pids are reused. A stale
// file therefore reports running=false, and the caller is free to replace it.
// A missing file, an unparseable file and a dead pid are all "not running"
// rather than errors, because each is a normal thing to find on disk; only an
// unreadable file is an error worth surfacing.
func ReadPID(path string) (pid int, running bool, err error) {
	if path == "" {
		return 0, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("irc supervisor: read pidfile %s: %w", path, err)
	}

	pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if convErr != nil || pid <= 0 {
		// A corrupt pidfile is as good as no pidfile.
		return 0, false, nil
	}

	return pid, alive(pid), nil
}

// alive reports whether pid names a live process, with kill -0 semantics:
// signal 0 performs the permission and existence checks without delivering
// anything.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means the process exists but belongs to another user, which
	// still counts as running.
	return errors.Is(err, syscall.EPERM)
}

// exitReason renders a child's exit into a status-friendly string.
func exitReason(err error) string {
	if err == nil {
		return "ergo exited cleanly"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("ergo exited with status %d", exitErr.ExitCode())
	}
	return fmt.Sprintf("ergo exited: %v", err)
}
