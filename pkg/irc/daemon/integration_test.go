//go:build integration_irc

// Package daemon's integration suite runs the provider stack against a real
// ergo server.
//
// Everything here is skipped unless ERGO_BIN names an ergo binary, so the
// default suite stays green and fast on a machine with no IRC server. What it
// buys over the unit tests is the half no fake can answer: a scripted
// transport agrees with whatever the provider sends it, so a wrong command
// spelling, a wrong JSON field name or a credential the server never stored
// all pass against a fake and fail against a server.
//
// Each test provisions its own ergo in its own temp dir on its own port
// slice, so cases neither share a database nor race for a listener.
package daemon_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provider/ergo"
	"hop.top/wsm/pkg/irc/provider/plain"
)

// opTimeout bounds every individual provider call, so a wedged server fails
// the case rather than hanging the suite.
const opTimeout = 30 * time.Second

// TestErgoAPICreatedAccountCanAuthenticate is the case the whole suite exists
// for.
//
// Managed mode creates every member account over ergo's HTTP API
// (Accounts.CreateAccount -> Client.SARegister -> POST /v1/ns/saregister) and
// then hands the member those credentials to log in with. Nothing in the
// creation reply proves the passphrase was stored: ergo answers
// {"success":true} for a registration it completed, and a passphrase it never
// received is not a failure it reports. The only honest check is to create an
// account the way managed mode does and then authenticate as it.
//
// This failed against ergo 2.19.1 before the request body was corrected: the
// API decodes the passphrase from the JSON key "passphrase", the client sent
// "password", the unknown key was dropped, and every account was registered
// with an empty passphrase. The account existed, was verified, and could not
// be logged into by anybody — including wsm's own bot, which made the managed
// happy path fail silently and completely.
//
// The assertion is deliberately "the account can authenticate" rather than
// "the request carries a particular key": the field name is ergo's to change,
// and what wsm actually depends on is a member being able to log in.
func TestErgoAPICreatedAccountCanAuthenticate(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	if !providers.Accounts.Caps().Has(provider.CapCreateAccount) {
		t.Fatal("the ergo provider must declare CapCreateAccount when built with an API config")
	}

	account := srv.name("member")
	const passphrase = "integration-passphrase-2f4b"

	if err := providers.Accounts.CreateAccount(ctx, account, passphrase); err != nil {
		t.Fatalf("CreateAccount(%q): %v", account, err)
	}

	if err := srv.authenticate(ctx, account, passphrase); err != nil {
		t.Fatalf("account %q was created through the HTTP API but cannot authenticate over SASL: %v\n"+
			"managed mode provisions every member account this way, so an account that cannot log in "+
			"is a silent, total failure of the managed happy path", account, err)
	}
}

// TestErgoSetPasswordRestoresLogin covers the other half of the credential
// path: the rotation managed mode uses to recover an account whose secret wsm
// no longer holds.
//
// It shares the failure mode above — ergo's passwd handler decodes the same
// request shape — so a rotation that silently stored nothing would leave a
// reclaimed account permanently unusable while reconcile reported it repaired.
func TestErgoSetPasswordRestoresLogin(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	account := srv.name("rotate")
	if err := providers.Accounts.CreateAccount(ctx, account, "first-passphrase-2f4b"); err != nil {
		t.Fatalf("CreateAccount(%q): %v", account, err)
	}

	const rotated = "second-passphrase-2f4b"
	if err := providers.Accounts.SetPassword(ctx, account, rotated); err != nil {
		t.Fatalf("SetPassword(%q): %v", account, err)
	}

	if err := srv.authenticate(ctx, account, rotated); err != nil {
		t.Fatalf("account %q cannot authenticate with the rotated passphrase: %v", account, err)
	}
}

// TestErgoCreateAccountTwiceReportsExists pins the branch reconcile depends on
// to tell "already provisioned" from "provisioning failed".
//
// ergo reports the collision in the JSON body with an HTTP 200, so a provider
// that judged on the status code would report every rerun as a success and
// overwrite nothing, and one that judged on any error would report a healthy
// rerun as a failure.
func TestErgoCreateAccountTwiceReportsExists(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	account := srv.name("twice")
	const passphrase = "integration-passphrase-2f4b"

	if err := providers.Accounts.CreateAccount(ctx, account, passphrase); err != nil {
		t.Fatalf("first CreateAccount(%q): %v", account, err)
	}

	err := providers.Accounts.CreateAccount(ctx, account, passphrase)
	if !errors.Is(err, provider.ErrExists) {
		t.Fatalf("second CreateAccount(%q) = %v, want ErrExists", account, err)
	}

	// The collision must not have disturbed the credentials already in force:
	// reconcile treats ErrExists as success and keeps the held secret.
	if err := srv.authenticate(ctx, account, passphrase); err != nil {
		t.Fatalf("account %q stopped authenticating after a redundant create: %v", account, err)
	}
}

// TestErgoSuspendDeniesLogin confirms the one account command whose effect is
// invisible in its own reply.
//
// NS SUSPEND ADD is fire-and-forget: nothing parses the response, so a wrong
// spelling leaves the account active rather than producing a misparse, and a
// removed member would quietly keep chat access. Authenticating afterwards is
// the only check that can tell those apart.
func TestErgoSuspendDeniesLogin(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	if !providers.Accounts.Caps().Has(provider.CapSuspendAccount) {
		t.Fatal("the bot holds oper, so the ergo provider must declare CapSuspendAccount")
	}

	account := srv.name("susp")
	const passphrase = "integration-passphrase-2f4b"

	if err := providers.Accounts.CreateAccount(ctx, account, passphrase); err != nil {
		t.Fatalf("CreateAccount(%q): %v", account, err)
	}
	// The account has to be usable before suspension, or the check below
	// would pass against an account that never worked in the first place.
	if err := srv.authenticate(ctx, account, passphrase); err != nil {
		t.Fatalf("account %q cannot authenticate before suspension: %v", account, err)
	}

	if err := providers.Accounts.Suspend(ctx, account); err != nil {
		t.Fatalf("Suspend(%q): %v", account, err)
	}
	// The command is asynchronous: the server applies it after the write
	// returns, so a login attempted immediately can still land.
	waitFor(t, func() bool { return srv.authenticate(ctx, account, passphrase) != nil })

	if err := srv.authenticate(ctx, account, passphrase); err == nil {
		t.Fatalf("account %q still authenticates after Suspend; the suspend command did not take effect", account)
	}
}

// TestErgoRolesRoundTripAgainstLiveServer covers the role path reconcile
// applies after a principal is added: register a channel, grant +o to a real
// account, read it back, then revoke it.
//
// The round trip is what proves the AMODE spelling and the NAMES parsing agree
// with the server. A fake transport would echo whatever the provider sent.
func TestErgoRolesRoundTripAgainstLiveServer(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	subject := srv.name("subj")
	if err := providers.Accounts.CreateAccount(ctx, subject, "integration-passphrase-2f4b"); err != nil {
		t.Fatalf("CreateAccount(%q): %v", subject, err)
	}

	channel := "#" + srv.name("roles")
	if err := providers.Roles.EnsureChannel(ctx, provider.Channel{Name: channel, Topic: "wsm integration"}); err != nil {
		t.Fatalf("EnsureChannel(%q): %v", channel, err)
	}

	add := []provider.ModeChange{{Account: subject, Role: irc.RoleOp}}
	if err := providers.Roles.ApplyRoles(ctx, channel, add, nil); err != nil {
		t.Fatalf("ApplyRoles(+o %s): %v", subject, err)
	}

	roles, err := providers.Roles.ListRoles(ctx, channel)
	if err != nil {
		t.Fatalf("ListRoles(%q): %v", channel, err)
	}
	if got := roles[subject]; got != irc.RoleOp {
		t.Fatalf("after +o, ListRoles[%s] = %q, want %q", subject, got, irc.RoleOp)
	}

	if err := providers.Roles.ApplyRoles(ctx, channel, nil, add); err != nil {
		t.Fatalf("ApplyRoles(-o %s): %v", subject, err)
	}
	roles, err = providers.Roles.ListRoles(ctx, channel)
	if err != nil {
		t.Fatalf("ListRoles(%q) after revoke: %v", channel, err)
	}
	if got, present := roles[subject]; present && got == irc.RoleOp {
		t.Fatalf("after -o, ListRoles[%s] is still %q", subject, got)
	}
}

// TestErgoEnsureChannelIsIdempotent pins the property every reconcile rerun
// depends on: the second pass must land where the first did.
func TestErgoEnsureChannelIsIdempotent(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	channel := "#" + srv.name("idem")
	desired := provider.Channel{Name: channel, Topic: "wsm integration"}

	if err := providers.Roles.EnsureChannel(ctx, desired); err != nil {
		t.Fatalf("first EnsureChannel(%q): %v", channel, err)
	}
	if err := providers.Roles.EnsureChannel(ctx, desired); err != nil {
		t.Fatalf("second EnsureChannel(%q): %v; registering a channel wsm already owns must not fail", channel, err)
	}
}

// TestErgoDirectoryListsRegisteredChannel covers the discovery input: the
// channel list is what classifies a registration as wsm's own, a peer's, or
// unmanaged, so a founder field read from the wrong JSON key would blank every
// owner and misclassify the lot.
func TestErgoDirectoryListsRegisteredChannel(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	channel := "#" + srv.name("dir")
	if err := providers.Roles.EnsureChannel(ctx, provider.Channel{Name: channel}); err != nil {
		t.Fatalf("EnsureChannel(%q): %v", channel, err)
	}

	channels, err := providers.Directory.ListChannels(ctx)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}

	for _, info := range channels {
		if !equalFold(info.Name, channel) {
			continue
		}
		if !info.Registered {
			t.Fatalf("ListChannels reports %q as unregistered although wsm just registered it", channel)
		}
		// The founder is what tells wsm's own channel from a peer bot's. ergo
		// names this field "owner" rather than "founder"; a provider reading
		// the interface's own vocabulary would blank it and classify every
		// channel as unowned.
		if !equalFold(info.Founder, srv.botAccount) {
			t.Fatalf("ListChannels[%q].Founder = %q, want the registrant %q", channel, info.Founder, srv.botAccount)
		}
		return
	}
	t.Fatalf("ListChannels did not report %q, which this test just registered", channel)
}

// TestErgoBorrowedUnregisteredChannelIsNotJoined pins the borrowed contract's
// load-bearing half against a live server.
//
// A borrowed channel belongs to another workspace's server; wsm only joins it.
// When the host has not registered it yet, the bot must stay out entirely —
// because the first client into an empty channel is handed +o by the server,
// which would silently make the borrower the de facto owner of a channel it
// meant only to borrow. Reconcile depends on EnsureChannel reporting exactly
// ErrHostChannelNotRegistered to make that decision.
//
// The conformance suite never sets Borrowed, so this is the only place the
// contract is checked against a real server rather than a scripted fake.
func TestErgoBorrowedUnregisteredChannelIsNotJoined(t *testing.T) {
	srv := startErgo(t)
	providers := srv.providers(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	// Never registered by anybody: exactly the state a guest meets when it
	// declares a borrowed channel before the host has created it.
	channel := "#" + srv.name("borrow")

	err := providers.Roles.EnsureChannel(ctx, provider.Channel{Name: channel, Borrowed: true})
	if !errors.Is(err, provider.ErrHostChannelNotRegistered) {
		t.Fatalf("EnsureChannel(%q, borrowed) = %v, want ErrHostChannelNotRegistered; "+
			"reconcile branches on that sentinel to decide not to join, and any other "+
			"outcome lets the bot enter an empty channel and be handed +o", channel, err)
	}

	// The report is only half the promise: the bot must actually be absent.
	// An unregistered channel has no founder, so a founder appearing here
	// would mean the bot joined and the server registered it to wsm.
	founder, err := providers.Directory.Founder(ctx, channel)
	if err != nil {
		t.Fatalf("Founder(%q): %v", channel, err)
	}
	if founder != "" {
		t.Fatalf("borrowed channel %q has founder %q; wsm joined a channel it only meant to borrow",
			channel, founder)
	}
}

// TestPlainProviderAgainstErgoDeclaresNoAccountCaps runs the services-less
// provider against the same server.
//
// The point is not that plain does less, but that it reports so exactly:
// reconcile branches on errors.Is(err, ErrUnsupported) to decide "skip and
// report" rather than "fail", and a plain provider that reached for services
// would not be services-less at all.
func TestPlainProviderAgainstErgoDeclaresNoAccountCaps(t *testing.T) {
	srv := startErgo(t)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	transport := bot.New(bot.Options{
		Addr:     srv.addr(),
		Account:  srv.botAccount,
		Password: srv.botSecret,
		Nick:     srv.botAccount,
	})
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("connect the plain transport: %v", err)
	}
	t.Cleanup(transport.Close)

	// No API config is passed even though this server has one: a plain
	// provider that used it would not be services-less.
	providers, err := plain.New(provider.Deps{Transport: transport})
	if err != nil {
		t.Fatalf("build the plain provider: %v", err)
	}
	if setter, ok := providers.Roles.(interface{ SetNick(string) }); ok {
		setter.SetNick(srv.botAccount)
	}

	if caps := providers.Accounts.Caps(); caps.Has(provider.CapCreateAccount) {
		t.Fatal("the plain provider must not declare CapCreateAccount: it has no services to register with")
	}

	err = providers.Accounts.CreateAccount(ctx, srv.name("plain"), "integration-passphrase-2f4b")
	if !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("plain CreateAccount = %v, want ErrUnsupported so reconcile skips rather than fails", err)
	}
}

// equalFold compares IRC names case-insensitively, which is what the server
// does with them.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// waitFor polls cond until it holds or the deadline passes. Several of the
// server's effects (a suspension, a mode write) are applied after the command
// that requested them returns, so a bare assertion would race them.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// providers builds the ergo provider trio against the running server, with the
// bot connected and opered.
//
// NewWithAccount rather than New: MyRole has to find the bot in a NAMES reply,
// and the registry factory knows no account name.
func (s *ergoServer) providers(t *testing.T) provider.Providers {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	transport := bot.New(bot.Options{
		Addr:     s.addr(),
		Account:  s.botAccount,
		Password: s.botSecret,
		Nick:     s.botAccount,
		OperName: s.botAccount,
		OperPass: s.botSecret,
	})
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("connect the bot to %s: %v", s.addr(), err)
	}
	t.Cleanup(transport.Close)

	providers, err := ergo.NewWithAccount(provider.Deps{
		Transport: transport,
		API:       &provider.APIConfig{BaseURL: s.apiURL(), Token: s.token},
	}, s.botAccount)
	if err != nil {
		t.Fatalf("build the ergo provider: %v", err)
	}
	return providers
}

// authenticate reports whether account can log in over SASL PLAIN, using
// wsm's own transport so the answer speaks for wsm's real client rather than
// for a test-only one.
func (s *ergoServer) authenticate(ctx context.Context, account, secret string) error {
	transport := bot.New(bot.Options{
		Addr:     s.addr(),
		Account:  account,
		Password: secret,
		// A distinct nick: nick reservation is strict, so a leftover session
		// holding the account's own nick would make a good login look bad.
		Nick: loginNick(account),
	})
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		return fmt.Errorf("authenticate as %q: %w", account, err)
	}
	transport.Close()
	return nil
}

// loginNick derives a nick that cannot collide with the account's reserved one.
func loginNick(account string) string {
	if len(account) > 24 {
		account = account[:24]
	}
	return account + "c"
}
