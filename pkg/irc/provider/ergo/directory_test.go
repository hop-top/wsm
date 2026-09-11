package ergo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc/bot/bottest"
	"hop.top/wsm/pkg/irc/provider"
)

// newStubAPI starts an httptest server and returns a Client for it. It lives
// here because both the directory and the account tests build stub APIs.
func newStubAPI(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return NewClient(&provider.APIConfig{BaseURL: srv.URL, Token: "token"})
}

func TestDirectoryCapsMatrix(t *testing.T) {
	// Listing, founder lookup and relay decoding need nothing privileged.
	// SAJOIN needs the sajoin oper capability and CS PURGE needs chanreg, so
	// neither is declared to a bot without oper.
	base := provider.CapListChannels | provider.CapChannelFounder | provider.CapRelayIdentity

	cases := []struct {
		name   string
		hasAPI bool
		oper   bool
		want   provider.Caps
	}{
		{name: "no api, no oper", want: base},
		{name: "api, no oper", hasAPI: true, want: base},
		{
			name: "oper",
			oper: true,
			want: base | provider.CapJoinHidden | provider.CapPurgeChannel,
		},
		{
			name:   "api and oper",
			hasAPI: true,
			oper:   true,
			want:   base | provider.CapJoinHidden | provider.CapPurgeChannel,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := bottest.New()
			transport.SetOper(tc.oper)

			dir := &Directory{transport: transport}
			if tc.hasAPI {
				dir.api = NewClient(&provider.APIConfig{BaseURL: "http://example.test", Token: "t"})
			}

			if got := dir.Caps(); got != tc.want {
				t.Errorf("caps = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestListChannelsPrefersAPI(t *testing.T) {
	// The API carries registration and owner, which LIST does not, so it is
	// preferred whenever it is configured.
	api := newStubAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"success":true,"channels":[{"name":"#probe","registered":true,"owner":"botty","userCount":2}]}`)
	})

	transport := bottest.New()
	dir := &Directory{api: api, transport: transport}

	got, err := dir.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(got) != 1 || got[0].Founder != "botty" {
		t.Fatalf("channels = %+v", got)
	}
	if len(transport.SentLines()) != 0 {
		t.Errorf("the API path must not also issue LIST, sent %q", transport.SentLines())
	}
}

func TestListChannelsFallsBackToLIST(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"LIST": {
			bottest.MustParse(":ergo.test 322 botty #probe 2 :a topic"),
			bottest.MustParse(":ergo.test 323 botty :End of LIST"),
		},
	})

	dir := &Directory{transport: transport}

	got, err := dir.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(got) != 1 || got[0].Name != "#probe" {
		t.Fatalf("channels = %+v", got)
	}
	// LIST carries no registration data, so the fallback must not invent it.
	if got[0].Registered || got[0].Founder != "" {
		t.Errorf("LIST fallback invented registration data: %+v", got[0])
	}
}

func TestFounderFromChanServ(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO": {
			notice("Channel #probe is registered"),
			notice("Founder: botty"),
		},
	})

	dir := &Directory{transport: transport}

	got, err := dir.Founder(context.Background(), "#probe")
	if err != nil {
		t.Fatalf("Founder: %v", err)
	}
	if got != "botty" {
		t.Errorf("founder = %q, want botty", got)
	}
}

func TestFounderUnregisteredIsEmpty(t *testing.T) {
	transport := bottest.New()
	transport.SetScript(map[string][]ircmsg.Message{
		"CS INFO": {notice("Channel #unreg is not registered")},
	})

	dir := &Directory{transport: transport}

	got, err := dir.Founder(context.Background(), "#unreg")
	if err != nil {
		t.Fatalf("Founder: %v", err)
	}
	if got != "" {
		t.Errorf("founder = %q, want empty for an unregistered channel", got)
	}
}

func TestJoinHiddenRequiresOper(t *testing.T) {
	transport := bottest.New()
	dir := &Directory{transport: transport}

	err := dir.JoinHidden(context.Background(), "#locked")
	if !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("JoinHidden without oper = %v, want ErrUnsupported", err)
	}
	if len(transport.SentLines()) != 0 {
		t.Errorf("an unsupported join must send nothing, got %q", transport.SentLines())
	}
}

func TestJoinHiddenAsOper(t *testing.T) {
	transport := bottest.New()
	transport.SetOper(true)
	dir := &Directory{transport: transport}

	if err := dir.JoinHidden(context.Background(), "#locked"); err != nil {
		t.Fatalf("JoinHidden: %v", err)
	}
	if !sentContains(transport.SentLines(), "SAJOIN #locked") {
		t.Errorf("sent = %q, want SAJOIN", transport.SentLines())
	}
}

// TestPurgeIsTwoStep is the correction the task text got wrong twice over:
// there is no CS UNPURGE, and PURGE ADD is a two-step confirmation. The first
// call returns a per-request code that must be echoed back as the SOLE
// trailing argument — appending anything after it restarts the confirmation
// instead of completing it, leaving the channel un-purged.
func TestPurgeIsTwoStep(t *testing.T) {
	transport := bottest.New()
	transport.SetOper(true)
	transport.SetScript(map[string][]ircmsg.Message{
		"CS PURGE ADD #purge5": {
			notice("Warning: you are about to empty this channel and remove it from the server."),
			notice("To confirm, run this command: /CS PURGE ADD #purge5 dmjqy"),
		},
		// Longest-prefix matching makes this the reply to the confirmation.
		"CS PURGE ADD #purge5 dmjqy": {
			notice("Successfully purged channel #purge5 from the server"),
		},
	})

	dir := &Directory{transport: transport}

	if err := dir.Purge(context.Background(), "#purge5"); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	sent := transport.SentLines()
	if len(sent) != 2 {
		t.Fatalf("sent = %q, want two commands", sent)
	}
	if sent[0] != "CS PURGE ADD #purge5" {
		t.Errorf("first command = %q", sent[0])
	}
	// The code must be last, with nothing after it.
	if sent[1] != "CS PURGE ADD #purge5 dmjqy" {
		t.Errorf("confirmation = %q, want the code as the sole trailing argument", sent[1])
	}
}

func TestPurgeRequiresOper(t *testing.T) {
	transport := bottest.New()
	dir := &Directory{transport: transport}

	err := dir.Purge(context.Background(), "#purge5")
	if !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("Purge without oper = %v, want ErrUnsupported", err)
	}
	if len(transport.SentLines()) != 0 {
		t.Errorf("an unsupported purge must send nothing, got %q", transport.SentLines())
	}
}

func TestPurgeWithoutCodeFails(t *testing.T) {
	// No code and no confirmation means the purge did not happen; reporting
	// success would leave a live channel wsm believes is gone.
	transport := bottest.New()
	transport.SetOper(true)
	transport.SetScript(map[string][]ircmsg.Message{
		"CS PURGE ADD": {notice("Invalid parameters")},
	})

	dir := &Directory{transport: transport}

	if err := dir.Purge(context.Background(), "#purge5"); err == nil {
		t.Fatal("a purge without a confirmation code should error")
	}
}

// TestUnpurgeUsesPurgeDel pins the other half of the correction: undoing a
// purge is CS PURGE DEL, not a nonexistent CS UNPURGE, and it needs no code.
func TestUnpurgeUsesPurgeDel(t *testing.T) {
	transport := bottest.New()
	transport.SetOper(true)
	transport.SetScript(map[string][]ircmsg.Message{
		"CS PURGE DEL": {notice("Successfully unpurged channel #purge5 from the server")},
	})

	dir := &Directory{transport: transport}

	if err := dir.Unpurge(context.Background(), "#purge5"); err != nil {
		t.Fatalf("Unpurge: %v", err)
	}

	sent := transport.SentLines()
	if len(sent) != 1 || sent[0] != "CS PURGE DEL #purge5" {
		t.Fatalf("sent = %q, want a single PURGE DEL", sent)
	}
	if sentContains(sent, "CS UNPURGE") {
		t.Error("CS UNPURGE is not a command on ergo")
	}
}

func TestUnpurgeIdempotent(t *testing.T) {
	transport := bottest.New()
	transport.SetOper(true)
	transport.SetScript(map[string][]ircmsg.Message{
		"CS PURGE DEL": {notice("Channel #purge4 wasn't previously purged from the server")},
	})

	dir := &Directory{transport: transport}

	if err := dir.Unpurge(context.Background(), "#purge4"); err != nil {
		t.Fatalf("un-purging a channel that was not purged should succeed: %v", err)
	}
}

func TestRelayedIdentityDelegatesToTag(t *testing.T) {
	dir := &Directory{transport: bottest.New()}

	tagged := bottest.MustParse("@draft/relaymsg=botty :slackuser/slack!~u@host PRIVMSG #relay :hi")
	identity, ok := dir.RelayedIdentity(tagged)
	if !ok {
		t.Fatal("a tagged message should decode")
	}
	if identity.Nick != "slackuser" || identity.Service != "slack" || identity.Bot != "botty" {
		t.Errorf("identity = %+v", identity)
	}

	// Without the server-set tag it is an ordinary message, whatever the
	// nick looks like.
	untagged := bottest.MustParse(":slackuser/slack!~u@host PRIVMSG #relay :hi")
	if _, ok := dir.RelayedIdentity(untagged); ok {
		t.Error("an untagged message must not decode as relayed")
	}
}
