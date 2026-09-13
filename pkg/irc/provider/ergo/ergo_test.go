package ergo

import (
	"testing"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/bot/bottest"
	"hop.top/wsm/pkg/irc/provider"
)

// TestRegisteredUnderErgo pins the wiring: importing this package registers
// the factory, so provider.Open finds it by the name the config uses.
func TestRegisteredUnderErgo(t *testing.T) {
	providers, err := provider.Open(Name, provider.Deps{Transport: bottest.New()})
	if err != nil {
		t.Fatalf("Open(%q): %v", Name, err)
	}

	// Every field is non-nil even on an impoverished runtime, so callers
	// never nil-check before asking for a capability.
	if providers.Accounts == nil || providers.Roles == nil || providers.Directory == nil {
		t.Fatalf("providers = %+v, want every field populated", providers)
	}
}

func TestOpenRequiresTransport(t *testing.T) {
	if _, err := provider.Open(Name, provider.Deps{}); err == nil {
		t.Fatal("a provider without a transport should fail to open")
	}
}

func TestNewWithoutAPIWithholdsAccountCaps(t *testing.T) {
	// The capability set is the contract reconcile reads before calling, so
	// a runtime without an API must advertise neither account capability.
	providers, err := New(provider.Deps{Transport: bottest.New()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	caps := providers.Accounts.Caps()
	if caps.HasAny(provider.CapCreateAccount | provider.CapSetPassword) {
		t.Errorf("caps = %d, want no account capabilities without an API", caps)
	}
}

func TestNewWithAPIDeclaresAccountCaps(t *testing.T) {
	deps := provider.Deps{
		Transport: bottest.New(),
		API:       &provider.APIConfig{BaseURL: "http://example.test", Token: "t"},
	}

	providers, err := New(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	caps := providers.Accounts.Caps()
	if !caps.Has(provider.CapCreateAccount | provider.CapSetPassword) {
		t.Errorf("caps = %d, want both account capabilities", caps)
	}
}

// TestNewWithAccountFeedsMyRole covers why the extra constructor exists:
// provider.Deps carries no bot name, so a trio built through the registry
// cannot recognise itself in a NAMES reply and reports no role. That caps
// grants rather than inflating them, but it also disables the live-MODE
// fallback, which is what NewWithAccount restores.
func TestNewWithAccountFeedsMyRole(t *testing.T) {
	transport := bottest.New()
	transport.SetNames(map[string]string{"wsm-bot": "@"})

	deps := provider.Deps{Transport: transport}

	anonymous, err := New(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := anonymous.Roles.MyRole(t.Context(), "#ws")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != "" {
		t.Errorf("role = %q, want empty without a configured account", got)
	}

	named, err := NewWithAccount(deps, "wsm-bot")
	if err != nil {
		t.Fatalf("NewWithAccount: %v", err)
	}
	got, err = named.Roles.MyRole(t.Context(), "#ws")
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if got != irc.RoleOp {
		t.Errorf("role = %q, want op once the account is known", got)
	}
}
