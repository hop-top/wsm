package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The extras verbs are exercised against their own stub daemon rather
// than the lifecycle one. What they contract on is wider: a status
// document carrying the discovery arrays, and a daemon that answers
// SIGUSR1 by publishing a backfill report. The lifecycle stub writes
// neither, so extending it would change what those tests assert.
const extrasDaemonSource = `package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func statusDoc(backfill map[string]any) map[string]any {
	doc := map[string]any{
		"mode":     os.Getenv("FAKE_DAEMON_MODE"),
		"provider": "ergo",
		"state":    "running",
		"pid":      os.Getpid(),
		"since":    time.Now().UTC().Format(time.RFC3339Nano),
		"server": map[string]any{
			"id": "wsm:///irc", "host": "127.0.0.1", "port": 6700, "tls": false,
		},
		"caps": map[string]any{"negotiated": []string{"sasl"}},
		"discovered": []map[string]any{{
			"name":   "#acme-team-sami-notes",
			"owner":  "sami",
			"since":  time.Now().UTC().Format(time.RFC3339Nano),
			"joined": true,
		}},
		"peer": []map[string]any{{"name": "#other-dev", "founder": "wsm-other"}},
		"unmanaged": []map[string]any{{
			"name":    "#squatted",
			"founder": "someone",
			"seen":    time.Now().UTC().Format(time.RFC3339Nano),
		}},
		"cursors": map[string]any{
			"#acme-team": map[string]any{
				"last_msgid": "m1",
				"last_time":  time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
			},
		},
	}
	if backfill != nil {
		doc["last_backfill"] = backfill
	}
	return doc
}

func write(dir string, doc map[string]any) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		os.Exit(1)
	}
	tmp := filepath.Join(dir, ".status.tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		os.Exit(1)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "status.json")); err != nil {
		os.Exit(1)
	}
}

func main() {
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

	write(dir, statusDoc(nil))

	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGUSR1)
	for {
		sig := <-ch
		switch sig {
		case syscall.SIGHUP:
			_ = os.WriteFile(filepath.Join(dir, "sighup"), []byte("1"), 0o600)
		case syscall.SIGUSR1:
			// Answer the request the way the daemon does: publish a
			// report stamped later than the one the CLI saw before it
			// signalled.
			write(dir, statusDoc(map[string]any{
				"at": time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
				"channels": []map[string]any{{
					"channel": "#acme-team", "fetched": 7, "recorded": 5, "skipped": 2,
				}},
			}))
		default:
			_ = os.Remove(filepath.Join(dir, "status.json"))
			return
		}
	}
}
`

var (
	extrasDaemonOnce sync.Once
	extrasDaemonPath string
	extrasDaemonErr  error
)

// buildExtrasDaemon compiles the extras stub once per test binary.
func buildExtrasDaemon(t *testing.T) string {
	t.Helper()
	extrasDaemonOnce.Do(func() {
		root := findRepoRoot(t)
		dir := filepath.Join(root, ".tmp", "e2e", "extrasdaemon")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			extrasDaemonErr = err
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(extrasDaemonSource), 0o644); err != nil {
			extrasDaemonErr = err
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"),
			[]byte("module extrasdaemon\n\ngo 1.24\n"), 0o644); err != nil {
			extrasDaemonErr = err
			return
		}
		extrasDaemonPath = filepath.Join(dir, binaryName("extras-irc-daemon"))
		extrasDaemonErr = goBuildIn(dir, extrasDaemonPath, ".")
	})
	if extrasDaemonErr != nil {
		t.Fatalf("build extras daemon: %v", extrasDaemonErr)
	}
	return extrasDaemonPath
}

// extrasEnv points the CLI at the extras stub.
func extrasEnv(t *testing.T, r *e2eRunner, mode string) map[string]string {
	t.Helper()
	env := ircEnv(t, r, mode)
	env["WSM_IRC_DAEMON_EXE"] = buildExtrasDaemon(t)
	return env
}

// writeManagedProfile writes a managed-mode profile carrying a port
// triple, which the lifecycle helper's fixed key list cannot express.
func writeManagedProfile(t *testing.T, r *e2eRunner, wsID, root string, extra string) {
	t.Helper()
	dir := filepath.Join(r.dataDir, "irc", wsID)
	mustMkdirAll(t, dir)

	var b strings.Builder
	fmt.Fprintf(&b, "mode: %q\n", "managed")
	fmt.Fprintf(&b, "provider: %q\n", "ergo")
	fmt.Fprintf(&b, "server_id: %q\n", "wsm:///irc")
	fmt.Fprintf(&b, "root: %q\n", root)
	fmt.Fprintf(&b, "bot_account: %q\n", "wsm")
	fmt.Fprintf(&b, "created_at: %q\n", time.Now().UTC().Format(time.RFC3339Nano))
	b.WriteString("ports:\n  plain: 6700\n  remote: 6701\n  api: 6702\n")
	b.WriteString("remote: true\n")
	b.WriteString(extra)
	writeFile(t, filepath.Join(dir, "profile.yaml"), []byte(b.String()))
}

// TestIRCExtras_DisabledGate locks the feature gate on every new verb:
// with irc.enabled off each exits 2 before the workspace is looked up,
// so somebody who never turned IRC on is not sent hunting for a
// workspace name.
func TestIRCExtras_DisabledGate(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("extras-gate")

	invocations := [][]string{
		{"irc", "creds", "extras-gate", "someone"},
		{"irc", "url", "extras-gate"},
		{"irc", "backfill", "extras-gate"},
		{"irc", "channel", "list", "extras-gate"},
		{"irc", "channel", "add", "extras-gate", "#c"},
		{"irc", "channel", "rm", "extras-gate", "#c"},
		{"irc", "account", "create", "extras-gate", "someone"},
		{"irc", "account", "rotate", "extras-gate", "someone"},
		{"irc", "account", "suspend", "extras-gate", "someone"},
	}
	for _, args := range invocations {
		t.Run(strings.Join(args[1:3], " "), func(t *testing.T) {
			res := r.runWithEnv(map[string]string{"WSM_IRC_ENABLED": "false"}, args...)
			requireExitCode(t, res, 2)
			if !strings.Contains(res.Combined(), "set irc.enabled: true") {
				t.Fatalf("expected the remedy in the output, got:\n%s", res.Combined())
			}
		})
	}
}

// TestIRCExtras_UnknownWorkspace locks exit 3 across the new verbs.
func TestIRCExtras_UnknownWorkspace(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "external")

	invocations := [][]string{
		{"irc", "url", "no-such-workspace"},
		{"irc", "channel", "list", "no-such-workspace"},
		{"irc", "backfill", "no-such-workspace"},
		{"irc", "creds", "no-such-workspace", "someone"},
	}
	for _, args := range invocations {
		t.Run(strings.Join(args[1:3], " "), func(t *testing.T) {
			requireExitCode(t, r.runWithEnv(env, args...), 3)
		})
	}
}

// TestIRCExtras_URLPerMode locks the --json shape and the URL each mode
// produces, including the two refusals that keep wsm from advertising an
// endpoint it does not own.
func TestIRCExtras_URLPerMode(t *testing.T) {
	t.Run("managed", func(t *testing.T) {
		r := newRunner(t)
		id := r.mustCreateWorkspace("extras-url-managed")
		env := extrasEnv(t, r, "managed")
		writeManagedProfile(t, r, id, mustWorkspaceRoot(t), "")

		res := r.runWithEnv(env, "irc", "url", "extras-url-managed", "--format", "json")
		requireExitCode(t, res, 0)

		var doc struct {
			URL    string `json:"url"`
			Scheme string `json:"scheme"`
			Port   int    `json:"port"`
			TLS    bool   `json:"tls"`
		}
		mustUnmarshalJSON(t, res.Stdout, &doc)
		if doc.URL != "irc://127.0.0.1:6700" {
			t.Fatalf("url = %q; want irc://127.0.0.1:6700", doc.URL)
		}
		if doc.TLS {
			t.Fatal("the managed loopback listener is plaintext and must not claim TLS")
		}

		// --web needs the remote host, which the config supplies.
		web := r.runWithEnv(
			mergeEnv(env, map[string]string{"WSM_IRC_REMOTE_HOST": "chat.example.com"}),
			"irc", "url", "extras-url-managed", "--web", "--format", "json")
		requireExitCode(t, web, 0)

		var webDoc struct {
			URL string `json:"url"`
		}
		mustUnmarshalJSON(t, web.Stdout, &webDoc)
		if webDoc.URL != "wss://chat.example.com:6701" {
			t.Fatalf("web url = %q; want wss://chat.example.com:6701", webDoc.URL)
		}
	})

	t.Run("external refuses web", func(t *testing.T) {
		r := newRunner(t)
		r.mustCreateWorkspace("extras-url-ext")
		env := extrasEnv(t, r, "external")
		writeExternalProfile(t, r, "extras-url-ext", mustWorkspaceRoot(t))

		plain := r.runWithEnv(
			mergeEnv(env, map[string]string{
				"WSM_IRC_SERVER_HOST": "irc.example.com",
				"WSM_IRC_SERVER_PORT": "6697",
			}),
			"irc", "url", "extras-url-ext", "--format", "json")
		requireExitCode(t, plain, 0)

		var doc struct {
			URL string `json:"url"`
		}
		mustUnmarshalJSON(t, plain.Stdout, &doc)
		if !strings.HasPrefix(doc.URL, "ircs://irc.example.com:6697") {
			t.Fatalf("url = %q; want ircs://irc.example.com:6697", doc.URL)
		}

		web := r.runWithEnv(
			mergeEnv(env, map[string]string{"WSM_IRC_SERVER_HOST": "irc.example.com"}),
			"irc", "url", "extras-url-ext", "--web")
		requireExitCode(t, web, 2)
	})
}

// TestIRCExtras_StatusSurfacesDiscoveryArrays locks the status doc
// addition: the three arrays the sweeper publishes reach --json, and
// they are reported from the daemon's document rather than being
// recomputed, so they survive on a workspace whose daemon is stopped.
func TestIRCExtras_StatusSurfacesDiscoveryArrays(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("extras-status")
	env := extrasEnv(t, r, "external")
	writeExternalProfile(t, r, "extras-status", mustWorkspaceRoot(t))

	start := r.runWithEnv(env, "irc", "start", "extras-status", "--format", "json")
	requireExitCode(t, start, 0)
	t.Cleanup(func() { _ = r.runWithEnv(env, "irc", "stop", "extras-status") })

	res := r.runWithEnv(env, "irc", "status", "extras-status", "--format", "json")
	requireExitCode(t, res, 0)

	var doc struct {
		Discovered []struct {
			Name   string `json:"name"`
			Owner  string `json:"owner"`
			Joined bool   `json:"joined"`
		} `json:"discovered"`
		Peer []struct {
			Name    string `json:"name"`
			Founder string `json:"founder"`
		} `json:"peer"`
		Unmanaged []struct {
			Name string `json:"name"`
		} `json:"unmanaged"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)

	if len(doc.Discovered) != 1 || doc.Discovered[0].Owner != "sami" {
		t.Fatalf("status must carry the adopted channels, got %+v", doc.Discovered)
	}
	if len(doc.Peer) != 1 || doc.Peer[0].Founder != "wsm-other" {
		t.Fatalf("status must carry peer channels, got %+v", doc.Peer)
	}
	if len(doc.Unmanaged) != 1 || doc.Unmanaged[0].Name != "#squatted" {
		t.Fatalf("status must carry unmanaged channels, got %+v", doc.Unmanaged)
	}

	_ = id
}

// TestIRCExtras_ChannelAddRefusesCreatorNamespace is the mutation guard
// for the seizure refusal: a name inside a creator's namespace is a
// conflict (exit 4), and --force is the only way past it.
func TestIRCExtras_ChannelAddRefusesCreatorNamespace(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("extras-seize")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	// A creator member, whose namespace the daemon would adopt from.
	writeFile(t, filepath.Join(root, ".wsm", "members.yaml"), []byte(strings.Join([]string{
		"members:",
		"  - profile_id: p-sami",
		"    audit_label: sami",
		"    capacity: principal",
		"    channel_creator: true",
		"",
	}, "\n")))
	writeFile(t, filepath.Join(root, ".wsm", "config.yaml"), []byte(strings.Join([]string{
		"irc:",
		"  channel_prefix: acme-",
		"",
	}, "\n")))

	inside := "#acme-extras-seize-sami-notes"

	refused := r.runWithEnv(env, "irc", "channel", "add", "extras-seize", inside)
	requireExitCode(t, refused, 4)
	if !strings.Contains(refused.Combined(), "creator namespace") {
		t.Fatalf("expected the refusal to name the namespace, got:\n%s", refused.Combined())
	}

	// The refusal must not have written anything.
	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(raw), inside) {
		t.Fatalf("a refused add must not touch the config:\n%s", string(raw))
	}

	// --force is the deliberate override.
	forced := r.runWithEnv(env, "irc", "channel", "add", "extras-seize", inside,
		"--force", "--format", "json")
	requireExitCode(t, forced, 0)

	raw, err = os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		t.Fatalf("read config after force: %v", err)
	}
	if !strings.Contains(string(raw), inside) {
		t.Fatalf("--force must write the channel, got:\n%s", string(raw))
	}
	// The unrelated key must survive the edit.
	if !strings.Contains(string(raw), "channel_prefix: acme-") {
		t.Fatalf("the edit dropped an unrelated key:\n%s", string(raw))
	}
}

// TestIRCExtras_ChannelAddRefusesDiscovered locks the other half of the
// seizure refusal: a channel the daemon already adopted is refused even
// when it falls outside every creator glob.
func TestIRCExtras_ChannelAddRefusesDiscovered(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("extras-adopted")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)

	writeManagedProfile(t, r, id, root, strings.Join([]string{
		"discovered:",
		"  \"#already-adopted\":",
		"    owner: sami",
		"    since: " + time.Now().UTC().Format(time.RFC3339Nano),
		"    joined: true",
		"",
	}, "\n"))

	res := r.runWithEnv(env, "irc", "channel", "add", "extras-adopted", "#already-adopted")
	requireExitCode(t, res, 4)
	if !strings.Contains(res.Combined(), "discovered") {
		t.Fatalf("expected the refusal to name discovery, got:\n%s", res.Combined())
	}
}

// TestIRCExtras_ChannelAddAndList drives a plain add through to the
// listing, locking both --json shapes.
func TestIRCExtras_ChannelAddAndList(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("extras-channels")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	add := r.runWithEnv(env, "irc", "channel", "add", "extras-channels", "#team-ops",
		"--members", "principals", "--format", "json")
	requireExitCode(t, add, 0)

	var addDoc struct {
		Channel string `json:"channel"`
		Action  string `json:"action"`
		Config  string `json:"config"`
		Synced  bool   `json:"synced"`
	}
	mustUnmarshalJSON(t, add.Stdout, &addDoc)
	if addDoc.Channel != "#team-ops" || addDoc.Action != "add" {
		t.Fatalf("add doc = %+v; want the declared channel", addDoc)
	}
	if addDoc.Synced {
		t.Fatal("no daemon is running, so the edit must report synced=false")
	}
	if !strings.Contains(addDoc.Config, ".wsm") {
		t.Fatalf("config path %q must sit under the workspace root", addDoc.Config)
	}

	list := r.runWithEnv(env, "irc", "channel", "list", "extras-channels", "--format", "json")
	requireExitCode(t, list, 0)

	rows := mustUnmarshalJSONArray[struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}](t, list.Stdout)

	found := false
	for _, row := range rows {
		if row.Name == "#team-ops" && row.Source == "template" {
			found = true
		}
	}
	if !found {
		t.Fatalf("list must carry the declared channel, got %+v", rows)
	}

	// Removing it again empties the override.
	rm := r.runWithEnv(env, "irc", "channel", "rm", "extras-channels", "#team-ops",
		"--format", "json")
	requireExitCode(t, rm, 0)

	// Removing a channel that is not declared is exit 3.
	missing := r.runWithEnv(env, "irc", "channel", "rm", "extras-channels", "#never")
	requireExitCode(t, missing, 3)
}

// TestIRCExtras_ChannelRmPurgeIsGated locks --purge behind the same
// confirmation as every other destructive verb, and pins the
// unsupported report when no provider can drop the registration.
func TestIRCExtras_ChannelRmPurgeIsGated(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("extras-purge")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	add := r.runWithEnv(env, "irc", "channel", "add", "extras-purge", "#doomed", "--format", "json")
	requireExitCode(t, add, 0)

	// Without a confirmation on a non-TTY the purge is refused.
	refused := r.runWithEnv(env, "irc", "channel", "rm", "extras-purge", "#doomed", "--purge")
	if refused.ExitCode == 0 {
		t.Fatalf("purge without --confirm must be refused, got exit 0:\n%s", refused.Combined())
	}
	if !strings.Contains(refused.Combined(), "--purge refused") {
		t.Fatalf("expected the refusal to name --purge, got:\n%s", refused.Combined())
	}
}

// TestIRCExtras_BackfillViaDaemon locks the running-daemon path: the
// request is filed, the daemon is signalled, and the CLI reports the
// document the daemon published rather than doing the work itself.
func TestIRCExtras_BackfillViaDaemon(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("extras-backfill")
	env := extrasEnv(t, r, "external")
	writeExternalProfile(t, r, "extras-backfill", mustWorkspaceRoot(t))

	start := r.runWithEnv(env, "irc", "start", "extras-backfill", "--format", "json")
	requireExitCode(t, start, 0)
	t.Cleanup(func() { _ = r.runWithEnv(env, "irc", "stop", "extras-backfill") })

	res := r.runWithEnv(env, "irc", "backfill", "extras-backfill",
		"--channel", "#acme-team", "--format", "json")
	requireExitCode(t, res, 0)

	var doc struct {
		Mode      string `json:"mode"`
		RequestID string `json:"request_id"`
		Channels  []struct {
			Channel  string `json:"channel"`
			Fetched  int    `json:"fetched"`
			Recorded int    `json:"recorded"`
		} `json:"channels"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)

	if doc.Mode != "daemon" {
		t.Fatalf("mode = %q; want daemon — a running daemon owns the session", doc.Mode)
	}
	if doc.RequestID == "" {
		t.Fatal("a filed request must be identified")
	}
	if len(doc.Channels) != 1 || doc.Channels[0].Recorded != 5 {
		t.Fatalf("expected the daemon's own report, got %+v", doc.Channels)
	}

	// The request file is what the daemon consumes; it must name the
	// channels that were asked for.
	raw, err := os.ReadFile(filepath.Join(r.ircStateDir(t), "irc", id, "backfill.request"))
	if err != nil {
		t.Fatalf("read backfill request: %v", err)
	}
	var req struct {
		ID       string   `json:"id"`
		Channels []string `json:"channels"`
	}
	if uerr := json.Unmarshal(raw, &req); uerr != nil {
		t.Fatalf("parse backfill request: %v", uerr)
	}
	if req.ID != doc.RequestID {
		t.Fatalf("request id %q does not match the reported %q", req.ID, doc.RequestID)
	}
	if len(req.Channels) != 1 || req.Channels[0] != "#acme-team" {
		t.Fatalf("request must record the requested channels, got %+v", req.Channels)
	}
}

// TestIRCExtras_BackfillOfflineHoldsTheLock is the mutation guard for
// the single-writer invariant: with no daemon running the CLI takes the
// daemon's own exclusive lock, and a run that cannot take it refuses
// rather than writing cursors behind the holder's back.
func TestIRCExtras_BackfillOfflineHoldsTheLock(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("extras-lock")
	env := extrasEnv(t, r, "external")
	writeExternalProfile(t, r, "extras-lock", mustWorkspaceRoot(t))

	stateDir := filepath.Join(r.ircStateDir(t), "irc", id)
	mustMkdirAll(t, stateDir)
	cursorsPath := filepath.Join(stateDir, "cursors.yaml")

	// No cursor file: nothing has ever been captured, so there is no
	// position to resume from and the run reports no channels rather
	// than replaying a channel's whole history.
	res := r.runWithEnv(env, "irc", "backfill", "extras-lock", "--format", "json")
	requireExitCode(t, res, 0)

	var doc struct {
		Mode     string `json:"mode"`
		Channels []struct {
			Channel string `json:"channel"`
		} `json:"channels"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)
	if doc.Mode != "offline" {
		t.Fatalf("mode = %q; want offline — no daemon is running", doc.Mode)
	}
	if len(doc.Channels) != 0 {
		t.Fatalf("a workspace with no cursors must replay nothing, got %+v", doc.Channels)
	}
	if _, err := os.Stat(cursorsPath); err == nil {
		t.Fatal("a run with nothing to replay must not create a cursor file")
	}

	// Hold the daemon's lock from outside and the run must refuse:
	// cursors are only ever written by the lock holder.
	lock := holdIRCLock(t, filepath.Join(stateDir, "irc.lock"))
	defer lock()

	blocked := r.runWithEnv(env, "irc", "backfill", "extras-lock")
	requireExitCode(t, blocked, 1)
	if !strings.Contains(blocked.Combined(), "lock holder") {
		t.Fatalf("expected the refusal to explain the lock invariant, got:\n%s",
			blocked.Combined())
	}
}

// mergeEnv overlays extra onto base without mutating either.
func mergeEnv(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
