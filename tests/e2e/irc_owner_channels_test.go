package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// channelRow is one row of `wsm irc channel list --json`, in any section.
type channelRow struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Registered bool   `json:"registered"`
	Owner      string `json:"owner,omitempty"`
}

// channelSections is the `wsm irc channel list --json` document.
//
// The four arrays are the contract: a consumer reads the one it cares
// about rather than filtering a flat list on a source string, so each
// must be present even when empty.
type channelSections struct {
	Managed    []channelRow `json:"managed"`
	Borrowed   []channelRow `json:"borrowed"`
	Discovered []channelRow `json:"discovered"`
	Unmanaged  []channelRow `json:"unmanaged"`
}

// adoptDoc is the `wsm irc channel adopt|release --json` document.
type adoptDoc struct {
	Workspace string `json:"workspace"`
	Channel   string `json:"channel"`
	Action    string `json:"action"`
	Owner     string `json:"owner,omitempty"`
	Config    string `json:"config"`
	Synced    bool   `json:"synced"`
}

// TestIRCOwner_ChannelAddOwnerReachesConfig pins the owner-managed
// channel end to end: --owner lands in the template the daemon reads.
//
// The owner is what Desired turns into a +o grant while the wsm bot
// stays founder, so an owner that never reached the config is a channel
// its owner cannot run. Nothing else asserted this: the flag was wired
// but untested.
func TestIRCOwner_ChannelAddOwnerReachesConfig(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("owner-add")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	add := r.runWithEnv(env, "irc", "channel", "add", "owner-add", "#team-ops",
		"--owner", "p-sami", "--members", "principals", "--format", "json")
	requireExitCode(t, add, 0)

	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(raw), "owner: p-sami") {
		t.Fatalf("--owner must reach the template the daemon reads:\n%s", string(raw))
	}

	// The listing surfaces it, so an operator can see who runs the
	// channel without opening the config file.
	list := r.runWithEnv(env, "irc", "channel", "list", "owner-add", "--format", "json")
	requireExitCode(t, list, 0)

	var sections channelSections
	mustUnmarshalJSON(t, list.Stdout, &sections)

	var found bool
	for _, row := range sections.Managed {
		if row.Name == "#team-ops" {
			found = true
			if row.Owner != "p-sami" {
				t.Fatalf("owner = %q; want p-sami", row.Owner)
			}
		}
	}
	if !found {
		t.Fatalf("the managed section must carry the owned channel, got %+v", sections)
	}
}

// TestIRCOwner_ChannelListSections locks the four-section --json shape
// against a daemon publishing all three discovery arrays.
//
// The sections come from status.json rather than being recomputed, and
// each is a separate array so a consumer never has to filter on a
// source string.
func TestIRCOwner_ChannelListSections(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("owner-sections")
	env := extrasEnv(t, r, "external")
	writeExternalProfile(t, r, "owner-sections", mustWorkspaceRoot(t))

	start := r.runWithEnv(env, "irc", "start", "owner-sections", "--format", "json")
	requireExitCode(t, start, 0)
	t.Cleanup(func() { _ = r.runWithEnv(env, "irc", "stop", "owner-sections") })

	list := r.runWithEnv(env, "irc", "channel", "list", "owner-sections", "--format", "json")
	requireExitCode(t, list, 0)

	var sections channelSections
	mustUnmarshalJSON(t, list.Stdout, &sections)

	// The stub publishes one adopted, one peer and one unmanaged channel.
	// Adopted and peer share the discovered section: wsm captures neither,
	// but neither is purge-eligible the way an unmanaged registration is.
	var adopted, peer bool
	for _, row := range sections.Discovered {
		switch row.Name {
		case "#acme-team-sami-notes":
			adopted = true
			if row.Owner != "sami" {
				t.Errorf("adopted owner = %q; want sami", row.Owner)
			}
		case "#other-dev":
			peer = true
		}
	}
	if !adopted {
		t.Errorf("the discovered section must carry the adopted channel, got %+v", sections.Discovered)
	}
	if !peer {
		t.Errorf("the discovered section must carry the peer channel, got %+v", sections.Discovered)
	}

	var unmanaged bool
	for _, row := range sections.Unmanaged {
		if row.Name == "#squatted" {
			unmanaged = true
		}
	}
	if !unmanaged {
		t.Errorf("the unmanaged section must carry the squatted channel, got %+v", sections.Unmanaged)
	}

	// An empty section is an empty array, never null: a consumer
	// indexing borrowed on a workspace with none must not break.
	if sections.Borrowed == nil {
		t.Error("borrowed must render as [] rather than null")
	}
	if sections.Managed == nil {
		t.Error("managed must render as [] rather than null")
	}

	_ = id
}

// TestIRCOwner_AdoptWritesRequestAndSignals is the mutation guard for
// the central invariant: adopt records a request and signals the
// daemon, and never joins the channel itself.
//
// The CLI's connection dies with the process, so a channel it joined
// would be captured by nothing. The proof that it does not join is
// twofold: the request reaches irc.adopted, and the daemon is
// signalled — the stub records SIGHUP delivery, and the daemon's
// discovery pass is what acts on it.
func TestIRCOwner_AdoptWritesRequestAndSignals(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("owner-adopt")
	env := extrasEnv(t, r, "external")
	root := mustWorkspaceRoot(t)
	writeExternalProfile(t, r, "owner-adopt", root)

	start := r.runWithEnv(env, "irc", "start", "owner-adopt", "--format", "json")
	requireExitCode(t, start, 0)
	t.Cleanup(func() { _ = r.runWithEnv(env, "irc", "stop", "owner-adopt") })

	adopt := r.runWithEnv(env, "irc", "channel", "adopt", "owner-adopt", "#sami-notes",
		"--owner", "sami", "--format", "json")
	requireExitCode(t, adopt, 0)

	var doc adoptDoc
	mustUnmarshalJSON(t, adopt.Stdout, &doc)
	if doc.Action != "adopt" || doc.Channel != "#sami-notes" {
		t.Fatalf("adopt doc = %+v; want the adopted channel", doc)
	}
	if doc.Owner != "sami" {
		t.Fatalf("owner = %q; want sami", doc.Owner)
	}
	if !doc.Synced {
		t.Fatal("adopt must report the daemon was signalled")
	}

	// The request lands in the override the daemon reads.
	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "adopted:") || !strings.Contains(got, "#sami-notes") {
		t.Fatalf("adopt must write irc.adopted:\n%s", got)
	}
	if !strings.Contains(got, "owner: sami") {
		t.Fatalf("adopt must record the owner:\n%s", got)
	}

	// The daemon was actually signalled: the stub records the delivery.
	// This is what makes the join the daemon's work rather than the
	// CLI's — the CLI asks, the discovery pass acts.
	waitForFile(t, filepath.Join(r.ircStateDir(t), "irc", id, "sighup"))
}

// TestIRCOwner_ReleaseDropsAdoption pins that release writes
// irc.released and drops the matching adoption.
//
// A config that both adopts and releases one channel is a contradiction
// an operator would have to untangle; the adoption is removed rather
// than left to be outranked.
func TestIRCOwner_ReleaseDropsAdoption(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("owner-release")
	env := extrasEnv(t, r, "external")
	root := mustWorkspaceRoot(t)
	writeExternalProfile(t, r, "owner-release", root)

	start := r.runWithEnv(env, "irc", "start", "owner-release", "--format", "json")
	requireExitCode(t, start, 0)
	t.Cleanup(func() { _ = r.runWithEnv(env, "irc", "stop", "owner-release") })

	adopt := r.runWithEnv(env, "irc", "channel", "adopt", "owner-release", "#sami-notes",
		"--owner", "sami", "--format", "json")
	requireExitCode(t, adopt, 0)

	release := r.runWithEnv(env, "irc", "channel", "release", "owner-release", "#sami-notes",
		"--format", "json")
	requireExitCode(t, release, 0)

	var doc adoptDoc
	mustUnmarshalJSON(t, release.Stdout, &doc)
	if doc.Action != "release" {
		t.Fatalf("release doc = %+v; want action release", doc)
	}
	if !doc.Synced {
		t.Fatal("release must report the daemon was signalled")
	}

	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "released:") {
		t.Fatalf("release must write irc.released:\n%s", got)
	}
	// The adoption is gone: the key itself is dropped once its last
	// entry goes, because an empty list is a different statement.
	if strings.Contains(got, "adopted:") {
		t.Fatalf("release must drop the matching adoption:\n%s", got)
	}

	_ = id
}

// TestIRCOwner_AdoptReleaseNeedDaemon is the mutation guard for the
// exit-1 path: both verbs refuse when no daemon is running.
//
// The join and the part happen in the discovery pass, so a write with
// nothing running would report success for work nobody is going to do.
// add and rm may defer to the next start because their templates are
// read at startup; an adoption request is not.
func TestIRCOwner_AdoptReleaseNeedDaemon(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("owner-nodaemon")
	env := extrasEnv(t, r, "external")
	root := mustWorkspaceRoot(t)
	writeExternalProfile(t, r, "owner-nodaemon", root)

	for _, verb := range []string{"adopt", "release"} {
		t.Run(verb, func(t *testing.T) {
			res := r.runWithEnv(env, "irc", "channel", verb, "owner-nodaemon", "#sami-notes")
			requireExitCode(t, res, 1)
			if !strings.Contains(res.Combined(), "no irc daemon is running") {
				t.Fatalf("expected the refusal to name the stopped daemon, got:\n%s",
					res.Combined())
			}
		})
	}

	// A refused request must not have touched the config: reporting the
	// failure and writing anyway would leave intent the daemon adopts on
	// its next start, which is the outcome the refusal exists to prevent.
	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(raw), "#sami-notes") {
		t.Fatalf("a refused request must not write the channel:\n%s", string(raw))
	}

	_ = id
}

// TestIRCOwner_PurgeIsGatedAndUnsupported locks the purge leaf: the
// --confirm gate stands in front of it, and a provider that cannot
// purge reports exit 7 rather than appearing to succeed.
func TestIRCOwner_PurgeIsGatedAndUnsupported(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("owner-purge")
	env := extrasEnv(t, r, "managed")
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, "")

	// Without a confirmation on a non-TTY the purge is refused before
	// anything reaches the server.
	refused := r.runWithEnv(env, "irc", "channel", "purge", "owner-purge", "#doomed")
	if refused.ExitCode == 0 {
		t.Fatalf("purge without --confirm must be refused, got exit 0:\n%s", refused.Combined())
	}
	// The gate names the command it stood in front of, so an operator
	// reading a script's output can tell which verb was refused.
	if !strings.Contains(refused.Combined(), "wsm irc channel purge refused") {
		t.Fatalf("expected the refusal to name the gate, got:\n%s", refused.Combined())
	}

	// Confirmed, the purge reaches the provider. No server is listening,
	// so it cannot complete: either the session fails (exit 1) or the
	// provider reports the operation unsupported (exit 7). Both are
	// honest reports; exit 0 would not be.
	confirmed := r.runWithEnv(env, "irc", "channel", "purge", "owner-purge", "#doomed",
		"--confirm", "yes")
	if confirmed.ExitCode != 1 && confirmed.ExitCode != 7 {
		t.Fatalf("a purge that cannot reach the server must fail, got exit %d:\n%s",
			confirmed.ExitCode, confirmed.Combined())
	}
}
