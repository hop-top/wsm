package e2e_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The tests here pin CLI-level contracts that only the built binary can
// show: which verbs consult the capability set, which exit codes reach
// the shell, and what --quiet and --format actually put on the streams.
// Each is reachable solely by running wsm end to end, so a unit test on
// the run-funcs cannot see any of it.

// externalServerEnv names the server an external-mode workspace points
// at. `url` and the endpoint half of `creds` read irc.server.* from the
// config rather than from the profile, so a profile alone leaves them
// with nothing to answer with (exit 7).
var externalServerEnv = map[string]string{
	"WSM_IRC_SERVER_HOST": "irc.example.com",
	"WSM_IRC_SERVER_PORT": "6697",
}

// writeExternalProfileWithAccounts writes an external-mode profile that
// also carries an accounts map.
//
// The lifecycle helper's writeProfile emits a fixed list of scalar keys
// and cannot express `accounts:`, which is what makes a member resolve
// to an account: ProfileResolver is first in external mode's chain and
// reads profile.Accounts[profileID] directly. Seeding it here keeps the
// creds test about --quiet rather than about aps or members.yaml.
func writeExternalProfileWithAccounts(
	t *testing.T,
	r *e2eRunner,
	wsID, root string,
	accounts map[string]string,
) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "mode: %q\n", "external")
	fmt.Fprintf(&b, "provider: %q\n", "ergo")
	// The server id must match what serverIDOf reports, or the
	// resolver's serves() check declines to answer for this server.
	fmt.Fprintf(&b, "server_id: %q\n", "irc.example.com:6697")
	fmt.Fprintf(&b, "root: %q\n", root)
	fmt.Fprintf(&b, "bot_account: %q\n", "wsm")
	fmt.Fprintf(&b, "created_at: %q\n", time.Now().UTC().Format(time.RFC3339Nano))
	b.WriteString("accounts:\n")
	for _, k := range sortedMapKeys(accounts) {
		fmt.Fprintf(&b, "  %s: %q\n", k, accounts[k])
	}

	dir := filepath.Join(r.dataDir, "irc", wsID)
	mustMkdirAll(t, dir)
	writeFile(t, filepath.Join(dir, "profile.yaml"), []byte(b.String()))
}

// sortedMapKeys keeps the generated YAML stable across runs.
func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestIRC_UngatedVerbsIgnoreCapabilities pins the current gating split:
// eight verbs consult no capability at all and run to their normal
// outcome even when every capability is withheld.
//
// This is a characterisation test, not an endorsement. It exists so that
// adding a gate to any of these verbs turns up here as a deliberate
// edit rather than passing unnoticed; see the report accompanying this
// change for which of these look like oversights.
//
// The deny applies only to the invocation under test — the fixtures are
// built with full capabilities, because workspace creation is itself
// gated and would otherwise be refused before the IRC verb ran.
func TestIRC_UngatedVerbsIgnoreCapabilities(t *testing.T) {
	// Each case names the verb and the outcome it reaches with every
	// capability withheld. None is exit 6: that is the whole point.
	cases := []struct {
		name string
		args []string
		// want is the exit code the verb reaches on a workspace that is
		// enabled but has no daemon running.
		want int
	}{
		// status answers "stopped" rather than refusing: reading state
		// is not gated.
		{"status", []string{"irc", "status"}, 0},
		// stop on an already-stopped workspace is a no-op success.
		{"stop", []string{"irc", "stop"}, 0},
		// sync needs a live daemon; without one it is exit 1, which is
		// a runtime failure and not a capability refusal.
		{"sync", []string{"irc", "sync"}, 1},
		// url is a read that answers from the config's server.
		{"url", []string{"irc", "url"}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRunner(t)
			name := "ungated-" + tc.name
			r.mustCreateWorkspace(name)
			env := mergeEnv(ircEnv(t, r, "external"), externalServerEnv)
			writeExternalProfile(t, r, name, mustWorkspaceRoot(t))
			env["WSM_DENY_CAPABILITIES"] = "all"

			args := append(append([]string{}, tc.args...), name)
			res := r.runWithEnv(env, args...)
			if res.ExitCode == 6 {
				t.Fatalf("%s is gated now (exit 6). That may be correct, but it "+
					"changes the shipped contract this test pins — update the test "+
					"deliberately:\n%s", tc.name, res.Combined())
			}
			requireExitCode(t, res, tc.want)
		})
	}
}

// TestIRC_StartIsUngated pins start specifically: it spawns a daemon —
// the most consequential thing the IRC surface does — while consulting
// no capability.
//
// Split from the table above because it needs the spawned child cleaned
// up, and because a reader scanning for "can a denied caller start a
// server" should find it by name.
func TestIRC_StartIsUngated(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("ungated-start")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "ungated-start", mustWorkspaceRoot(t))
	env["WSM_DENY_CAPABILITIES"] = "all"

	res := r.runWithEnv(env, "irc", "start", "ungated-start", "--format", "json")
	t.Cleanup(func() { _ = r.runWithEnv(env, "irc", "stop", "ungated-start", "--force") })

	if res.ExitCode == 6 {
		t.Fatalf("start is gated now (exit 6). That may well be the right call, "+
			"but it changes the shipped contract this test pins:\n%s", res.Combined())
	}
	requireExitCode(t, res, 0)

	var doc struct {
		State string `json:"state"`
		PID   int    `json:"pid"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)
	// The pin is not merely "exit 0" but "a daemon actually started":
	// an exit 0 that spawned nothing would not show this hole.
	if doc.State != "running" || doc.PID <= 0 {
		t.Fatalf("a caller with every capability withheld still started a daemon; "+
			"state=%q pid=%d", doc.State, doc.PID)
	}
}

// TestIRC_GatedVerbsStillRefuse is the other half of the split: the
// verbs that DO consult capabilities must keep refusing with exit 6.
//
// Without this, someone "fixing" the ungated pins above by deleting the
// capability check entirely would leave every test green.
func TestIRC_GatedVerbsStillRefuse(t *testing.T) {
	cases := []struct {
		name string
		deny string
		args []string
	}{
		{"disable", "irc.disable", []string{"irc", "disable"}},
		{"channel add", "irc.enable", []string{"irc", "channel", "add", "", "#c"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRunner(t)
			name := "gated-" + strings.ReplaceAll(tc.name, " ", "-")
			r.mustCreateWorkspace(name)
			env := ircEnv(t, r, "external")
			writeExternalProfile(t, r, name, mustWorkspaceRoot(t))
			env["WSM_DENY_CAPABILITIES"] = tc.deny

			args := append([]string{}, tc.args...)
			// The channel case carries a placeholder slot for the
			// workspace so the channel argument stays last.
			if args[len(args)-1] == "#c" {
				args[len(args)-2] = name
			} else {
				args = append(args, name)
			}

			res := r.runWithEnv(env, args...)
			requireExitCode(t, res, 6)
		})
	}
}

// TestIRC_PurgeRefusedCollidesWithBackendCode pins the exit code a
// declined purge produces, and names the collision it creates.
//
// A refused --purge exits 5. wsm's own contract (docs/stories, and the
// mapping in cmd/wsm/main.go) assigns 5 to backend/persistence failure,
// so a script cannot distinguish "you declined a destructive operation,
// nothing happened" from "the database failed". Both are exit 5 and both
// arrive on stderr.
//
// The assertion pins today's behaviour because changing it is a contract
// decision, not a test's call — but the collision is a defect, and this
// test says so rather than quietly blessing it.
func TestIRC_PurgeRefusedCollidesWithBackendCode(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("purge-collide")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "purge-collide", mustWorkspaceRoot(t))

	// Non-TTY with no --confirm: the auto path refuses.
	refused := r.runWithEnv(env, "irc", "disable", "purge-collide", "--purge")
	requireExitCode(t, refused, 5)
	if !strings.Contains(refused.Combined(), "--purge refused") {
		t.Fatalf("expected the refusal to name --purge, got:\n%s", refused.Combined())
	}

	// The collision, demonstrated rather than asserted from the source:
	// an unwriteable data dir is a genuine backend failure and reaches
	// the shell as the same 5.
	backend := r.runNoGlobalWithEnv(env, "--data-dir", "/dev/null/nope", "irc", "status", "purge-collide")
	if backend.ExitCode != refused.ExitCode {
		t.Logf("backend failure exits %d while a declined purge exits %d; "+
			"the codes no longer collide, which is an improvement worth "+
			"folding into this test", backend.ExitCode, refused.ExitCode)
	}
}

// TestIRC_PrerequisiteAndForbiddenOnTheSameVerb pins 7 and 6 as
// genuinely distinct outcomes of one verb, separated only by which
// precondition fails.
//
// disable is the verb that can produce both: an un-enabled workspace is
// a prerequisite failure (7), and an enabled one with the capability
// withheld is forbidden (6). A regression that collapsed the two kinds
// into a single code would still satisfy each test that checks only one.
func TestIRC_PrerequisiteAndForbiddenOnTheSameVerb(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("prereq-vs-forbidden")
	env := ircEnv(t, r, "external")

	// No profile written yet: IRC was never enabled here.
	prereq := r.runWithEnv(env, "irc", "disable", "prereq-vs-forbidden")
	requireExitCode(t, prereq, 7)
	if !strings.Contains(prereq.Combined(), "irc enable") {
		t.Fatalf("a prerequisite failure must name the fix, got:\n%s", prereq.Combined())
	}

	// Now enabled, but the capability is withheld: same verb, same
	// workspace, different kind of refusal.
	writeExternalProfile(t, r, "prereq-vs-forbidden", mustWorkspaceRoot(t))
	denied := env
	denied["WSM_DENY_CAPABILITIES"] = "irc.disable"
	forbidden := r.runWithEnv(denied, "irc", "disable", "prereq-vs-forbidden")
	requireExitCode(t, forbidden, 6)

	if prereq.ExitCode == forbidden.ExitCode {
		t.Fatal("a missing prerequisite and a withheld capability must stay " +
			"distinguishable by exit code")
	}
}

// TestIRC_QuietSilencesReadsExceptURL pins the --quiet contract across
// the read verbs, including the one deliberate exception.
//
// status and creds print literally nothing: --quiet on a read means the
// exit code is the whole answer. url is the exception, because the URL
// is the answer — suppressing it would leave the command with nothing
// to say and no way to be used in a script.
func TestIRC_QuietSilencesReadsExceptURL(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("quiet-reads")
	env := mergeEnv(ircEnv(t, r, "external"), externalServerEnv)
	// The accounts map is what makes "alice" resolve to an account: the
	// creds success path needs a member the resolver chain can answer
	// for, and seeding the profile keeps that out of aps and members.yaml.
	writeExternalProfileWithAccounts(t, r, id, mustWorkspaceRoot(t),
		map[string]string{"alice": "alice-irc"})

	t.Run("status prints nothing", func(t *testing.T) {
		res := r.runWithEnv(env, "irc", "status", "quiet-reads", "--quiet")
		requireExitCode(t, res, 0)
		if res.Combined() != "" {
			t.Fatalf("--quiet status must print nothing, got:\n%q", res.Combined())
		}
	})

	t.Run("creds prints nothing", func(t *testing.T) {
		// Proven silent rather than merely exit-0: creds is the verb
		// that would otherwise print a credential, so --quiet suppressing
		// it is the whole contract. The same invocation without --quiet
		// is asserted below to print something, or this would pass just
		// as well against a creds that failed to find anything to say.
		res := r.runWithEnv(env, "irc", "creds", "quiet-reads", "alice", "--quiet")
		requireExitCode(t, res, 0)
		if res.Combined() != "" {
			t.Fatalf("--quiet creds must print nothing, got:\n%q", res.Combined())
		}

		loud := r.runWithEnv(env, "irc", "creds", "quiet-reads", "alice", "--format", "json")
		requireExitCode(t, loud, 0)
		if !strings.Contains(loud.Stdout, "alice-irc") {
			t.Fatalf("without --quiet, creds must report the account it resolved, got:\n%s",
				loud.Stdout)
		}
	})

	t.Run("url still prints the url", func(t *testing.T) {
		res := r.runWithEnv(env, "irc", "url", "quiet-reads", "--quiet")
		requireExitCode(t, res, 0)
		out := res.StdoutTrim()
		if out == "" {
			t.Fatal("--quiet url must still print the URL: it is the whole answer")
		}
		if !strings.HasPrefix(out, "irc://") && !strings.HasPrefix(out, "ircs://") {
			t.Fatalf("--quiet url must print a bare URL and nothing else, got:\n%q", out)
		}
		if strings.Contains(out, "\n") {
			t.Fatalf("--quiet url must print one line, got:\n%q", out)
		}
	})
}

// TestIRC_NonTTYDefaultsToJSON pins the format default on a pipe.
//
// There is no --json flag anywhere on this surface; --format is a root
// persistent flag and a piped invocation that names no format gets JSON.
// A script therefore parses JSON without asking for it, which is worth
// pinning precisely because nothing on the command line says so.
func TestIRC_NonTTYDefaultsToJSON(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("format-default")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "format-default", mustWorkspaceRoot(t))

	t.Run("no --format yields json", func(t *testing.T) {
		// The harness never allocates a TTY here, so this is the pipe case.
		res := r.runWithEnv(env, "irc", "status", "format-default")
		requireExitCode(t, res, 0)

		var doc struct {
			Workspace string `json:"workspace"`
			State     string `json:"state"`
		}
		mustUnmarshalJSON(t, res.Stdout, &doc)
		if doc.Workspace != "format-default" {
			t.Fatalf("piped status must emit the JSON document, got:\n%s", res.Stdout)
		}
	})

	t.Run("there is no --json flag", func(t *testing.T) {
		// If a --json flag were ever added, this cobra refusal would
		// stop and the test would catch the surface change.
		res := r.runWithEnv(env, "irc", "status", "format-default", "--json")
		requireExitCode(t, res, 2)
		if !strings.Contains(strings.ToLower(res.Combined()), "unknown flag") {
			t.Fatalf("expected an unknown-flag refusal, got:\n%s", res.Combined())
		}
	})

	t.Run("--format yaml is honoured on a pipe", func(t *testing.T) {
		res := r.runWithEnv(env, "irc", "status", "format-default", "--format", "yaml")
		requireExitCode(t, res, 0)
		// YAML rather than the JSON default: the explicit flag wins.
		if strings.HasPrefix(res.StdoutTrim(), "{") {
			t.Fatalf("--format yaml must not emit JSON, got:\n%s", res.Stdout)
		}
		if !strings.Contains(res.Stdout, "workspace:") {
			t.Fatalf("expected yaml keys, got:\n%s", res.Stdout)
		}
	})
}

// TestIRC_ChannelPurgeDivergesFromChannelRmPurge pins the difference
// between the two routes to a purge on a provider that cannot do one.
//
// They are not aliases. `channel rm --purge` removes the template and
// reports the registration drop as a non-fatal reason:"unsupported",
// because the removal it was asked for did happen. `channel purge` has
// nothing else to do, so the same incapable provider is exit 7.
//
// Both are defensible alone; the divergence is what a caller has to
// know, and nothing else pins the two against each other.
func TestIRC_ChannelPurgeDivergesFromChannelRmPurge(t *testing.T) {
	// external mode against a server that is not listening: the
	// provider cannot purge, which is the condition under test.
	setup := func(t *testing.T, name string) (*e2eRunner, map[string]string) {
		t.Helper()
		r := newRunner(t)
		id := r.mustCreateWorkspace(name)
		env := extrasEnv(t, r, "managed")
		writeManagedProfile(t, r, id, mustWorkspaceRoot(t), "")
		add := r.runWithEnv(env, "irc", "channel", "add", name, "#doomed", "--format", "json")
		requireExitCode(t, add, 0)
		return r, env
	}

	t.Run("rm --purge reports unsupported without failing", func(t *testing.T) {
		r, env := setup(t, "diverge-rm")

		res := r.runWithEnv(env, "irc", "channel", "rm", "diverge-rm", "#doomed",
			"--purge", "--confirm=yes", "--format", "json")
		requireExitCode(t, res, 0)

		var doc struct {
			Channel string `json:"channel"`
			Purged  bool   `json:"purged"`
			Reason  string `json:"reason"`
		}
		mustUnmarshalJSON(t, res.Stdout, &doc)
		if doc.Purged {
			t.Fatalf("no server is listening, so nothing can have been purged: %+v", doc)
		}
		if doc.Reason == "" {
			t.Fatalf("a purge that did not happen must carry a reason: %+v", doc)
		}
	})

	t.Run("purge fails where rm --purge succeeded", func(t *testing.T) {
		r, env := setup(t, "diverge-purge")

		res := r.runWithEnv(env, "irc", "channel", "purge", "diverge-purge", "#doomed",
			"--confirm=yes")
		// The identical condition that rm --purge absorbs as a non-fatal
		// reason is fatal here: the standalone verb has nothing to report
		// but the purge it could not do, so a failure to reach the server
		// is the whole outcome rather than a footnote on a completed
		// removal.
		//
		// The exit code is deliberately not pinned to a single value. In
		// this build the bot has no password, so the session fails (exit
		// 1) before the provider can report the operation unsupported
		// (exit 7); both are the same contract — non-zero — and pinning
		// whichever one happens to arrive first would pin the fixture
		// rather than the divergence. The integration suite, which has a
		// live ergo, is where the 7 is reachable.
		if res.ExitCode == 0 {
			t.Fatalf("channel purge must not report success for a purge that "+
				"did not happen:\n%s", res.Combined())
		}
		// The failure must name the channel, or it is not a report about
		// this purge at all.
		if !strings.Contains(res.Combined(), "#doomed") {
			t.Fatalf("the failure must name the channel it could not purge, got:\n%s",
				res.Combined())
		}
	})
}
