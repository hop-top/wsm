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

// TestIRC_MutatingVerbsRefuseWithoutCapability is the converted half of
// what used to be TestIRC_UngatedVerbsIgnoreCapabilities.
//
// That test pinned eight verbs as consulting no capability at all, and
// deliberately failed if any of them ever started refusing. The gating
// model changed: every verb that changes state now checks a capability,
// so the old pins are obsolete by decision and this asserts the new
// contract in their place. The reads that stayed open are pinned
// separately below, so "gate everything" would not pass either.
//
// The deny applies only to the invocation under test — the fixtures are
// built with full capabilities, because workspace creation is itself
// gated and would otherwise be refused before the IRC verb ran.
func TestIRC_MutatingVerbsRefuseWithoutCapability(t *testing.T) {
	// Each case names the verb and the single capability that must be
	// enough, on its own, to refuse it. Denying one rather than all is
	// what proves the verb consults that specific capability instead of
	// happening to fail for some unrelated reason.
	cases := []struct {
		name string
		deny string
		args []string
	}{
		{"stop", "irc.stop", []string{"irc", "stop"}},
		{"restart", "irc.stop", []string{"irc", "restart"}},
		{"start", "irc.start", []string{"irc", "start"}},
		{"sync", "irc.sync", []string{"irc", "sync"}},
		{"backfill", "irc.sync", []string{"irc", "backfill"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRunner(t)
			name := "gated-" + tc.name
			r.mustCreateWorkspace(name)
			env := mergeEnv(ircEnv(t, r, "external"), externalServerEnv)
			writeExternalProfile(t, r, name, mustWorkspaceRoot(t))
			env["WSM_DENY_CAPABILITIES"] = tc.deny

			args := append(append([]string{}, tc.args...), name)
			requireExitCode(t, r.runWithEnv(env, args...), 6)
		})
	}
}

// TestIRC_StartRefusedWithoutCapability is the converted form of
// TestIRC_StartIsUngated.
//
// The original pinned that a caller with every capability withheld
// could still spawn a daemon, and asserted the daemon really came up
// (state running, a live pid) so the hole could not be papered over
// with a bare exit 0. The inversion keeps that rigour: the refusal must
// be exit 6 AND nothing may have been spawned, because a gate that
// refuses after starting the daemon would satisfy the exit code alone.
func TestIRC_StartRefusedWithoutCapability(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("denied-start")
	env := ircEnv(t, r, "external")
	writeExternalProfile(t, r, "denied-start", mustWorkspaceRoot(t))
	env["WSM_DENY_CAPABILITIES"] = "irc.start"

	res := r.runWithEnv(env, "irc", "start", "denied-start", "--format", "json")
	requireExitCode(t, res, 6)

	// Nothing may have been spawned. status is ungated, so it answers
	// even for a caller that was just refused, which is exactly what
	// makes it usable as the check here.
	full := mergeEnv(ircEnv(t, r, "external"), externalServerEnv)
	status := r.runWithEnv(full, "irc", "status", "denied-start", "--format", "json")
	requireExitCode(t, status, 0)

	var doc struct {
		State string `json:"state"`
		PID   int    `json:"pid"`
	}
	mustUnmarshalJSON(t, status.Stdout, &doc)
	if doc.State == "running" || doc.PID > 0 {
		t.Fatalf("a refused start must not have spawned a daemon; state=%q pid=%d",
			doc.State, doc.PID)
	}
}

// TestIRC_ForceDoesNotBypassCapability pins the hole this change closed.
//
// --force exists to override the shared-host guard: stopping a server
// that hosts guests is refused (exit 4) unless the operator insists.
// It was also, accidentally, a way around having no authority at all —
// guardSharedHost returned nil immediately on --force and neither stop
// nor restart consulted a capability, so a caller with everything
// withheld could take down a server hosting other workspaces.
//
// The workspace here genuinely hosts a guest, so the shared-host guard
// is live: without the capability check the flag would sail past it.
func TestIRC_ForceDoesNotBypassCapability(t *testing.T) {
	for _, verb := range []string{"stop", "restart"} {
		t.Run(verb, func(t *testing.T) {
			r := newRunner(t)
			name := "force-" + verb
			id := r.mustCreateWorkspace(name)
			env := mergeEnv(ircEnv(t, r, "external"), externalServerEnv)

			// A profile naming a guest is what arms guardSharedHost.
			writeProfile(t, r, id, map[string]any{
				"mode": "external", "provider": "ergo",
				"server_id": "irc.example.com:6697", "root": mustWorkspaceRoot(t),
				"bot_account": "wsm",
				"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
				"shared_with": []string{"some-guest"},
			})

			// With the capability held, --force is what gets past the
			// guest refusal: exit 4 without it, not 4 with it. This is
			// the control that proves the guard is actually armed, so
			// the denial below cannot be passing for the wrong reason.
			guarded := r.runWithEnv(env, "irc", verb, name)
			requireExitCode(t, guarded, 4)

			// Denied and forced: the capability refusal must win. Exit 4
			// here would mean the guard answered first; exit 0 would
			// mean --force had bypassed the gate entirely.
			denied := mergeEnv(env, map[string]string{"WSM_DENY_CAPABILITIES": "irc.stop"})
			res := r.runWithEnv(denied, "irc", verb, name, "--force")
			requireExitCode(t, res, 6)

			// Denied WITHOUT --force, on the same guest-hosting
			// workspace. Both refusals are live here and they disagree
			// about the code, so this pins which one answers: a caller
			// with no authority is told it is forbidden (6), not that it
			// collided with someone else's guests (4). Nothing else in
			// the suite pins that ordering — with the gate placed after
			// the guard this case returns 4 while every other assertion
			// in this test still passes.
			unforced := r.runWithEnv(denied, "irc", verb, name)
			requireExitCode(t, unforced, 6)
		})
	}
}

// TestIRC_ReadsStayUngated pins the other side of the decision: the
// verbs deliberately left open.
//
// Without this, "gate every irc verb" would pass the suite while
// breaking the reads. Each of these reports state that anyone able to
// read the workspace root can already read from its profile and config,
// so a capability check there would be theatre rather than control.
func TestIRC_ReadsStayUngated(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		// status answers "stopped" rather than refusing.
		{"status", []string{"irc", "status"}, 0},
		// url answers from the config's server.
		{"url", []string{"irc", "url"}, 0},
		// channel list reports the declared channels.
		{"channel list", []string{"irc", "channel", "list"}, 0},
		// delegate list reports the delegations on file.
		{"delegate list", []string{"irc", "delegate", "list"}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRunner(t)
			name := "ungated-" + strings.ReplaceAll(tc.name, " ", "-")
			r.mustCreateWorkspace(name)
			env := mergeEnv(ircEnv(t, r, "external"), externalServerEnv)
			writeExternalProfile(t, r, name, mustWorkspaceRoot(t))
			env["WSM_DENY_CAPABILITIES"] = "all"

			args := append(append([]string{}, tc.args...), name)
			res := r.runWithEnv(env, args...)
			if res.ExitCode == 6 {
				t.Fatalf("%s is a read and must not consult a capability:\n%s",
					tc.name, res.Combined())
			}
			requireExitCode(t, res, tc.want)
		})
	}
}

// TestIRC_CredsRefusesWithoutCapability pins the verb the gating
// decision turned on.
//
// creds is the one read-shaped verb that is gated, because it is not a
// read of workspace state: it renders a stored passphrase, and --env
// emits it as a shell assignment ready to be sourced. The refusal is
// asserted to be silent about the secret as well as non-zero — a gate
// that exits 6 after printing the credential would not be a gate.
func TestIRC_CredsRefusesWithoutCapability(t *testing.T) {
	r := newRunner(t)
	id := r.mustCreateWorkspace("creds-denied")
	env := mergeEnv(ircEnv(t, r, "external"), externalServerEnv)
	writeExternalProfileWithAccounts(t, r, id, mustWorkspaceRoot(t),
		map[string]string{"alice": "alice-irc"})

	denied := mergeEnv(env, map[string]string{"WSM_DENY_CAPABILITIES": "irc.creds"})
	res := r.runWithEnv(denied, "irc", "creds", "creds-denied", "alice")
	requireExitCode(t, res, 6)
	if strings.Contains(res.Combined(), "alice-irc") {
		t.Fatalf("a refused creds must not disclose the account it would have "+
			"resolved, got:\n%s", res.Combined())
	}

	// The control: the same invocation with the capability held answers,
	// so the refusal above is the capability's doing and not a broken
	// fixture that would fail whatever the policy said.
	allowed := r.runWithEnv(env, "irc", "creds", "creds-denied", "alice", "--format", "json")
	requireExitCode(t, allowed, 0)
	if !strings.Contains(allowed.Stdout, "alice-irc") {
		t.Fatalf("with irc.creds held, creds must report the account:\n%s", allowed.Stdout)
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
