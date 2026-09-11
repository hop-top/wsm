package ergo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot/bottest"
	"hop.top/wsm/pkg/irc/provider"
)

// notice builds a ChanServ NOTICE reply, which is how every ChanServ answer
// arrives.
func notice(text string) ircmsg.Message {
	return bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :" + text)
}

// sentContains reports whether any sent line starts with prefix.
func sentContains(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func TestRolesCapsMatrix(t *testing.T) {
	// Registering a channel needs only an account. Persistent roles on a
	// channel the bot does not own need the chanreg oper capability, so
	// persistence is claimed only when the bot holds oper.
	cases := []struct {
		name string
		oper bool
		want provider.Caps
	}{
		{
			name: "not oper",
			want: provider.CapRegisterChannel,
		},
		{
			name: "oper",
			oper: true,
			want: provider.CapRegisterChannel | provider.CapPersistentRoles,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := bottest.New()
			transport.SetOper(tc.oper)

			if got := (&Roles{transport: transport}).Caps(); got != tc.want {
				t.Errorf("caps = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestEnsureChannelBorrowedJoinsOnly pins the guest-side rule: once the host's
// registration is confirmed, a borrowed channel gets a JOIN and nothing else.
// Registering, setting modes or setting the topic would be a guest
// reconfiguring someone else's channel.
func TestEnsureChannelBorrowedJoinsOnly(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO": {
			notice("Channel #a-dev is registered"),
			notice("Founder: hostbot"),
		},
	})
	roles := &Roles{transport: transport, nick: "botty"}

	ch := provider.Channel{
		Name:     "#a-dev",
		Borrowed: true,
		Modes:    "nt",
		Topic:    "should not be set",
	}

	err := roles.EnsureChannel(context.Background(), ch)
	if !errors.Is(err, provider.ErrForeignChannel) {
		t.Fatalf("EnsureChannel = %v, want ErrForeignChannel", err)
	}

	sent := transport.SentLines()
	if !sentContains(sent, "JOIN #a-dev") {
		t.Fatalf("sent = %q, want a JOIN once the host registration is confirmed", sent)
	}
	// CS INFO is the one ChanServ command a borrower may issue: it asks a
	// question rather than changing anything.
	for _, forbidden := range []string{"CS REGISTER", "CS AMODE", "CS TRANSFER", "MODE", "TOPIC"} {
		if sentContains(sent, forbidden) {
			t.Errorf("a borrowed channel must not issue %s", forbidden)
		}
	}
}

// TestEnsureChannelBorrowedUnregisteredDoesNotJoin is the seizure guard.
//
// The first client into an empty channel is handed +o by the server, and on a
// server with services that is the account which then registers it. A guest
// that joined a host channel the host had not created yet would therefore end
// up owning the channel it came to borrow. So an unregistered host channel is
// reported and NOT joined — the probe has to precede the JOIN, because a check
// made afterwards would already have taken the channel.
func TestEnsureChannelBorrowedUnregisteredDoesNotJoin(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO": {notice("Channel #a-dev is not registered")},
	})
	roles := &Roles{transport: transport, nick: "botty"}

	ch := provider.Channel{Name: "#a-dev", Borrowed: true}

	err := roles.EnsureChannel(context.Background(), ch)
	if !errors.Is(err, provider.ErrHostChannelNotRegistered) {
		t.Fatalf("EnsureChannel = %v, want ErrHostChannelNotRegistered", err)
	}

	sent := transport.SentLines()
	if sentContains(sent, "JOIN") {
		t.Fatalf("an unregistered host channel must not be joined: sent = %q", sent)
	}
	// Nor may it be claimed by any other route.
	for _, forbidden := range []string{"CS REGISTER", "CS TRANSFER"} {
		if sentContains(sent, forbidden) {
			t.Errorf("a borrower must never issue %s", forbidden)
		}
	}
}

// TestApplyRolesBorrowedFallsBackToLiveMode pins the borrowed provider path
// end to end: a guest bot that the host gave halfop is refused a persistent
// CS AMODE and falls back to a live MODE for a role within its ceiling, while
// a role above it is refused outright rather than sent.
//
// Verified live on ergo 2.19.1: a non-founder halfop may set AMODE +v but is
// told "Insufficient privileges" for +o, and live MODE agrees with 482.
func TestApplyRolesBorrowedFallsBackToLiveMode(t *testing.T) {
	transport := bottest.New()
	transport.SetNames(map[string]string{"botty": "%"}) // halfop, host-granted
	transport.SetScript(map[string][]ircmsg.Message{
		"CS AMODE": {notice("Insufficient privileges")},
	})
	roles := &Roles{transport: transport, nick: "botty"}

	add := []provider.ModeChange{{Account: "alice", Role: irc.RoleVoice}}
	if err := roles.ApplyRoles(context.Background(), "#a-dev", add, nil); err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}

	sent := transport.SentLines()
	if !sentContains(sent, "CS AMODE #a-dev +v alice") {
		t.Errorf("the persistent path must be tried first: sent = %q", sent)
	}
	if !sentContains(sent, "MODE #a-dev +v alice") {
		t.Errorf("a refused AMODE must fall back to a live MODE: sent = %q", sent)
	}

	// The same bot may not grant op, in either form.
	above := []provider.ModeChange{{Account: "bob", Role: irc.RoleOp}}
	err := roles.ApplyRoles(context.Background(), "#a-dev", above, nil)
	if !errors.Is(err, provider.ErrPrivilege) {
		t.Fatalf("ApplyRoles(op) = %v, want ErrPrivilege", err)
	}
	if sentContains(transport.SentLines(), "MODE #a-dev +o") {
		t.Error("a grant above the bot's own role must not be attempted")
	}
}

func TestEnsureChannelRegistersUnregistered(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO":     {notice("Channel #ws is not registered")},
		"CS REGISTER": {notice("Channel #ws successfully registered")},
	})

	roles := &Roles{transport: transport, nick: "botty"}
	ch := provider.Channel{Name: "#ws", Modes: "nt", Topic: "hello"}

	if err := roles.EnsureChannel(context.Background(), ch); err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}

	sent := transport.SentLines()
	for _, want := range []string{"JOIN #ws", "CS INFO #ws", "CS REGISTER #ws", "MODE #ws +nt", "TOPIC #ws :hello"} {
		if !sentContains(sent, want) {
			t.Errorf("missing %q in %q", want, sent)
		}
	}
}

func TestEnsureChannelAlreadyOursSkipsRegister(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO": {
			notice("Channel #ws is registered"),
			notice("Founder: botty"),
		},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	if err := roles.EnsureChannel(context.Background(), provider.Channel{Name: "#ws"}); err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if sentContains(transport.SentLines(), "CS REGISTER") {
		t.Error("a channel wsm already owns must not be re-registered")
	}
}

func TestEnsureChannelForeignWithoutOper(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO": {
			notice("Channel #ws is registered"),
			notice("Founder: someone-else"),
		},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	err := roles.EnsureChannel(context.Background(), provider.Channel{Name: "#ws"})
	if !errors.Is(err, provider.ErrForeignChannel) {
		t.Fatalf("EnsureChannel = %v, want ErrForeignChannel", err)
	}
	if sentContains(transport.SentLines(), "CS TRANSFER") {
		t.Error("a non-oper bot must not attempt a transfer")
	}
}

func TestEnsureChannelForeignWithOperTransfers(t *testing.T) {
	transport := bottest.New()
	transport.SetOper(true)
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO": {
			notice("Channel #ws is registered"),
			notice("Founder: someone-else"),
		},
		"CS TRANSFER": {notice("Successfully transferred channel #ws")},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	if err := roles.EnsureChannel(context.Background(), provider.Channel{Name: "#ws"}); err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if !sentContains(transport.SentLines(), "CS TRANSFER #ws botty") {
		t.Errorf("an oper should transfer a foreign channel, sent %q", transport.SentLines())
	}
}

func TestListRolesFromAMode(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS AMODE": {
			notice("Channel #deleg has 4 persistent modes set"),
			notice("Account botty receives mode +q"),
			notice("Account carol receives mode +o"),
			notice("Account bob receives mode +h"),
			notice("Account alice receives mode +v"),
		},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	got, err := roles.ListRoles(context.Background(), "#deleg")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}

	want := map[string]irc.Role{
		"botty": irc.RoleOp,
		"carol": irc.RoleOp,
		"bob":   irc.RoleHalfop,
		"alice": irc.RoleVoice,
	}
	for account, wantRole := range want {
		if got[account] != wantRole {
			t.Errorf("role[%s] = %q, want %q", account, got[account], wantRole)
		}
	}
}

func TestApplyRolesPersistentPath(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS AMODE": {notice("Successfully set persistent mode +o on carol")},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	add := []provider.ModeChange{{Account: "carol", Role: irc.RoleOp}}
	if err := roles.ApplyRoles(context.Background(), "#ws", add, nil); err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}

	sent := transport.SentLines()
	if !sentContains(sent, "CS AMODE #ws +o carol") {
		t.Errorf("sent = %q, want a persistent set", sent)
	}
	// The persistent path succeeded, so no live MODE should follow.
	if sentContains(sent, "MODE #ws") {
		t.Error("a successful AMODE must not fall back to a live MODE")
	}
}

func TestApplyRolesNoChangeCountsAsApplied(t *testing.T) {
	// "No changes were made" means the desired state already holds; falling
	// back to a live MODE would issue a pointless command.
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS AMODE": {notice("No changes were made")},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	add := []provider.ModeChange{{Account: "carol", Role: irc.RoleVoice}}
	if err := roles.ApplyRoles(context.Background(), "#ws", add, nil); err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}
	if sentContains(transport.SentLines(), "MODE #ws") {
		t.Error("an already-satisfied mode must not fall back to a live MODE")
	}
}

// TestApplyRolesFallsBackWithinCeiling covers the delegated case verified
// live: a non-founder halfop is refused a persistent set, but may still apply
// what its own privilege allows as a live MODE.
func TestApplyRolesFallsBackWithinCeiling(t *testing.T) {
	transport := bottest.New()
	transport.SetNames(map[string]string{"botty": "%"}) // halfop
	transport.SetScript(map[string][]ircmsg.Message{
		"CS AMODE": {notice("Insufficient privileges")},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	// Voice is at or below halfop, so the fallback is allowed.
	add := []provider.ModeChange{{Account: "dave", Role: irc.RoleVoice}}
	if err := roles.ApplyRoles(context.Background(), "#deleg", add, nil); err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}
	if !sentContains(transport.SentLines(), "MODE #deleg +v dave") {
		t.Errorf("sent = %q, want a live MODE fallback", transport.SentLines())
	}
}

// TestApplyRolesRefusesAboveCeiling is the privilege ceiling: a halfop asked
// to grant +o must refuse rather than send a command the server will reject.
// Sending it anyway would let reconcile record an authored role that was
// never granted.
func TestApplyRolesRefusesAboveCeiling(t *testing.T) {
	transport := bottest.New()
	transport.SetNames(map[string]string{"botty": "%"}) // halfop
	transport.SetScript(map[string][]ircmsg.Message{
		"CS AMODE": {notice("Insufficient privileges")},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	add := []provider.ModeChange{{Account: "dave", Role: irc.RoleOp}}
	err := roles.ApplyRoles(context.Background(), "#deleg", add, nil)
	if !errors.Is(err, provider.ErrPrivilege) {
		t.Fatalf("ApplyRoles = %v, want ErrPrivilege", err)
	}
	if sentContains(transport.SentLines(), "MODE #deleg +o") {
		t.Error("a grant above the bot's own role must not be attempted")
	}
}

func TestApplyRolesRemoveUsesMinusForm(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS AMODE": {notice("Successfully set persistent mode -v on alice")},
	})

	roles := &Roles{transport: transport, nick: "botty"}

	remove := []provider.ModeChange{{Account: "alice", Role: irc.RoleVoice}}
	if err := roles.ApplyRoles(context.Background(), "#ws", nil, remove); err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}
	if !sentContains(transport.SentLines(), "CS AMODE #ws -v alice") {
		t.Errorf("sent = %q, want a minus form", transport.SentLines())
	}
}

func TestApplyRolesEmptyIsNoOp(t *testing.T) {
	transport := bottest.New()
	roles := &Roles{transport: transport, nick: "botty"}

	if err := roles.ApplyRoles(context.Background(), "#ws", nil, nil); err != nil {
		t.Fatalf("ApplyRoles: %v", err)
	}
	if len(transport.SentLines()) != 0 {
		t.Errorf("an empty apply must send nothing, got %q", transport.SentLines())
	}
}

// TestMyRoleReadsMultiPrefixRun pins the founder case: with multi-prefix the
// bot's entry carries a RUN of prefixes ("~@"), and the highest wins.
func TestMyRoleReadsMultiPrefixRun(t *testing.T) {
	transport := bottest.New()
	transport.SetNames(map[string]string{
		"botty": "~@",
		"carol": "@",
		"bob":   "%",
	})

	roles := &Roles{transport: transport, nick: "botty"}

	got, err := roles.MyRole(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != irc.RoleOp {
		t.Errorf("role = %q, want op", got)
	}
}

func TestMyRoleTable(t *testing.T) {
	cases := []struct {
		name     string
		prefixes string
		want     irc.Role
	}{
		{name: "op", prefixes: "@", want: irc.RoleOp},
		{name: "halfop", prefixes: "%", want: irc.RoleHalfop},
		{name: "voice", prefixes: "+", want: irc.RoleVoice},
		{name: "none", prefixes: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := bottest.New()
			transport.SetNames(map[string]string{"botty": tc.prefixes})

			roles := &Roles{transport: transport, nick: "botty"}
			got, err := roles.MyRole(context.Background(), "#ws")
			if err != nil {
				t.Fatalf("MyRole: %v", err)
			}
			if got != tc.want {
				t.Errorf("role = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMyRoleAbsentBotHasNoRole(t *testing.T) {
	// A bot that is not in the reply holds nothing, which reconcile reads as
	// "grant nothing here" rather than as an error.
	transport := bottest.New()
	transport.SetNames(map[string]string{"carol": "@"})

	roles := &Roles{transport: transport, nick: "botty"}

	got, err := roles.MyRole(context.Background(), "#ws")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != "" {
		t.Errorf("role = %q, want empty", got)
	}
}

func TestMyRoleIsCaseInsensitive(t *testing.T) {
	// ergo's CASEMAPPING is ascii, so a reply differing only in case is
	// still the bot.
	transport := bottest.New()
	transport.SetNames(map[string]string{"BOTTY": "@"})

	roles := &Roles{transport: transport, nick: "botty"}

	got, err := roles.MyRole(context.Background(), "#ws")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != irc.RoleOp {
		t.Errorf("role = %q, want op", got)
	}
}
