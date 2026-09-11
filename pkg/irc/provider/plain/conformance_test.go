package plain_test

import (
	"context"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc/bot"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provider/conformance"
	"hop.top/wsm/pkg/irc/provider/plain"
)

// TestConformance runs the shared provider suite against a live server driven
// without services.
//
// It skips unless IRC_CONFORMANCE_PLAIN names one. CI points this at the same
// ergo the ergo runner uses: the provider simply never issues a services
// command, which is exactly the property worth checking. The suite therefore
// asserts the ErrUnsupported side of the contract for every account and
// registration operation, and the live-mode side for roles.
func TestConformance(t *testing.T) {
	conformance.Run(t, plain.Name, func() (provider.Providers, func()) {
		target, _, err := conformance.Lookup(plain.Name)
		if err != nil {
			t.Fatalf("conformance: %v", err)
		}

		account := conformance.BotAccount()
		transport := bot.New(conformance.Deps(target, account, conformance.BotSecret()))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := transport.Connect(ctx); err != nil {
			t.Fatalf("conformance: connect to %s: %v", target.Addr, err)
		}

		// No API is passed even when the spec carries one: a plain provider
		// that reached for an HTTP API would not be services-less.
		providers, err := plain.New(provider.Deps{Transport: transport})
		if err != nil {
			transport.Close()
			t.Fatalf("conformance: build plain provider: %v", err)
		}

		// The transport carries no identity, and the bot's own NAMES entry
		// decides whether a role apply can succeed at all, so the provider
		// has to be told what the bot is called.
		if setter, ok := providers.Roles.(interface{ SetNick(string) }); ok {
			setter.SetNick(account)
		}
		return providers, transport.Close
	})
}
