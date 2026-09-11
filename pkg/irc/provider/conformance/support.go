package conformance

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc/bot"
)

// Optional environment variables. Each one enables a case that needs
// something the suite cannot create for itself, and each case skips with a
// message naming its variable rather than failing.
const (
	// EnvBotAccount names the account the suite's own bot logs in as, which
	// is the expected founder of the channels it registers.
	EnvBotAccount = "IRC_CONFORMANCE_BOT_ACCOUNT"
	// EnvForeignChannel names a channel registered to a different account,
	// which the foreign-channel case needs and must not create itself.
	EnvForeignChannel = "IRC_CONFORMANCE_FOREIGN_CHANNEL"
	// EnvHiddenChannel names an existing +i channel for the JoinHidden case.
	EnvHiddenChannel = "IRC_CONFORMANCE_HIDDEN_CHANNEL"
	// EnvRoleSubject names the account the role round trip grants and
	// revokes. Without it the suite picks one it can justify.
	EnvRoleSubject = "IRC_CONFORMANCE_ROLE_SUBJECT"
)

// passphrase is the secret for every account the suite creates. These
// accounts exist only on a throwaway conformance server, so the value is
// fixed rather than generated: a rerun that wants to log in as an account an
// earlier run created needs to know what it was.
const passphrase = "conformance-passphrase-2f4b"

// The relay fixture's expected decoding, taken from a live ergo 2.19.1
// capture (docs/plans/2026-09-10-irc-workspace-chat-design/probes/
// privmsg-tags.txt). The tag carries the RELAYING bot's account; the
// "nick/service" shape of the source is the relayed user's claim.
const (
	relayNick    = "slackuser"
	relayService = "slack"
	relayBot     = "botty"
)

// relayLine is a verbatim relayed PRIVMSG. It is a fixture rather than a live
// capture because producing one needs a second privileged client with
// RELAYMSG rights, which the suite has no way to arrange; the decoding under
// test is pure parsing, so a fixture settles it either way.
const relayLine = "@draft/relaymsg=" + relayBot +
	";msgid=sn3f83npt75td3cf6jrs6wud5w;time=2026-09-11T10:23:20.402Z " +
	":" + relayNick + "/" + relayService + "!~u@ne8kpbr53qbra.irc PRIVMSG #relay :relayed hello"

// RelayFixture parses the relayed-message fixture the RelayedIdentity case
// decodes.
func RelayFixture() (ircmsg.Message, error) {
	msg, err := ircmsg.ParseLine(relayLine)
	if err != nil {
		return ircmsg.Message{}, fmt.Errorf("conformance: parse relay fixture: %w", err)
	}
	return msg, nil
}

// counter makes every generated name unique within a process, so two cases in
// the same run never collide on a channel or account.
var counter atomic.Uint64

// scratchName builds a unique account-safe name for one case.
//
// The run is stamped into the name because a conformance server is not
// necessarily wiped between runs: reusing a fixed name would make the second
// run's "create twice" case see the first run's account and pass for the
// wrong reason.
func scratchName(kind string) string {
	return fmt.Sprintf("wsmconf%s%d%d", kind, time.Now().Unix()%100000, counter.Add(1))
}

// scratchChannel builds a unique channel name for one case.
func scratchChannel(kind string) string {
	return "#" + scratchName(kind)
}

// CanAuthenticate reports whether account can still log in with passphrase.
//
// This is the only way to observe a suspension: the suspend command is
// fire-and-forget on at least one backend, so nothing in its own reply proves
// it took effect. A nil error means the credentials were accepted.
func CanAuthenticate(ctx context.Context, target Target, account, secret string) error {
	transport := bot.New(bot.Options{
		Addr:     target.Addr,
		Account:  account,
		Password: secret,
		Nick:     loginNick(account),
	})

	if err := transport.Connect(ctx); err != nil {
		return fmt.Errorf("conformance: authenticate as %q: %w", account, err)
	}
	transport.Close()
	return nil
}

// loginNick derives a nick that will not collide with the account's own
// reserved nick when something else is already using it.
//
// Nick reservation is strict on a services-backed server: an authenticated
// session may take its account's nick, but a leftover session holding it
// would make a perfectly good login look like a failure.
func loginNick(account string) string {
	nick := account
	if len(nick) > 24 {
		nick = nick[:24]
	}
	return nick + "c"
}

// Deps builds the transport dependencies a provider factory needs from a
// parsed target, so both runners wire themselves the same way.
//
// The bot account is read from the environment rather than invented: the
// server has to already know it, because the suite authenticates as it.
func Deps(target Target, account, secret string) bot.Options {
	return bot.Options{
		Addr:     target.Addr,
		Account:  account,
		Password: secret,
		Nick:     account,
		OperName: target.OperName,
		OperPass: target.OperPass,
	}
}

// BotAccount reports the account the suite's bot logs in as, falling back to
// a default when the environment names none.
func BotAccount() string {
	if account := strings.TrimSpace(os.Getenv(EnvBotAccount)); account != "" {
		return account
	}
	return "wsm"
}

// BotSecret reports the passphrase the suite's bot logs in with.
//
// It defaults to the suite's own passphrase so a server provisioned by an
// earlier run of this suite works without extra configuration.
func BotSecret() string {
	if secret := strings.TrimSpace(os.Getenv(EnvBotSecret)); secret != "" {
		return secret
	}
	return passphrase
}

// EnvBotSecret names the passphrase the suite's bot authenticates with.
const EnvBotSecret = "IRC_CONFORMANCE_BOT_SECRET"
