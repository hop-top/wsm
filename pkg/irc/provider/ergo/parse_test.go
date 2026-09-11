package ergo

import (
	"testing"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot/bottest"
)

// probes is the fixture directory, relative to this package. Every parser
// table below is driven from these captures rather than from invented lines,
// so a server-side wording change breaks a test instead of silently changing
// behaviour.
const probes = "../../../../docs/plans/2026-09-10-irc-workspace-chat-design/probes"

// fixture loads one probe file as messages, failing the test if it cannot.
func fixture(t *testing.T, name string) []ircmsg.Message {
	t.Helper()

	msgs, err := bottest.Lines(probes + "/" + name)
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return msgs
}

func TestParseChanServInfoUnregistered(t *testing.T) {
	// "Channel #unreg is not registered" contains "is registered" as a
	// substring, so a parser testing the positive form first would read an
	// unregistered channel as registered and let EnsureChannel skip
	// CS REGISTER entirely.
	replies := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Channel #unreg is not registered"),
	}

	state := ParseChanServInfo(replies)
	if state.Registered {
		t.Error("an unregistered channel must not read as registered")
	}
	if state.Founder != "" {
		t.Errorf("founder = %q, want empty", state.Founder)
	}
}

func TestParseChanServInfoRegistered(t *testing.T) {
	// The founder line is not adjacent to the "is registered" line in every
	// capture, so it is matched by prefix rather than by position.
	replies := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Channel #probe is registered"),
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Founder: botty"),
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Registered at: Fri, 11 Sep 2026 10:22:38 UTC"),
	}

	state := ParseChanServInfo(replies)
	if !state.Registered {
		t.Error("a registered channel must read as registered")
	}
	if state.Founder != "botty" {
		t.Errorf("founder = %q, want botty", state.Founder)
	}
}

func TestParseChanServInfoIgnoresPurgeState(t *testing.T) {
	// A purged channel still reports "is registered": purge state is only
	// visible through PURGE LIST. Inferring it here would make EnsureChannel
	// believe a purged channel is usable.
	replies := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Channel #purge4 is registered"),
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Founder: botty"),
	}

	if state := ParseChanServInfo(replies); !state.Registered {
		t.Error("a purged channel still reports its registration")
	}
}

func TestParseAModeListFromFixture(t *testing.T) {
	// Driven from the real capture: the listing carries a header line plus
	// one entry per account, including a halfop, and the founder's +q must
	// read as the highest role wsm models rather than as no role.
	roles := ParseAModeList(fixture(t, "chanserv-amode.txt"))

	want := map[string]irc.Role{
		"botty": irc.RoleOp,     // +q founder
		"carol": irc.RoleOp,     // +o
		"dave":  irc.RoleOp,     // +o
		"bob":   irc.RoleHalfop, // +h
		"alice": irc.RoleVoice,  // +v
	}
	for account, wantRole := range want {
		if got := roles[account]; got != wantRole {
			t.Errorf("role[%s] = %q, want %q", account, got, wantRole)
		}
	}
}

func TestParseAModeListSkipsUnknownMode(t *testing.T) {
	// An unrecognised prefix mode must not be guessed into a privilege wsm
	// would then enforce.
	replies := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Account zed receives mode +Z"),
	}

	if roles := ParseAModeList(replies); len(roles) != 0 {
		t.Errorf("roles = %v, want none for an unknown mode", roles)
	}
}

func TestParseAModeResultTable(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want AModeOutcome
	}{
		{
			name: "applied",
			raw:  ":ChanServ!ChanServ@localhost NOTICE botty :Successfully set persistent mode +v on alice",
			want: AModeApplied,
		},
		{
			// The desired state already holds, which is success: treating it
			// as failure would send a live-MODE fallback for a mode already
			// set.
			name: "no change",
			raw:  ":ChanServ!ChanServ@localhost NOTICE carol :No changes were made",
			want: AModeNoChange,
		},
		{
			name: "refused",
			raw:  ":ChanServ!ChanServ@localhost NOTICE bob :Insufficient privileges",
			want: AModeRefused,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAModeResult([]ircmsg.Message{bottest.MustParse(tc.raw)})
			if got != tc.want {
				t.Errorf("outcome = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseAModeResultRefusalWins(t *testing.T) {
	// A batch that set one mode and was refused another must not read as
	// wholly applied, or the caller records a grant that never happened.
	replies := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Successfully set persistent mode +v on alice"),
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE bob :Insufficient privileges"),
	}

	if got := ParseAModeResult(replies); got != AModeRefused {
		t.Errorf("outcome = %v, want refused", got)
	}
}

func TestParseRegisterResultTable(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want RegisterOutcome
	}{
		{
			name: "registered",
			raw:  ":ChanServ!ChanServ@localhost NOTICE botty :Channel #probe successfully registered",
			want: RegisterOK,
		},
		{
			name: "taken",
			raw:  ":ChanServ!ChanServ@localhost NOTICE botty :Channel is already registered",
			want: RegisterTaken,
		},
		{
			name: "refused",
			raw:  ":ChanServ!ChanServ@localhost NOTICE botty :Insufficient privileges",
			want: RegisterRefused,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseRegisterResult([]ircmsg.Message{bottest.MustParse(tc.raw)})
			if got != tc.want {
				t.Errorf("outcome = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParsePurgeCodeFromConfirmation(t *testing.T) {
	// PURGE ADD answers with a per-request code inside a quoted command. The
	// code is the last field, and it must not be confused with the channel.
	replies := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Warning: you are about to empty this channel and remove it from the server."),
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :To confirm, run this command: /CS PURGE ADD #purge5 dmjqy"),
	}

	code, ok := ParsePurgeCode(replies, "#purge5")
	if !ok {
		t.Fatal("a confirmation code should have been found")
	}
	if code != "dmjqy" {
		t.Errorf("code = %q, want dmjqy", code)
	}
}

func TestParsePurgeCodeAbsent(t *testing.T) {
	replies := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Successfully purged channel #purge5 from the server"),
	}

	if _, ok := ParsePurgeCode(replies, "#purge5"); ok {
		t.Error("a success reply carries no confirmation code")
	}
}

func TestPurgeAppliedAndRemoved(t *testing.T) {
	applied := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Successfully purged channel #purge5 from the server"),
	}
	if !PurgeApplied(applied) {
		t.Error("a purge confirmation should read as applied")
	}

	removed := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Successfully unpurged channel #purge5 from the server"),
	}
	if !PurgeRemoved(removed) {
		t.Error("an unpurge confirmation should read as removed")
	}

	// PURGE DEL is idempotent: a channel that was never purged is the
	// desired end state, not a failure to retry.
	never := []ircmsg.Message{
		bottest.MustParse(":ChanServ!ChanServ@localhost NOTICE botty :Channel #purge4 wasn't previously purged from the server"),
	}
	if !PurgeRemoved(never) {
		t.Error("an un-purged channel should read as already removed")
	}
}

func TestParseNamesRoleMultiPrefixRun(t *testing.T) {
	// With multi-prefix the value is a RUN of prefix characters: a founder
	// who is also opped reads "~@". Consuming only the first character would
	// still work here, but the run must not confuse the parser, and the
	// highest role in it wins.
	cases := []struct {
		prefixes string
		want     irc.Role
		found    bool
	}{
		{prefixes: "~@", want: irc.RoleOp, found: true},
		{prefixes: "@", want: irc.RoleOp, found: true},
		{prefixes: "%", want: irc.RoleHalfop, found: true},
		{prefixes: "+", want: irc.RoleVoice, found: true},
		{prefixes: "", found: false},
		{prefixes: "?", found: false},
	}

	for _, tc := range cases {
		t.Run(tc.prefixes, func(t *testing.T) {
			got, ok := ParseNamesRole(tc.prefixes)
			if ok != tc.found {
				t.Fatalf("found = %v, want %v", ok, tc.found)
			}
			if ok && got != tc.want {
				t.Errorf("role = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseNamesRoleHighestWins(t *testing.T) {
	// A voice who is later opped carries both prefixes; reporting voice
	// would under-read the bot's own privilege and cap its grants wrongly.
	got, ok := ParseNamesRole("+@")
	if !ok {
		t.Fatal("a recognised run should yield a role")
	}
	if got != irc.RoleOp {
		t.Errorf("role = %q, want op", got)
	}
}

func TestParseListReplyNumerics(t *testing.T) {
	replies := []ircmsg.Message{
		bottest.MustParse(":ergo.test 322 botty #probe 3 :a topic"),
		bottest.MustParse(":ergo.test 322 botty #empty 0 :"),
		bottest.MustParse(":ergo.test 323 botty :End of LIST"),
		// Anything after the terminator is not part of the listing.
		bottest.MustParse(":ergo.test 322 botty #after 9 :too late"),
	}

	got := ParseListReply(replies)
	if len(got) != 2 {
		t.Fatalf("channels = %d, want 2", len(got))
	}
	if got[0].Name != "#probe" || got[0].Users != 3 || got[0].Topic != "a topic" {
		t.Errorf("first channel = %+v", got[0])
	}
	// LIST carries neither registration nor founder, so both stay zero
	// rather than being guessed from the numeric.
	if got[0].Registered || got[0].Founder != "" {
		t.Errorf("LIST must not invent registration data: %+v", got[0])
	}
}

func TestParseListReplySkipsMalformed(t *testing.T) {
	replies := []ircmsg.Message{
		bottest.MustParse(":ergo.test 322 botty"),
	}

	if got := ParseListReply(replies); len(got) != 0 {
		t.Errorf("channels = %+v, want none from a malformed 322", got)
	}
}

func TestParseRelayIdentityRequiresServerTag(t *testing.T) {
	// The tag is the only trigger. A nick shaped like a relay but carrying no
	// server-set tag is a client-side claim and must be ignored, or any user
	// could impersonate a bridge by choosing a nick.
	msg := bottest.MustParse(":slackuser/slack!~u@ne8kpbr53qbra.irc PRIVMSG #relay :relayed hello")

	if _, ok := ParseRelayIdentity(msg); ok {
		t.Error("the nick shape alone must never trigger relay decoding")
	}
}

func TestParseRelayIdentityFromTag(t *testing.T) {
	// Verbatim from the capture: the tag value is the ACCOUNT of the
	// relaying bot, not the relayed nick.
	msg := bottest.MustParse("@draft/relaymsg=botty;msgid=sn3f83npt75td3cf6jrs6wud5w;time=2026-09-11T10:23:20.402Z :slackuser/slack!~u@ne8kpbr53qbra.irc PRIVMSG #relay :relayed hello")

	identity, ok := ParseRelayIdentity(msg)
	if !ok {
		t.Fatal("a tagged message should decode")
	}
	if identity.Nick != "slackuser" {
		t.Errorf("nick = %q, want slackuser", identity.Nick)
	}
	if identity.Service != "slack" {
		t.Errorf("service = %q, want slack", identity.Service)
	}
	if identity.Bot != "botty" {
		t.Errorf("bot = %q, want botty", identity.Bot)
	}
}

func TestParseRelayIdentityTagOrderIndependent(t *testing.T) {
	// Tag order is not stable across lines (echo-message reorders them), so
	// the decoder must look the tag up by name.
	msg := bottest.MustParse("@msgid=sn3f83npt75td3cf6jrs6wud5w;time=2026-09-11T10:23:20.402Z;draft/relaymsg=botty :slackuser/slack!~u@ne8kpbr53qbra.irc PRIVMSG #relay :relayed hello")

	identity, ok := ParseRelayIdentity(msg)
	if !ok {
		t.Fatal("tag order must not affect decoding")
	}
	if identity.Bot != "botty" || identity.Nick != "slackuser" {
		t.Errorf("identity = %+v", identity)
	}
}

func TestParseRelayIdentityWithoutSeparator(t *testing.T) {
	// The tag already proved the message is relayed; a nick lacking the
	// separator still yields the sender rather than being dropped.
	msg := bottest.MustParse("@draft/relaymsg=botty :plainrelay!~u@host PRIVMSG #relay :hello")

	identity, ok := ParseRelayIdentity(msg)
	if !ok {
		t.Fatal("a tagged message should decode regardless of nick shape")
	}
	if identity.Nick != "plainrelay" || identity.Service != "" {
		t.Errorf("identity = %+v, want the whole nick and no service", identity)
	}
}
