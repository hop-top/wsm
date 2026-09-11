// Package plain implements the services-less IRC backend: an external server
// with no ChanServ and no NickServ, where the only durable fact is who is in
// the channel right now.
//
// Nothing here persists. There are no accounts to create, no channel to
// register, and no AMODE list the server replays when the bot is away. NAMES is
// the single source of truth about who holds what, and every role the bot
// grants lasts exactly as long as the bot stays present and opped. That one
// constraint shapes the whole package:
//
//   - Roles are keyed by nick, not by account. The design assumes nick equals
//     account on a plain server, because there is no account layer to map
//     through.
//   - A role for a nick that is not in the channel cannot be applied at all, so
//     it is remembered in a pending set and re-applied when that nick's JOIN
//     arrives. Without that, a member who is offline during reconcile would
//     silently never receive their role.
//   - The bot must hold +o to grant anything. NAMES decides that, and a bot
//     without +o reports provider.ErrNotOpped so reconcile degrades the channel
//     rather than issuing modes the server will refuse.
//
// The provider drives provider.Transport and imports no bot package, keeping
// the dependency arrow pointing one way.
//
// This package must stay free of any UI/presentation dependency (CI enforces
// `make verify-no-ui-leak`): it is consumed by both the CLI and the daemon.
package plain

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// Name is the provider name this package registers under.
const Name = "plain"

// Prefixes is the membership prefix set this provider recognises, in descending
// order of privilege: owner, admin, op, halfop, voice.
//
// A plain server reports prefixes without an accompanying account layer, and
// with multi-prefix negotiated a user carries every prefix they hold at once.
// The run is what matters, never its first character: a founder who is also
// opped arrives as "~@", captured live from ergo 2.19.1 in
// docs/plans/2026-09-10-irc-workspace-chat-design/probes/names-multi-prefix.txt.
const Prefixes = "~&@%+"

// numericNotOpped is ERR_CHANOPRIVSNEEDED, which a server sends when the bot
// issues a MODE it lacks the privilege for. It arrives asynchronously and is
// never the return value of a mode command, so it is observed rather than
// awaited.
const numericNotOpped = "482"

// init registers the provider so a config naming "plain" resolves through
// provider.Open without the caller importing this package for its symbols.
func init() {
	provider.Register(Name, New)
}

// New builds the plain provider trio from its dependencies. It satisfies
// provider.Factory.
//
// The three returned providers share one state object: the role provider needs
// the transport and the pending set, and the other two need the transport only,
// but a single owner keeps the JOIN subscription and the 482 record in one
// place rather than duplicated per interface.
func New(deps provider.Deps) (provider.Providers, error) {
	if deps.Transport == nil {
		return provider.Providers{}, fmt.Errorf("irc/plain: transport is required")
	}

	p := &plain{
		transport: deps.Transport,
		log:       deps.Logger(),
		nick:      irc.DefaultBotAccount,
		pending:   map[string]map[string]irc.Role{},
		refused:   map[string]bool{},
	}
	p.watchRefusals()

	return provider.Providers{
		Accounts:  (*accounts)(p),
		Roles:     p,
		Directory: (*directory)(p),
	}, nil
}

// plain is the shared state behind all three provider interfaces.
//
// The JOIN subscription fires on the transport's own goroutine while
// ApplyRoles may be running on the caller's, so every field below the mutex is
// read and written under it. The transport itself is independently safe, and is
// deliberately never called while the mutex is held: a mode write that blocked
// on the network would otherwise stall every other channel's bookkeeping.
type plain struct {
	transport provider.Transport
	log       *slog.Logger

	mu sync.Mutex
	// nick is what the bot is called on this server, which is how it finds
	// itself in a NAMES table. A plain server has no account layer, so the
	// bot's account name is its nick.
	nick string
	// pending holds, per channel, the roles desired for nicks that were not
	// present when ApplyRoles ran. They are applied on that nick's JOIN.
	pending map[string]map[string]irc.Role
	// refused records channels where the server answered a mode with 482.
	// EnsureChannel is best-effort about template modes, so this is reported
	// rather than raised.
	refused map[string]bool
	// cancelJoin cancels the JOIN subscription, and cancelRefusal the 482
	// one. Both are nil until subscribed.
	cancelJoin    func()
	cancelRefusal func()
}

// SetNick tells the provider what the bot is called, overriding the default.
//
// The transport carries no identity of its own, and the bot's own NAMES entry
// is what decides whether a role apply can succeed at all, so a deployment
// whose bot nick is not the default must say so before the first reconcile.
func (p *plain) SetNick(nick string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if nick != "" {
		p.nick = nick
	}
}

// Nick reports the nick the provider looks for in NAMES.
func (p *plain) Nick() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nick
}

// Refused reports the channels where a mode was answered with 482, in sorted
// order. EnsureChannel does not fail on a refused template mode, so this is how
// a caller learns the channel is not fully configured.
func (p *plain) Refused() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]string, 0, len(p.refused))
	for channel := range p.refused {
		out = append(out, channel)
	}
	sort.Strings(out)
	return out
}

// Pending reports the roles still waiting for their nick to join channel.
func (p *plain) Pending(channel string) map[string]irc.Role {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make(map[string]irc.Role, len(p.pending[channel]))
	for nick, role := range p.pending[channel] {
		out[nick] = role
	}
	return out
}

// Close drops the provider's subscriptions. It is safe to call more than once.
func (p *plain) Close() {
	p.mu.Lock()
	join, refusal := p.cancelJoin, p.cancelRefusal
	p.cancelJoin, p.cancelRefusal = nil, nil
	p.mu.Unlock()

	if join != nil {
		join()
	}
	if refusal != nil {
		refusal()
	}
}

// watchRefusals subscribes to 482 so a refused mode is recorded instead of
// lost. The numeric names the channel in its second parameter.
func (p *plain) watchRefusals() {
	cancel := p.transport.Subscribe(numericNotOpped, func(msg ircmsg.Message) {
		channel := refusedChannel(msg)
		if channel == "" {
			return
		}
		p.mu.Lock()
		p.refused[channel] = true
		p.mu.Unlock()
		p.log.Warn("irc/plain: mode refused, bot not opped", "channel", channel)
	})

	p.mu.Lock()
	p.cancelRefusal = cancel
	p.mu.Unlock()
}

// refusedChannel reports the channel a 482 refers to. The numeric is shaped
// `482 <nick> <channel> :You're not channel operator`, so the channel is the
// first parameter that looks like one; a malformed line contributes nothing
// rather than recording a refusal against the wrong channel.
func refusedChannel(msg ircmsg.Message) string {
	for _, param := range msg.Params {
		if isChannel(param) {
			return param
		}
	}
	return ""
}

// isChannel reports whether s is shaped like a channel name.
func isChannel(s string) bool {
	return s != "" && (s[0] == '#' || s[0] == '&')
}

// Caps reports no channel capabilities. A plain server persists nothing and
// registers nothing, so every capability-gated operation is absent and the
// caller reads that from the bitset rather than from a failed call.
func (p *plain) Caps() provider.Caps { return 0 }

// EnsureChannel joins the channel and brings what it can of the desired state.
//
// Everything past the join is best-effort by design. A plain server may well
// refuse the template modes and the topic, and a channel wsm merely sits in is
// still worth capturing from, so a refusal is recorded and reported rather than
// failing the reconcile. A borrowed channel is joined and otherwise left
// entirely alone: the host owns its modes and topic.
func (p *plain) EnsureChannel(ctx context.Context, ch provider.Channel) error {
	if ch.Name == "" {
		return fmt.Errorf("irc/plain: ensure channel: name is required")
	}

	if err := p.transport.Join(ctx, ch.Name); err != nil {
		return fmt.Errorf("irc/plain: join %s: %w", ch.Name, err)
	}
	if ch.Borrowed {
		return nil
	}

	if ch.Modes != "" {
		// A refusal arrives asynchronously as 482 and is recorded by the
		// subscription, so a send error is the only failure visible here.
		if err := p.transport.Mode(ctx, ch.Name, ch.Modes); err != nil {
			p.log.Warn("irc/plain: template modes not applied",
				"channel", ch.Name, "modes", ch.Modes, "error", err)
		}
	}

	// The topic is only worth attempting when the bot is opped: on a +t
	// channel, which is the template default, an unopped TOPIC is refused.
	if ch.Topic != "" {
		opped, err := p.opped(ctx, ch.Name)
		if err != nil {
			return err
		}
		if !opped {
			return fmt.Errorf("irc/plain: %s: %w", ch.Name, provider.ErrNotOpped)
		}
		if err := p.transport.Topic(ctx, ch.Name, ch.Topic); err != nil {
			return fmt.Errorf("irc/plain: topic %s: %w", ch.Name, err)
		}
	}
	return nil
}

// ListRoles reports the roles held in channel, keyed by nick.
//
// On a server with no account layer the nick is the account: the design assumes
// they are the same name, and reconcile's diff is keyed on whatever this
// returns. A nick holding several prefixes is reported at the highest of them,
// because that is the privilege it actually has.
func (p *plain) ListRoles(ctx context.Context, channel string) (map[string]irc.Role, error) {
	names, err := p.transport.Names(ctx, channel)
	if err != nil {
		return nil, fmt.Errorf("irc/plain: names %s: %w", channel, err)
	}

	out := make(map[string]irc.Role, len(names))
	for nick, held := range names {
		if role, ok := roleFor(held); ok {
			out[nick] = role
		}
	}
	return out, nil
}

// MyRole reports the role the bot holds in channel, read from NAMES. A bot with
// no prefix holds no role, which is reported as an empty role rather than an
// error: reconcile caps grants against it and an empty value means "cap
// nothing", which the not-opped check below then catches.
func (p *plain) MyRole(ctx context.Context, channel string) (irc.Role, error) {
	names, err := p.transport.Names(ctx, channel)
	if err != nil {
		return "", fmt.Errorf("irc/plain: names %s: %w", channel, err)
	}

	role, _ := roleFor(names[p.Nick()])
	return role, nil
}

// ApplyRoles grants add and revokes remove, for the nicks present right now.
//
// A nick that is absent cannot be given a mode at all, so its desired role is
// remembered and applied when it joins. That is the whole point of this
// provider: roles do not persist, so they have to be re-asserted, and a member
// who happened to be offline during reconcile must not be skipped forever.
//
// Revokes are only sent for nicks that are present. A revoke for an absent nick
// is already satisfied — they hold nothing on a server that forgot them — and
// it also clears any pending grant, so a role added and then removed before the
// member ever appeared does not arrive later as a surprise.
func (p *plain) ApplyRoles(ctx context.Context, channel string, add, remove []provider.ModeChange) error {
	names, err := p.transport.Names(ctx, channel)
	if err != nil {
		return fmt.Errorf("irc/plain: names %s: %w", channel, err)
	}

	// The bot grants privilege it holds itself; without +o the server refuses
	// every mode, so the honest answer is to apply nothing and let reconcile
	// report the channel degraded.
	myRole, _ := roleFor(names[p.Nick()])
	if myRole != irc.RoleOp {
		return fmt.Errorf("irc/plain: %s: %w", channel, provider.ErrNotOpped)
	}

	// A role above the bot's own cannot be granted. Refusing up front keeps
	// the report honest instead of filling it with server refusals.
	for _, change := range add {
		if !change.Role.AtMost(myRole) {
			return fmt.Errorf("irc/plain: %s: grant %s to %s: %w",
				channel, change.Role, change.Account, provider.ErrPrivilege)
		}
	}

	for _, change := range remove {
		p.forgetPending(channel, change.Account)
		if _, present := names[change.Account]; !present {
			continue
		}
		if err := p.mode(ctx, channel, "-", change); err != nil {
			return err
		}
	}

	for _, change := range add {
		if _, present := names[change.Account]; !present {
			p.rememberPending(channel, change.Account, change.Role)
			continue
		}
		p.forgetPending(channel, change.Account)
		if err := p.mode(ctx, channel, "+", change); err != nil {
			return err
		}
	}

	p.watchJoins()
	return nil
}

// mode sends one role change as a channel mode. sign is "+" or "-".
func (p *plain) mode(ctx context.Context, channel, sign string, change provider.ModeChange) error {
	letter, ok := modeLetter(change.Role)
	if !ok {
		return fmt.Errorf("irc/plain: %s: unknown role %q for %s: %w",
			channel, change.Role, change.Account, provider.ErrUnsupported)
	}
	if err := p.transport.Mode(ctx, channel, sign+letter, change.Account); err != nil {
		return fmt.Errorf("irc/plain: mode %s%s %s in %s: %w",
			sign, letter, change.Account, channel, err)
	}
	return nil
}

// rememberPending records a role to apply when nick next joins channel.
func (p *plain) rememberPending(channel, nick string, role irc.Role) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pending[channel] == nil {
		p.pending[channel] = map[string]irc.Role{}
	}
	p.pending[channel][nick] = role
}

// forgetPending drops any pending role for nick in channel.
func (p *plain) forgetPending(channel, nick string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.pending[channel], nick)
	if len(p.pending[channel]) == 0 {
		delete(p.pending, channel)
	}
}

// takePending removes and returns the role pending for nick in channel.
func (p *plain) takePending(channel, nick string) (irc.Role, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	role, ok := p.pending[channel][nick]
	if !ok {
		return "", false
	}
	delete(p.pending[channel], nick)
	if len(p.pending[channel]) == 0 {
		delete(p.pending, channel)
	}
	return role, true
}

// watchJoins subscribes to JOIN once, so a pending role is applied the moment
// its nick appears. Subscribing is idempotent: ApplyRoles calls it on every
// pass and only the first one registers.
func (p *plain) watchJoins() {
	p.mu.Lock()
	if p.cancelJoin != nil {
		p.mu.Unlock()
		return
	}
	// Claim the slot before subscribing so two concurrent ApplyRoles cannot
	// both register. The real cancel replaces this placeholder below.
	p.cancelJoin = func() {}
	p.mu.Unlock()

	cancel := p.transport.Subscribe("JOIN", p.onJoin)

	p.mu.Lock()
	// Close may have run between the claim and here, in which case the
	// subscription is already unwanted and is dropped rather than stored.
	if p.cancelJoin == nil {
		p.mu.Unlock()
		cancel()
		return
	}
	p.cancelJoin = cancel
	p.mu.Unlock()
}

// onJoin applies the role pending for the joining nick, if any.
//
// This runs on the transport's goroutine, so it touches the pending set only
// through the locked helpers and sends its mode with the lock released.
func (p *plain) onJoin(msg ircmsg.Message) {
	var (
		nick    = msg.Nick()
		channel = joinedChannel(msg)
	)
	if nick == "" || channel == "" {
		return
	}

	role, ok := p.takePending(channel, nick)
	if !ok {
		return
	}

	change := provider.ModeChange{Account: nick, Role: role}
	if err := p.mode(context.Background(), channel, "+", change); err != nil {
		// The grant is put back: the nick is present and a later reconcile
		// or a reconnect should try again rather than drop the role.
		p.rememberPending(channel, nick, role)
		p.log.Warn("irc/plain: pending role not applied on join",
			"channel", channel, "nick", nick, "role", role, "error", err)
	}
}

// joinedChannel reports the channel a JOIN refers to. Plain servers put it in
// the first parameter; the extended-join form adds an account and realname
// after it, which are not this provider's concern.
func joinedChannel(msg ircmsg.Message) string {
	if len(msg.Params) == 0 {
		return ""
	}
	if channel := msg.Params[0]; isChannel(channel) {
		return channel
	}
	return ""
}

// opped reports whether the bot holds +o in channel, per NAMES.
func (p *plain) opped(ctx context.Context, channel string) (bool, error) {
	names, err := p.transport.Names(ctx, channel)
	if err != nil {
		return false, fmt.Errorf("irc/plain: names %s: %w", channel, err)
	}
	role, _ := roleFor(names[p.Nick()])
	return role == irc.RoleOp, nil
}

// roleFor maps a prefix run to the role it represents, taking the highest
// prefix present.
//
// With multi-prefix a user carries every prefix they hold, so the run has to be
// scanned in the server's own precedence order rather than read at its first
// character: "~@" is an opped founder, not something less. Owner and admin both
// outrank op and carry its privileges, so they report op, which is the highest
// role wsm models.
func roleFor(held string) (irc.Role, bool) {
	for i := 0; i < len(Prefixes); i++ {
		if !strings.ContainsRune(held, rune(Prefixes[i])) {
			continue
		}
		switch Prefixes[i] {
		case '~', '&', '@':
			return irc.RoleOp, true
		case '%':
			return irc.RoleHalfop, true
		case '+':
			return irc.RoleVoice, true
		}
	}
	return "", false
}

// modeLetter maps a role to its channel mode letter.
func modeLetter(role irc.Role) (string, bool) {
	switch role {
	case irc.RoleOp:
		return "o", true
	case irc.RoleHalfop:
		return "h", true
	case irc.RoleVoice:
		return "v", true
	default:
		return "", false
	}
}

// accounts is the account provider for a server with no account layer. It is
// the same state as plain, under a type that carries only the account methods.
type accounts plain

// Caps reports no account capabilities: a plain server has no NickServ, so
// there is nothing to create, rotate or suspend.
func (a *accounts) Caps() provider.Caps { return 0 }

// CreateAccount is unsupported: registration needs services.
func (a *accounts) CreateAccount(_ context.Context, account, _ string) error {
	return fmt.Errorf("irc/plain: create account %s: %w", account, provider.ErrUnsupported)
}

// SetPassword is unsupported: there is no account to hold a passphrase.
func (a *accounts) SetPassword(_ context.Context, account, _ string) error {
	return fmt.Errorf("irc/plain: set password for %s: %w", account, provider.ErrUnsupported)
}

// Suspend is unsupported: there is no account to disable.
func (a *accounts) Suspend(_ context.Context, account string) error {
	return fmt.Errorf("irc/plain: suspend %s: %w", account, provider.ErrUnsupported)
}

// directory is the directory provider. Only the channel listing is possible
// without services; everything else needs a registration database or oper
// privilege this provider does not assume.
type directory plain

// Caps reports the channel listing only.
func (d *directory) Caps() provider.Caps { return provider.CapListChannels }

// ListChannels enumerates visible channels with LIST.
//
// Registered is always false and Founder always empty: a plain server has no
// registration database, so every channel it reports is unregistered by
// definition. Saying so explicitly is what stops a caller from reading a
// missing founder as an unknown one.
func (d *directory) ListChannels(ctx context.Context) ([]provider.ChannelInfo, error) {
	replies, err := d.transport.SendLabeled(ctx, "LIST")
	if err != nil {
		return nil, fmt.Errorf("irc/plain: list channels: %w", err)
	}

	var out []provider.ChannelInfo
	for _, msg := range replies {
		info, ok := listEntry(msg)
		if !ok {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

// listEntry folds one RPL_LIST (322) into a channel info.
//
// The numeric is shaped `322 <nick> <channel> <visible> :<topic>`. A short or
// malformed reply contributes nothing rather than erroring: one odd line from
// the server must not fail an otherwise good listing, and the surrounding 321
// and 323 markers are skipped the same way.
func listEntry(msg ircmsg.Message) (provider.ChannelInfo, bool) {
	if msg.Command != "322" || len(msg.Params) < 3 {
		return provider.ChannelInfo{}, false
	}
	if !isChannel(msg.Params[1]) {
		return provider.ChannelInfo{}, false
	}

	info := provider.ChannelInfo{
		Name:  msg.Params[1],
		Users: atoi(msg.Params[2]),
	}
	if len(msg.Params) >= 4 {
		info.Topic = msg.Params[3]
	}
	return info, true
}

// atoi parses a non-negative decimal count, reporting 0 for anything else. A
// user count wsm cannot read is better reported as unknown than as an error
// that discards the whole listing.
func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// Founder is unsupported: nothing records who created a channel.
func (d *directory) Founder(_ context.Context, channel string) (string, error) {
	return "", fmt.Errorf("irc/plain: founder of %s: %w", channel, provider.ErrUnsupported)
}

// JoinHidden is unsupported: overriding +i/+k needs oper privilege this
// provider does not assume.
func (d *directory) JoinHidden(_ context.Context, channel string) error {
	return fmt.Errorf("irc/plain: join hidden %s: %w", channel, provider.ErrUnsupported)
}

// Purge is unsupported: there is no registration to drop.
func (d *directory) Purge(_ context.Context, channel string) error {
	return fmt.Errorf("irc/plain: purge %s: %w", channel, provider.ErrUnsupported)
}

// RelayedIdentity reports no relay: decoding a bridge's messages needs the
// relay tags a plain server does not send.
func (d *directory) RelayedIdentity(ircmsg.Message) (provider.RelayIdentity, bool) {
	return provider.RelayIdentity{}, false
}

// The three interfaces must stay satisfied; a change to provider should break
// these lines rather than the factory's return statement.
var (
	_ provider.AccountProvider   = (*accounts)(nil)
	_ provider.RoleProvider      = (*plain)(nil)
	_ provider.DirectoryProvider = (*directory)(nil)
	_ provider.Factory           = New
)
