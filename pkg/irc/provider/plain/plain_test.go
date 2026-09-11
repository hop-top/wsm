package plain

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot/bottest"
	"hop.top/wsm/pkg/irc/provider"
)

// probes is the captured-evidence directory, relative to this package.
const probes = "../../../../docs/plans/2026-09-10-irc-workspace-chat-design/probes"

// newProvider builds the trio over a fake transport, returning both so a test
// can drive the transport and assert on the provider.
func newProvider(t *testing.T, names map[string]string) (*bottest.FakeTransport, provider.Providers) {
	t.Helper()

	f := bottest.New("multi-prefix")
	f.SetNames(names)

	providers, err := New(provider.Deps{Transport: f})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := providers.Roles.(interface{ Close() }); ok {
			closer.Close()
		}
	})
	return f, providers
}

// oppedBot is the NAMES table for a channel where the bot holds +o alone.
func oppedBot() map[string]string {
	return map[string]string{irc.DefaultBotAccount: "@"}
}

func TestRegisteredUnderPlain(t *testing.T) {
	// init() must have registered the factory, so a config naming "plain"
	// resolves without the caller importing this package for its symbols.
	var found bool
	for _, name := range provider.Registered() {
		if name == Name {
			found = true
		}
	}
	if !found {
		t.Fatalf("Registered() = %v, want it to contain %q", provider.Registered(), Name)
	}

	f := bottest.New()
	providers, err := provider.Open(Name, provider.Deps{Transport: f})
	if err != nil {
		t.Fatalf("Open(%q): %v", Name, err)
	}
	if providers.Accounts == nil || providers.Roles == nil || providers.Directory == nil {
		t.Fatal("Open returned a nil provider; every field must be populated")
	}
}

func TestNewRequiresTransport(t *testing.T) {
	if _, err := New(provider.Deps{}); err == nil {
		t.Fatal("New with no transport returned no error")
	}
}

func TestAccountsUnsupported(t *testing.T) {
	_, providers := newProvider(t, nil)
	accounts := providers.Accounts
	ctx := context.Background()

	if got := accounts.Caps(); got != 0 {
		t.Errorf("Caps() = %d, want 0: a plain server has no account layer", got)
	}

	// Every method must report the same sentinel, so reconcile skips and
	// reports rather than treating a plain server as broken.
	if err := accounts.CreateAccount(ctx, "alice", "secret"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("CreateAccount err = %v, want ErrUnsupported", err)
	}
	if err := accounts.SetPassword(ctx, "alice", "secret"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("SetPassword err = %v, want ErrUnsupported", err)
	}
	if err := accounts.Suspend(ctx, "alice"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("Suspend err = %v, want ErrUnsupported", err)
	}
}

func TestAccountErrorsNameTheAccount(t *testing.T) {
	_, providers := newProvider(t, nil)

	// The operator has to know which account failed, so the sentinel is
	// wrapped with it rather than returned bare.
	err := providers.Accounts.CreateAccount(context.Background(), "alice", "secret")
	if !strings.Contains(err.Error(), "alice") {
		t.Errorf("error %q should name the account", err)
	}
}

func TestDirectoryCapsListChannelsOnly(t *testing.T) {
	_, providers := newProvider(t, nil)

	if got := providers.Directory.Caps(); got != provider.CapListChannels {
		t.Fatalf("Caps() = %d, want exactly CapListChannels (%d)", got, provider.CapListChannels)
	}
}

func TestDirectoryUnsupportedBeyondListing(t *testing.T) {
	_, providers := newProvider(t, nil)
	dir := providers.Directory
	ctx := context.Background()

	if _, err := dir.Founder(ctx, "#a"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("Founder err = %v, want ErrUnsupported", err)
	}
	if err := dir.JoinHidden(ctx, "#a"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("JoinHidden err = %v, want ErrUnsupported", err)
	}
	if err := dir.Purge(ctx, "#a"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("Purge err = %v, want ErrUnsupported", err)
	}

	// A plain server sends no relay tags, so nothing is ever decoded.
	if _, ok := dir.RelayedIdentity(bottest.Msg("PRIVMSG", "#a", "hi")); ok {
		t.Error("RelayedIdentity reported a relay on a plain server")
	}
}

func TestListChannelsFromLIST(t *testing.T) {
	f, providers := newProvider(t, nil)
	f.SetScript(map[string][]ircmsg.Message{
		"LIST": {
			bottest.Msg("321", "botty", "Channel", "Users  Name"),
			bottest.Msg("322", "botty", "#ops", "3", "ops and chatter"),
			bottest.Msg("322", "botty", "#quiet", "1", ""),
			bottest.Msg("323", "botty", "End of /LIST"),
		},
	})

	got, err := providers.Directory.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListChannels returned %d entries, want 2 (the 321/323 markers are not channels)", len(got))
	}

	first := got[0]
	if first.Name != "#ops" || first.Users != 3 || first.Topic != "ops and chatter" {
		t.Errorf("first entry = %+v, want #ops/3/topic", first)
	}

	// A plain server has no registration database, so every channel it
	// reports is unregistered with no founder. Saying so explicitly stops a
	// caller reading a missing founder as an unknown one.
	for _, info := range got {
		if info.Registered {
			t.Errorf("%s reported Registered; a plain server registers nothing", info.Name)
		}
		if info.Founder != "" {
			t.Errorf("%s reported founder %q; a plain server records none", info.Name, info.Founder)
		}
	}
}

func TestListChannelsSkipsMalformedEntries(t *testing.T) {
	f, providers := newProvider(t, nil)
	f.SetScript(map[string][]ircmsg.Message{
		"LIST": {
			bottest.Msg("322", "botty"),                     // too short
			bottest.Msg("322", "botty", "notachannel", "2"), // not a channel name
			bottest.Msg("322", "botty", "#good", "notanumber"),
		},
	})

	got, err := providers.Directory.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	// One odd line must not discard an otherwise good listing.
	if len(got) != 1 || got[0].Name != "#good" {
		t.Fatalf("got %+v, want just #good", got)
	}
	if got[0].Users != 0 {
		t.Errorf("Users = %d, want 0 for an unreadable count", got[0].Users)
	}
}

func TestRolesCapsZero(t *testing.T) {
	_, providers := newProvider(t, nil)

	// No persistence and no registration: the caller reads that from the
	// bitset rather than from a failed call.
	if got := providers.Roles.Caps(); got != 0 {
		t.Fatalf("Caps() = %d, want 0", got)
	}
}

func TestListRolesFromNamesPrefixes(t *testing.T) {
	_, providers := newProvider(t, map[string]string{
		"carol": "@",
		"bob":   "%",
		"alice": "+",
		"dave":  "",
	})

	got, err := providers.Roles.ListRoles(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}

	want := map[string]irc.Role{
		"carol": irc.RoleOp,
		"bob":   irc.RoleHalfop,
		"alice": irc.RoleVoice,
	}
	for nick, role := range want {
		if got[nick] != role {
			t.Errorf("%s = %q, want %q", nick, got[nick], role)
		}
	}
	// A prefixless occupant holds nothing and must not appear at all: an
	// empty role in the map would diff as a role wsm could revoke.
	if _, ok := got["dave"]; ok {
		t.Errorf("dave appears with role %q, want absent", got["dave"])
	}
}

func TestListRolesMultiPrefixRunTakesHighest(t *testing.T) {
	// Straight from the live ergo 2.19.1 capture: the founder carries two
	// prefix chars. Reading only the first character would still say op
	// here, so the run is also checked in the order that would misparse.
	_, providers := newProvider(t, map[string]string{
		"botty":    "~@",
		"admin":    "&@",
		"voiced":   "+",
		"hopvoice": "%+",
		// A run whose FIRST character is not its highest privilege. This is
		// what separates consuming the run from reading its first byte:
		// every other case here happens to lead with its highest prefix, so
		// a first-character parser would pass them all.
		"outoforder": "+@",
	})

	got, err := providers.Roles.ListRoles(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}

	if got["botty"] != irc.RoleOp {
		t.Errorf("botty = %q, want op: a ~@ run is an opped founder, not a misparse", got["botty"])
	}
	if got["admin"] != irc.RoleOp {
		t.Errorf("admin = %q, want op for a &@ run", got["admin"])
	}
	// Highest wins, not first-seen: %+ is a halfop who also holds voice.
	if got["hopvoice"] != irc.RoleHalfop {
		t.Errorf("hopvoice = %q, want halfop for a %%+ run", got["hopvoice"])
	}
	// The discriminating case: a run led by its lowest prefix. Reading the
	// first character here would report voice and silently under-privilege
	// an operator.
	if got["outoforder"] != irc.RoleOp {
		t.Errorf("outoforder = %q, want op for a +@ run: the run must be scanned in precedence order", got["outoforder"])
	}
}

func TestListRolesMatchesCapturedFixture(t *testing.T) {
	// The fixture is the evidence the parsing rule was written from; parsing
	// it here keeps the rule pinned to the capture rather than to a
	// hand-written restatement of it.
	msgs, err := bottest.Lines(filepath.Join(probes, "names-multi-prefix.txt"))
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}

	var entries []string
	for _, msg := range msgs {
		if msg.Command != "353" {
			continue
		}
		trailing := msg.Params[len(msg.Params)-1]
		if strings.Contains(trailing, "~@botty") && strings.Contains(trailing, "carol") {
			entries = strings.Fields(trailing)
		}
	}
	if len(entries) != 4 {
		t.Fatalf("fixture entries = %v, want the four-occupant 353", entries)
	}

	// Split each captured entry the way a NAMES table arrives: prefix run,
	// then nick.
	names := map[string]string{}
	for _, entry := range entries {
		i := 0
		for i < len(entry) && strings.ContainsRune(Prefixes, rune(entry[i])) {
			i++
		}
		names[entry[i:]] = entry[:i]
	}

	f := bottest.New("multi-prefix")
	f.SetNames(names)
	providers, err := New(provider.Deps{Transport: f})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := providers.Roles.ListRoles(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	want := map[string]irc.Role{
		"botty": irc.RoleOp,
		"carol": irc.RoleOp,
		"bob":   irc.RoleHalfop,
		"alice": irc.RoleVoice,
	}
	for nick, role := range want {
		if got[nick] != role {
			t.Errorf("%s = %q, want %q", nick, got[nick], role)
		}
	}
}

func TestListRolesNamesError(t *testing.T) {
	f, providers := newProvider(t, nil)
	boom := errors.New("boom")
	f.SetNamesErr(boom)

	if _, err := providers.Roles.ListRoles(context.Background(), "#a"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the transport error", err)
	}
}

func TestMyRoleFromNames(t *testing.T) {
	cases := []struct {
		name string
		held string
		want irc.Role
	}{
		{"opped", "@", irc.RoleOp},
		{"founder run", "~@", irc.RoleOp},
		{"halfop", "%", irc.RoleHalfop},
		{"voice", "+", irc.RoleVoice},
		{"nothing", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, providers := newProvider(t, map[string]string{irc.DefaultBotAccount: tc.held})

			got, err := providers.Roles.MyRole(context.Background(), "#a")
			if err != nil {
				t.Fatalf("MyRole: %v", err)
			}
			if got != tc.want {
				t.Fatalf("MyRole = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMyRoleAbsentBotHoldsNothing(t *testing.T) {
	// A bot not in the channel at all is not an error: reconcile caps grants
	// against the empty role and ApplyRoles reports not-opped separately.
	_, providers := newProvider(t, map[string]string{"someone": "@"})

	got, err := providers.Roles.MyRole(context.Background(), "#a")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != "" {
		t.Fatalf("MyRole = %q, want empty", got)
	}
}

func TestEnsureChannelJoinsAndSetsModes(t *testing.T) {
	f, providers := newProvider(t, oppedBot())

	err := providers.Roles.EnsureChannel(context.Background(), provider.Channel{
		Name:  "#ops",
		Modes: "+ntR",
		Topic: "operations",
	})
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}

	sent := f.SentLines()
	wantOrder := []string{"JOIN #ops", "MODE #ops +ntR"}
	for i, want := range wantOrder {
		if i >= len(sent) || sent[i] != want {
			t.Fatalf("sent = %q, want %q at index %d", sent, want, i)
		}
	}
	if !contains(sent, "TOPIC #ops :operations") {
		t.Errorf("sent = %q, want a TOPIC line", sent)
	}
}

func TestEnsureChannelTopicOnlyWhenOpped(t *testing.T) {
	// The template default is +t, so an unopped TOPIC is refused by the
	// server. Reporting not-opped lets reconcile degrade the channel instead
	// of issuing a command that cannot work.
	f, providers := newProvider(t, map[string]string{irc.DefaultBotAccount: "+"})

	err := providers.Roles.EnsureChannel(context.Background(), provider.Channel{
		Name:  "#ops",
		Modes: "+nt",
		Topic: "operations",
	})
	if !errors.Is(err, provider.ErrNotOpped) {
		t.Fatalf("err = %v, want ErrNotOpped", err)
	}

	for _, line := range f.SentLines() {
		if strings.HasPrefix(line, "TOPIC") {
			t.Fatalf("sent %q; an unopped bot must not attempt the topic", line)
		}
	}
	// The join and the best-effort modes still happened: sitting in the
	// channel is worth having even when the topic is not settable.
	if !contains(f.SentLines(), "JOIN #ops") {
		t.Errorf("sent = %q, want the JOIN to have happened anyway", f.SentLines())
	}
}

func TestEnsureChannelRecords482RatherThanFailing(t *testing.T) {
	f, providers := newProvider(t, oppedBot())

	// A refused mode arrives asynchronously as 482, never as the return of
	// the mode command, so it is observed and recorded.
	f.Emit(bottest.MustParse(":irc.example 482 wsm #ops :You're not channel operator"))

	err := providers.Roles.EnsureChannel(context.Background(), provider.Channel{
		Name:  "#ops",
		Modes: "+ntR",
	})
	if err != nil {
		t.Fatalf("EnsureChannel = %v, want nil: a refused template mode is best-effort", err)
	}

	refused := providers.Roles.(interface{ Refused() []string }).Refused()
	if len(refused) != 1 || refused[0] != "#ops" {
		t.Fatalf("Refused() = %v, want [#ops]: the refusal must be recorded, not lost", refused)
	}
}

func TestEnsureChannelBorrowedJoinsOnly(t *testing.T) {
	f, providers := newProvider(t, oppedBot())

	err := providers.Roles.EnsureChannel(context.Background(), provider.Channel{
		Name:     "#host-ops",
		Modes:    "+ntR",
		Topic:    "not mine to set",
		Borrowed: true,
	})
	// The channel is joined, so the outcome is ErrForeignChannel rather than
	// nil: a plain server has no services, so the provider cannot claim to
	// have registered anything.
	if !errors.Is(err, provider.ErrForeignChannel) {
		t.Fatalf("EnsureChannel = %v, want ErrForeignChannel", err)
	}

	// The host owns a borrowed channel's modes and topic; wsm only sits in it.
	got := f.SentLines()
	if len(got) != 1 || got[0] != "JOIN #host-ops" {
		t.Fatalf("sent = %q, want just the JOIN", got)
	}
}

// TestEnsureChannelBorrowedNotReportedRegistered pins the status consequence of
// the borrowed outcome: reconcile records a nil return as Registered, and a
// services-less provider has no basis for that claim.
//
// The assertion is on how reconcile classifies the error, not on its wording,
// so it fails if the provider ever goes back to returning nil.
func TestEnsureChannelBorrowedNotReportedRegistered(t *testing.T) {
	_, providers := newProvider(t, oppedBot())

	err := providers.Roles.EnsureChannel(context.Background(), provider.Channel{
		Name:     "#host-ops",
		Borrowed: true,
	})

	// nil is what reconcile turns into Registered=true. Anything else must
	// not be the host-not-registered sentinel either, since wsm did join.
	if err == nil {
		t.Fatal("EnsureChannel = nil: reconcile records that as Registered, " +
			"but a plain server registers nothing and cannot know whether the host did")
	}
	if errors.Is(err, provider.ErrHostChannelNotRegistered) {
		t.Fatalf("EnsureChannel = %v, want ErrForeignChannel: "+
			"the channel was joined, so it must not be reported as absent on the host", err)
	}
	if !errors.Is(err, provider.ErrForeignChannel) {
		t.Fatalf("EnsureChannel = %v, want ErrForeignChannel", err)
	}
	if !strings.Contains(err.Error(), "#host-ops") {
		t.Errorf("error %q should name the channel", err)
	}
}

// TestEnsureChannelBorrowedStillAppliesRoles checks that only the reported
// status changed: the JOIN still happens and roles are still applied in the
// borrowed channel afterwards.
//
// reconcile treats ErrForeignChannel as an expected borrowed outcome and falls
// through to role application, so a borrowed channel must remain fully usable.
func TestEnsureChannelBorrowedStillAppliesRoles(t *testing.T) {
	f, providers := newProvider(t, map[string]string{
		irc.DefaultBotAccount: "@",
		"alice":               "",
	})
	ctx := context.Background()

	if err := providers.Roles.EnsureChannel(ctx, provider.Channel{
		Name:     "#host-ops",
		Borrowed: true,
	}); !errors.Is(err, provider.ErrForeignChannel) {
		t.Fatalf("EnsureChannel = %v, want ErrForeignChannel", err)
	}
	if got := f.SentLines(); len(got) != 1 || got[0] != "JOIN #host-ops" {
		t.Fatalf("sent = %q, want the JOIN to have happened", got)
	}

	// The bot is opped here, so the grant goes through exactly as it would
	// in an owned channel.
	if err := providers.Roles.ApplyRoles(ctx, "#host-ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil,
	); err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}
	if !contains(f.SentLines(), "MODE #host-ops +o alice") {
		t.Errorf("sent = %q, want the role applied in the borrowed channel", f.SentLines())
	}
}

func TestApplyRolesPresentNicks(t *testing.T) {
	f, providers := newProvider(t, map[string]string{
		irc.DefaultBotAccount: "@",
		"alice":               "",
		"bob":                 "+",
	})

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}},
		[]provider.ModeChange{{Account: "bob", Role: irc.RoleVoice}},
	)
	if err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}

	sent := f.SentLines()
	if !contains(sent, "MODE #ops +o alice") {
		t.Errorf("sent = %q, want +o for the present nick", sent)
	}
	if !contains(sent, "MODE #ops -v bob") {
		t.Errorf("sent = %q, want -v for the present nick", sent)
	}
}

func TestApplyRolesNotOppedRefusesEverything(t *testing.T) {
	// The bot grants privilege it holds itself. Without +o the server refuses
	// every mode, so the honest answer is to send nothing and report.
	f, providers := newProvider(t, map[string]string{
		irc.DefaultBotAccount: "+",
		"alice":               "",
	})

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil)
	if !errors.Is(err, provider.ErrNotOpped) {
		t.Fatalf("err = %v, want ErrNotOpped", err)
	}

	for _, line := range f.SentLines() {
		if strings.HasPrefix(line, "MODE") {
			t.Fatalf("sent %q; an unopped bot must issue no modes", line)
		}
	}
}

func TestApplyRolesBotAbsentIsNotOpped(t *testing.T) {
	_, providers := newProvider(t, map[string]string{"alice": ""})

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleVoice}}, nil)
	if !errors.Is(err, provider.ErrNotOpped) {
		t.Fatalf("err = %v, want ErrNotOpped when the bot is not even present", err)
	}
}

func TestApplyRolesRefusesAboveMyRole(t *testing.T) {
	// Roles are capped at what the bot holds. A halfop bot cannot grant op,
	// and reconcile turns the refusal into a degraded reason.
	_, providers := newProvider(t, map[string]string{
		irc.DefaultBotAccount: "@",
		"alice":               "",
	})

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.Role("admin")}}, nil)
	if !errors.Is(err, provider.ErrPrivilege) {
		t.Fatalf("err = %v, want ErrPrivilege for a role above the bot's own", err)
	}
}

func TestApplyRolesAbsentNickBecomesPending(t *testing.T) {
	f, providers := newProvider(t, oppedBot())

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleVoice}}, nil)
	if err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}

	// Nothing can be sent for a nick that is not there.
	for _, line := range f.SentLines() {
		if strings.Contains(line, "alice") {
			t.Fatalf("sent %q for an absent nick", line)
		}
	}

	pending := providers.Roles.(interface {
		Pending(string) map[string]irc.Role
	}).Pending("#ops")
	if pending["alice"] != irc.RoleVoice {
		t.Fatalf("pending = %v, want alice at voice", pending)
	}
}

func TestPendingRoleAppliedOnJoin(t *testing.T) {
	// This is the heart of the provider: roles do not persist, so a member
	// who was offline during reconcile must still get their role when they
	// appear.
	f, providers := newProvider(t, oppedBot())

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil)
	if err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}

	f.Emit(bottest.MustParse(":alice!u@h JOIN #ops"))

	if !contains(f.SentLines(), "MODE #ops +o alice") {
		t.Fatalf("sent = %q, want the pending role applied on JOIN", f.SentLines())
	}

	// Applied once and cleared: a second JOIN must not re-issue it.
	pending := providers.Roles.(interface {
		Pending(string) map[string]irc.Role
	}).Pending("#ops")
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want empty once applied", pending)
	}
}

func TestPendingRoleIgnoresOtherChannelsAndNicks(t *testing.T) {
	f, providers := newProvider(t, oppedBot())

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil)
	if err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}

	// Right nick, wrong channel; then wrong nick, right channel.
	f.Emit(bottest.MustParse(":alice!u@h JOIN #other"))
	f.Emit(bottest.MustParse(":mallory!u@h JOIN #ops"))

	for _, line := range f.SentLines() {
		if strings.HasPrefix(line, "MODE") {
			t.Fatalf("sent %q; neither JOIN matches the pending grant", line)
		}
	}
}

func TestRevokeClearsPendingGrant(t *testing.T) {
	// A role added and then removed before the member ever appeared must not
	// arrive later as a surprise.
	f, providers := newProvider(t, oppedBot())
	roles := providers.Roles

	if err := roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil); err != nil {
		t.Fatalf("ApplyRoles add: %v", err)
	}
	if err := roles.ApplyRoles(context.Background(), "#ops", nil,
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}); err != nil {
		t.Fatalf("ApplyRoles remove: %v", err)
	}

	f.Emit(bottest.MustParse(":alice!u@h JOIN #ops"))

	for _, line := range f.SentLines() {
		if strings.HasPrefix(line, "MODE") {
			t.Fatalf("sent %q; the grant was revoked before it was ever applied", line)
		}
	}
}

func TestCloseStopsReapplyingOnJoin(t *testing.T) {
	// A cancelled subscription must stop re-applying: a provider whose owner
	// has gone away must not keep issuing modes on the transport.
	f, providers := newProvider(t, oppedBot())

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil)
	if err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}

	providers.Roles.(interface{ Close() }).Close()
	f.Emit(bottest.MustParse(":alice!u@h JOIN #ops"))

	for _, line := range f.SentLines() {
		if strings.HasPrefix(line, "MODE") {
			t.Fatalf("sent %q after Close; the subscription must be cancelled", line)
		}
	}
}

func TestJoinSubscribedOnce(t *testing.T) {
	f, providers := newProvider(t, oppedBot())
	roles := providers.Roles

	// Several passes must not stack subscriptions, or one JOIN would apply
	// the same mode several times.
	for range 3 {
		if err := roles.ApplyRoles(context.Background(), "#ops",
			[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil); err != nil {
			t.Fatalf("ApplyRoles: %v", err)
		}
	}

	f.Emit(bottest.MustParse(":alice!u@h JOIN #ops"))

	var modes int
	for _, line := range f.SentLines() {
		if strings.HasPrefix(line, "MODE") {
			modes++
		}
	}
	if modes != 1 {
		t.Fatalf("issued %d mode lines, want exactly 1", modes)
	}
}

func TestSetNickChangesWhoTheBotLooksFor(t *testing.T) {
	// The transport carries no identity, so a deployment whose bot nick is
	// not the default says so before the first reconcile.
	_, providers := newProvider(t, map[string]string{"wsm-acme": "@"})
	roles := providers.Roles

	if got, _ := roles.MyRole(context.Background(), "#a"); got != "" {
		t.Fatalf("MyRole = %q under the default nick, want empty", got)
	}

	roles.(interface{ SetNick(string) }).SetNick("wsm-acme")

	got, err := roles.MyRole(context.Background(), "#a")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != irc.RoleOp {
		t.Fatalf("MyRole = %q after SetNick, want op", got)
	}
}

func TestConcurrentApplyAndJoin(t *testing.T) {
	// The JOIN subscription fires on the transport's goroutine while
	// ApplyRoles runs on the caller's. Under -race this is what catches an
	// unguarded pending set.
	f, providers := newProvider(t, oppedBot())
	roles := providers.Roles

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 25 {
				_ = roles.ApplyRoles(context.Background(), "#ops",
					[]provider.ModeChange{{Account: "alice", Role: irc.RoleOp}}, nil)
			}
		}()
		go func() {
			defer wg.Done()
			for range 25 {
				f.Emit(bottest.MustParse(":alice!u@h JOIN #ops"))
			}
		}()
		_ = i
	}
	wg.Wait()

	// Also exercise the readers while nothing else holds the lock.
	_ = roles.(interface{ Refused() []string }).Refused()
	_ = roles.(interface {
		Pending(string) map[string]irc.Role
	}).Pending("#ops")
}

// contains reports whether lines holds want.
func contains(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// TestApplyRolesPresentButUnprivilegedIsNotOpped pins the empty role for a bot
// that IS in NAMES but carries no prefix.
//
// TestApplyRolesBotAbsentIsNotOpped covers the bot missing from the reply
// entirely; this is the other source of an empty role, and it is the one a
// host revoke produces — the bot is still in the channel, holding nothing.
// Both must refuse, because an empty role is the strictest ceiling rather than
// the absence of one.
//
// Voice is asserted rather than op: it is the lowest rung wsm can grant, so a
// gate that admits it admits everything.
func TestApplyRolesPresentButUnprivilegedIsNotOpped(t *testing.T) {
	f, providers := newProvider(t, map[string]string{
		irc.DefaultBotAccount: "", // present, no prefix: holds no role
		"alice":               "",
	})

	err := providers.Roles.ApplyRoles(context.Background(), "#ops",
		[]provider.ModeChange{{Account: "alice", Role: irc.RoleVoice}}, nil)
	if !errors.Is(err, provider.ErrNotOpped) {
		t.Fatalf("err = %v, want ErrNotOpped for a bot holding no role", err)
	}

	for _, line := range f.SentLines() {
		if strings.HasPrefix(line, "MODE") {
			t.Fatalf("sent %q; a bot holding no role must issue no modes", line)
		}
	}
}

// TestMyRoleUnprivilegedBotIsEmpty pins the value the ceiling is read from: a
// bot present with no prefix reports an empty role, not an error and not a
// default rung.
func TestMyRoleUnprivilegedBotIsEmpty(t *testing.T) {
	_, providers := newProvider(t, map[string]string{
		irc.DefaultBotAccount: "",
		"carol":               "@",
	})

	got, err := providers.Roles.MyRole(context.Background(), "#ops")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != "" {
		t.Errorf("role = %q, want empty for a bot holding no prefix", got)
	}
}
