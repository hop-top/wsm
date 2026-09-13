package ergo

import (
	"fmt"

	"hop.top/wsm/pkg/irc/provider"
)

// Name is the provider name this package registers under, and the value the
// irc.provider config field takes to select it.
const Name = "ergo"

// init registers the factory, so importing this package for its side effect
// is all wiring needs. Register panics on a duplicate name, which turns a
// double import path into a startup failure rather than a silent shadow.
func init() {
	provider.Register(Name, New)
}

// New builds the ergo provider trio from its dependencies.
//
// Every field of the result is non-nil even when the runtime is impoverished:
// a missing HTTP API removes the account capabilities rather than the
// account provider, so callers never nil-check and always get
// ErrUnsupported from the methods that cannot work.
func New(deps provider.Deps) (provider.Providers, error) {
	if deps.Transport == nil {
		return provider.Providers{}, fmt.Errorf("irc/ergo: a transport is required")
	}

	api := NewClient(deps.API)

	return provider.Providers{
		Accounts:  &Accounts{api: api, transport: deps.Transport},
		Roles:     &Roles{transport: deps.Transport},
		Directory: &Directory{api: api, transport: deps.Transport},
	}, nil
}

// NewWithAccount builds the trio for a bot that knows its own account name.
//
// provider.Deps carries a transport, an optional API config and a logger, and
// nothing that names the bot. Two operations need that name: MyRole has to
// find the bot's own entry in a NAMES reply, and CS TRANSFER has to name the
// new founder. Rather than redeclare Deps in this package or guess the name
// from the connection, the wiring layer passes it here — it is the side that
// holds irc.Config.Bot.Account and irc.Profile.BotAccount.
//
// New remains the registered factory, so a provider opened through the
// registry works with an empty account: MyRole then matches nobody and
// reports no role, which reconcile reads as "grant nothing here". That is the
// safe direction — it caps grants rather than inflating them — but it does
// disable the live-MODE fallback and CS TRANSFER, so a daemon that wants
// either must build the trio through this constructor.
func NewWithAccount(deps provider.Deps, account string) (provider.Providers, error) {
	providers, err := New(deps)
	if err != nil {
		return provider.Providers{}, err
	}
	providers.Roles.(*Roles).nick = account
	return providers, nil
}
