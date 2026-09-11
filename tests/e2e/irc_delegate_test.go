package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The delegation verbs are exercised against the extras stub daemon, like
// the rest of the IRC CLI surface: what these tests contract on is the
// configuration the commands author and the exit codes they return, not
// anything a live ergo would do.

// hostWithChannel enables a managed host workspace declaring one channel,
// and returns the workspace id and its root.
func hostWithChannel(t *testing.T, r *e2eRunner, name, channel string, extraProfile string) (string, string) {
	t.Helper()
	id := r.mustCreateWorkspace(name)
	root := mustWorkspaceRoot(t)
	writeManagedProfile(t, r, id, root, extraProfile)

	env := extrasEnv(t, r, "managed")
	add := r.runWithEnv(env, "irc", "channel", "add", name, channel,
		"--members", "principals", "--format", "json")
	requireExitCode(t, add, 0)
	return id, root
}

// TestIRCDelegate_RefusesAGuestWithNoStanding pins the central guard: a
// workspace that neither shares this host's server nor arrives with an
// explicit --bot has no account here, so there is nothing to delegate to.
//
// Exit 4 rather than 2: the invocation is well-formed and collides with
// the state of the world, which is a conflict. Authoring the entry anyway
// would leave the host reconciling against an account that does not exist.
func TestIRCDelegate_RefusesAGuestWithNoStanding(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	_, root := hostWithChannel(t, r, "deleg-host", "#host-ops", "")
	r.mustCreateWorkspace("deleg-stranger")

	res := r.runWithEnv(env, "irc", "delegate", "add", "deleg-host", "deleg-stranger",
		"--max-role", "voice", "--format", "json")
	requireExitCode(t, res, 4)

	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(raw), "delegates") {
		t.Fatalf("a refused delegation must not reach the config:\n%s", string(raw))
	}
}

// TestIRCDelegate_BotFlagStandsInForSharing pins the cross-machine case:
// a guest the host provisioned an account for by hand is delegable even
// though it does not share this server through the loopback attach.
func TestIRCDelegate_BotFlagStandsInForSharing(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	_, root := hostWithChannel(t, r, "deleg-bot-host", "#bot-ops", "")
	r.mustCreateWorkspace("deleg-remote")

	res := r.runWithEnv(env, "irc", "delegate", "add", "deleg-bot-host", "deleg-remote",
		"--bot", "wsm-remote", "--max-role", "voice", "--format", "json")
	requireExitCode(t, res, 0)

	var doc struct {
		Guest    string `json:"guest"`
		Action   string `json:"action"`
		Bot      string `json:"bot"`
		MaxRole  string `json:"max_role"`
		Channels int    `json:"channels"`
		Synced   bool   `json:"synced"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)

	if doc.Action != "add" || doc.Guest != "deleg-remote" {
		t.Fatalf("add doc = %+v; want the delegation just authored", doc)
	}
	if doc.Bot != "wsm-remote" {
		t.Errorf("bot = %q; --bot must name the account verbatim", doc.Bot)
	}
	// The ceiling the operator asked for is voice; the privilege granted is
	// the rung that makes the server enforce it.
	if doc.MaxRole != "halfop" {
		t.Errorf("max_role = %q; --max-role voice must grant halfop, because a "+
			"+v bot could hand out nothing at all", doc.MaxRole)
	}
	if doc.Channels != 1 {
		t.Errorf("channels = %d; want the one declared channel", doc.Channels)
	}

	raw := mustReadConfig(t, root)
	if !strings.Contains(raw, "delegates:") || !strings.Contains(raw, "wsm-remote") {
		t.Fatalf("the delegation never reached the config:\n%s", raw)
	}
	if !strings.Contains(raw, "max_role: halfop") {
		t.Errorf("the stored ceiling must be the granted privilege:\n%s", raw)
	}
}

// TestIRCDelegate_OpCeilingIsStoredAsOp pins the other half of the mapping.
// A chanop delegate genuinely can op others; that is a consequence of the
// grant the operator asked for, so it must be written through unchanged
// rather than quietly lowered.
func TestIRCDelegate_OpCeilingIsStoredAsOp(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	_, root := hostWithChannel(t, r, "deleg-op-host", "#op-ops", "")
	r.mustCreateWorkspace("deleg-op-guest")

	res := r.runWithEnv(env, "irc", "delegate", "add", "deleg-op-host", "deleg-op-guest",
		"--bot", "wsm-op-guest", "--max-role", "op", "--format", "json")
	requireExitCode(t, res, 0)

	var doc struct {
		MaxRole string `json:"max_role"`
	}
	mustUnmarshalJSON(t, res.Stdout, &doc)
	if doc.MaxRole != "op" {
		t.Errorf("max_role = %q; --max-role op must stay op", doc.MaxRole)
	}
	if raw := mustReadConfig(t, root); !strings.Contains(raw, "max_role: op") {
		t.Errorf("an op ceiling must be written as op:\n%s", raw)
	}
}

// TestIRCDelegate_RejectsHalfopAndUnknownCeilings pins the flag's accepted
// values. Halfop is the mechanism, not the intent: offering it would invite
// "--max-role halfop" to be read as "may hand out halfop", which is not what
// the rung does.
func TestIRCDelegate_RejectsHalfopAndUnknownCeilings(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	_, root := hostWithChannel(t, r, "deleg-bad-host", "#bad-ops", "")
	r.mustCreateWorkspace("deleg-bad-guest")

	for _, role := range []string{"halfop", "founder", ""} {
		res := r.runWithEnv(env, "irc", "delegate", "add", "deleg-bad-host",
			"deleg-bad-guest", "--bot", "wsm-bad", "--max-role", role)
		requireExitCode(t, res, 2)

		if raw := mustReadConfig(t, root); strings.Contains(raw, "wsm-bad") {
			t.Fatalf("--max-role %q was refused but a delegation was written:\n%s",
				role, raw)
		}
	}
}

// TestIRCDelegate_RmDropsTheAuthoredEntry pins what drives the host's revoke
// sweep. Removing the entry is the whole of the operator-visible action: the
// daemon's next reconcile sees a delegate role it authored that the config no
// longer asks for, and strips it.
func TestIRCDelegate_RmDropsTheAuthoredEntry(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	_, root := hostWithChannel(t, r, "deleg-rm-host", "#rm-ops", "")
	r.mustCreateWorkspace("deleg-rm-guest")

	add := r.runWithEnv(env, "irc", "delegate", "add", "deleg-rm-host", "deleg-rm-guest",
		"--bot", "wsm-rm-guest", "--max-role", "voice", "--format", "json")
	requireExitCode(t, add, 0)
	if raw := mustReadConfig(t, root); !strings.Contains(raw, "wsm-rm-guest") {
		t.Fatalf("setup failed to author the delegation:\n%s", raw)
	}

	rm := r.runWithEnv(env, "irc", "delegate", "rm", "deleg-rm-host", "deleg-rm-guest",
		"--format", "json")
	requireExitCode(t, rm, 0)

	var doc struct {
		Action   string `json:"action"`
		Channels int    `json:"channels"`
	}
	mustUnmarshalJSON(t, rm.Stdout, &doc)
	if doc.Action != "rm" || doc.Channels != 1 {
		t.Fatalf("rm doc = %+v; want one channel revoked", doc)
	}

	raw := mustReadConfig(t, root)
	if strings.Contains(raw, "wsm-rm-guest") {
		t.Errorf("the authored entry must be gone; the revoke sweep is driven "+
			"by its absence:\n%s", raw)
	}
	// An emptied list drops its key rather than leaving `delegates: []`.
	if strings.Contains(raw, "delegates:") {
		t.Errorf("an emptied delegate list must drop its key:\n%s", raw)
	}
}

// TestIRCDelegate_RmUnknownGuestIsNotFound pins that revoking a delegation
// nobody holds is reported rather than silently succeeding.
func TestIRCDelegate_RmUnknownGuestIsNotFound(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	hostWithChannel(t, r, "deleg-rm404-host", "#rm404-ops", "")

	res := r.runWithEnv(env, "irc", "delegate", "rm", "deleg-rm404-host", "nobody")
	requireExitCode(t, res, 3)
}

// TestIRCDelegate_ListReportsTheAuthoredCeiling pins the listing: one row per
// delegation, carrying the ceiling in force.
func TestIRCDelegate_ListReportsTheAuthoredCeiling(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	hostWithChannel(t, r, "deleg-list-host", "#list-ops", "")
	r.mustCreateWorkspace("deleg-list-guest")

	add := r.runWithEnv(env, "irc", "delegate", "add", "deleg-list-host",
		"deleg-list-guest", "--bot", "wsm-list-guest", "--max-role", "op",
		"--format", "json")
	requireExitCode(t, add, 0)

	list := r.runWithEnv(env, "irc", "delegate", "list", "deleg-list-host",
		"--format", "json")
	requireExitCode(t, list, 0)

	rows := mustUnmarshalJSONArray[struct {
		Channel   string `json:"channel"`
		Workspace string `json:"workspace"`
		Bot       string `json:"bot"`
		MaxRole   string `json:"max_role"`
		Foreign   int    `json:"foreign"`
	}](t, list.Stdout)

	if len(rows) != 1 {
		t.Fatalf("listed %+v; want the one delegation authored", rows)
	}
	row := rows[0]
	if row.Channel != "#list-ops" || row.Workspace != "deleg-list-guest" {
		t.Errorf("row = %+v; want the delegation on the declared channel", row)
	}
	if row.Bot != "wsm-list-guest" || row.MaxRole != "op" {
		t.Errorf("row = %+v; want the authored bot at the op ceiling", row)
	}
	// No daemon has reconciled, so nothing foreign has been observed.
	if row.Foreign != 0 {
		t.Errorf("foreign = %d; no reconcile has run", row.Foreign)
	}
}

// TestIRCDelegate_AddWithoutChannelCoversEveryTemplate pins the omitted-flag
// shape against the real CLI: a delegation with no --channel reaches every
// channel the host declares.
func TestIRCDelegate_AddWithoutChannelCoversEveryTemplate(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	hostWithChannel(t, r, "deleg-all-host", "#all-one", "")
	second := r.runWithEnv(env, "irc", "channel", "add", "deleg-all-host", "#all-two",
		"--members", "all", "--format", "json")
	requireExitCode(t, second, 0)
	r.mustCreateWorkspace("deleg-all-guest")

	add := r.runWithEnv(env, "irc", "delegate", "add", "deleg-all-host",
		"deleg-all-guest", "--bot", "wsm-all-guest", "--max-role", "voice",
		"--format", "json")
	requireExitCode(t, add, 0)

	var doc struct {
		Channels int `json:"channels"`
	}
	mustUnmarshalJSON(t, add.Stdout, &doc)
	if doc.Channels != 2 {
		t.Fatalf("channels = %d; an omitted --channel must cover both templates",
			doc.Channels)
	}

	list := r.runWithEnv(env, "irc", "delegate", "list", "deleg-all-host",
		"--format", "json")
	requireExitCode(t, list, 0)
	rows := mustUnmarshalJSONArray[struct {
		Channel string `json:"channel"`
	}](t, list.Stdout)
	if len(rows) != 2 {
		t.Fatalf("listed %+v; want a row per channel", rows)
	}
}

// TestIRCDelegate_UnknownChannelIsNotFound pins that --channel naming a
// template the host does not declare is reported rather than writing nothing
// and claiming success.
func TestIRCDelegate_UnknownChannelIsNotFound(t *testing.T) {
	r := newRunner(t)
	env := extrasEnv(t, r, "managed")
	hostWithChannel(t, r, "deleg-chan404-host", "#declared", "")
	r.mustCreateWorkspace("deleg-chan404-guest")

	res := r.runWithEnv(env, "irc", "delegate", "add", "deleg-chan404-host",
		"deleg-chan404-guest", "--bot", "wsm-c404", "--channel", "#undeclared",
		"--max-role", "voice")
	requireExitCode(t, res, 3)
}

// TestIRCDelegate_DisabledGate locks the feature gate on the new verbs.
func TestIRCDelegate_DisabledGate(t *testing.T) {
	r := newRunner(t)
	r.mustCreateWorkspace("deleg-gate")

	for _, args := range [][]string{
		{"irc", "delegate", "add", "deleg-gate", "guest"},
		{"irc", "delegate", "rm", "deleg-gate", "guest"},
		{"irc", "delegate", "list", "deleg-gate"},
	} {
		res := r.run(args...)
		requireExitCode(t, res, 2)
	}
}

// mustReadConfig reads a workspace override, treating absence as empty.
func mustReadConfig(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".wsm", "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read config: %v", err)
	}
	return string(raw)
}
