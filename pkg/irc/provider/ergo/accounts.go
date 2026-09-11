package ergo

import (
	"context"
	"fmt"

	"hop.top/wsm/pkg/irc/provider"
)

// Accounts manages ergo accounts. Creation and password rotation go over the
// HTTP API, which is the only way to touch an account without a live nick;
// suspension goes over IRC as a NickServ command, because the API has no
// endpoint for it.
type Accounts struct {
	// api is the HTTP client, nil when Deps carried no API config. A nil
	// client is the reason CapCreateAccount and CapSetPassword are absent
	// rather than a call-time failure.
	api *Client
	// transport carries the NS SUSPEND command and reports oper status.
	transport provider.Transport
}

// Accounts must satisfy the interface it is built for.
var _ provider.AccountProvider = (*Accounts)(nil)

// Caps reports only what this runtime can actually honour.
//
// Account creation and password rotation need the HTTP API: without
// Deps.API there is no endpoint to call, so the capabilities are withheld
// rather than advertised and failed. Suspension needs the accreg oper
// capability, so it is gated on the bot holding oper. Declaring a capability
// the runtime cannot honour would make reconcile treat a guaranteed failure
// as a real attempt.
func (a *Accounts) Caps() provider.Caps {
	var caps provider.Caps
	if a.api != nil {
		caps |= provider.CapCreateAccount | provider.CapSetPassword
	}
	if a.transport != nil && a.transport.IsOper() {
		caps |= provider.CapSuspendAccount
	}
	return caps
}

// CreateAccount registers account with passphrase.
//
// An account that already exists comes back as provider.ErrExists, which the
// caller treats as success and keeps the existing secret. ergo reports that
// case in the JSON body with an HTTP 200, so the decision is made on the body
// (see Client.SARegister) and never on the status code.
func (a *Accounts) CreateAccount(ctx context.Context, account, passphrase string) error {
	if !a.Caps().Has(provider.CapCreateAccount) {
		return errNoAPI
	}
	return a.api.SARegister(ctx, account, passphrase)
}

// SetPassword rotates the passphrase of an existing account.
func (a *Accounts) SetPassword(ctx context.Context, account, passphrase string) error {
	if !a.Caps().Has(provider.CapSetPassword) {
		return errNoAPI
	}
	return a.api.Passwd(ctx, account, passphrase)
}

// Suspend disables an account without deleting it, so event attribution stays
// stable.
//
// This is a NickServ command rather than an API call: ergo's HTTP surface has
// no suspend endpoint. It needs the accreg oper capability, which is why the
// capability is gated on oper rather than on the API.
//
// Unlike every other command in this package, the spelling below was not
// pinned by a live capture: the probe run covered ChanServ and the HTTP API
// but never exercised NickServ suspension. It follows ergo's documented
// NS SUSPEND ADD <account> form and should be confirmed against a live
// server before anything depends on the reply text. Nothing parses that reply
// today — the command is fire-and-forget — so a wrong spelling surfaces as an
// account that stays active rather than as a misparse.
func (a *Accounts) Suspend(ctx context.Context, account string) error {
	if !a.Caps().Has(provider.CapSuspendAccount) {
		return unsupported("suspend account: bot does not hold oper")
	}
	if err := a.transport.Send(ctx, fmt.Sprintf("NS SUSPEND ADD %s", account)); err != nil {
		return fmt.Errorf("irc/ergo: suspend %q: %w", account, err)
	}
	return nil
}
