package ergo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"hop.top/wsm/pkg/irc/bot/bottest"
	"hop.top/wsm/pkg/irc/provider"
)

// TestAccountsCapsMatrix is the whole point of gating: a capability is
// declared only when the runtime can honour it. The API supplies account
// creation and password rotation; oper supplies suspension. Declaring one the
// runtime cannot honour would make reconcile treat a guaranteed failure as a
// real attempt.
func TestAccountsCapsMatrix(t *testing.T) {
	cases := []struct {
		name   string
		hasAPI bool
		oper   bool
		want   provider.Caps
	}{
		{
			name: "no api, no oper",
			want: 0,
		},
		{
			name: "no api, oper",
			oper: true,
			want: provider.CapSuspendAccount,
		},
		{
			name:   "api, no oper",
			hasAPI: true,
			want:   provider.CapCreateAccount | provider.CapSetPassword,
		},
		{
			name:   "api and oper",
			hasAPI: true,
			oper:   true,
			want:   provider.CapCreateAccount | provider.CapSetPassword | provider.CapSuspendAccount,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := bottest.New()
			transport.SetOper(tc.oper)

			accounts := &Accounts{transport: transport}
			if tc.hasAPI {
				accounts.api = NewClient(&provider.APIConfig{BaseURL: "http://example.test", Token: "t"})
			}

			if got := accounts.Caps(); got != tc.want {
				t.Errorf("caps = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAccountsWithoutAPIAreUnsupported(t *testing.T) {
	transport := bottest.New()
	accounts := &Accounts{transport: transport}

	ctx := context.Background()
	if err := accounts.CreateAccount(ctx, "alice", "secret"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("CreateAccount = %v, want ErrUnsupported", err)
	}
	if err := accounts.SetPassword(ctx, "alice", "secret"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("SetPassword = %v, want ErrUnsupported", err)
	}
}

func TestSuspendRequiresOper(t *testing.T) {
	transport := bottest.New()
	accounts := &Accounts{transport: transport}

	err := accounts.Suspend(context.Background(), "alice")
	if !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("Suspend without oper = %v, want ErrUnsupported", err)
	}
	if len(transport.SentLines()) != 0 {
		t.Errorf("an unsupported suspend must send nothing, got %q", transport.SentLines())
	}
}

func TestSuspendAsOperSendsCommand(t *testing.T) {
	transport := bottest.New()
	transport.SetOper(true)
	accounts := &Accounts{transport: transport}

	if err := accounts.Suspend(context.Background(), "alice"); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	sent := transport.SentLines()
	if len(sent) != 1 || !strings.Contains(sent[0], "alice") {
		t.Fatalf("sent = %q, want one suspend naming the account", sent)
	}
}

func TestCreateAccountPassesThroughErrExists(t *testing.T) {
	// The sentinel has to survive the provider layer, because the caller
	// keys "keep the existing secret" off it.
	srv := newStubAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"success":false,"error":"Name reserved","errorCode":"ACCOUNT_EXISTS"}`)
	})

	accounts := &Accounts{api: srv, transport: bottest.New()}

	err := accounts.CreateAccount(context.Background(), "alice", "secret")
	if !errors.Is(err, provider.ErrExists) {
		t.Fatalf("CreateAccount = %v, want ErrExists", err)
	}
}
