package bot

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc/bot/bottest"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
)

// fakeRecorder collects the events capture hands it, and can be made to fail
// so the cursor guard is exercised.
type fakeRecorder struct {
	events []model.Event
	err    error
	calls  int
}

func (r *fakeRecorder) Record(_ context.Context, ev model.Event) (model.Event, error) {
	r.calls++
	if r.err != nil {
		return model.Event{}, r.err
	}
	r.events = append(r.events, ev)
	return ev, nil
}

// mapReverse resolves accounts from a fixed table, and can report an error to
// exercise the degraded path.
type mapReverse struct {
	byAccount map[string]string
	err       error
}

func (m mapReverse) ProfileFor(_ context.Context, account, _ string) (string, bool, error) {
	if m.err != nil {
		return "", false, m.err
	}
	id, ok := m.byAccount[account]
	return id, ok, nil
}

// relayDirectory decodes a message as relayed when it carries the
// draft/relaymsg tag, mirroring what ergo puts on the wire. Only
// RelayedIdentity is exercised here; the rest satisfies the interface.
type relayDirectory struct{}

func (relayDirectory) Caps() provider.Caps { return provider.CapRelayIdentity }
func (relayDirectory) ListChannels(context.Context) ([]provider.ChannelInfo, error) {
	return nil, nil
}
func (relayDirectory) Founder(context.Context, string) (string, error) { return "", nil }
func (relayDirectory) JoinHidden(context.Context, string) error        { return nil }
func (relayDirectory) Purge(context.Context, string) error             { return nil }

func (relayDirectory) RelayedIdentity(msg ircmsg.Message) (provider.RelayIdentity, bool) {
	ok, bot := msg.GetTag("draft/relaymsg")
	if !ok || bot == "" {
		return provider.RelayIdentity{}, false
	}
	// The relayed nick is the nick/service form; the tag names the
	// authenticated relaying bot.
	nick, service, _ := strings.Cut(msg.Nick(), "/")
	return provider.RelayIdentity{Nick: nick, Service: service, Bot: bot}, true
}

// mustParse is bottest.MustParse, wrapped so the intent reads at call sites.
func mustParse(raw string) ircmsg.Message { return bottest.MustParse(raw) }

// decode pulls the MessageData payload out of a recorded event.
func decode(t *testing.T, ev model.Event) model.MessageData {
	t.Helper()
	var data model.MessageData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("unmarshal message data: %v", err)
	}
	return data
}

// newCapture builds a Capture wired to a recorder, with the account table the
// attribution cases share.
func newCapture(rec Recorder, dir provider.DirectoryProvider) *Capture {
	return &Capture{
		Directory:   dir,
		Reverse:     mapReverse{byAccount: map[string]string{"alice": "p-alice", "botty": "p-relaybot"}},
		ServerID:    "srv",
		BotAccount:  "wsm-bot",
		WorkspaceID: "ws-1",
		WriterID:    "writer-1",
		Recorder:    rec,
	}
}

// TestCaptureAttribution walks every Attribution value the event log can
// carry, because each one means something different to a consumer reading the
// log back.
func TestCaptureAttribution(t *testing.T) {
	cases := []struct {
		name            string
		line            string
		directory       provider.DirectoryProvider
		reverseErr      error
		wantAgentID     string
		wantAttribution string
		wantNick        string
		wantContent     string
	}{
		{
			name:            "account tag resolves to a profile",
			line:            "@time=2026-09-11T10:22:10.885Z;account=alice;msgid=pxsd :alice!~u@h.irc PRIVMSG #tags :hello from alice",
			wantAgentID:     "p-alice",
			wantAttribution: AttributionAccount,
			wantNick:        "alice",
			wantContent:     "hello from alice",
		},
		{
			// The server vouched for the account even though wsm cannot
			// name the profile, so the evidence stays "account".
			name:            "account tag unknown to the resolver stays account",
			line:            "@time=2026-09-11T10:22:10.885Z;account=stranger;msgid=abcd :stranger!~u@h.irc PRIVMSG #tags :who am i",
			wantAgentID:     "",
			wantAttribution: AttributionAccount,
			wantNick:        "stranger",
			wantContent:     "who am i",
		},
		{
			name:            "no account tag falls back to the nick",
			line:            "@time=2026-09-11T10:22:10.885Z;msgid=abcd :alice!~u@h.irc PRIVMSG #tags :no account tag here",
			wantAgentID:     "p-alice",
			wantAttribution: AttributionNick,
			wantNick:        "alice",
			wantContent:     "no account tag here",
		},
		{
			name:            "relayed message attributes to the relaying bot",
			line:            "@draft/relaymsg=botty;msgid=sn3f;time=2026-09-11T10:23:20.402Z :slackuser/slack!~u@h.irc PRIVMSG #relay :relayed hello",
			directory:       relayDirectory{},
			wantAgentID:     "p-relaybot",
			wantAttribution: AttributionRelay,
			// The bridge's claim about who spoke, recorded as-is.
			wantNick:    "slackuser",
			wantContent: "relayed hello",
		},
		{
			// A relay whose bot is unknown degrades to the weaker nick
			// evidence rather than claiming a relay it cannot vouch for.
			name:            "relayed message with an unknown bot degrades to nick",
			line:            "@draft/relaymsg=unknownbot;msgid=sn3f;time=2026-09-11T10:23:20.402Z :slackuser/slack!~u@h.irc PRIVMSG #relay :relayed hello",
			directory:       relayDirectory{},
			wantAgentID:     "",
			wantAttribution: AttributionNick,
			wantNick:        "slackuser",
			wantContent:     "relayed hello",
		},
		{
			name:            "unresolvable sender is recorded as none",
			line:            "@time=2026-09-11T10:22:10.885Z;msgid=abcd :nobody!~u@h.irc PRIVMSG #tags :still worth keeping",
			wantAgentID:     "",
			wantAttribution: AttributionNone,
			wantNick:        "nobody",
			wantContent:     "still worth keeping",
		},
		{
			// A broken resolver must not stop capture; the message lands
			// unattributed rather than being dropped.
			name:            "resolver error degrades to none",
			line:            "@time=2026-09-11T10:22:10.885Z;account=alice;msgid=abcd :alice!~u@h.irc PRIVMSG #tags :resolver is down",
			reverseErr:      errors.New("resolver down"),
			wantAgentID:     "",
			wantAttribution: AttributionAccount,
			wantNick:        "alice",
			wantContent:     "resolver is down",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			c := newCapture(rec, tc.directory)
			if tc.reverseErr != nil {
				c.Reverse = mapReverse{err: tc.reverseErr}
			}

			recorded, err := c.Handle(context.Background(), mustParse(tc.line))
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if !recorded {
				t.Fatalf("message was skipped, want it recorded")
			}
			if len(rec.events) != 1 {
				t.Fatalf("recorded %d events, want 1", len(rec.events))
			}

			ev := rec.events[0]
			if ev.AgentID != tc.wantAgentID {
				t.Errorf("AgentID = %q, want %q", ev.AgentID, tc.wantAgentID)
			}
			if ev.Type != model.EventInteractionMessage {
				t.Errorf("Type = %q, want %q", ev.Type, model.EventInteractionMessage)
			}
			if ev.WorkspaceID != "ws-1" || ev.WriterID != "writer-1" {
				t.Errorf("WorkspaceID/WriterID = %q/%q, want ws-1/writer-1", ev.WorkspaceID, ev.WriterID)
			}

			data := decode(t, ev)
			if data.Attribution != tc.wantAttribution {
				t.Errorf("Attribution = %q, want %q", data.Attribution, tc.wantAttribution)
			}
			if data.Nick != tc.wantNick {
				t.Errorf("Nick = %q, want %q", data.Nick, tc.wantNick)
			}
			if data.Content != tc.wantContent {
				t.Errorf("Content = %q, want %q", data.Content, tc.wantContent)
			}
			if data.Role != messageRole || data.Format != messageFormat {
				t.Errorf("Role/Format = %q/%q, want %q/%q", data.Role, data.Format, messageRole, messageFormat)
			}
		})
	}
}

// TestCaptureSkips covers every message that must never reach the event log.
func TestCaptureSkips(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{
			// The single most damaging case: HistServ synthesises join
			// notices into a CHATHISTORY batch as ordinary PRIVMSGs, and
			// recording one would put "alice joined the channel" in the log
			// as something alice said.
			name: "HistServ synthetic join",
			line: ":HistServ!HistServ@localhost PRIVMSG #hist :alice [account: alice] joined the channel",
		},
		{
			name: "the bot's own echoed message, by account",
			line: "@time=2026-09-11T10:22:10.885Z;account=wsm-bot;msgid=abcd :wsm-bot!~u@h.irc PRIVMSG #tags :i am the bot",
		},
		{
			// Without account-tag there is no account to compare, so the
			// nick is the only thing standing between the bot and an echo
			// loop.
			name: "the bot's own message, by nick alone",
			line: ":wsm-bot!~u@h.irc PRIVMSG #tags :no account tag on this one",
		},
		{
			name: "private message to the bot",
			line: "@time=2026-09-11T10:22:10.885Z;account=alice;msgid=abcd :alice!~u@h.irc PRIVMSG wsm-bot :a DM",
		},
		{
			name: "a command that is not a message",
			line: ":alice!~u@h.irc JOIN #tags",
		},
		{
			name: "a malformed message with no body",
			line: ":alice!~u@h.irc PRIVMSG #tags",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			c := newCapture(rec, relayDirectory{})

			recorded, err := c.Handle(context.Background(), mustParse(tc.line))
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if recorded {
				t.Errorf("message was recorded, want it skipped")
			}
			if rec.calls != 0 {
				t.Errorf("recorder called %d times, want 0", rec.calls)
			}
		})
	}
}

// TestCaptureNoticeIsCaptured pins that NOTICE is captured too: bots and
// services speak in notices, and dropping them would lose half of some
// channels.
func TestCaptureNoticeIsCaptured(t *testing.T) {
	rec := &fakeRecorder{}
	c := newCapture(rec, nil)

	recorded, err := c.Handle(context.Background(),
		mustParse("@time=2026-09-11T10:22:10.885Z;account=alice;msgid=abcd :alice!~u@h.irc NOTICE #tags :a notice"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !recorded {
		t.Fatalf("NOTICE was skipped, want it recorded")
	}
	if got := decode(t, rec.events[0]).Content; got != "a notice" {
		t.Errorf("Content = %q, want %q", got, "a notice")
	}
}

// TestCaptureNilDirectoryIsSafe pins the nil-safety the field documents: a
// provider without relay support leaves Directory nil.
func TestCaptureNilDirectoryIsSafe(t *testing.T) {
	rec := &fakeRecorder{}
	c := newCapture(rec, nil)

	recorded, err := c.Handle(context.Background(),
		mustParse("@draft/relaymsg=botty;msgid=sn3f;time=2026-09-11T10:23:20.402Z :slackuser/slack!~u@h.irc PRIVMSG #relay :relayed hello"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !recorded {
		t.Fatalf("message was skipped, want it recorded")
	}
	// With no Directory the relay tag means nothing; it is an ordinary
	// message from a nick nobody knows.
	if got := decode(t, rec.events[0]).Attribution; got != AttributionNone {
		t.Errorf("Attribution = %q, want %q", got, AttributionNone)
	}
}

// TestCaptureServerTime pins both timestamp paths: the server's tag when
// present, receipt time when not.
func TestCaptureServerTime(t *testing.T) {
	t.Run("server-time tag is honoured", func(t *testing.T) {
		rec := &fakeRecorder{}
		c := newCapture(rec, nil)

		if _, err := c.Handle(context.Background(),
			mustParse("@time=2026-09-11T10:22:10.885Z;account=alice;msgid=abcd :alice!~u@h.irc PRIVMSG #tags :hi")); err != nil {
			t.Fatalf("Handle: %v", err)
		}

		want := time.Date(2026, 9, 11, 10, 22, 10, 885000000, time.UTC)
		if got := rec.events[0].Timestamp; !got.Equal(want) {
			t.Errorf("Timestamp = %s, want %s", got, want)
		}
	})

	t.Run("without server-time the receipt time is used", func(t *testing.T) {
		rec := &fakeRecorder{}
		c := newCapture(rec, nil)

		before := time.Now().UTC().Add(-time.Second)
		if _, err := c.Handle(context.Background(),
			mustParse(":alice!~u@h.irc PRIVMSG #tags :hi")); err != nil {
			t.Fatalf("Handle: %v", err)
		}
		after := time.Now().UTC().Add(time.Second)

		got := rec.events[0].Timestamp
		if got.Before(before) || got.After(after) {
			t.Errorf("Timestamp = %s, want a receipt time within [%s, %s]", got, before, after)
		}
	})
}

// TestCaptureMsgIDDegradation pins the two MsgID paths. The hashed form is
// what lets a backfill dedupe on a server that sends no msgid at all.
func TestCaptureMsgIDDegradation(t *testing.T) {
	t.Run("server msgid is used verbatim", func(t *testing.T) {
		rec := &fakeRecorder{}
		c := newCapture(rec, nil)

		if _, err := c.Handle(context.Background(),
			mustParse("@time=2026-09-11T10:22:10.885Z;account=alice;msgid=pxsdgspzmwt5w9wgb3netamua2 :alice!~u@h.irc PRIVMSG #tags :hi")); err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if got := decode(t, rec.events[0]).MsgID; got != "pxsdgspzmwt5w9wgb3netamua2" {
			t.Errorf("MsgID = %q, want the server's own", got)
		}
	})

	t.Run("without msgid a stable content hash is synthesised", func(t *testing.T) {
		const line = "@time=2026-09-11T10:22:10.885Z;account=alice :alice!~u@h.irc PRIVMSG #tags :hi"

		rec := &fakeRecorder{}
		c := newCapture(rec, nil)
		if _, err := c.Handle(context.Background(), mustParse(line)); err != nil {
			t.Fatalf("Handle: %v", err)
		}

		got := decode(t, rec.events[0]).MsgID
		if !strings.HasPrefix(got, hashedIDPrefix) {
			t.Errorf("MsgID = %q, want the %q prefix", got, hashedIDPrefix)
		}
		if len(got) != len(hashedIDPrefix)+hashedIDLength {
			t.Errorf("MsgID = %q, want %d characters", got, len(hashedIDPrefix)+hashedIDLength)
		}

		// The same message seen again (live, then via backfill) must hash
		// identically, or dedupe cannot work.
		second := &fakeRecorder{}
		again := newCapture(second, nil)
		if _, err := again.Handle(context.Background(), mustParse(line)); err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if other := decode(t, second.events[0]).MsgID; other != got {
			t.Errorf("MsgID = %q on a second pass, want the stable %q", other, got)
		}

		// A different body must not collide.
		third := &fakeRecorder{}
		differing := newCapture(third, nil)
		if _, err := differing.Handle(context.Background(),
			mustParse("@time=2026-09-11T10:22:10.885Z;account=alice :alice!~u@h.irc PRIVMSG #tags :different")); err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if other := decode(t, third.events[0]).MsgID; other == got {
			t.Errorf("a different message hashed to the same MsgID %q", got)
		}
	})
}

// TestCaptureTagOrderIsIrrelevant pins the probe's warning that ergo does not
// emit tags in a stable order: the same message with tags permuted must
// capture identically.
func TestCaptureTagOrderIsIrrelevant(t *testing.T) {
	lines := []string{
		"@time=2026-09-11T10:22:16.570Z;batch=1;account=alice;msgid=tzh4 :alice!~u@h.irc PRIVMSG #hist :msg",
		"@batch=1;account=alice;msgid=tzh4;time=2026-09-11T10:22:16.570Z :alice!~u@h.irc PRIVMSG #hist :msg",
		"@msgid=tzh4;time=2026-09-11T10:22:16.570Z;batch=1;account=alice :alice!~u@h.irc PRIVMSG #hist :msg",
	}

	var first model.MessageData
	for i, line := range lines {
		rec := &fakeRecorder{}
		c := newCapture(rec, nil)
		if _, err := c.Handle(context.Background(), mustParse(line)); err != nil {
			t.Fatalf("Handle: %v", err)
		}

		data := decode(t, rec.events[0])
		if i == 0 {
			first = data
			continue
		}
		if data != first {
			t.Errorf("permutation %d captured as %+v, want %+v", i, data, first)
		}
	}
}

// TestCaptureFromHistoryFixture runs the real recorded CHATHISTORY batch
// through capture: seven user messages plus one HistServ join line, of which
// exactly the seven must land.
func TestCaptureFromHistoryFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "plans",
		"2026-09-10-irc-workspace-chat-design", "probes", "chathistory-paging.txt")
	msgs, err := bottest.Lines(path)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	rec := &fakeRecorder{}
	c := newCapture(rec, nil)
	for _, msg := range msgs {
		if _, err := c.Handle(context.Background(), msg); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}

	if len(rec.events) != 7 {
		t.Fatalf("recorded %d events, want the 7 user messages", len(rec.events))
	}
	for i, ev := range rec.events {
		data := decode(t, ev)
		if strings.Contains(data.Content, "joined the channel") {
			t.Errorf("event %d recorded a HistServ join line: %q", i, data.Content)
		}
		if data.Channel != "#hist" {
			t.Errorf("event %d Channel = %q, want #hist", i, data.Channel)
		}
	}
}

// TestCaptureRunSubscribesToChannelTraffic drives capture through the fake
// transport the way the daemon will, rather than calling Handle directly.
func TestCaptureRunSubscribesToChannelTraffic(t *testing.T) {
	fake := bottest.New("message-tags", "account-tag", "server-time", "echo-message")
	rec := &fakeRecorder{}
	c := newCapture(rec, nil)
	c.Transport = fake

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	// Wait for Run to register its subscriptions before emitting, so the
	// test cannot race past them.
	waitFor(t, 2*time.Second, "capture to subscribe", func() bool {
		fake.Emit(mustParse("@time=2026-09-11T10:22:10.885Z;account=alice;msgid=m1 :alice!~u@h.irc PRIVMSG #tags :first"))
		return len(rec.events) > 0
	})

	fake.Emit(mustParse(":HistServ!HistServ@localhost PRIVMSG #tags :alice [account: alice] joined the channel"))
	fake.Emit(mustParse("@time=2026-09-11T10:22:12.885Z;account=alice;msgid=m2 :alice!~u@h.irc NOTICE #tags :second"))

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}

	// The first emit may have run more than once while waiting; what matters
	// is that the join line never landed and the notice did.
	var contents []string
	for _, ev := range rec.events {
		contents = append(contents, decode(t, ev).Content)
	}
	for _, got := range contents {
		if strings.Contains(got, "joined the channel") {
			t.Fatalf("recorded a HistServ join line: %q", got)
		}
	}
	if !contains(contents, "second") {
		t.Errorf("contents = %v, want the NOTICE %q among them", contents, "second")
	}
}

// TestCaptureRunWithoutTransport pins the guard rather than a nil dereference.
func TestCaptureRunWithoutTransport(t *testing.T) {
	c := &Capture{}
	if err := c.Run(context.Background()); err == nil {
		t.Fatal("Run with no transport returned nil, want an error")
	}
}

// TestCaptureWithoutRecorder pins that a misconfigured Capture reports itself
// instead of silently dropping traffic.
func TestCaptureWithoutRecorder(t *testing.T) {
	c := newCapture(nil, nil)
	c.Recorder = nil

	if _, err := c.Handle(context.Background(),
		mustParse("@account=alice;msgid=abcd :alice!~u@h.irc PRIVMSG #tags :hi")); err == nil {
		t.Fatal("Handle with no recorder returned nil, want an error")
	}
}

// contains reports whether want is in values.
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
