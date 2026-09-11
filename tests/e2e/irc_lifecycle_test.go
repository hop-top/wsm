package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The IRC lifecycle verbs are exercised against a stub daemon rather
// than the real one. The genuine daemon wants a live ergo, a bot
// session and a workspace's member list; none of that is what these
// tests are about. What the verbs actually contract on is narrower —
// a detached child that writes status.json and stays alive until it is
// signalled — and the stub provides exactly that, which is why the
// exit codes and JSON shapes below mean what they say.

// fakeDaemonSource is the stub `wsm irc daemon <ws>` spawned through
// WSM_IRC_DAEMON_EXE.
//
// It mirrors the real daemon's observable contract: resolve the
// workspace's state dir, write a status.json naming its own pid, then
// block until SIGTERM. The status document is the real
// daemon.Status shape, because the CLI decodes it with the same reader.
const fakeDaemonSource = `package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	// argv: irc daemon <workspace-id>
	if len(os.Args) < 4 {
		os.Exit(2)
	}
	wsID := os.Args[3]

	stateDir := os.Getenv("FAKE_DAEMON_STATE_DIR")
	if stateDir == "" {
		os.Exit(2)
	}
	dir := filepath.Join(stateDir, "irc", wsID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		os.Exit(1)
	}

	status := map[string]any{
		"mode":     os.Getenv("FAKE_DAEMON_MODE"),
		"provider": "ergo",
		"state":    "running",
		"pid":      os.Getpid(),
		"since":    time.Now().UTC().Format(time.RFC3339Nano),
		"server": map[string]any{
			"id":   "wsm:///irc",
			"host": "127.0.0.1",
			"port": 6700,
			"tls":  false,
		},
		"caps": map[string]any{
			"negotiated": []string{"sasl", "message-tags"},
			"provider":   []string{"create_account"},
		},
	}
	raw, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "status.json"), raw, 0o600); err != nil {
		os.Exit(1)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	for {
		sig := <-ch
		if sig == syscall.SIGHUP {
			// Record the reconcile request so a test can prove the
			// signal was delivered rather than merely sent.
			_ = os.WriteFile(filepath.Join(dir, "sighup"), []byte("1"), 0o600)
			continue
		}
		_ = os.Remove(filepath.Join(dir, "status.json"))
		return
	}
}
`

var (
	fakeDaemonOnce sync.Once
	fakeDaemonPath string
	fakeDaemonErr  error
)

// buildFakeDaemon compiles the stub daemon once per test binary.
func buildFakeDaemon(t *testing.T) string {
	t.Helper()
	fakeDaemonOnce.Do(func() {
		root := findRepoRoot(t)
		dir := filepath.Join(root, ".tmp", "e2e", "fakedaemon")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fakeDaemonErr = err
			return
		}
		src := filepath.Join(dir, "main.go")
		if err := os.WriteFile(src, []byte(fakeDaemonSource), 0o644); err != nil {
			fakeDaemonErr = err
			return
		}
		// A standalone main package needs its own module file, or the
		// build is resolved against the repo's module and fails.
		mod := filepath.Join(dir, "go.mod")
		if err := os.WriteFile(mod, []byte("module fakedaemon\n\ngo 1.24\n"), 0o644); err != nil {
			fakeDaemonErr = err
			return
		}
		fakeDaemonPath = filepath.Join(dir, binaryName("fake-irc-daemon"))
		fakeDaemonErr = goBuildIn(dir, fakeDaemonPath, ".")
	})
	if fakeDaemonErr != nil {
		t.Fatalf("build fake daemon: %v", fakeDaemonErr)
	}
	return fakeDaemonPath
}

// ircEnv is the environment that points the CLI at the stub daemon and
// turns the feature on.
//
// WSM_STATE_DIR is pinned rather than left to XDG resolution: the
// volatile half of a workspace's IRC state (status.json, pidfiles) has
// to be the same directory for the CLI and the spawned daemon, and a
// test that let each side resolve it independently would be asserting
// against whichever one happened to win.
func ircEnv(t *testing.T, r *e2eRunner, mode string) map[string]string {
	t.Helper()
	return map[string]string{
		"WSM_IRC_ENABLED":       "true",
		"WSM_IRC_MODE":          mode,
		"WSM_STATE_DIR":         r.ircStateDir(t),
		"WSM_IRC_DAEMON_EXE":    buildFakeDaemon(t),
		"FAKE_DAEMON_STATE_DIR": r.ircStateDir(t),
		"FAKE_DAEMON_MODE":      mode,
	}
}

// ircStateDir is the volatile state root these tests pin, created on
// first use and stable for the runner's lifetime.
func (r *e2eRunner) ircStateDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(r.dataDir), "state")
	mustMkdirAll(t, dir)
	return dir
}

// goBuildIn compiles pkg from dir into output.
//
// The harness's goBuild always builds ./cmd/wsm from the repo root;
// the stub daemon is its own module in its own directory, so it needs
// a build that takes both.
func goBuildIn(dir, output, pkg string) error {
	root, err := filepath.Abs(filepath.Join(dir, "..", "..", ".."))
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", output, pkg)
	cmd.Dir = dir
	cmd.Env = upsertEnv(os.Environ(), "GOCACHE", filepath.Join(root, ".tmp", "gocache"))
	out, berr := cmd.CombinedOutput()
	if berr != nil {
		return fmt.Errorf("go build %s in %s: %w\n%s", pkg, dir, berr, string(out))
	}
	return nil
}

// mustWorkspaceRoot creates a directory carrying the .wsm/ marker that
// enable requires, and returns it.
func mustWorkspaceRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, ".wsm"))
	return root
}

// TestIRC_DisabledGate locks the feature gate: with irc.enabled unset
// every verb exits 2 and says how to turn it on. This is the gate that
// keeps a half-configured wsm from spawning daemons.
func TestIRC_DisabledGate(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("irc-gate")

	for _, verb := range []string{"enable", "disable", "start", "stop", "restart", "status", "sync"} {
		t.Run(verb, func(t *testing.T) {
			res := r.runWithEnv(map[string]string{"WSM_IRC_ENABLED": "false"}, "irc", verb, "irc-gate")
			requireExitCode(t, res, 2)
			if !strings.Contains(res.Combined(), "set irc.enabled: true") {
				t.Fatalf("expected the remedy in the output, got:\n%s", res.Combined())
			}
		})
	}
}

// TestIRC_UnknownWorkspace locks exit 3: the workspace is resolved
// through the same path every other verb uses.
func TestIRC_UnknownWorkspace(t *testing.T) {
	r := newRunner(t)
	env := ircEnv(t, r, "managed")

	for _, verb := range []string{"status", "start", "stop", "sync", "disable"} {
		t.Run(verb, func(t *testing.T) {
			res := r.runWithEnv(env, "irc", verb, "no-such-workspace")
			requireExitCode(t, res, 3)
		})
	}
}

// TestIRC_CapabilityDenied locks exit 6. The capability gate is
// library-level policy with no flag behind it, so the seam is an
// environment variable — the same shape the policy engine uses.
func TestIRC_CapabilityDenied(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("irc-denied")
	root := mustWorkspaceRoot(t)

	t.Run("deny all", func(t *testing.T) {
		env := ircEnv(t, r, "managed")
		env["WSM_DENY_CAPABILITIES"] = "all"
		res := r.runWithEnv(env, "irc", "enable", "irc-denied", "--dir", root)
		requireExitCode(t, res, 6)
	})

	t.Run("deny only irc.enable", func(t *testing.T) {
		env := ircEnv(t, r, "managed")
		env["WSM_DENY_CAPABILITIES"] = "irc.enable"
		res := r.runWithEnv(env, "irc", "enable", "irc-denied", "--dir", root)
		requireExitCode(t, res, 6)
	})
}

// TestIRC_EnableRejectsNonWorkspaceRoot locks exit 2 for a --dir that
// is not a workspace root. The daemon reads members.yaml from the
// recorded root long after this command exits, so a root that resolves
// to nothing has to fail here.
func TestIRC_EnableRejectsNonWorkspaceRoot(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("irc-badroot")

	res := r.runWithEnv(ircEnv(t, r, "managed"), "irc", "enable", "irc-badroot", "--dir", t.TempDir())
	requireExitCode(t, res, 2)
	if !strings.Contains(res.Combined(), ".wsm") {
		t.Fatalf("expected the message to name the expected marker, got:\n%s", res.Combined())
	}
}

// TestIRC_SharedWithRefusals locks the exit codes of every attach that
// is refused before a server is ever contacted.
//
// The successful attach is deliberately not here: it registers an
// account over the host's live ergo HTTP API, which the stub daemon
// does not serve. Faking it would assert against the fake rather than
// against ergo, so the happy path belongs to the integration suite that
// runs a real server.
func TestIRC_SharedWithRefusals(t *testing.T) {
	t.Run("unknown host workspace exits 3", func(t *testing.T) {
		r := newRunner(t)
		r.mustCreateWorkspace("irc-guest")

		res := r.runWithEnv(ircEnv(t, r, "managed"), "irc", "enable", "irc-guest",
			"--shared-with", "no-such-workspace", "--dir", mustWorkspaceRoot(t))
		requireExitCode(t, res, 3)
	})

	t.Run("host without irc enabled exits 7", func(t *testing.T) {
		r := newRunner(t)
		r.mustCreateWorkspace("irc-guest")
		r.mustCreateWorkspace("irc-host")

		res := r.runWithEnv(ircEnv(t, r, "managed"), "irc", "enable", "irc-guest",
			"--shared-with", "irc-host", "--dir", mustWorkspaceRoot(t))
		requireExitCode(t, res, 7)
		if !strings.Contains(res.Combined(), "irc enable") {
			t.Fatalf("expected the remedy to name enable, got:\n%s", res.Combined())
		}
	})

	t.Run("a workspace cannot host itself, exit 2", func(t *testing.T) {
		r := newRunner(t)
		r.mustCreateWorkspace("irc-self")

		res := r.runWithEnv(ircEnv(t, r, "managed"), "irc", "enable", "irc-self",
			"--shared-with", "irc-self", "--dir", mustWorkspaceRoot(t))
		requireExitCode(t, res, 2)
	})

	t.Run("an already-enabled guest exits 4", func(t *testing.T) {
		r := newRunner(t)
		r.mustCreateWorkspace("irc-enabled")
		r.mustCreateWorkspace("irc-host")
		writeExternalProfile(t, r, "irc-enabled", mustWorkspaceRoot(t))

		res := r.runWithEnv(ircEnv(t, r, "external"), "irc", "enable", "irc-enabled",
			"--shared-with", "irc-host", "--dir", mustWorkspaceRoot(t))
		requireExitCode(t, res, 4)
		if !strings.Contains(res.Combined(), "already enabled") {
			t.Fatalf("expected the refusal to say the workspace is already "+
				"enabled, got:\n%s", res.Combined())
		}
	})
}

// TestIRC_GuestDetachOnDisable locks the guest half of detach through
// the CLI: a workspace whose host's state is gone (purged out from
// under it) must still be able to disable, and must come back with its
// host cleared rather than stuck believing it is attached.
func TestIRC_GuestDetachOnDisable(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("irc-orphan")
	env := ircEnv(t, r, "external")

	// A guest pointing at a host whose IRC state never existed: the
	// same shape a purge leaves behind.
	writeProfile(t, r, id, map[string]any{
		"mode": "external", "provider": "ergo",
		"server_id": "wsm:///irc", "root": mustWorkspaceRoot(t),
		"bot_account":    "wsm-irc-orphan",
		"host_workspace": "01JQZZZZZZZZZZZZZZZZZZZZZZ",
		"created_at":     time.Now().UTC().Format(time.RFC3339Nano),
	})

	res := r.runWithEnv(env, "irc", "disable", "irc-orphan", "--format", "json")
	requireExitCode(t, res, 0)

	raw, err := os.ReadFile(filepath.Join(r.dataDir, "irc", id, "profile.yaml"))
	if err != nil {
		t.Fatalf("read guest profile: %v", err)
	}
	if strings.Contains(string(raw), "host_workspace") {
		t.Fatalf("disabling a guest whose host is gone must clear its host:\n%s", raw)
	}
}

// TestIRC_SyncWithoutDaemon locks exit 1: sync is a signal to a running
// process, so a stopped workspace is a failure rather than a no-op —
// nothing would have reconciled.
func TestIRC_SyncWithoutDaemon(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("irc-nosync")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "irc-nosync", mustWorkspaceRoot(t))

	res := r.runWithEnv(env, "irc", "sync", "irc-nosync")
	requireExitCode(t, res, 1)
	if !strings.Contains(strings.ToLower(res.Combined()), "not running") {
		t.Fatalf("expected a 'not running' diagnosis, got:\n%s", res.Combined())
	}
}

// TestIRC_StatusBeforeEnable locks exit 7: status on a workspace that
// was never enabled is a prerequisite failure naming the fix, not a
// crash and not a fabricated "stopped".
func TestIRC_StatusBeforeEnable(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("irc-unenabled")

	res := r.runWithEnv(ircEnv(t, r, "managed"), "irc", "status", "irc-unenabled")
	requireExitCode(t, res, 7)
	if !strings.Contains(res.Combined(), "irc enable") {
		t.Fatalf("expected the remedy to name enable, got:\n%s", res.Combined())
	}
}

// TestIRC_ExternalLifecycle drives the full external-mode lifecycle
// against the stub daemon and asserts the --json shapes at each step.
func TestIRC_ExternalLifecycle(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("irc-ext")
	env := ircEnv(t, r, "external")
	root := mustWorkspaceRoot(t)
	writeExternalProfile(t, r, "irc-ext", root)

	// start: spawns the stub and waits for status.json.
	start := r.runWithEnv(env, "irc", "start", "irc-ext", "--format", "json")
	requireExitCode(t, start, 0)

	var started struct {
		Workspace string `json:"workspace"`
		ID        string `json:"id"`
		Mode      string `json:"mode"`
		State     string `json:"state"`
		PID       int    `json:"pid"`
	}
	mustUnmarshalJSON(t, start.Stdout, &started)
	if started.ID != id {
		t.Fatalf("start reported id %q; want %q", started.ID, id)
	}
	if started.Mode != "external" {
		t.Fatalf("start reported mode %q; want external", started.Mode)
	}
	if started.State != "running" {
		t.Fatalf("start reported state %q; want running", started.State)
	}
	if started.PID <= 0 {
		t.Fatalf("start reported pid %d; want a live pid", started.PID)
	}
	t.Cleanup(func() { _ = r.runWithEnv(env, "irc", "stop", "irc-ext") })

	// status: the daemon's document plus what only the CLI knows.
	status := r.runWithEnv(env, "irc", "status", "irc-ext", "--format", "json")
	requireExitCode(t, status, 0)

	var doc struct {
		Workspace string `json:"workspace"`
		ID        string `json:"id"`
		Mode      string `json:"mode"`
		State     string `json:"state"`
		PID       int    `json:"pid"`
		Server    struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"server"`
		Caps struct {
			Negotiated []string `json:"negotiated"`
		} `json:"caps"`
	}
	mustUnmarshalJSON(t, status.Stdout, &doc)
	if doc.Workspace != "irc-ext" {
		t.Fatalf("status workspace = %q; want irc-ext", doc.Workspace)
	}
	if doc.State != "running" {
		t.Fatalf("status state = %q; want running", doc.State)
	}
	if doc.PID != started.PID {
		t.Fatalf("status pid = %d; want the started daemon's %d", doc.PID, started.PID)
	}
	if doc.Server.Host == "" {
		t.Fatal("status must carry the server the bot connects to")
	}
	if len(doc.Caps.Negotiated) == 0 {
		t.Fatal("status must carry the negotiated capabilities from the daemon")
	}

	// start again: idempotent, and must not spawn a second daemon.
	again := r.runWithEnv(env, "irc", "start", "irc-ext", "--format", "json")
	requireExitCode(t, again, 0)
	var restarted struct {
		PID int `json:"pid"`
	}
	mustUnmarshalJSON(t, again.Stdout, &restarted)
	if restarted.PID != started.PID {
		t.Fatalf("a second start produced pid %d; want the running daemon's %d "+
			"(a second daemon would have been refused by the lock)",
			restarted.PID, started.PID)
	}

	// sync: signals the running daemon, which records the delivery.
	sync := r.runWithEnv(env, "irc", "sync", "irc-ext", "--format", "json")
	requireExitCode(t, sync, 0)
	sighup := filepath.Join(r.ircStateDir(t), "irc", id, "sighup")
	waitForFile(t, sighup)

	// stop: terminates it and reports that it did.
	stop := r.runWithEnv(env, "irc", "stop", "irc-ext", "--format", "json")
	requireExitCode(t, stop, 0)
	var stopped struct {
		Stopped bool `json:"stopped"`
	}
	mustUnmarshalJSON(t, stop.Stdout, &stopped)
	if !stopped.Stopped {
		t.Fatal("stop must report that it stopped a running daemon")
	}

	// status after stop: "not running" is an answer, not a failure.
	after := r.runWithEnv(env, "irc", "status", "irc-ext", "--format", "json")
	requireExitCode(t, after, 0)
	var afterDoc struct {
		State string `json:"state"`
	}
	mustUnmarshalJSON(t, after.Stdout, &afterDoc)
	if afterDoc.State != "stopped" {
		t.Fatalf("status after stop = %q; want stopped", afterDoc.State)
	}

	// stop again: idempotent.
	stopAgain := r.runWithEnv(env, "irc", "stop", "irc-ext", "--format", "json")
	requireExitCode(t, stopAgain, 0)
	var stoppedAgain struct {
		Stopped bool `json:"stopped"`
	}
	mustUnmarshalJSON(t, stopAgain.Stdout, &stoppedAgain)
	if stoppedAgain.Stopped {
		t.Fatal("stopping an already-stopped workspace must report no daemon stopped")
	}
}

// TestIRC_StaleStatusReadsStopped locks the stale-state rule: a
// status.json naming a pid that is gone reports stopped. A pidfile is
// evidence a daemon was started, never that one is still running.
func TestIRC_StaleStatusReadsStopped(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("irc-stale")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "irc-stale", mustWorkspaceRoot(t))

	dir := filepath.Join(r.ircStateDir(t), "irc", id)
	mustMkdirAll(t, dir)
	// A pid above the kernel's maximum can never name a live process,
	// which is the honest stale case: pid 1 is alive but is not ours.
	stale := map[string]any{
		"mode": "external", "provider": "ergo", "state": "running",
		"pid": 2147483000, "since": time.Now().UTC().Format(time.RFC3339Nano),
		"server": map[string]any{"id": "x", "host": "h", "port": 6697, "tls": true},
		"caps":   map[string]any{},
	}
	raw, err := json.MarshalIndent(stale, "", "  ")
	if err != nil {
		t.Fatalf("marshal stale status: %v", err)
	}
	writeFile(t, filepath.Join(dir, "status.json"), raw)

	res := r.runWithEnv(env, "irc", "status", "irc-stale", "--format", "json")
	requireExitCode(t, res, 0)
	var doc struct {
		State string `json:"state"`
		PID   int    `json:"pid"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)
	if doc.State != "stopped" {
		t.Fatalf("a stale status must read as stopped, got %q", doc.State)
	}
	if doc.PID != 0 {
		t.Fatalf("a stale status must not report a pid, got %d", doc.PID)
	}
}

// TestIRC_DisableBareStopIsUngated locks the split that the verb-level
// tag cannot express: stopping a daemon is reversible and must not
// demand a confirmation, even though the same verb's --purge does.
func TestIRC_DisableBareStopIsUngated(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("irc-bare-disable")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "irc-bare-disable", mustWorkspaceRoot(t))

	res := r.runWithEnv(env, "irc", "disable", "irc-bare-disable", "--format", "json")
	requireExitCode(t, res, 0)

	var doc struct {
		Purged bool `json:"purged"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)
	if doc.Purged {
		t.Fatal("a bare disable must not purge anything")
	}
}

// TestIRC_DisableRefusesSharedServer locks exit 4: a workspace hosting
// guests is not this workspace's decision alone to stop.
func TestIRC_DisableRefusesSharedServer(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("irc-host")
	env := ircEnv(t, r, "external")
	writeProfile(t, r, id, map[string]any{
		"mode": "external", "provider": "ergo", "server_id": "irc.example.com:6697",
		"root": mustWorkspaceRoot(t), "bot_account": "wsm",
		"shared_with": []string{"guest-a", "guest-b"},
		"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
	})

	refused := r.runWithEnv(env, "irc", "disable", "irc-host")
	requireExitCode(t, refused, 4)
	if !strings.Contains(refused.Combined(), "guest") {
		t.Fatalf("expected the refusal to name the guests, got:\n%s", refused.Combined())
	}

	// --force overrides it: the guests are someone else's problem then.
	forced := r.runWithEnv(env, "irc", "disable", "irc-host", "--force", "--format", "json")
	requireExitCode(t, forced, 0)
}

// TestIRC_StopRefusesSharedServer locks exit 4 on the other verbs that
// take the server down. disable is not the only route to a stopped
// daemon: stop does it outright and restart does it as its first half,
// so an unguarded stop would hand a shared server the teardown that
// disable refuses.
func TestIRC_StopRefusesSharedServer(t *testing.T) {
	sharedHost := func(t *testing.T, name string) (*e2eRunner, map[string]string) {
		t.Helper()
		r := newRunner(t)
		id := r.mustCreateWorkspace(name)
		env := ircEnv(t, r, "external")
		writeProfile(t, r, id, map[string]any{
			"mode": "external", "provider": "ergo", "server_id": "irc.example.com:6697",
			"root": mustWorkspaceRoot(t), "bot_account": "wsm",
			"shared_with": []string{"guest-a", "guest-b"},
			"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
		})
		return r, env
	}

	t.Run("stop", func(t *testing.T) {
		r, env := sharedHost(t, "irc-stop-host")

		refused := r.runWithEnv(env, "irc", "stop", "irc-stop-host")
		requireExitCode(t, refused, 4)
		if !strings.Contains(refused.Combined(), "guest") {
			t.Fatalf("expected the refusal to name the guests, got:\n%s", refused.Combined())
		}

		// --force overrides it, exactly as it does on disable.
		forced := r.runWithEnv(env, "irc", "stop", "irc-stop-host", "--force", "--format", "json")
		requireExitCode(t, forced, 0)
	})

	// restart stops before it starts, so the same hole reaches the same
	// server by another verb.
	t.Run("restart", func(t *testing.T) {
		r, env := sharedHost(t, "irc-restart-host")

		refused := r.runWithEnv(env, "irc", "restart", "irc-restart-host")
		requireExitCode(t, refused, 4)
		if !strings.Contains(refused.Combined(), "guest") {
			t.Fatalf("expected the refusal to name the guests, got:\n%s", refused.Combined())
		}
	})
}

// TestIRC_DisablePurgeRemovesState locks --purge behind the same
// --confirm gate workspace delete uses, and proves it removes the
// directories rather than merely claiming to.
func TestIRC_DisablePurgeRemovesState(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("irc-purge")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "irc-purge", mustWorkspaceRoot(t))

	dataDir := filepath.Join(r.dataDir, "irc", id)
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("expected the profile to have created %s: %v", dataDir, err)
	}

	// Without a confirmation on a non-TTY the purge is refused, while
	// the bare stop that shares this verb stays ungated.
	refused := r.runWithEnv(env, "irc", "disable", "irc-purge", "--purge")
	if refused.ExitCode == 0 {
		t.Fatalf("purge without --confirm must be refused, got exit 0:\n%s", refused.Combined())
	}
	if !strings.Contains(refused.Combined(), "--purge refused") {
		t.Fatalf("expected the refusal to name --purge as the gated half, got:\n%s",
			refused.Combined())
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("a refused purge must leave the data dir in place: %v", err)
	}

	confirmed := r.runWithEnv(env, "irc", "disable", "irc-purge", "--purge",
		"--confirm=yes", "--format", "json")
	requireExitCode(t, confirmed, 0)

	var doc struct {
		Purged bool `json:"purged"`
	}
	mustUnmarshalJSON(t, confirmed.Stdout, &doc)
	if !doc.Purged {
		t.Fatal("a confirmed purge must report purged=true")
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("purge must remove %s, stat err = %v", dataDir, err)
	}
}

// TestIRC_DryRunAppliesNothing locks --dry-run on the two mutating
// verbs: a plan on stdout and no daemon started, no state removed.
func TestIRC_DryRunAppliesNothing(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("irc-dry")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "irc-dry", mustWorkspaceRoot(t))

	t.Run("disable purge", func(t *testing.T) {
		res := r.runWithEnv(env, "irc", "disable", "irc-dry", "--purge",
			"--dry-run", "--format", "json")
		requireExitCode(t, res, 0)

		var plan struct {
			Command string `json:"command"`
			Effects []struct {
				Kind       string `json:"kind"`
				Target     string `json:"target"`
				Reversible bool   `json:"reversible"`
			} `json:"effects"`
		}
		mustUnmarshalJSON(t, res.Stdout, &plan)
		if plan.Command != "wsm irc disable" {
			t.Fatalf("plan.command = %q; want 'wsm irc disable'", plan.Command)
		}
		if len(plan.Effects) == 0 || plan.Effects[0].Kind != "delete" {
			t.Fatalf("expected a delete effect, got %+v", plan.Effects)
		}
		if plan.Effects[0].Reversible {
			t.Fatal("a purge is not reversible and must not claim to be")
		}
		if !strings.Contains(plan.Effects[0].Target, id) {
			t.Fatalf("effect target %q must name the workspace", plan.Effects[0].Target)
		}

		if _, err := os.Stat(filepath.Join(r.dataDir, "irc", id)); err != nil {
			t.Fatalf("a dry-run purge must remove nothing: %v", err)
		}
	})
}

// writeExternalProfile writes a minimal external-mode profile so the
// lifecycle verbs treat the workspace as enabled.
func writeExternalProfile(t *testing.T, r *e2eRunner, name, root string) {
	t.Helper()
	m, cleanup := r.openManager()
	ws, err := m.ResolveWorkspaceRef(t.Context(), name)
	cleanup()
	if err != nil {
		t.Fatalf("resolve workspace %q: %v", name, err)
	}
	writeProfile(t, r, ws.ID, map[string]any{
		"mode": "external", "provider": "ergo",
		"server_id": "irc.example.com:6697", "root": root,
		"bot_account": "wsm",
		"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// writeProfile writes a profile.yaml for the given workspace id.
func writeProfile(t *testing.T, r *e2eRunner, wsID string, profile map[string]any) {
	t.Helper()
	dir := filepath.Join(r.dataDir, "irc", wsID)
	mustMkdirAll(t, dir)

	var b strings.Builder
	for _, key := range []string{
		"mode", "provider", "server_id", "root", "bot_account",
		"host_workspace", "created_at",
	} {
		if v, ok := profile[key]; ok {
			fmt.Fprintf(&b, "%s: %q\n", key, v)
		}
	}
	if guests, ok := profile["shared_with"].([]string); ok && len(guests) > 0 {
		b.WriteString("shared_with:\n")
		for _, g := range guests {
			fmt.Fprintf(&b, "  - %q\n", g)
		}
	}
	writeFile(t, filepath.Join(dir, "profile.yaml"), []byte(b.String()))
}

// waitForFile blocks until path exists, or fails the test.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
