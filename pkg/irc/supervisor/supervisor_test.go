package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc"
)

// fakeClock is a Clock that fires immediately and records every delay it was
// asked to wait for, so the backoff schedule can be asserted without any real
// sleeping.
type fakeClock struct {
	mu     sync.Mutex
	waited []time.Duration
}

// Now returns a fixed instant; nothing under test measures elapsed time.
func (f *fakeClock) Now() time.Time { return time.Unix(0, 0) }

// After records d and fires at once.
func (f *fakeClock) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	f.waited = append(f.waited, d)
	f.mu.Unlock()

	ch := make(chan time.Time, 1)
	ch <- time.Unix(0, 0)
	return ch
}

// delays returns a copy of the recorded schedule.
func (f *fakeClock) delays() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]time.Duration, len(f.waited))
	copy(out, f.waited)
	return out
}

// steppedClock advances a fixed amount on every reading, so elapsed time can
// be simulated without any real waiting. After fires immediately like
// fakeClock, keeping the backoff instant.
type steppedClock struct {
	mu     sync.Mutex
	now    time.Time
	step   time.Duration
	waited []time.Duration
}

// Now advances the clock by step and returns the new instant.
func (c *steppedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
}

// After records d and fires at once.
func (c *steppedClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.waited = append(c.waited, d)
	c.mu.Unlock()

	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

// fakeBinary writes an executable shell script into dir and returns its path.
// The script stands in for ergo so no real server is ever needed: the body
// decides whether this "ergo" sleeps, exits, or logs.
func fakeBinary(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}

// newSupervisor builds a Supervisor over a fake binary with the paths a real
// one would get from irc.Paths, and guarantees teardown so no test can leave a
// child behind.
func newSupervisor(t *testing.T, body string, b irc.Backoff) (*Supervisor, string) {
	t.Helper()

	dir := t.TempDir()
	paths, err := irc.ResolvePaths(irc.PathsInput{
		DataDir:  filepath.Join(dir, "data"),
		StateDir: filepath.Join(dir, "state"),
	}, "01HTESTWORKSPACEID")
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}

	s := &Supervisor{
		Binary:   fakeBinary(t, dir, "fake-ergo", body),
		ConfPath: paths.ErgoYAML,
		PIDFile:  paths.ErgoPID,
		LogPath:  paths.ErgoLog,
		Backoff:  b,
	}

	// Every test tears the supervisor down, so a failing assertion cannot
	// orphan the fake child.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})

	return s, dir
}

// testBackoff is a fast, fully injected curve: the fake clock makes every
// delay instant while still recording what was asked for.
func testBackoff(clock irc.Clock, attempts int) irc.Backoff {
	return irc.Backoff{
		Min:      time.Second,
		Max:      30 * time.Second,
		Attempts: attempts,
		Clock:    clock,
	}
}

func TestStart_RunsChildAndWritesPIDFile(t *testing.T) {
	clock := &fakeClock{}
	// A child that stays up for the life of the test.
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := s.Status()
	if got.Phase != PhaseRunning {
		t.Errorf("Phase = %q, want %q", got.Phase, PhaseRunning)
	}
	if got.PID <= 0 {
		t.Errorf("PID = %d, want a live pid", got.PID)
	}
	if got.Restarts != 0 {
		t.Errorf("Restarts = %d, want 0", got.Restarts)
	}
	if got.LastErr != "" {
		t.Errorf("LastErr = %q, want empty", got.LastErr)
	}

	// The pidfile must name the live child.
	pid, running, err := ReadPID(s.PIDFile)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if !running {
		t.Error("ReadPID reported not running for a live child")
	}
	if pid != got.PID {
		t.Errorf("pidfile pid = %d, want %d", pid, got.PID)
	}
}

func TestStart_PassesConfPathToBinary(t *testing.T) {
	clock := &fakeClock{}
	// Record the arguments this "ergo" was invoked with.
	s, dir := newSupervisor(t, `echo "$@" > "$(dirname "$0")/args.txt"; sleep 30`, testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The child writes the file as it starts; give it a moment.
	argsPath := filepath.Join(dir, "args.txt")
	waitForFile(t, argsPath)

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := "run --conf " + s.ConfPath
	if got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestStart_ReadyFailureIsAFailedStart(t *testing.T) {
	clock := &fakeClock{}
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))
	s.Ready = func(ctx context.Context) error {
		return fmt.Errorf("status endpoint never answered")
	}

	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("Start returned nil for a child that never became ready")
	}
	if !strings.Contains(err.Error(), "never answered") {
		t.Errorf("error = %v, want the readiness failure", err)
	}

	got := s.Status()
	if got.Phase != PhaseStopped {
		t.Errorf("Phase = %q, want %q", got.Phase, PhaseStopped)
	}

	// A failed start leaves no pidfile and no child behind.
	if _, err := os.Stat(s.PIDFile); !os.IsNotExist(err) {
		t.Errorf("pidfile survived a failed start: %v", err)
	}
}

func TestStart_ReadyReceivesContext(t *testing.T) {
	clock := &fakeClock{}
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))

	// Start must pass its caller's context through, so a caller's 10s cap
	// is the policy that applies rather than anything baked in here.
	var gotCtx context.Context
	s.Ready = func(ctx context.Context) error {
		gotCtx = ctx
		return nil
	}

	type ctxKey string
	key := ctxKey("probe")
	ctx := context.WithValue(context.Background(), key, "yes")

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if gotCtx == nil {
		t.Fatal("Ready was never called")
	}
	if gotCtx.Value(key) != "yes" {
		t.Error("Ready did not receive the caller's context")
	}
}

func TestRestart_BackoffScheduleAndDegraded(t *testing.T) {
	clock := &fakeClock{}
	// A child that exits immediately, every time: the restart budget can
	// only ever be exhausted.
	s, _ := newSupervisor(t, "exit 3", testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the supervisor to give up rather than polling a timer.
	waitForPhase(t, s, PhaseDegraded)

	got := s.Status()
	if got.Phase != PhaseDegraded {
		t.Fatalf("Phase = %q, want %q", got.Phase, PhaseDegraded)
	}
	if got.PID != 0 {
		t.Errorf("PID = %d, want 0 once degraded", got.PID)
	}
	if got.LastErr == "" {
		t.Error("LastErr is empty, want the reason the budget was spent")
	}

	// Five attempts were allowed, so five delays were waited out on the
	// documented curve before the supervisor degraded.
	want := []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}
	delays := clock.delays()
	if len(delays) != len(want) {
		t.Fatalf("waited %d times %v, want %d delays %v", len(delays), delays, len(want), want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Errorf("delay %d = %v, want %v", i+1, delays[i], want[i])
		}
	}

	// The daemon stays up: degrading is not exiting. The supervisor is
	// still answering, and its pidfile is gone because no child is left.
	if _, err := os.Stat(s.PIDFile); !os.IsNotExist(err) {
		t.Errorf("pidfile survived degradation: %v", err)
	}
}

func TestRestart_RecoveryResetsAttemptBudget(t *testing.T) {
	clock := &fakeClock{}
	dir := t.TempDir()

	// This child exits once, then stays up: the supervisor must restart it
	// and settle back into running rather than degrading.
	marker := filepath.Join(dir, "once")
	body := fmt.Sprintf(`if [ ! -f %q ]; then touch %q; exit 1; fi; sleep 30`, marker, marker)

	s := &Supervisor{
		Binary:   fakeBinary(t, dir, "fake-ergo", body),
		ConfPath: filepath.Join(dir, "ergo.yaml"),
		PIDFile:  filepath.Join(dir, "ergo.pid"),
		LogPath:  filepath.Join(dir, "ergo.log"),
		Backoff:  testBackoff(clock, 5),
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first := s.Status().PID

	// Waiting for a live pid that is not the original child's: the restart
	// counter increments when the old child dies, which is before the
	// replacement exists, and the original is already Running with a pid.
	waitForReplacementPID(t, s, first)

	got := s.Status()
	if got.Phase != PhaseRunning {
		t.Errorf("Phase = %q, want %q after a successful restart", got.Phase, PhaseRunning)
	}
	if got.Restarts != 1 {
		t.Errorf("Restarts = %d, want 1", got.Restarts)
	}
	if got.PID <= 0 {
		t.Errorf("PID = %d, want the restarted child's pid", got.PID)
	}

	// Only the first delay was needed.
	if delays := clock.delays(); len(delays) != 1 || delays[0] != time.Second {
		t.Errorf("delays = %v, want exactly [1s]", delays)
	}
}

func TestRestart_CrashLoopDegradesRatherThanSpinning(t *testing.T) {
	clock := &fakeClock{}
	// A binary that forks successfully and dies immediately, every single
	// time. Spawning it "works", so a supervisor that counted a successful
	// fork as a recovery would reset its budget every cycle and respawn
	// ergo forever. The budget must keep draining until it degrades.
	s, _ := newSupervisor(t, "exit 0", testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitForPhase(t, s, PhaseDegraded)

	got := s.Status()
	if got.Phase != PhaseDegraded {
		t.Fatalf("Phase = %q, want %q", got.Phase, PhaseDegraded)
	}
	// Five attempts, so the child was started at most six times total: the
	// original plus five restarts. A spinning supervisor would show orders
	// of magnitude more.
	if got.Restarts > 6 {
		t.Errorf("Restarts = %d, want at most 6; the supervisor is respawning in a loop", got.Restarts)
	}
	if len(clock.delays()) != 5 {
		t.Errorf("waited %d times, want exactly 5 backoff delays", len(clock.delays()))
	}
}

func TestRestart_LongLivedChildRestoresBudget(t *testing.T) {
	// A clock the test advances by hand, so "the child stayed up a long
	// time" can be expressed without waiting for it.
	clock := &steppedClock{now: time.Unix(0, 0), step: time.Minute}
	dir := t.TempDir()

	// Exits once, then stays up. With the stepped clock each child appears
	// to live a minute, comfortably past StableAfter, so the restart counts
	// as a real recovery.
	marker := filepath.Join(dir, "once")
	body := fmt.Sprintf(`if [ ! -f %q ]; then touch %q; exit 1; fi; sleep 30`, marker, marker)

	s := &Supervisor{
		Binary:      fakeBinary(t, dir, "fake-ergo", body),
		ConfPath:    filepath.Join(dir, "ergo.yaml"),
		PIDFile:     filepath.Join(dir, "ergo.pid"),
		LogPath:     filepath.Join(dir, "ergo.log"),
		Backoff:     testBackoff(clock, 5),
		Clock:       clock,
		StableAfter: 10 * time.Second,
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first := s.Status().PID

	// The replacement, not the original: the initial child is already
	// Running with a pid, so waiting on the phase alone would pass before
	// any restart had happened.
	waitForReplacementPID(t, s, first)

	got := s.Status()
	if got.Phase != PhaseRunning {
		t.Errorf("Phase = %q, want %q", got.Phase, PhaseRunning)
	}
	if got.Restarts != 1 {
		t.Errorf("Restarts = %d, want 1", got.Restarts)
	}
}

func TestRestart_LogIsAppendedNotTruncated(t *testing.T) {
	clock := &fakeClock{}
	dir := t.TempDir()

	// Each run writes a distinct line and exits, so the log can only hold
	// both lines if it was appended to across the restart.
	marker := filepath.Join(dir, "once")
	body := fmt.Sprintf(
		`if [ ! -f %q ]; then touch %q; echo FIRST-RUN; exit 1; fi; echo SECOND-RUN; sleep 30`,
		marker, marker,
	)

	logPath := filepath.Join(dir, "ergo.log")
	// Pre-existing content from an earlier daemon must survive too.
	if err := os.WriteFile(logPath, []byte("EARLIER-RUN\n"), 0o600); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	s := &Supervisor{
		Binary:   fakeBinary(t, dir, "fake-ergo", body),
		ConfPath: filepath.Join(dir, "ergo.yaml"),
		PIDFile:  filepath.Join(dir, "ergo.pid"),
		LogPath:  logPath,
		Backoff:  testBackoff(clock, 5),
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitForRestarts(t, s, 1)
	waitForLogContains(t, logPath, "SECOND-RUN")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)

	for _, want := range []string{"EARLIER-RUN", "FIRST-RUN", "SECOND-RUN"} {
		if !strings.Contains(log, want) {
			t.Errorf("log is missing %q; a restart truncated it instead of appending\nlog:\n%s", want, log)
		}
	}

	// Order matters: appending preserves the sequence.
	if idx1, idx2 := strings.Index(log, "FIRST-RUN"), strings.Index(log, "SECOND-RUN"); idx1 > idx2 {
		t.Errorf("log is out of order; FIRST-RUN must precede SECOND-RUN\nlog:\n%s", log)
	}
}

func TestStop_TerminatesChildAndRemovesPIDFile(t *testing.T) {
	clock := &fakeClock{}
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := s.Status().PID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got := s.Status()
	if got.Phase != PhaseStopped {
		t.Errorf("Phase = %q, want %q", got.Phase, PhaseStopped)
	}
	if got.PID != 0 {
		t.Errorf("PID = %d, want 0 after Stop", got.PID)
	}

	// The child is really gone, not merely forgotten.
	waitForDead(t, pid)

	if _, err := os.Stat(s.PIDFile); !os.IsNotExist(err) {
		t.Errorf("pidfile survived Stop: %v", err)
	}
}

func TestStop_EscalatesToKillWhenSIGTERMIgnored(t *testing.T) {
	clock := &fakeClock{}
	// A child that traps SIGTERM and refuses to leave: only SIGKILL ends
	// it, which is exactly the escalation Stop promises.
	s, _ := newSupervisor(t, `trap "" TERM; while true; do sleep 0.05; done`, testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := s.Status().PID

	// A context shorter than StopGrace makes Stop escalate promptly, so
	// the test does not sit through the full five-second grace period.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	waitForDead(t, pid)

	if got := s.Status().Phase; got != PhaseStopped {
		t.Errorf("Phase = %q, want %q", got, PhaseStopped)
	}
}

func TestStop_NoChildIsNotAnError(t *testing.T) {
	clock := &fakeClock{}
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))

	// Never started.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Errorf("Stop on an unstarted supervisor: %v", err)
	}
	if got := s.Status().Phase; got != PhaseStopped {
		t.Errorf("Phase = %q, want %q", got, PhaseStopped)
	}
}

func TestStop_IsIdempotent(t *testing.T) {
	clock := &fakeClock{}
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Stop(ctx); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := s.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestStop_DuringBackoffDoesNotRestart(t *testing.T) {
	clock := &fakeClock{}
	// A child that always dies: without Stop this would restart until it
	// degraded.
	s, _ := newSupervisor(t, "exit 1", testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Stop as soon as the first unexpected exit has been noticed.
	waitForRestarts(t, s, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got := s.Status()
	if got.Phase != PhaseStopped {
		t.Errorf("Phase = %q, want %q; Stop must win against a pending restart", got.Phase, PhaseStopped)
	}

	// The restart count must not keep climbing after Stop returned.
	settled := s.Status().Restarts
	time.Sleep(100 * time.Millisecond)
	if after := s.Status().Restarts; after != settled {
		t.Errorf("Restarts kept climbing after Stop: %d then %d", settled, after)
	}
}

func TestStop_DuringRespawnLeavesNoOrphan(t *testing.T) {
	// Stop landing while a restart is in flight must still end with every
	// child dead. A slow Ready probe holds the respawn open so Stop is
	// guaranteed to arrive mid-restart rather than by luck.
	//
	// Note on coverage: this asserts the observable contract (nothing
	// survives Stop), not the specific mechanism. Stop waits for the
	// supervise loop to drain, and that loop reaps its own in-flight child,
	// so the explicit stopping-channel guards in spawnAndWaitReady are
	// defence in depth here rather than the only thing preventing an
	// orphan — deleting them does not turn this test red.
	clock := &fakeClock{}
	dir := t.TempDir()

	// Children record their own pid, so the test can check the whole
	// lineage rather than only the one the Supervisor last reported.
	pidDir := filepath.Join(dir, "pids")
	if err := os.MkdirAll(pidDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := fmt.Sprintf(`echo $$ >> %q; sleep 30`, filepath.Join(pidDir, "all"))

	respawning := make(chan struct{}, 16)
	s := &Supervisor{
		Binary:   fakeBinary(t, dir, "fake-ergo", body),
		ConfPath: filepath.Join(dir, "ergo.yaml"),
		PIDFile:  filepath.Join(dir, "ergo.pid"),
		LogPath:  filepath.Join(dir, "ergo.log"),
		Backoff:  testBackoff(clock, 5),
	}

	var probes int
	s.Ready = func(ctx context.Context) error {
		probes++
		// The first probe (from Start) returns at once; later ones are
		// respawns, and they dawdle so Stop can land mid-flight.
		if probes > 1 {
			select {
			case respawning <- struct{}{}:
			default:
			}
			time.Sleep(150 * time.Millisecond)
		}
		return nil
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first := s.Status().PID

	// Start returns as soon as the fork succeeds, which can be before the
	// child has run its first line. Wait for the lineage file, otherwise
	// killing the child immediately can leave it never recorded and the
	// assertion below with nothing to check.
	lineage := filepath.Join(pidDir, "all")
	waitForFile(t, lineage)

	// Kill the child out from under the supervisor so it starts a restart
	// that the slow probe holds open.
	proc, err := os.FindProcess(first)
	if err != nil {
		t.Fatalf("find first child: %v", err)
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("kill first child: %v", err)
	}

	// Stop exactly while a respawn is inside its readiness probe.
	select {
	case <-respawning:
	case <-time.After(10 * time.Second):
		t.Fatal("no respawn was observed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if got := s.Status().Phase; got != PhaseStopped {
		t.Errorf("Phase = %q, want %q", got, PhaseStopped)
	}

	// Every child this test ever spawned must now be dead. A surviving one
	// is the orphan the guards exist to prevent.
	//
	// Give any respawn that was in flight during Stop a moment to record
	// itself, so an orphan cannot escape the check by being written late.
	time.Sleep(300 * time.Millisecond)

	data, err := os.ReadFile(lineage)
	if err != nil {
		t.Fatalf("read child pids: %v", err)
	}
	if len(strings.Fields(string(data))) == 0 {
		t.Fatal("no child pids were recorded; the test proves nothing")
	}
	for _, line := range strings.Fields(string(data)) {
		pid, convErr := strconv.Atoi(line)
		if convErr != nil {
			continue
		}
		waitForDead(t, pid)
	}
}

func TestStart_TwiceIsRejected(t *testing.T) {
	clock := &fakeClock{}
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// A second Start would orphan the first child.
	if err := s.Start(context.Background()); err == nil {
		t.Error("second Start returned nil, want an error")
	}
}

func TestStart_MissingBinaryFails(t *testing.T) {
	dir := t.TempDir()
	s := &Supervisor{
		Binary:   filepath.Join(dir, "does-not-exist"),
		ConfPath: filepath.Join(dir, "ergo.yaml"),
		PIDFile:  filepath.Join(dir, "ergo.pid"),
		LogPath:  filepath.Join(dir, "ergo.log"),
		Backoff:  testBackoff(&fakeClock{}, 5),
	}

	if err := s.Start(context.Background()); err == nil {
		t.Fatal("Start returned nil for a missing binary")
	}
	if got := s.Status().Phase; got != PhaseStopped {
		t.Errorf("Phase = %q, want %q", got, PhaseStopped)
	}
	if _, err := os.Stat(s.PIDFile); !os.IsNotExist(err) {
		t.Errorf("pidfile written despite a failed spawn: %v", err)
	}
}

func TestStart_RequiresBinaryAndConf(t *testing.T) {
	// Built per case rather than stored in the table: a Supervisor carries
	// a mutex and must never be copied.
	tests := []struct {
		name  string
		build func() *Supervisor
	}{
		{name: "no binary", build: func() *Supervisor { return &Supervisor{ConfPath: "ergo.yaml"} }},
		{name: "no conf", build: func() *Supervisor { return &Supervisor{Binary: "ergo"} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.build()
			if err := s.Start(context.Background()); err == nil {
				t.Error("Start returned nil, want a validation error")
			}
		})
	}
}

func TestReadPID_StalePIDFileIsNotRunning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ergo.pid")

	// Spawn a real process and reap it, so its pid is genuinely dead and
	// the file left behind is genuinely stale. Exec'd directly rather than
	// through a shell: nothing here needs one.
	cmd := exec.Command(fakeBinary(t, dir, "exit-now", "exit 0"))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start throwaway process: %v", err)
	}
	dead := cmd.Process.Pid
	_ = cmd.Wait()
	waitForDead(t, dead)

	if err := os.WriteFile(path, []byte(strconv.Itoa(dead)+"\n"), 0o600); err != nil {
		t.Fatalf("write stale pidfile: %v", err)
	}

	pid, running, err := ReadPID(path)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if pid != dead {
		t.Errorf("pid = %d, want %d", pid, dead)
	}
	if running {
		t.Error("a stale pidfile was mistaken for a live child")
	}
}

func TestReadPID_LivePIDFileIsRunning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ergo.pid")

	// The test process itself is indisputably alive.
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}

	pid, running, err := ReadPID(path)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", pid, os.Getpid())
	}
	if !running {
		t.Error("a live pid was reported as not running")
	}
}

func TestReadPID_EdgeCases(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		write       bool
		wantPID     int
		wantRunning bool
	}{
		{name: "missing file", write: false},
		{name: "empty file", content: "", write: true},
		{name: "garbage", content: "not-a-pid\n", write: true},
		{name: "zero", content: "0\n", write: true},
		{name: "negative", content: "-1\n", write: true},
		{name: "trailing whitespace tolerated", content: "  " + strconv.Itoa(os.Getpid()) + "  \n", write: true, wantPID: os.Getpid(), wantRunning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-")+".pid")
			if tt.write {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
			}

			pid, running, err := ReadPID(path)
			if err != nil {
				t.Fatalf("ReadPID: %v", err)
			}
			if pid != tt.wantPID {
				t.Errorf("pid = %d, want %d", pid, tt.wantPID)
			}
			if running != tt.wantRunning {
				t.Errorf("running = %v, want %v", running, tt.wantRunning)
			}
		})
	}
}

func TestReadPID_EmptyPathIsNoOp(t *testing.T) {
	pid, running, err := ReadPID("")
	if err != nil || pid != 0 || running {
		t.Errorf("ReadPID(\"\") = (%d, %v, %v), want (0, false, nil)", pid, running, err)
	}
}

func TestStart_StalePIDFileIsReplaced(t *testing.T) {
	clock := &fakeClock{}
	s, _ := newSupervisor(t, "sleep 30", testBackoff(clock, 5))

	// A pidfile left by a crashed daemon, naming a pid that is long gone.
	stale := 999999
	if err := os.WriteFile(s.PIDFile, []byte(strconv.Itoa(stale)+"\n"), 0o600); err != nil {
		t.Fatalf("write stale pidfile: %v", err)
	}
	if _, running, _ := ReadPID(s.PIDFile); running {
		t.Skip("pid 999999 happens to be live on this machine")
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	pid, running, err := ReadPID(s.PIDFile)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if !running {
		t.Error("pidfile does not name a live child after Start")
	}
	if pid == stale {
		t.Error("the stale pid was left in place instead of being replaced")
	}
	if pid != s.Status().PID {
		t.Errorf("pidfile pid = %d, want the live child %d", pid, s.Status().PID)
	}
}

func TestStatus_ZeroValueIsStopped(t *testing.T) {
	var s Supervisor
	got := s.Status()
	if got.Phase != "" && got.Phase != PhaseStopped {
		t.Errorf("Phase = %q, want the zero value to read as stopped", got.Phase)
	}
	if got.PID != 0 || got.Restarts != 0 {
		t.Errorf("Status() = %+v, want a zeroed snapshot", got)
	}
}

// waitForPhase blocks until the supervisor reaches phase or the test times
// out, so no assertion races the supervise goroutine.
func waitForPhase(t *testing.T, s *Supervisor, phase Phase) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.Status().Phase == phase {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for phase %q; status = %+v", phase, s.Status())
}

// waitForRestarts blocks until at least n restarts have been recorded.
func waitForRestarts(t *testing.T, s *Supervisor, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.Status().Restarts >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d restarts; status = %+v", n, s.Status())
}

// waitForReplacementPID blocks until the supervisor reports a running child
// whose pid differs from prev, i.e. the restart actually produced a new
// process. Waiting on the restart counter alone is not enough: it increments
// when the old child dies, before the replacement has been spawned.
func waitForReplacementPID(t *testing.T, s *Supervisor, prev int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got := s.Status(); got.Phase == PhaseRunning && got.PID > 0 && got.PID != prev {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for a replacement child; status = %+v", s.Status())
}

// waitForFile blocks until path exists.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// waitForLogContains blocks until the file at path contains want.
func waitForLogContains(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %s", want, path)
}

// waitForDead blocks until pid is no longer a live process, proving a test
// left no child behind.
func waitForDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		proc, err := os.FindProcess(pid)
		if err != nil {
			return
		}
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("process %d is still alive; a test leaked a child", pid)
}
