package share

import (
	"context"
	"fmt"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provider/ergo"
)

// HostAccounts creates accounts on a host workspace's ergo over its loopback
// HTTP API.
//
// It exists because the registered ergo factory needs a live IRC transport,
// and attach has none: it runs from the CLI, not from a connected bot. Account
// creation is pure HTTP, so the API client alone is enough — the capabilities
// withheld here (suspend, which is a NickServ command) are exactly the ones
// that would need the session attach does not have.
type HostAccounts struct {
	client *ergo.Client
}

// HostAccounts must satisfy the interface Attach consumes.
var _ provider.AccountProvider = (*HostAccounts)(nil)

// NewHostAccounts builds an account provider against the host's loopback API.
//
// The token is read from the HOST's own api.token at call time and handed
// straight to the client. It is not returned, stored, or written anywhere: the
// caller never holds it, and nothing persists it into the guest's tree.
func NewHostAccounts(hostProfile irc.Profile, hostPaths irc.Paths) (*HostAccounts, error) {
	if !hostProfile.Ports.Valid() {
		return nil, fmt.Errorf("irc/share: host has no allocated port triple")
	}
	token, err := irc.LoadAPIToken(hostPaths.APIToken)
	if err != nil {
		return nil, err
	}
	client := ergo.NewClient(&provider.APIConfig{
		BaseURL: hostAPIBase(hostProfile),
		Token:   token,
	})
	if client == nil {
		return nil, fmt.Errorf("irc/share: could not address the host's ergo API")
	}
	return &HostAccounts{client: client}, nil
}

// Caps reports what an API-only provider can honour: creation and password
// rotation. Suspension is a NickServ command needing a live oper session, so
// it is withheld rather than advertised and failed.
func (h *HostAccounts) Caps() provider.Caps {
	return provider.CapCreateAccount | provider.CapSetPassword
}

// CreateAccount registers account on the host. An account that already exists
// comes back as provider.ErrExists, which Attach refuses rather than adopts.
func (h *HostAccounts) CreateAccount(ctx context.Context, account, passphrase string) error {
	return h.client.SARegister(ctx, account, passphrase)
}

// SetPassword rotates an existing account's passphrase.
func (h *HostAccounts) SetPassword(ctx context.Context, account, passphrase string) error {
	return h.client.Passwd(ctx, account, passphrase)
}

// Suspend is not available without a live oper session.
func (h *HostAccounts) Suspend(context.Context, string) error {
	return fmt.Errorf("irc/share: suspend needs a live session: %w", provider.ErrUnsupported)
}
