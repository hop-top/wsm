package provider

import (
	"context"

	"github.com/ergochat/irc-go/ircmsg"
)

// CapSet is the set of IRCv3 capabilities the server granted during
// negotiation. Providers degrade on what is missing rather than assuming.
type CapSet interface {
	// Has reports whether name was negotiated.
	Has(name string) bool
	// Names lists the negotiated capabilities.
	Names() []string
}

// Transport is the connection a provider drives. pkg/irc/bot implements it;
// provider only consumes it, so the dependency arrow never points back.
//
// Wire formats are deliberately absent from this interface: Send and
// SendLabeled take raw lines and hand back parsed messages, leaving every
// backend-specific command spelling to the concrete provider.
type Transport interface {
	// Connect dials, authenticates and negotiates capabilities.
	Connect(ctx context.Context) error
	// Close tears the connection down.
	Close()
	// Negotiated reports the capabilities the server granted.
	Negotiated() CapSet
	// IsOper reports whether the bot holds server operator privilege, which
	// gates the privileged directory operations.
	IsOper() bool
	// Send writes a raw line without waiting for a reply.
	Send(ctx context.Context, raw string) error
	// SendLabeled writes a raw line and collects the replies correlated to
	// it. Without labeled-response the implementation falls back to a
	// best-effort collection, so callers must tolerate a partial set.
	SendLabeled(ctx context.Context, raw string) ([]ircmsg.Message, error)
	// Subscribe calls fn for every inbound message with the given command.
	// The returned function cancels the subscription.
	Subscribe(cmd string, fn func(ircmsg.Message)) (cancel func())
	// SubscribeBatch calls fn once per completed batch of the given type.
	// The returned function cancels the subscription.
	SubscribeBatch(typ string, fn func([]ircmsg.Message)) (cancel func())
	// Join enters a channel.
	Join(ctx context.Context, ch string) error
	// Part leaves a channel.
	Part(ctx context.Context, ch string) error
	// Mode applies channel modes.
	Mode(ctx context.Context, ch, modes string, args ...string) error
	// Topic sets a channel topic.
	Topic(ctx context.Context, ch, text string) error
	// Names lists channel occupants as nick to mode prefixes. With
	// multi-prefix the value carries every prefix the user holds.
	Names(ctx context.Context, ch string) (map[string]string, error)
	// OnReconnect registers fn to run after each successful reconnect, so
	// providers can re-establish channel state the server forgot.
	OnReconnect(fn func())
}
