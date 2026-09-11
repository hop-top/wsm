package ergo

import (
	"context"
	"fmt"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc/provider"
)

// Directory inspects the server beyond wsm's own channels.
type Directory struct {
	// api is the HTTP client, nil when Deps carried no API config. With it
	// listing is one call that also carries registration and founder;
	// without it listing falls back to LIST over the transport.
	api *Client
	// transport carries LIST, SAJOIN and the ChanServ commands.
	transport provider.Transport
}

// Directory must satisfy the interface it is built for.
var _ provider.DirectoryProvider = (*Directory)(nil)

// Caps reports what this runtime can honour.
//
// Listing works either way, so CapListChannels is unconditional. Founder
// lookup goes through ChanServ, which any connected bot may ask. The
// privileged pair needs oper: SAJOIN needs the sajoin capability and CS PURGE
// needs chanreg, so neither is advertised to a bot that lacks oper. Relay
// decoding is pure parsing of a server-set tag and needs nothing.
func (d *Directory) Caps() provider.Caps {
	caps := provider.CapListChannels | provider.CapChannelFounder | provider.CapRelayIdentity
	if d.transport != nil && d.transport.IsOper() {
		caps |= provider.CapJoinHidden | provider.CapPurgeChannel
	}
	return caps
}

// ListChannels enumerates channels visible on the server.
//
// The HTTP API is preferred because /v1/list reports registration and the
// registered owner, which LIST does not carry at all. The LIST fallback
// therefore returns channels with Registered false and Founder empty rather
// than guessing; a caller that needs those must ask Founder per channel.
func (d *Directory) ListChannels(ctx context.Context) ([]provider.ChannelInfo, error) {
	if d.api != nil {
		return d.api.List(ctx)
	}

	replies, err := d.transport.SendLabeled(ctx, "LIST")
	if err != nil {
		return nil, fmt.Errorf("irc/ergo: list: %w", err)
	}
	return ParseListReply(replies), nil
}

// Founder reports the account that registered channel, via CS INFO. An
// unregistered channel has no founder, which is an empty string rather than
// an error: "nobody owns this" is a normal answer.
func (d *Directory) Founder(ctx context.Context, channel string) (string, error) {
	replies, err := d.transport.SendLabeled(ctx, "CS INFO "+channel)
	if err != nil {
		return "", fmt.Errorf("irc/ergo: cs info %s: %w", channel, err)
	}

	state := ParseChanServInfo(replies)
	if !state.Registered {
		return "", nil
	}
	return state.Founder, nil
}

// JoinHidden joins channel despite +i/+k, using the sajoin oper capability.
func (d *Directory) JoinHidden(ctx context.Context, channel string) error {
	if !d.Caps().Has(provider.CapJoinHidden) {
		return unsupported("join hidden channel: bot does not hold oper")
	}
	if err := d.transport.Send(ctx, "SAJOIN "+channel); err != nil {
		return fmt.Errorf("irc/ergo: sajoin %s: %w", channel, err)
	}
	return nil
}

// Purge drops the channel registration and empties it.
//
// This cannot be one fire-and-forget command. CS PURGE ADD is a two-step
// confirmation: the first call answers with a per-request code that must be
// echoed back, and the code must be the SOLE trailing argument — appending a
// reason after it restarts the confirmation instead of completing it, which
// would leave the channel un-purged while looking like it worked.
//
// There is no CS UNPURGE; the inverse is CS PURGE DEL, which Unpurge below
// issues.
func (d *Directory) Purge(ctx context.Context, channel string) error {
	if !d.Caps().Has(provider.CapPurgeChannel) {
		return unsupported("purge channel: bot does not hold oper")
	}

	first, err := d.transport.SendLabeled(ctx, "CS PURGE ADD "+channel)
	if err != nil {
		return fmt.Errorf("irc/ergo: cs purge add %s: %w", channel, err)
	}

	// An immediate confirmation means there was nothing to confirm.
	if PurgeApplied(first) {
		return nil
	}

	code, ok := ParsePurgeCode(first, channel)
	if !ok {
		return fmt.Errorf("irc/ergo: cs purge add %s: no confirmation code in reply", channel)
	}

	// The code is the last argument and nothing follows it.
	confirm := fmt.Sprintf("CS PURGE ADD %s %s", channel, code)
	second, err := d.transport.SendLabeled(ctx, confirm)
	if err != nil {
		return fmt.Errorf("irc/ergo: confirm purge %s: %w", channel, err)
	}
	if !PurgeApplied(second) {
		return fmt.Errorf("irc/ergo: purge %s was not confirmed", channel)
	}
	return nil
}

// Unpurge lifts a purge with CS PURGE DEL, which needs no confirmation code.
//
// It is exported beyond the DirectoryProvider interface because re-adopting a
// purged channel has to lift the purge before CS REGISTER can succeed: a
// purged channel refuses JOIN with a 403 while the registration itself still
// reads as intact through CS INFO.
func (d *Directory) Unpurge(ctx context.Context, channel string) error {
	if !d.Caps().Has(provider.CapPurgeChannel) {
		return unsupported("unpurge channel: bot does not hold oper")
	}

	replies, err := d.transport.SendLabeled(ctx, "CS PURGE DEL "+channel)
	if err != nil {
		return fmt.Errorf("irc/ergo: cs purge del %s: %w", channel, err)
	}
	if !PurgeRemoved(replies) {
		return fmt.Errorf("irc/ergo: unpurge %s was not confirmed", channel)
	}
	return nil
}

// RelayedIdentity decodes a bridge-relayed message.
//
// The trigger is the server-set draft/relaymsg tag and only that tag. The
// "nick/service" shape of the source is a claim by the relaying bot, so it is
// read for its parts but never used to decide that a message was relayed:
// that decision belongs to the server, and a client can shape a nick.
func (d *Directory) RelayedIdentity(msg ircmsg.Message) (provider.RelayIdentity, bool) {
	return ParseRelayIdentity(msg)
}
