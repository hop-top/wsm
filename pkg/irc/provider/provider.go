package provider

import (
	"context"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
)

// Caps is the set of operations a provider supports. A provider advertises
// what it can do; every interface method whose capability is absent returns
// ErrUnsupported rather than failing in a backend-specific way.
type Caps uint32

const (
	// CapCreateAccount creates server accounts.
	CapCreateAccount Caps = 1 << iota
	// CapSetPassword rotates an account passphrase.
	CapSetPassword
	// CapSuspendAccount suspends an account without deleting it, so event
	// attribution stays stable.
	CapSuspendAccount
	// CapPersistentRoles means roles survive the bot's absence, because the
	// server stores them (services). Without it the bot must stay present.
	CapPersistentRoles
	// CapRegisterChannel registers a channel to the bot.
	CapRegisterChannel
	// CapListChannels enumerates channels on the server.
	CapListChannels
	// CapChannelFounder reports who registered a channel.
	CapChannelFounder
	// CapJoinHidden joins a channel despite +i/+k using server privilege.
	CapJoinHidden
	// CapPurgeChannel drops a channel registration and empties it.
	CapPurgeChannel
	// CapRelayIdentity decodes bridge-relayed messages into the real sender.
	CapRelayIdentity
)

// Has reports whether every capability in want is present in c. Has(0) is
// true: the empty set is always satisfied.
func (c Caps) Has(want Caps) bool { return c&want == want }

// HasAny reports whether at least one capability in want is present.
func (c Caps) HasAny(want Caps) bool { return c&want != 0 }

// ModeChange is one role grant or revoke for one account.
type ModeChange struct {
	// Account is the account name, or the nick on a provider without
	// account-based roles.
	Account string
	// Role is the privilege added or removed.
	Role irc.Role
}

// Channel is the desired state of one channel, as reconcile wants it.
type Channel struct {
	// Name is the channel name including its prefix.
	Name string
	// Modes are the channel modes to set, without the leading '+'.
	Modes string
	// Topic is the channel topic.
	Topic string
	// Members maps account (or nick) to the role it should hold.
	Members map[string]irc.Role
	// Borrowed marks a channel wsm joined but does not own, so EnsureChannel
	// must not register it or change its modes.
	Borrowed bool
}

// ChannelInfo is one entry in a server channel listing.
type ChannelInfo struct {
	// Name is the channel name including its prefix.
	Name string
	// Topic is the current topic, empty when unset or not reported.
	Topic string
	// Registered reports whether the channel is registered with services.
	Registered bool
	// Founder is the account that registered it, when known.
	Founder string
	// Users is the visible user count.
	Users int
}

// RelayIdentity is the real sender behind a bridge-relayed message.
type RelayIdentity struct {
	// Nick is the relayed user's nick on the far side of the bridge.
	Nick string
	// Service labels the bridge (e.g. "discord", "matrix").
	Service string
	// Bot is the account of the relaying bot on this server.
	Bot string
}

// AccountProvider manages server accounts for workspace members.
type AccountProvider interface {
	// Caps reports which account operations this provider supports.
	Caps() Caps
	// CreateAccount registers account with passphrase. ErrExists is a
	// normal outcome and keeps the existing credentials.
	CreateAccount(ctx context.Context, account, passphrase string) error
	// SetPassword rotates the passphrase of an existing account.
	SetPassword(ctx context.Context, account, passphrase string) error
	// Suspend disables an account without deleting it.
	Suspend(ctx context.Context, account string) error
}

// RoleProvider drives channel state and the roles held in it.
type RoleProvider interface {
	// Caps reports which channel operations this provider supports.
	Caps() Caps
	// EnsureChannel brings the channel to the desired state: join, register
	// where supported, set modes and topic.
	EnsureChannel(ctx context.Context, ch Channel) error
	// ListRoles reports the roles currently held in channel, keyed by
	// account where the provider has account-based roles and by nick
	// otherwise.
	ListRoles(ctx context.Context, channel string) (map[string]irc.Role, error)
	// ApplyRoles grants add and revokes remove.
	ApplyRoles(ctx context.Context, channel string, add, remove []ModeChange) error
	// MyRole reports the role the bot itself holds in channel, which decides
	// whether a role apply can succeed at all.
	MyRole(ctx context.Context, channel string) (irc.Role, error)
}

// DirectoryProvider inspects the server beyond wsm's own channels.
type DirectoryProvider interface {
	// Caps reports which directory operations this provider supports.
	Caps() Caps
	// ListChannels enumerates channels visible on the server.
	ListChannels(ctx context.Context) ([]ChannelInfo, error)
	// Founder reports the account that registered channel.
	Founder(ctx context.Context, channel string) (string, error)
	// JoinHidden joins channel despite +i/+k, using server privilege.
	JoinHidden(ctx context.Context, channel string) error
	// Purge drops the channel registration and empties it.
	Purge(ctx context.Context, channel string) error
	// RelayedIdentity decodes a bridge-relayed message. The bool is false
	// when msg is not relayed or the provider cannot tell.
	RelayedIdentity(msg ircmsg.Message) (RelayIdentity, bool)
}

// Providers is the trio Open returns. Every field is non-nil: a backend that
// cannot do something supplies an implementation that reports no capability
// and returns ErrUnsupported, so callers never nil-check.
type Providers struct {
	// Accounts manages server accounts.
	Accounts AccountProvider
	// Roles drives channels and roles.
	Roles RoleProvider
	// Directory inspects the wider server.
	Directory DirectoryProvider
}
