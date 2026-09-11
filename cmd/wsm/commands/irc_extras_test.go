package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/model"
	"hop.top/wsm/pkg/workspace"
)

// TestIRCCreds_EnvOmitsAbsentPassword pins the rule that makes --env
// safe to source: a missing password produces no assignment at all.
//
// An empty WSM_IRC_CLIENT_PASSWORD= line would overwrite whatever the
// member already had exported, turning "wsm holds nothing for you" into
// "wsm just cleared your credential".
func TestIRCCreds_EnvOmitsAbsentPassword(t *testing.T) {
	withPassword := renderCredsEnvString(t, ircCredsDoc{
		Account: "noor", Host: "127.0.0.1", Port: 6700, Password: "s3cret",
	})
	if !strings.Contains(withPassword, EnvClientPassword+"='s3cret'") {
		t.Fatalf("expected the password assignment, got:\n%s", withPassword)
	}

	without := renderCredsEnvString(t, ircCredsDoc{
		Account: "noor", Host: "127.0.0.1", Port: 6700,
	})
	if strings.Contains(without, EnvClientPassword) {
		t.Fatalf("a held-nothing result must emit no password line, got:\n%s", without)
	}
	for _, want := range []string{
		EnvClientServer + "='127.0.0.1'",
		EnvClientPort + "=6700",
		EnvClientTLS + "=false",
		EnvClientAccount + "='noor'",
	} {
		if !strings.Contains(without, want) {
			t.Fatalf("expected %q in:\n%s", want, without)
		}
	}
}

// TestIRCCreds_EnvNamesAreNotConfigNames locks the namespace split. The
// client variables must never collide with the WSM_IRC_* names the
// config layer reads, or sourcing credentials would silently re-point
// wsm itself at whatever was printed.
func TestIRCCreds_EnvNamesAreNotConfigNames(t *testing.T) {
	configNames := map[string]bool{
		irc.EnvEnabled: true, irc.EnvMode: true, irc.EnvProvider: true,
		irc.EnvAutoProvision: true, irc.EnvBinary: true, irc.EnvBind: true,
		irc.EnvRemoteHost: true, irc.EnvServerHost: true, irc.EnvServerPort: true,
	}
	for _, name := range []string{
		EnvClientServer, EnvClientPort, EnvClientTLS, EnvClientAccount, EnvClientPassword,
	} {
		if configNames[name] {
			t.Fatalf("client variable %q collides with a wsm config variable", name)
		}
		if !strings.HasPrefix(name, "WSM_IRC_CLIENT_") {
			t.Fatalf("client variable %q must carry the CLIENT infix", name)
		}
	}
}

// TestIRCCreds_EnvQuotingSurvivesShellExpansion pins single-quoting: a
// passphrase carrying $ or a backtick must reach the client verbatim
// rather than being expanded by the shell that sources it.
func TestIRCCreds_EnvQuotingSurvivesShellExpansion(t *testing.T) {
	out := renderCredsEnvString(t, ircCredsDoc{
		Account: "noor", Host: "h", Port: 1, Password: "a$b`c'd",
	})
	if !strings.Contains(out, `WSM_IRC_CLIENT_PASSWORD='a$b`+"`"+`c'\''d'`) {
		t.Fatalf("expected a single-quoted, escape-safe password, got:\n%s", out)
	}
}

// renderCredsEnvString captures renderIRCCredsEnv's output.
func renderCredsEnvString(t *testing.T, doc ircCredsDoc) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "env")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if rerr := renderIRCCredsEnv(f, doc); rerr != nil {
		_ = f.Close()
		t.Fatalf("renderIRCCredsEnv: %v", rerr)
	}
	if cerr := f.Close(); cerr != nil {
		t.Fatalf("close: %v", cerr)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return string(raw)
}

// TestIRCChannel_OverrideMergePreservesForeignKeys is the regression
// guard for the whole reason this edit goes through a yaml.Node: a
// typed round-trip would serialise only the fields wsm models and
// silently delete everything else in the operator's config file.
func TestIRCChannel_OverrideMergePreservesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := strings.Join([]string{
		"# operator's own notes",
		"some_other_tool:",
		"  keep: me",
		"irc:",
		"  channel_prefix: acme-",
		"  channels:",
		"    - name: \"#existing\"",
		"      members: all",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	err := upsertChannelTemplate(path, irc.ChannelTemplate{
		Name: "#added", Members: irc.MembersPrincipals,
	})
	if err != nil {
		t.Fatalf("upsertChannelTemplate: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(raw)

	for _, want := range []string{"some_other_tool", "keep: me", "channel_prefix: acme-", "#existing", "#added"} {
		if !strings.Contains(got, want) {
			t.Fatalf("edit dropped %q from the document:\n%s", want, got)
		}
	}
}

// TestIRCChannel_OverrideMergeReplacesSameName pins that re-adding a
// channel updates it in place rather than duplicating the entry, which
// would leave the daemon reconciling one name from two templates.
func TestIRCChannel_OverrideMergeReplacesSameName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	if err := upsertChannelTemplate(path, irc.ChannelTemplate{
		Name: "#ops", Members: irc.MembersAll,
	}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := upsertChannelTemplate(path, irc.ChannelTemplate{
		Name: "#ops", Members: irc.MembersPrincipals, Topic: "second",
	}); err != nil {
		t.Fatalf("second add: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if n := strings.Count(string(raw), "#ops"); n != 1 {
		t.Fatalf("expected one #ops entry, found %d:\n%s", n, string(raw))
	}
	if !strings.Contains(string(raw), "principals") {
		t.Fatalf("expected the entry to be replaced, got:\n%s", string(raw))
	}
}

// TestIRCChannel_RemoveLastDropsTheKey pins the merge semantics: an
// empty list is not the same statement as no list. A workspace override
// that declares `channels: []` claims this workspace has no channels,
// replacing the global set; removing the last entry must instead leave
// the key absent so the global list applies again.
func TestIRCChannel_RemoveLastDropsTheKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := upsertChannelTemplate(path, irc.ChannelTemplate{Name: "#only"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := removeChannelTemplate(path, "#only"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(raw), "channels") {
		t.Fatalf("expected the channels key to be gone, got:\n%s", string(raw))
	}
}

// TestIRCChannel_RemoveUnknownIsNotFound pins exit 3 for a channel that
// was never declared: nothing was removed, and reporting success would
// tell a script the opposite.
func TestIRCChannel_RemoveUnknownIsNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := removeChannelTemplate(path, "#never-declared")
	if err == nil {
		t.Fatal("expected removing an undeclared channel to fail")
	}
	if !workspace.IsKind(err, workspace.ErrorKindNotFound) {
		t.Fatalf("expected a not-found error, got %T: %v", err, err)
	}
}

// TestIRCChannel_MatchesGlob pins the creator-namespace matcher against
// the exact shape irc.CreatorGlob produces.
//
// The bare stem must NOT match: the glob is "<stem>-*", and a channel
// named exactly the stem is not inside the namespace, it is the name the
// namespace was derived from. Anything strictly longer is inside it.
func TestIRCChannel_MatchesGlob(t *testing.T) {
	glob := irc.CreatorGlob("acme-", "Team Reed", "sami")

	cases := []struct {
		name string
		want bool
	}{
		{strings.TrimSuffix(glob, "*"), false},
		{strings.TrimSuffix(glob, "*") + "notes", true},
		{strings.TrimSuffix(glob, "*") + "a", true},
		{"#unrelated", false},
		{"#acme-team-reed-other-x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesGlob(glob, tc.name); got != tc.want {
				t.Fatalf("matchesGlob(%q, %q) = %v; want %v", glob, tc.name, got, tc.want)
			}
		})
	}
}

// TestIRCChannel_CreatorNamespacesOnlyCreators pins that a member
// without ChannelCreator contributes no namespace: a non-creator's
// label must never fence off a channel name.
func TestIRCChannel_CreatorNamespacesOnlyCreators(t *testing.T) {
	members := []model.Member{
		{ProfileID: "p1", AuditLabel: "sami", Capacity: model.CapacityPrincipal, ChannelCreator: true},
		{ProfileID: "p2", AuditLabel: "noor", Capacity: model.CapacityPrincipal},
	}
	got := creatorNamespaces(irc.Config{ChannelPrefix: "acme-"}, "Team", members)

	if _, ok := got["sami"]; !ok {
		t.Fatalf("expected a namespace for the creator, got %v", got)
	}
	if _, ok := got["noor"]; ok {
		t.Fatalf("a non-creator must contribute no namespace, got %v", got)
	}
}

// TestIRCChannel_ParseGuests pins the guest allow-list parsing,
// including the deliberate default: a guest with no explicit role gets
// voice, not op. The safe default for granting a persistent privilege
// to somebody outside the workspace is the one that lets them speak.
func TestIRCChannel_ParseGuests(t *testing.T) {
	guests, err := parseGuests([]string{"alice", "bob:op", "carol:voice"})
	if err != nil {
		t.Fatalf("parseGuests: %v", err)
	}
	want := []irc.Guest{
		{Profile: "alice", Role: irc.RoleVoice},
		{Profile: "bob", Role: irc.RoleOp},
		{Profile: "carol", Role: irc.RoleVoice},
	}
	if len(guests) != len(want) {
		t.Fatalf("parseGuests returned %d entries; want %d", len(guests), len(want))
	}
	for i := range want {
		if guests[i] != want[i] {
			t.Fatalf("guest %d = %+v; want %+v", i, guests[i], want[i])
		}
	}

	// halfop is a delegation ceiling, never a direct guest grant.
	if _, err := parseGuests([]string{"dave:halfop"}); err == nil {
		t.Fatal("expected --guest with halfop to be refused")
	}
}

// TestIRCChannel_ValidateChannelName pins the usage envelope for a
// name IRC could never accept.
func TestIRCChannel_ValidateChannelName(t *testing.T) {
	if err := validateChannelName("#fine"); err != nil {
		t.Fatalf("validateChannelName(#fine) = %v; want nil", err)
	}
	for _, bad := range []string{"", "no-sigil", "#has space", "#has,comma"} {
		t.Run(bad, func(t *testing.T) {
			err := validateChannelName(bad)
			if err == nil {
				t.Fatalf("expected %q to be refused", bad)
			}
			var oe *output.Error
			if !errors.As(err, &oe) || oe.ExitCode != 2 {
				t.Fatalf("expected a usage envelope (exit 2), got %T: %v", err, err)
			}
		})
	}
}

// TestIRCChannel_TemplateMembersListNeedsList pins that --members list
// without --member-list is refused rather than written as a template
// that selects nobody.
func TestIRCChannel_TemplateMembersListNeedsList(t *testing.T) {
	_, err := ircChannelFlags{Members: "list"}.template("#c")
	if err == nil {
		t.Fatal("expected --members list without --member-list to be refused")
	}
	var oe *output.Error
	if !errors.As(err, &oe) || oe.ExitCode != 2 {
		t.Fatalf("expected exit 2, got %T: %v", err, err)
	}

	tmpl, err := ircChannelFlags{Members: "list", MemberList: []string{"p1"}}.template("#c")
	if err != nil {
		t.Fatalf("with a member list: %v", err)
	}
	if tmpl.Members != irc.MembersList || len(tmpl.MemberList) != 1 {
		t.Fatalf("template = %+v; want a list selector carrying one profile", tmpl)
	}
}

// TestIRCAccount_UnsupportedNamesProviderAndOperation pins the exit-7
// contract: the refusal must name both halves, because "unsupported"
// alone leaves the operator guessing which one to fix.
func TestIRCAccount_UnsupportedNamesProviderAndOperation(t *testing.T) {
	profile := irc.Profile{Provider: irc.ProviderPlain}

	for _, op := range []ircAccountOp{opCreate, opRotate, opSuspend} {
		t.Run(string(op), func(t *testing.T) {
			err := unsupportedByProvider(profile, op, "noor")
			if !workspace.IsKind(err, workspace.ErrorKindPrerequisite) {
				t.Fatalf("expected a prerequisite error (exit 7), got %T: %v", err, err)
			}
			msg := err.Error()
			if !strings.Contains(msg, string(irc.ProviderPlain)) {
				t.Fatalf("message must name the provider, got: %s", msg)
			}
			if !strings.Contains(msg, string(op)) {
				t.Fatalf("message must name the operation, got: %s", msg)
			}
		})
	}
}

// TestIRCAccount_CapabilityPerOperation pins each verb to the
// capability it actually needs, so a provider that can create but not
// suspend is refused only for suspend.
func TestIRCAccount_CapabilityPerOperation(t *testing.T) {
	for _, op := range []ircAccountOp{opCreate, opRotate, opSuspend} {
		if op.capFor() == 0 {
			t.Fatalf("operation %q maps to no capability", op)
		}
	}
	// Distinct capabilities are the point: a provider that can create an
	// account but cannot suspend one must be refused for suspend alone.
	if opCreate.capFor() == opSuspend.capFor() {
		t.Fatal("create and suspend must not share a capability")
	}
	if opCreate.capFor() == opRotate.capFor() {
		t.Fatal("create and rotate must not share a capability")
	}
	if opRotate.capFor() == opSuspend.capFor() {
		t.Fatal("rotate and suspend must not share a capability")
	}
}

// TestIRCAccount_TargetValidation pins that exactly one of member or
// --raw is required: neither leaves nothing to act on, both is an
// ambiguous invocation.
func TestIRCAccount_TargetValidation(t *testing.T) {
	if err := validateAccountTarget("", ""); err == nil {
		t.Fatal("expected neither-member-nor-raw to be refused")
	}
	if err := validateAccountTarget("noor", "guest"); err == nil {
		t.Fatal("expected member plus --raw to be refused")
	}
	if err := validateAccountTarget("noor", ""); err != nil {
		t.Fatalf("a member alone must be accepted, got %v", err)
	}
	if err := validateAccountTarget("", "guest"); err != nil {
		t.Fatalf("--raw alone must be accepted, got %v", err)
	}
}

// TestIRCURL_PerMode pins the URL each mode and flag combination
// produces, including the two refusals.
func TestIRCURL_PerMode(t *testing.T) {
	managed := &ircRuntime{
		ws:  &model.Workspace{Name: "team", ID: "ws1"},
		cfg: irc.Config{Remote: irc.Remote{Host: "chat.example.com"}},
	}
	managedProfile := irc.Profile{
		Mode:   irc.ModeManaged,
		Ports:  irc.Ports{Plain: 6700, Remote: 6701, API: 6702},
		Remote: true,
	}

	t.Run("managed loopback", func(t *testing.T) {
		doc, err := ircClientURL(managed, managedProfile, false)
		if err != nil {
			t.Fatalf("ircClientURL: %v", err)
		}
		if doc.URL != "irc://127.0.0.1:6700" {
			t.Fatalf("url = %q; want irc://127.0.0.1:6700", doc.URL)
		}
	})

	t.Run("managed web", func(t *testing.T) {
		doc, err := ircClientURL(managed, managedProfile, true)
		if err != nil {
			t.Fatalf("ircClientURL: %v", err)
		}
		if doc.URL != "wss://chat.example.com:6701" {
			t.Fatalf("url = %q; want wss://chat.example.com:6701", doc.URL)
		}
	})

	t.Run("managed web without a remote listener", func(t *testing.T) {
		noRemote := managedProfile
		noRemote.Remote = false
		_, err := ircClientURL(managed, noRemote, true)
		if err == nil {
			t.Fatal("expected --web to be refused without a remote listener")
		}
		var oe *output.Error
		if !errors.As(err, &oe) || oe.ExitCode != 2 {
			t.Fatalf("expected exit 2, got %T: %v", err, err)
		}
	})

	external := &ircRuntime{
		ws:  &model.Workspace{Name: "team", ID: "ws1"},
		cfg: irc.Config{Server: irc.Server{Host: "irc.example.com", Port: 6697}},
	}
	externalProfile := irc.Profile{Mode: irc.ModeExternal}

	t.Run("external tls", func(t *testing.T) {
		doc, err := ircClientURL(external, externalProfile, false)
		if err != nil {
			t.Fatalf("ircClientURL: %v", err)
		}
		if doc.URL != "ircs://irc.example.com:6697" {
			t.Fatalf("url = %q; want ircs://irc.example.com:6697", doc.URL)
		}
	})

	t.Run("external plaintext", func(t *testing.T) {
		plain := external
		off := false
		plain.cfg.Server.TLS = &off
		doc, err := ircClientURL(plain, externalProfile, false)
		if err != nil {
			t.Fatalf("ircClientURL: %v", err)
		}
		if !strings.HasPrefix(doc.URL, "irc://") {
			t.Fatalf("url = %q; want an irc:// scheme with TLS off", doc.URL)
		}
	})

	t.Run("external web is refused", func(t *testing.T) {
		_, err := ircClientURL(external, externalProfile, true)
		if err == nil {
			t.Fatal("expected --web to be refused in external mode")
		}
		var oe *output.Error
		if !errors.As(err, &oe) || oe.ExitCode != 2 {
			t.Fatalf("expected exit 2, got %T: %v", err, err)
		}
	})
}

// TestIRCBackfill_NormalizeChannels pins de-duplication and ordering, so
// a filed request records what was asked for exactly once each.
func TestIRCBackfill_NormalizeChannels(t *testing.T) {
	got := normalizeChannels([]string{" #b ", "#a", "#b", "", "   "})
	want := []string{"#a", "#b"}
	if len(got) != len(want) {
		t.Fatalf("normalizeChannels = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeChannels = %v; want %v", got, want)
		}
	}
	if normalizeChannels(nil) != nil {
		t.Fatal("no channels must stay nil, meaning every channel with a cursor")
	}
}

// TestIRCBackfill_RequestIsAtomicAndReadable pins the request file's
// shape: the daemon may read it the instant the signal lands, so it
// must be complete and parseable, and it must carry the id the report
// is later correlated by.
func TestIRCBackfill_RequestIsAtomicAndReadable(t *testing.T) {
	state := t.TempDir()
	paths := irc.Paths{StateDir: state}

	if err := writeBackfillRequest(paths, "bf-test", []string{"#a"}); err != nil {
		t.Fatalf("writeBackfillRequest: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(state, backfillRequestName))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	for _, want := range []string{`"id": "bf-test"`, `"#a"`, `"at"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("request missing %q:\n%s", want, string(raw))
		}
	}

	// No temporary file may survive: a leftover .backfill-*.json would
	// accumulate in the state dir on every invocation.
	entries, err := os.ReadDir(state)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".backfill-") {
			t.Fatalf("temporary request file %q was left behind", e.Name())
		}
	}
}
