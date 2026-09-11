package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
)

// Attribution values recorded on a captured message. They are ordered here
// from strongest to weakest evidence, which is also the order Capture tries
// them in.
const (
	// AttributionRelay decodes a bridge-relayed message. The relaying bot is
	// authenticated; the far-side nick and the content are its claims.
	AttributionRelay = "relay"
	// AttributionAccount comes from an authenticated account tag.
	AttributionAccount = "account"
	// AttributionNick matched on nick alone. A nick is not owned, so this is
	// weak: it is recorded, and labelled, rather than trusted.
	AttributionNick = "nick"
	// AttributionNone means the sender could not be resolved. AgentID is
	// empty and the message is still recorded.
	AttributionNone = "none"
)

// messageFormat labels the payload as IRC-sourced text, so a consumer knows
// the content may carry IRC formatting codes it should strip or render.
const messageFormat = "text/irc"

// messageRole is the role every captured message carries: these are human
// members speaking, never the agent's own output.
const messageRole = "member"

// hashedIDPrefix marks a MsgID that Capture synthesised because the server
// sent no msgid tag. The prefix keeps a derived id from ever colliding with a
// real server-issued one.
const hashedIDPrefix = "h:"

// hashedIDLength is how many hex characters of the content hash a synthesised
// MsgID keeps. 16 hex characters is 64 bits, which is ample to dedupe one
// channel's history and short enough to stay readable in an event dump.
const hashedIDLength = 16

// histServSource is the nick HistServ replays channel history under. Ergo
// synthesises non-user events (joins, parts) into a CHATHISTORY batch as
// PRIVMSGs from this pseudo-client, so a capture that trusted the batch would
// record "alice joined the channel" as a message alice sent. Observed on ergo
// 2.19.1, probes/chathistory-paging.txt.
const histServSource = "histserv"

// Recorder persists one captured event. It is deliberately narrower than
// workspace.Manager.RecordEvent, which overwrites AgentID with the manager's
// own identity and never sets Timestamp: capture resolves a different AgentID
// per message and carries the server's own timestamp, so it must reach the
// backend through something that preserves both.
type Recorder interface {
	// Record appends ev and returns it as stored.
	Record(ctx context.Context, ev model.Event) (model.Event, error)
}

// Capture turns channel traffic into interaction.message events.
//
// It subscribes to PRIVMSG and NOTICE, resolves who sent each one, and hands
// the result to a Recorder. Everything it needs to decide attribution is
// injected, so the same type serves live capture and CHATHISTORY backfill:
// Handle is the whole capture path and takes one already-parsed message.
type Capture struct {
	// Transport is the connection to subscribe to. Run needs it; Handle does
	// not, so a backfill may leave it nil.
	Transport provider.Transport
	// Directory decodes bridge-relayed messages. It is optional: a nil
	// Directory simply means no message is ever treated as relayed.
	Directory provider.DirectoryProvider
	// Reverse maps an account back to the profile that owns it.
	Reverse provider.ReverseResolver
	// ServerID scopes reverse lookups to this server.
	ServerID string
	// BotAccount is wsm's own account. Messages from it are skipped, so the
	// bot never records its own output as a member message.
	BotAccount string
	// WorkspaceID owns the recorded events.
	WorkspaceID string
	// WriterID identifies this writer in the event log.
	WriterID string
	// Recorder persists each event.
	Recorder Recorder
	// Cursors tracks the last message recorded per channel, so a backfill
	// resumes where the previous one stopped. Optional: a nil Cursors
	// captures without remembering a position.
	Cursors *Cursors
}

// Run subscribes to channel traffic until ctx is done. Each message is handled
// synchronously on the transport's delivery goroutine, which keeps recorded
// order equal to arrival order.
//
// A message that fails to record is dropped after its error is returned to
// nobody: there is no caller to return to on a subscription callback. The
// cursor is what makes that recoverable — it is left unmoved, so the next
// backfill re-fetches from before the failure.
func (c *Capture) Run(ctx context.Context) error {
	if c.Transport == nil {
		return fmt.Errorf("irc/bot: capture has no transport")
	}

	handle := func(msg ircmsg.Message) {
		_, _ = c.Handle(ctx, msg)
	}
	cancelPrivmsg := c.Transport.Subscribe("PRIVMSG", handle)
	defer cancelPrivmsg()
	cancelNotice := c.Transport.Subscribe("NOTICE", handle)
	defer cancelNotice()

	<-ctx.Done()
	return ctx.Err()
}

// Handle captures one message. recorded is false when the message was
// deliberately skipped, which is a normal outcome and not an error: a private
// message, the bot's own echo, or a HistServ-synthesised join all take that
// path.
//
// On a successful record the channel's cursor advances. On a Recorder error it
// does not: a cursor moved past a message that was never stored would skip
// that message forever on the next backfill.
func (c *Capture) Handle(ctx context.Context, msg ircmsg.Message) (recorded bool, err error) {
	ev, ok := c.Event(ctx, msg)
	if !ok {
		return false, nil
	}

	if c.Recorder == nil {
		return false, fmt.Errorf("irc/bot: capture has no recorder")
	}
	stored, err := c.Recorder.Record(ctx, ev)
	if err != nil {
		return false, fmt.Errorf("irc/bot: record captured message: %w", err)
	}

	// Only now, with the event durably stored, may the cursor move.
	if c.Cursors != nil {
		var data model.MessageData
		if err := json.Unmarshal(stored.Data, &data); err == nil {
			c.Cursors.Set(data.Channel, Cursor{
				LastMsgID: data.MsgID,
				LastTime:  stored.Timestamp,
			})
		}
	}
	return true, nil
}

// Event builds the event for one message without recording it. ok is false
// when the message is not capturable at all.
//
// Splitting this out from Handle is what lets a backfill dedupe: it can see
// the MsgID a message would be recorded under before deciding whether it
// already holds that message.
func (c *Capture) Event(ctx context.Context, msg ircmsg.Message) (model.Event, bool) {
	if !capturable(msg) {
		return model.Event{}, false
	}

	var (
		channel = msg.Params[0]
		content = msg.Params[1]
		nick    = msg.Nick()
	)

	_, account := msg.GetTag("account")
	if c.isSelf(account, nick) {
		return model.Event{}, false
	}

	agentID, attribution, relayNick := c.attribute(ctx, msg, account, nick)
	if relayNick != "" {
		nick = relayNick
	}

	timestamp := messageTime(msg)
	return model.Event{
		WorkspaceID: c.WorkspaceID,
		Type:        model.EventInteractionMessage,
		AgentID:     agentID,
		WriterID:    c.WriterID,
		Timestamp:   timestamp,
		Data: mustMarshalMessage(model.MessageData{
			Role:        messageRole,
			Content:     content,
			Format:      messageFormat,
			Channel:     channel,
			Nick:        nick,
			MsgID:       messageID(msg, timestamp, nick, content),
			Attribution: attribution,
		}),
	}, true
}

// attribute resolves who sent a message, returning the agent id, how strongly
// it is believed, and a replacement nick when the message was relayed.
//
// The order is fixed and load-bearing, strongest evidence first:
//
//  1. A bridge relay. The Directory decodes it; the authenticated party is the
//     relaying bot, so the agent id comes from the bot's account. The far-side
//     nick and the message content are claims made by the bridge and are not
//     verified by this server — they are recorded as-is and labelled "relay"
//     so a consumer can weigh them accordingly.
//  2. An account tag. The server authenticated it, so it is trusted even when
//     the reverse resolver cannot name a profile: a reverse miss still records
//     "account" with an empty AgentID, because the evidence about the sender
//     is strong even though wsm does not know who they are.
//  3. A bare nick. Nobody owns a nick, so a match here is weak.
//
// Anything unresolved is "none" with an empty agent id, and is still recorded.
func (c *Capture) attribute(ctx context.Context, msg ircmsg.Message, account, nick string) (agentID, attribution, relayNick string) {
	if c.Directory != nil {
		if relay, ok := c.Directory.RelayedIdentity(msg); ok {
			// The relaying bot is who the server authenticated. Resolving
			// its account is what makes a bridged message attributable at
			// all; failing that, the relayed nick is the only handle left.
			if id, found := c.reverseLookup(ctx, relay.Bot); found {
				return id, AttributionRelay, relay.Nick
			}
			if id, found := c.reverseLookup(ctx, relay.Nick); found {
				return id, AttributionNick, relay.Nick
			}
			return "", AttributionNick, relay.Nick
		}
	}

	if account != "" {
		// A reverse miss does not weaken the evidence: the server vouched
		// for the account either way.
		id, _ := c.reverseLookup(ctx, account)
		return id, AttributionAccount, ""
	}

	if id, found := c.reverseLookup(ctx, nick); found {
		return id, AttributionNick, ""
	}
	return "", AttributionNone, ""
}

// reverseLookup asks the reverse resolver for the profile owning name. A nil
// resolver, an empty name, or a resolver error all mean "not known": capture
// must keep recording messages when identity resolution is degraded, since an
// unattributed message is still worth having.
func (c *Capture) reverseLookup(ctx context.Context, name string) (string, bool) {
	if c.Reverse == nil || name == "" {
		return "", false
	}
	id, found, err := c.Reverse.ProfileFor(ctx, name, c.ServerID)
	if err != nil || !found {
		return "", false
	}
	return id, true
}

// isSelf reports whether a message came from wsm's own bot. With echo-message
// negotiated the server echoes the bot's own PRIVMSGs back to it, so without
// this check every message the bot sent would be recorded as a member's.
//
// The account is the reliable signal; the nick is checked too because a
// message carries no account tag when account-tag was not negotiated.
func (c *Capture) isSelf(account, nick string) bool {
	if c.BotAccount == "" {
		return false
	}
	return strings.EqualFold(account, c.BotAccount) || strings.EqualFold(nick, c.BotAccount)
}

// capturable reports whether msg is a channel message worth recording.
//
// It rejects, in order: anything that is not a PRIVMSG or NOTICE; a malformed
// line missing a target or body; a private message, since only channel traffic
// belongs in a workspace log; and a HistServ replay artefact.
func capturable(msg ircmsg.Message) bool {
	switch msg.Command {
	case "PRIVMSG", "NOTICE":
	default:
		return false
	}
	if len(msg.Params) < 2 {
		return false
	}
	if !isChannel(msg.Params[0]) {
		return false
	}
	return !isSyntheticHistory(msg)
}

// isChannel reports whether target names a channel rather than a user. The
// sigils are those RFC 2812 defines: '#' and '&' are the two in live use, '+'
// (no modes) and '!' (safe channels) are rare but real.
func isChannel(target string) bool {
	if target == "" {
		return false
	}
	switch target[0] {
	case '#', '&', '+', '!':
		return true
	}
	return false
}

// isSyntheticHistory reports whether msg is a server-synthesised history line
// rather than something a user said.
//
// When replaying a channel with CHATHISTORY, ergo's HistServ injects join and
// part notices into the batch as ordinary-looking PRIVMSGs sourced from
// HistServ itself — for example:
//
//	:HistServ!HistServ@localhost PRIVMSG #hist :alice [account: alice] joined the channel
//
// Recording those would put "alice joined the channel" into the event log as a
// message alice wrote, permanently. The source is the discriminator, not the
// text: matching on wording would break on a different locale or ergo version,
// and would also swallow a real message that happened to quote the phrase. A
// genuine user cannot take this nick, because services nicks are reserved.
//
// Verified on ergo 2.19.1; see probes/chathistory-paging.txt.
func isSyntheticHistory(msg ircmsg.Message) bool {
	return strings.EqualFold(msg.Nick(), histServSource)
}

// messageTime is the instant a message was sent: the server-time tag when the
// server set one, else the time capture received it.
//
// Falling back to receipt time keeps the timeline monotonic-ish on a server
// without server-time, but it is genuinely a different measurement — during a
// backfill it is the replay time, not the send time.
func messageTime(msg ircmsg.Message) time.Time {
	if ok, value := msg.GetTag("time"); ok && value != "" {
		if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return ts.UTC()
		}
	}
	return time.Now().UTC()
}

// messageID is the id a message is deduped by: the server's own msgid when it
// sent one, else a hash of the fields that identify the message.
//
// The hash covers timestamp, nick and content, so the same message seen twice
// (live, then again through a backfill) hashes identically and dedupes, while
// a repeated line from the same nick at a different instant does not collapse
// into one. Two identical lines within the same timestamp tick are
// indistinguishable and will dedupe to one; that is the accepted cost of
// running without msgid.
func messageID(msg ircmsg.Message, timestamp time.Time, nick, content string) string {
	if ok, value := msg.GetTag("msgid"); ok && value != "" {
		return value
	}

	sum := sha256.Sum256([]byte(strings.Join([]string{
		timestamp.UTC().Format(time.RFC3339Nano),
		nick,
		content,
	}, "\x00")))
	return hashedIDPrefix + hex.EncodeToString(sum[:])[:hashedIDLength]
}

// mustMarshalMessage encodes a MessageData payload. MessageData is a flat
// struct of strings with no custom marshaller, so encoding cannot fail; an
// impossible error yields an empty payload rather than a panic, because
// dropping one message's body is a smaller failure than taking the daemon
// down.
func mustMarshalMessage(data model.MessageData) json.RawMessage {
	raw, err := json.Marshal(data)
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}
