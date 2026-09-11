package ergo_test

import (
	"context"
	"testing"
	"time"

	"hop.top/wsm/pkg/irc/bot"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provider/conformance"
	"hop.top/wsm/pkg/irc/provider/ergo"
)

// TestConformance runs the shared provider suite against a live ergo server.
//
// It skips unless IRC_CONFORMANCE_ERGO names one. The ergo half of the suite
// wants the HTTP API (account creation) and oper credentials (suspension, the
// privileged directory operations); without them the capability-gated cases
// assert the ErrUnsupported side of the contract instead.
func TestConformance(t *testing.T) {
	conformance.Run(t, ergo.Name, func() (provider.Providers, func()) {
		target, _, err := conformance.Lookup(ergo.Name)
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

		var api *provider.APIConfig
		if target.HasAPI() {
			api = &provider.APIConfig{BaseURL: target.APIURL, Token: target.Token}
		}

		// NewWithAccount rather than New: MyRole has to find the bot in a
		// NAMES reply and CS TRANSFER has to name the new founder, and
		// neither works through the registry factory, which knows no account.
		providers, err := ergo.NewWithAccount(provider.Deps{Transport: transport, API: api}, account)
		if err != nil {
			transport.Close()
			t.Fatalf("conformance: build ergo provider: %v", err)
		}
		return providers, transport.Close
	})
}
