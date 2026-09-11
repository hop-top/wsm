package ergo

import (
	"context"
	"fmt"
	"strings"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// Roles drives channel state and the roles held in it, over ChanServ.
type Roles struct {
	// transport carries every command and reports oper status.
	transport provider.Transport
	// nick is the bot's own nick, used to find itself in a NAMES reply and
	// to name the new founder in CS TRANSFER. provider.Deps does not carry
	// it, so it is empty unless the trio was built with NewWithAccount; an
	// empty nick makes MyRole report no role, which caps grants rather than
	// inflating them.
	nick string
}

// Roles must satisfy the interface it is built for.
var _ provider.RoleProvider = (*Roles)(nil)

// Caps reports what this runtime can honour.
//
// Registering a channel needs only an account, which any connected bot has.
// Persistent roles are a stronger claim: CS AMODE on a channel the bot does
// not own needs the chanreg oper capability, so persistence is advertised
// only when the bot holds oper. Without it the bot can still register its own
// channels and set AMODE there as founder, which is why RegisterChannel
// stands alone.
func (r *Roles) Caps() provider.Caps {
	caps := provider.CapRegisterChannel
	if r.transport != nil && r.transport.IsOper() {
		caps |= provider.CapPersistentRoles
	}
	return caps
}

// EnsureChannel brings the channel to its desired state.
//
// A borrowed channel gets a JOIN and nothing else. The host owns the
// registration, the modes and the topic; touching any of them would be the
// guest reconfiguring someone else's channel. It then reports
// ErrForeignChannel, which is the expected outcome reconcile records rather
// than a failure.
//
// An owned channel is joined, inspected with CS INFO and registered when
// unregistered. A channel already registered to someone else is transferred
// when the bot holds oper (chanreg), and otherwise reported as
// ErrForeignChannel so reconcile applies roles live and leaves the
// registration alone.
func (r *Roles) EnsureChannel(ctx context.Context, ch provider.Channel) error {
	if err := r.transport.Join(ctx, ch.Name); err != nil {
		return fmt.Errorf("irc/ergo: join %s: %w", ch.Name, err)
	}
	if ch.Borrowed {
		return fmt.Errorf("irc/ergo: %s is borrowed: %w", ch.Name, provider.ErrForeignChannel)
	}

	state, err := r.channelState(ctx, ch.Name)
	if err != nil {
		return err
	}

	if !state.Registered {
		if err := r.register(ctx, ch.Name); err != nil {
			return err
		}
	} else if state.Founder != "" && !strings.EqualFold(state.Founder, r.nick) {
		// Registered to someone else. chanreg lets an oper take it over;
		// without that the channel stays theirs and wsm only reports it.
		if !r.transport.IsOper() {
			return fmt.Errorf(
				"irc/ergo: %s registered to %q: %w",
				ch.Name, state.Founder, provider.ErrForeignChannel,
			)
		}
		if err := r.transfer(ctx, ch.Name); err != nil {
			return err
		}
	}

	if ch.Modes != "" {
		if err := r.transport.Mode(ctx, ch.Name, "+"+ch.Modes); err != nil {
			return fmt.Errorf("irc/ergo: set modes on %s: %w", ch.Name, err)
		}
	}
	if ch.Topic != "" {
		if err := r.transport.Topic(ctx, ch.Name, ch.Topic); err != nil {
			return fmt.Errorf("irc/ergo: set topic on %s: %w", ch.Name, err)
		}
	}
	return nil
}

// channelState asks CS INFO what the server thinks of the channel.
func (r *Roles) channelState(ctx context.Context, channel string) (ChannelState, error) {
	replies, err := r.transport.SendLabeled(ctx, "CS INFO "+channel)
	if err != nil {
		return ChannelState{}, fmt.Errorf("irc/ergo: cs info %s: %w", channel, err)
	}
	return ParseChanServInfo(replies), nil
}

// register claims an unregistered channel for the bot.
func (r *Roles) register(ctx context.Context, channel string) error {
	replies, err := r.transport.SendLabeled(ctx, "CS REGISTER "+channel)
	if err != nil {
		return fmt.Errorf("irc/ergo: cs register %s: %w", channel, err)
	}

	switch ParseRegisterResult(replies) {
	case RegisterOK:
		return nil

	case RegisterTaken:
		// A race: it was unregistered at CS INFO and is registered now. The
		// oper path can still take it; otherwise it belongs to whoever won.
		if !r.transport.IsOper() {
			return fmt.Errorf("irc/ergo: %s already registered: %w", channel, provider.ErrForeignChannel)
		}
		return r.transfer(ctx, channel)

	case RegisterRefused:
		return fmt.Errorf("irc/ergo: cs register %s refused: %w", channel, provider.ErrPrivilege)

	default:
		return fmt.Errorf("irc/ergo: cs register %s: unrecognised reply", channel)
	}
}

// transfer takes over a registration using the chanreg oper capability.
func (r *Roles) transfer(ctx context.Context, channel string) error {
	raw := fmt.Sprintf("CS TRANSFER %s %s", channel, r.nick)
	replies, err := r.transport.SendLabeled(ctx, raw)
	if err != nil {
		return fmt.Errorf("irc/ergo: cs transfer %s: %w", channel, err)
	}
	if ParseAModeResult(replies) == AModeRefused {
		return fmt.Errorf("irc/ergo: cs transfer %s refused: %w", channel, provider.ErrPrivilege)
	}
	return nil
}

// ListRoles reports the persistent roles held in channel, keyed by account.
//
// The source is CS AMODE, which is membership rather than presence: a member
// who is not currently joined still holds their role. NAMES would report only
// who is in the room, which is a different question and would make reconcile
// revoke absent members.
func (r *Roles) ListRoles(ctx context.Context, channel string) (map[string]irc.Role, error) {
	replies, err := r.transport.SendLabeled(ctx, "CS AMODE "+channel)
	if err != nil {
		return nil, fmt.Errorf("irc/ergo: cs amode %s: %w", channel, err)
	}
	return ParseAModeList(replies), nil
}

// ApplyRoles grants add and revokes remove.
//
// The persistent path is CS AMODE, which works for an oper (chanreg) and for
// the channel's own founder. A bot that is neither — the borrowed-channel
// case, where the host delegated it halfop or op — may still set AMODE up to
// its own level: ergo applies the ordinary MODE hierarchy to AMODE, verified
// live, so a halfop may grant +v persistently but not +o.
//
// When AMODE is refused the change is retried as a live MODE, which is all a
// delegated bot can do. Anything above the bot's own role is refused up front
// with ErrPrivilege rather than sent: the server would refuse it anyway, and
// reconcile must not record an authored role that was never granted.
func (r *Roles) ApplyRoles(ctx context.Context, channel string, add, remove []provider.ModeChange) error {
	changes := make([]modeDelta, 0, len(add)+len(remove))
	for _, mc := range add {
		changes = append(changes, modeDelta{change: mc, adding: true})
	}
	for _, mc := range remove {
		changes = append(changes, modeDelta{change: mc, adding: false})
	}
	if len(changes) == 0 {
		return nil
	}

	// The ceiling is only consulted once a persistent set is actually
	// refused, so the ordinary founder path costs no extra round trip.
	var (
		ceiling    irc.Role
		haveCeil   bool
		ceilingErr error
	)
	ensureCeiling := func() (irc.Role, error) {
		if !haveCeil {
			ceiling, ceilingErr = r.MyRole(ctx, channel)
			haveCeil = true
		}
		return ceiling, ceilingErr
	}

	for _, delta := range changes {
		letter, ok := roleMode[delta.change.Role]
		if !ok {
			return fmt.Errorf(
				"irc/ergo: %s: unknown role %q for %s",
				channel, delta.change.Role, delta.change.Account,
			)
		}

		applied, err := r.setAMode(ctx, channel, delta, letter)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		// Persistent set refused: fall back to a live MODE, but only within
		// the bot's own privilege.
		mine, err := ensureCeiling()
		if err != nil {
			return err
		}
		if mine == "" || !delta.change.Role.AtMost(mine) {
			return fmt.Errorf(
				"irc/ergo: %s: %s %s exceeds the bot's own role %q: %w",
				channel, delta.change.Account, delta.change.Role, mine, provider.ErrPrivilege,
			)
		}
		if err := r.transport.Mode(ctx, channel, delta.sign()+string(letter), delta.change.Account); err != nil {
			return fmt.Errorf("irc/ergo: mode %s on %s: %w", channel, delta.change.Account, err)
		}
	}
	return nil
}

// modeDelta is one role change with its direction.
type modeDelta struct {
	change provider.ModeChange
	adding bool
}

// sign renders the direction as the mode prefix.
func (d modeDelta) sign() string {
	if d.adding {
		return "+"
	}
	return "-"
}

// setAMode issues one CS AMODE change and reports whether it took effect. A
// refusal is not an error: the caller falls back to a live MODE.
func (r *Roles) setAMode(ctx context.Context, channel string, delta modeDelta, letter byte) (bool, error) {
	raw := fmt.Sprintf("CS AMODE %s %s%c %s", channel, delta.sign(), letter, delta.change.Account)
	replies, err := r.transport.SendLabeled(ctx, raw)
	if err != nil {
		return false, fmt.Errorf("irc/ergo: cs amode %s: %w", channel, err)
	}

	switch ParseAModeResult(replies) {
	case AModeApplied, AModeNoChange:
		// "No changes were made" means the desired state already holds.
		return true, nil
	case AModeRefused:
		return false, nil
	default:
		// An unrecognised reply is not evidence the mode was set, so the
		// live-MODE fallback runs rather than reporting a silent success.
		return false, nil
	}
}

// MyRole reports the role the bot holds in channel, from the NAMES prefixes
// against its own nick.
//
// With multi-prefix the value is a RUN of prefix characters, so a founder who
// is also opped reads as "~@"; the highest role in the run wins. A bot that
// is not in the reply holds no role, which is reported as an empty role
// rather than an error: reconcile reads it as "grant nothing here".
func (r *Roles) MyRole(ctx context.Context, channel string) (irc.Role, error) {
	names, err := r.transport.Names(ctx, channel)
	if err != nil {
		return "", fmt.Errorf("irc/ergo: names %s: %w", channel, err)
	}

	prefixes, present := names[r.nick]
	if !present {
		// Nick comparison on ergo is ASCII-case-insensitive (CASEMAPPING=
		// ascii in ISUPPORT), so a reply that differs only in case is still
		// the bot.
		for nick, held := range names {
			if strings.EqualFold(nick, r.nick) {
				prefixes, present = held, true
				break
			}
		}
	}
	if !present {
		return "", nil
	}

	role, ok := ParseNamesRole(prefixes)
	if !ok {
		return "", nil
	}
	return role, nil
}
