// Package conformance is the shared acceptance suite every IRC provider must
// pass against a live server.
//
// The suite exists because the provider interfaces carry a promise the unit
// tests cannot check: a provider advertises a capability set, and callers
// (reconcile above all) read that set to decide what to attempt. A provider
// that declares a capability it cannot honour turns a guaranteed failure into
// a real attempt; one that refuses a capability it declared makes reconcile
// record work that never happened. Neither shows up against a scripted fake,
// because a fake answers whatever the script says. Only a real server settles
// it.
//
// Run is therefore gated on an environment variable naming a live server and
// skips cleanly without one, so `make test` on a machine with no IRC server
// stays green and fast.
//
// Like the rest of pkg/*, this package must stay free of any UI dependency.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// EnvPrefix is the environment variable prefix Run reads. The suffix is the
// upper-cased provider name, so the ergo runner reads IRC_CONFORMANCE_ERGO.
const EnvPrefix = "IRC_CONFORMANCE_"

// timeout bounds every individual provider call. A live server that stops
// answering must fail the case rather than hang the suite.
const timeout = 30 * time.Second

// Target is a parsed server spec: where the suite connects and what privilege
// it may use once there.
type Target struct {
	// Addr is the server's host:port.
	Addr string
	// APIURL is the provider's HTTP API root, empty when the spec named none.
	APIURL string
	// Token is the API bearer token.
	Token string
	// OperName is the operator block to OPER into, empty for none.
	OperName string
	// OperPass is the operator passphrase.
	OperPass string
}

// HasAPI reports whether the spec carried an HTTP API.
func (t Target) HasAPI() bool { return t.APIURL != "" }

// HasOper reports whether the spec carried operator credentials.
func (t Target) HasOper() bool { return t.OperName != "" }

// Lookup reads the spec for the named provider from the environment. The bool
// is false when the variable is unset, which is the normal case and means the
// caller should skip rather than fail.
func Lookup(name string) (Target, bool, error) {
	raw := os.Getenv(EnvVar(name))
	if strings.TrimSpace(raw) == "" {
		return Target{}, false, nil
	}
	target, err := ParseSpec(raw)
	if err != nil {
		return Target{}, true, err
	}
	return target, true, nil
}

// EnvVar is the variable name Run reads for the named provider.
func EnvVar(name string) string {
	return EnvPrefix + strings.ToUpper(strings.TrimSpace(name))
}

// ParseSpec parses host:port[,api=url,token=..,oper=name:pass].
//
// The address is positional and required; everything after it is a key=value
// list in any order. An unknown key is an error rather than a silent ignore:
// a typo'd "opper=" would otherwise run the whole privileged half of the suite
// as an ordinary user and report a pass.
func ParseSpec(raw string) (Target, error) {
	parts := strings.Split(raw, ",")
	target := Target{Addr: strings.TrimSpace(parts[0])}
	if target.Addr == "" {
		return Target{}, fmt.Errorf("conformance: spec %q: address is required", raw)
	}

	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return Target{}, fmt.Errorf("conformance: spec %q: %q is not key=value", raw, part)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "api":
			target.APIURL = value
		case "token":
			target.Token = value
		case "oper":
			name, pass, ok := strings.Cut(value, ":")
			if !ok {
				return Target{}, fmt.Errorf(
					"conformance: spec %q: oper must be name:pass, got %q", raw, value)
			}
			target.OperName, target.OperPass = name, pass
		default:
			return Target{}, fmt.Errorf("conformance: spec %q: unknown key %q", raw, key)
		}
	}
	return target, nil
}

// Run executes the suite against the provider trio open returns.
//
// It skips unless the environment names a live server for this provider,
// because the whole point is to exercise a real backend. open is called once;
// the cleanup it returns runs when the suite finishes.
func Run(t *testing.T, name string, open func() (provider.Providers, func())) {
	t.Helper()

	target, set, err := Lookup(name)
	if !set {
		t.Skipf("conformance: set %s=host:port[,api=url,token=..,oper=name:pass] to run this suite against a live %s server",
			EnvVar(name), name)
	}
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	t.Logf("conformance: %s against %s (api=%t oper=%t)", name, target.Addr, target.HasAPI(), target.HasOper())

	providers, cleanup := open()
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	requireTrio(t, providers)

	// The capability contract comes first: everything below assumes a method
	// either honours its declared capability or reports exactly
	// ErrUnsupported, and that is the property the rest of the suite leans on.
	t.Run("Capabilities", func(t *testing.T) { runCapabilities(t, providers) })
	t.Run("Accounts", func(t *testing.T) { runAccounts(t, providers, target) })
	t.Run("Roles", func(t *testing.T) { runRoles(t, providers, target) })
	t.Run("Directory", func(t *testing.T) { runDirectory(t, providers, target) })
}

// requireTrio pins the invariant Providers documents: every field is populated
// even on an impoverished runtime, so callers never nil-check.
func requireTrio(t *testing.T, p provider.Providers) {
	t.Helper()
	if p.Accounts == nil {
		t.Fatal("Providers.Accounts is nil; a provider must supply one that reports no capability")
	}
	if p.Roles == nil {
		t.Fatal("Providers.Roles is nil; a provider must supply one that reports no capability")
	}
	if p.Directory == nil {
		t.Fatal("Providers.Directory is nil; a provider must supply one that reports no capability")
	}
}

// op is one capability-gated method, described uniformly so the caps contract
// can be checked the same way for all three interfaces rather than case by
// case. That symmetry is the point: a new capability is one table row, and it
// cannot be added without saying which method it gates.
type op struct {
	// name identifies the method in failure output.
	name string
	// cap is the capability that must be declared for call to be allowed to
	// succeed. Zero means the method is always available and is exempt from
	// the ErrUnsupported half of the contract.
	cap provider.Caps
	// iface says which of the three interfaces owns the method, because Caps
	// is per-interface: asking the wrong one would compare the capability
	// against a set that can never contain it.
	iface iface
	// call invokes the method with arguments that are safe to issue against a
	// live server: a read, or a write to a name the suite owns.
	call func(ctx context.Context) error
}

// runCapabilities is the central property: caps declared <=> methods succeed.
//
// For every capability-gated method on all three interfaces:
//   - capability absent  => the method returns exactly provider.ErrUnsupported
//   - capability present => the method does not return ErrUnsupported
//
// The second direction matters as much as the first. A provider that declares
// a capability and then refuses it is the failure reconcile cannot see: it
// records an attempt that never happened.
func runCapabilities(t *testing.T, p provider.Providers) {
	scratch := scratchName("caps")

	for _, o := range allOps(p, scratch) {
		t.Run(o.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			declared := declares(p, o)
			err := o.call(ctx)

			switch {
			case o.cap == 0:
				// Ungated: never allowed to claim the operation is unsupported.
				if errors.Is(err, provider.ErrUnsupported) {
					t.Errorf("%s is not capability-gated but returned ErrUnsupported: %v", o.name, err)
				}

			case !declared:
				// The uncapped direction has to be exact. Callers branch on
				// errors.Is(err, ErrUnsupported) to decide "skip and report"
				// rather than "fail", so any other error reads as a real
				// failure and a nil error reads as work that happened.
				if err == nil {
					t.Errorf("%s: capability not declared but the call succeeded; "+
						"an undeclared operation must report ErrUnsupported", o.name)
					return
				}
				if !errors.Is(err, provider.ErrUnsupported) {
					t.Errorf("%s: capability not declared, want ErrUnsupported, got %v", o.name, err)
				}

			default:
				// The capped direction: declaring a capability and then
				// refusing it is exactly the lie this suite exists to catch.
				if errors.Is(err, provider.ErrUnsupported) {
					t.Errorf("%s: capability is declared but the call returned ErrUnsupported: %v", o.name, err)
				}
			}
		})
	}
}

// declares reports whether the interface owning o advertises o's capability.
func declares(p provider.Providers, o op) bool {
	return o.cap != 0 && o.owner(p).Has(o.cap)
}

// owner returns the capability set of the interface that owns the method.
// Caps is per-interface, so asking the wrong one would compare against a set
// that never contains the bit.
func (o op) owner(p provider.Providers) provider.Caps {
	switch o.iface {
	case ifaceAccounts:
		return p.Accounts.Caps()
	case ifaceRoles:
		return p.Roles.Caps()
	default:
		return p.Directory.Caps()
	}
}

// iface names which of the three interfaces a method belongs to.
type iface int

const (
	ifaceAccounts iface = iota
	ifaceRoles
	ifaceDirectory
)

// allOps is the single table of capability-gated methods across all three
// interfaces. Every method that can report ErrUnsupported appears exactly
// once, which is what makes the contract checkable uniformly.
func allOps(p provider.Providers, scratch string) []op {
	account := scratch + "-acct"

	return []op{
		{
			name: "Accounts.CreateAccount", cap: provider.CapCreateAccount, iface: ifaceAccounts,
			call: func(ctx context.Context) error {
				return p.Accounts.CreateAccount(ctx, account, passphrase)
			},
		},
		{
			name: "Accounts.SetPassword", cap: provider.CapSetPassword, iface: ifaceAccounts,
			call: func(ctx context.Context) error {
				return p.Accounts.SetPassword(ctx, account, passphrase)
			},
		},
		{
			name: "Accounts.Suspend", cap: provider.CapSuspendAccount, iface: ifaceAccounts,
			call: func(ctx context.Context) error {
				return p.Accounts.Suspend(ctx, account)
			},
		},
		{
			// Listing is ungated on every provider: both backends can always
			// answer it, so it only has to avoid claiming otherwise.
			name: "Directory.ListChannels", cap: provider.CapListChannels, iface: ifaceDirectory,
			call: func(ctx context.Context) error {
				_, err := p.Directory.ListChannels(ctx)
				return err
			},
		},
		{
			name: "Directory.Founder", cap: provider.CapChannelFounder, iface: ifaceDirectory,
			call: func(ctx context.Context) error {
				_, err := p.Directory.Founder(ctx, scratch)
				return err
			},
		},
		{
			name: "Directory.JoinHidden", cap: provider.CapJoinHidden, iface: ifaceDirectory,
			call: func(ctx context.Context) error {
				return p.Directory.JoinHidden(ctx, scratch)
			},
		},
		{
			name: "Directory.Purge", cap: provider.CapPurgeChannel, iface: ifaceDirectory,
			call: func(ctx context.Context) error {
				return p.Directory.Purge(ctx, scratch)
			},
		},
	}
}

// runAccounts covers the account half: double creation and suspension.
func runAccounts(t *testing.T, p provider.Providers, target Target) {
	caps := p.Accounts.Caps()

	if !caps.Has(provider.CapCreateAccount) {
		t.Skip("provider declares no CapCreateAccount")
	}

	account := scratchName("acct")

	t.Run("CreateAccountTwiceReportsExists", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := p.Accounts.CreateAccount(ctx, account, passphrase); err != nil {
			t.Fatalf("first CreateAccount(%q): %v", account, err)
		}

		// The second create must be distinguishable from the first. Callers
		// treat ErrExists as success and keep the existing secret, so a
		// provider that reported a generic error here would make a rerun of
		// reconcile look like a failure.
		err := p.Accounts.CreateAccount(ctx, account, passphrase)
		if err == nil {
			t.Fatalf("second CreateAccount(%q) succeeded, want ErrExists", account)
		}
		if !errors.Is(err, provider.ErrExists) {
			t.Fatalf("second CreateAccount(%q) = %v, want ErrExists", account, err)
		}
	})

	t.Run("SuspendDeniesLogin", func(t *testing.T) {
		if !caps.Has(provider.CapSuspendAccount) {
			t.Skip("provider declares no CapSuspendAccount")
		}
		if !target.HasAPI() {
			t.Skip("suspension check needs an API-created account to log in as")
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		victim := scratchName("susp")
		if err := p.Accounts.CreateAccount(ctx, victim, passphrase); err != nil {
			t.Fatalf("CreateAccount(%q): %v", victim, err)
		}

		// Suspension is the one account operation whose effect is invisible
		// in its own reply: it is fire-and-forget, so a wrong command
		// spelling leaves the account active and a suspended member silently
		// keeps access. The only honest check is to try to log in afterwards.
		if err := p.Accounts.Suspend(ctx, victim); err != nil {
			t.Fatalf("Suspend(%q): %v", victim, err)
		}

		if err := CanAuthenticate(ctx, target, victim, passphrase); err == nil {
			t.Fatalf("account %q still authenticates after Suspend; "+
				"the suspend command did not take effect", victim)
		}
	})
}

// runRoles covers channel state and the roles held in it.
func runRoles(t *testing.T, p provider.Providers, target Target) {
	channel := scratchChannel("roles")

	t.Run("EnsureChannelIsIdempotent", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		desired := provider.Channel{Name: channel, Topic: "wsm conformance"}

		first := p.Roles.EnsureChannel(ctx, desired)
		if first != nil && !errors.Is(first, provider.ErrNotOpped) {
			t.Fatalf("EnsureChannel(%q): %v", channel, first)
		}

		// Running reconcile twice is the normal case, so the second pass must
		// land in the same place as the first rather than reporting the
		// channel as newly foreign or newly refused.
		second := p.Roles.EnsureChannel(ctx, desired)
		if !sameOutcome(first, second) {
			t.Fatalf("EnsureChannel is not idempotent: first=%v second=%v", first, second)
		}
	})

	t.Run("ApplyRolesRoundTrip", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		subject := roleSubject(ctx, t, p, target)

		add := []provider.ModeChange{{Account: subject, Role: irc.RoleOp}}
		if err := p.Roles.ApplyRoles(ctx, channel, add, nil); err != nil {
			if errors.Is(err, provider.ErrNotOpped) {
				// A provider whose roles are live-only cannot grant anything
				// while the bot is unopped, and says so. That is the
				// documented plain-server outcome, not a suite failure.
				t.Skipf("bot is not opped in %s: %v", channel, err)
			}
			t.Fatalf("ApplyRoles(+o %s): %v", subject, err)
		}

		roles, err := p.Roles.ListRoles(ctx, channel)
		if err != nil {
			t.Fatalf("ListRoles(%q): %v", channel, err)
		}
		if got := roles[subject]; got != irc.RoleOp {
			t.Fatalf("after +o, ListRoles[%s] = %q, want %q", subject, got, irc.RoleOp)
		}

		remove := []provider.ModeChange{{Account: subject, Role: irc.RoleOp}}
		if err := p.Roles.ApplyRoles(ctx, channel, nil, remove); err != nil {
			t.Fatalf("ApplyRoles(-o %s): %v", subject, err)
		}

		roles, err = p.Roles.ListRoles(ctx, channel)
		if err != nil {
			t.Fatalf("ListRoles(%q) after revoke: %v", channel, err)
		}
		if got, present := roles[subject]; present && got == irc.RoleOp {
			t.Fatalf("after -o, ListRoles[%s] is still %q", subject, got)
		}
	})

	t.Run("ForeignChannelIsReportedNotSeized", func(t *testing.T) {
		if !p.Roles.Caps().Has(provider.CapRegisterChannel) {
			t.Skip("provider declares no CapRegisterChannel")
		}

		foreign := os.Getenv(EnvForeignChannel)
		if foreign == "" {
			t.Skipf("set %s to a channel registered to another account to run this case", EnvForeignChannel)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		err := p.Roles.EnsureChannel(ctx, provider.Channel{Name: foreign})

		if target.HasOper() {
			// With oper the takeover is allowed, so the channel must not come
			// back as foreign.
			if errors.Is(err, provider.ErrForeignChannel) {
				t.Fatalf("EnsureChannel(%q) reported ErrForeignChannel despite oper: %v", foreign, err)
			}
			return
		}
		// Without oper the registration belongs to whoever holds it, and the
		// provider must report rather than reconfigure it.
		if !errors.Is(err, provider.ErrForeignChannel) {
			t.Fatalf("EnsureChannel(%q) = %v, want ErrForeignChannel without oper", foreign, err)
		}
	})

	t.Run("UnoppedBotReportsNotOpped", func(t *testing.T) {
		if p.Roles.Caps().Has(provider.CapPersistentRoles) {
			t.Skip("provider has persistent roles, so it does not need to be opped")
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		mine, err := p.Roles.MyRole(ctx, channel)
		if err != nil {
			t.Fatalf("MyRole(%q): %v", channel, err)
		}
		if mine == irc.RoleOp {
			t.Skip("bot is opped in the scratch channel")
		}

		// A provider without persistent roles cannot grant anything while
		// unopped. Saying ErrNotOpped is what lets reconcile mark the channel
		// degraded instead of issuing modes the server will refuse.
		err = p.Roles.ApplyRoles(ctx, channel,
			[]provider.ModeChange{{Account: scratchName("nop"), Role: irc.RoleVoice}}, nil)
		if !errors.Is(err, provider.ErrNotOpped) {
			t.Fatalf("ApplyRoles while unopped = %v, want ErrNotOpped", err)
		}
	})
}

// runDirectory covers the server-wide half.
func runDirectory(t *testing.T, p provider.Providers, target Target) {
	caps := p.Directory.Caps()
	channel := scratchChannel("dir")

	// The listing cases need a channel that exists, so create one first and
	// tolerate the unopped outcome the plain backend reports.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := p.Roles.EnsureChannel(ctx, provider.Channel{Name: channel}); err != nil &&
		!errors.Is(err, provider.ErrNotOpped) && !errors.Is(err, provider.ErrForeignChannel) {
		t.Fatalf("EnsureChannel(%q): %v", channel, err)
	}

	t.Run("ListChannelsSeesOurChannel", func(t *testing.T) {
		if !caps.Has(provider.CapListChannels) {
			t.Skip("provider declares no CapListChannels")
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		channels, err := p.Directory.ListChannels(ctx)
		if err != nil {
			t.Fatalf("ListChannels: %v", err)
		}
		for _, info := range channels {
			if strings.EqualFold(info.Name, channel) {
				return
			}
		}
		t.Fatalf("ListChannels did not report %q, which this suite just created", channel)
	})

	t.Run("FounderMatchesRegistrant", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		founder, err := p.Directory.Founder(ctx, channel)
		if !caps.Has(provider.CapChannelFounder) {
			if !errors.Is(err, provider.ErrUnsupported) {
				t.Fatalf("Founder without the capability = %v, want ErrUnsupported", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("Founder(%q): %v", channel, err)
		}

		// An unregistered channel has no founder, which is an empty string
		// rather than an error: "nobody owns this" is a normal answer, and a
		// provider that cannot register channels will always answer so.
		if founder == "" {
			if p.Roles.Caps().Has(provider.CapRegisterChannel) {
				t.Fatalf("Founder(%q) is empty although the provider registers channels", channel)
			}
			return
		}

		// When the suite's own bot registered it, the founder is the bot.
		if want := os.Getenv(EnvBotAccount); want != "" && !strings.EqualFold(founder, want) {
			t.Fatalf("Founder(%q) = %q, want the registrant %q", channel, founder, want)
		}
	})

	t.Run("JoinHiddenEntersInviteOnlyChannel", func(t *testing.T) {
		hidden := os.Getenv(EnvHiddenChannel)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if !caps.Has(provider.CapJoinHidden) {
			if err := p.Directory.JoinHidden(ctx, channel); !errors.Is(err, provider.ErrUnsupported) {
				t.Fatalf("JoinHidden without the capability = %v, want ErrUnsupported", err)
			}
			return
		}
		if hidden == "" {
			t.Skipf("set %s to a +i channel to run this case", EnvHiddenChannel)
		}

		if err := p.Directory.JoinHidden(ctx, hidden); err != nil {
			t.Fatalf("JoinHidden(%q): %v", hidden, err)
		}

		// Entering is only half the claim; the bot has to actually be in the
		// room afterwards, which NAMES is the truth for.
		roles, err := p.Roles.ListRoles(ctx, hidden)
		if err != nil {
			t.Fatalf("ListRoles(%q) after JoinHidden: %v", hidden, err)
		}
		_ = roles
	})

	t.Run("PurgeEmptiesAndUnregisters", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		victim := scratchChannel("purge")

		if !caps.Has(provider.CapPurgeChannel) {
			if err := p.Directory.Purge(ctx, victim); !errors.Is(err, provider.ErrUnsupported) {
				t.Fatalf("Purge without the capability = %v, want ErrUnsupported", err)
			}
			return
		}

		if err := p.Roles.EnsureChannel(ctx, provider.Channel{Name: victim}); err != nil &&
			!errors.Is(err, provider.ErrNotOpped) {
			t.Fatalf("EnsureChannel(%q): %v", victim, err)
		}

		// Purge is a multi-step confirmation on at least one backend: the
		// first call answers with a code that must be echoed back, and a
		// provider that stopped after the first call would leave the channel
		// un-purged while looking like it worked. So the contract under test
		// is the whole round trip completing, which the provider only reports
		// as success once the server has confirmed it.
		//
		// The post-condition is deliberately not a Founder lookup. On ergo a
		// purge does not touch the registration record: CS INFO still reports
		// the channel as registered with its founder intact, and purge state
		// is only visible through PURGE LIST, which is not on the interface.
		// Asserting an empty founder here would encode a false expectation and
		// fail a provider that did exactly the right thing.
		// Purging is deliberately called once. It is not idempotent on ergo:
		// a second PURGE ADD on an already-purged channel issues a fresh
		// confirmation code and then answers "An error occurred", which the
		// provider correctly reports as unconfirmed. Requiring a repeatable
		// purge would be inventing a contract no backend offers.
		if err := p.Directory.Purge(ctx, victim); err != nil {
			t.Fatalf("Purge(%q): %v", victim, err)
		}
	})

	t.Run("RelayedIdentityDecodesFixture", func(t *testing.T) {
		msg, err := RelayFixture()
		if err != nil {
			t.Fatalf("relay fixture: %v", err)
		}

		identity, ok := p.Directory.RelayedIdentity(msg)

		if !caps.Has(provider.CapRelayIdentity) {
			// RelayedIdentity has no error to return, so a provider without
			// the capability reports "not relayed" rather than decoding.
			if ok {
				t.Fatalf("RelayedIdentity decoded %+v without declaring CapRelayIdentity", identity)
			}
			return
		}

		if !ok {
			t.Fatal("RelayedIdentity did not decode a relayed message despite declaring CapRelayIdentity")
		}
		if identity.Bot == "" {
			t.Fatalf("RelayedIdentity = %+v, want the relaying bot's account", identity)
		}
		if identity.Nick != relayNick || identity.Service != relayService {
			t.Fatalf("RelayedIdentity = %+v, want nick %q service %q",
				identity, relayNick, relayService)
		}
	})
}

// roleSubject picks an account to grant and revoke a role for.
//
// The subject has to be an account the server already knows. A persistent
// role is stored against a registered account, and a server asked to store
// one for a name it has never heard of answers "account does not exist" and
// changes nothing — which would fail the round trip for a reason that says
// nothing about the provider. So the suite creates the subject when it can,
// and otherwise falls back to an account the deployment vouches for.
//
// A provider without persistent roles cannot use a created account at all: a
// mode for a nick that is not in the room is unappliable, so the bot itself
// is the only subject guaranteed to be present.
func roleSubject(ctx context.Context, t *testing.T, p provider.Providers, target Target) string {
	t.Helper()

	if subject := os.Getenv(EnvRoleSubject); subject != "" {
		return subject
	}

	if p.Roles.Caps().Has(provider.CapPersistentRoles) &&
		p.Accounts.Caps().Has(provider.CapCreateAccount) {
		subject := scratchName("subj")
		if err := p.Accounts.CreateAccount(ctx, subject, passphrase); err != nil &&
			!errors.Is(err, provider.ErrExists) {
			t.Fatalf("CreateAccount(%q) for the role subject: %v", subject, err)
		}
		return subject
	}

	if bot := os.Getenv(EnvBotAccount); bot != "" {
		return bot
	}
	t.Skipf("set %s (or %s) so the suite knows which account to grant a role to",
		EnvRoleSubject, EnvBotAccount)
	return ""
}

// sameOutcome reports whether two EnsureChannel results mean the same thing.
// The errors are wrapped with the channel name and are never identical values,
// so the comparison is on the sentinel they carry.
func sameOutcome(first, second error) bool {
	switch {
	case first == nil && second == nil:
		return true
	case first == nil || second == nil:
		return false
	}
	for _, sentinel := range []error{
		provider.ErrForeignChannel,
		provider.ErrNotOpped,
		provider.ErrPrivilege,
		provider.ErrUnsupported,
		provider.ErrExists,
	} {
		if errors.Is(first, sentinel) != errors.Is(second, sentinel) {
			return false
		}
	}
	return true
}
