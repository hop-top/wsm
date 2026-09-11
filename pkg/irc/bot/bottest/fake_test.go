package bottest

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ergochat/irc-go/ircmsg"
)

// probes is the fixture directory, relative to this package.
const probes = "../../../../docs/plans/2026-09-10-irc-workspace-chat-design/probes"

func TestSendLabeledMatchedPrefixReturnsScript(t *testing.T) {
	f := New()
	f.Script = map[string][]ircmsg.Message{
		"NAMES": {Msg("353", "botty", "=", "#multi", "~@botty")},
	}

	replies, err := f.SendLabeled(context.Background(), "NAMES #multi")
	if err != nil {
		t.Fatalf("SendLabeled: %v", err)
	}
	if len(replies) != 1 || replies[0].Command != "353" {
		t.Fatalf("replies = %+v, want one 353", replies)
	}
	if got := f.SentLines(); len(got) != 1 || got[0] != "NAMES #multi" {
		t.Fatalf("Sent = %q, want [NAMES #multi]", got)
	}
}

func TestSendLabeledLongestPrefixWins(t *testing.T) {
	f := New()
	f.Script = map[string][]ircmsg.Message{
		"NAMES":       {Msg("353", "general")},
		"NAMES #spec": {Msg("353", "specific")},
	}

	// Run repeatedly: a map-order-dependent implementation passes this only
	// by luck, and Go randomizes that order per iteration.
	for i := 0; i < 50; i++ {
		replies, err := f.SendLabeled(context.Background(), "NAMES #spec")
		if err != nil {
			t.Fatalf("SendLabeled: %v", err)
		}
		if replies[0].Params[0] != "specific" {
			t.Fatalf("matched %q, want the longer key", replies[0].Params[0])
		}
	}
}

func TestSendLabeledUnmatchedPrefixErrors(t *testing.T) {
	f := New()
	f.Script = map[string][]ircmsg.Message{"NAMES": {Msg("353")}}

	replies, err := f.SendLabeled(context.Background(), "TOPIC #x :hello")
	if !errors.Is(err, ErrNoScript) {
		t.Fatalf("err = %v, want ErrNoScript", err)
	}
	if replies != nil {
		t.Fatalf("replies = %+v, want nil on error", replies)
	}
	// The wrong command still shows up in the log, so a test can report
	// what was issued instead of only that nothing matched.
	if got := f.SentLines(); len(got) != 1 || got[0] != "TOPIC #x :hello" {
		t.Fatalf("Sent = %q, want the unmatched line", got)
	}
}

func TestSendLabeledReturnsCopy(t *testing.T) {
	f := New()
	f.Script = map[string][]ircmsg.Message{"NAMES": {Msg("353", "original")}}

	replies, err := f.SendLabeled(context.Background(), "NAMES #x")
	if err != nil {
		t.Fatalf("SendLabeled: %v", err)
	}
	replies[0] = Msg("366", "mutated")

	again, err := f.SendLabeled(context.Background(), "NAMES #x")
	if err != nil {
		t.Fatalf("SendLabeled: %v", err)
	}
	if again[0].Command != "353" {
		t.Fatalf("script mutated through returned slice: %+v", again[0])
	}
}

func TestEmitDeliversToMatchingSubscribers(t *testing.T) {
	f := New()

	var privmsgs, notices int
	cancel := f.Subscribe("PRIVMSG", func(ircmsg.Message) { privmsgs++ })
	defer cancel()
	defer f.Subscribe("NOTICE", func(ircmsg.Message) { notices++ })()

	f.Emit(Msg("PRIVMSG", "#a", "hello"))
	f.Emit(Msg("PRIVMSG", "#a", "again"))

	if privmsgs != 2 {
		t.Fatalf("privmsgs = %d, want 2", privmsgs)
	}
	if notices != 0 {
		t.Fatalf("notices = %d, want 0: command must be matched exactly", notices)
	}
}

func TestCancelUnsubscribes(t *testing.T) {
	f := New()

	var got int
	cancel := f.Subscribe("PRIVMSG", func(ircmsg.Message) { got++ })

	f.Emit(Msg("PRIVMSG", "#a", "before"))
	cancel()
	f.Emit(Msg("PRIVMSG", "#a", "after"))

	if got != 1 {
		t.Fatalf("callback fired %d times, want 1: cancel must unsubscribe", got)
	}

	// Cancelling twice must not panic or disturb other subscriptions.
	cancel()

	var other int
	defer f.Subscribe("PRIVMSG", func(ircmsg.Message) { other++ })()
	f.Emit(Msg("PRIVMSG", "#a", "third"))
	if got != 1 || other != 1 {
		t.Fatalf("got = %d, other = %d, want 1 and 1", got, other)
	}
}

func TestEmitBatchDeliversByType(t *testing.T) {
	f := New()

	var batches [][]ircmsg.Message
	defer f.SubscribeBatch("chathistory", func(msgs []ircmsg.Message) {
		batches = append(batches, msgs)
	})()

	var otherType int
	defer f.SubscribeBatch("netjoin", func([]ircmsg.Message) { otherType++ })()

	f.EmitBatch("chathistory", []ircmsg.Message{
		Msg("PRIVMSG", "#a", "one"),
		Msg("PRIVMSG", "#a", "two"),
	})

	if len(batches) != 1 || len(batches[0]) != 2 {
		t.Fatalf("batches = %+v, want one batch of two", batches)
	}
	if otherType != 0 {
		t.Fatalf("otherType = %d, want 0", otherType)
	}
}

func TestSubscribeBatchIgnoresEmit(t *testing.T) {
	f := New()

	var batch, plain int
	defer f.SubscribeBatch("chathistory", func([]ircmsg.Message) { batch++ })()
	defer f.Subscribe("PRIVMSG", func(ircmsg.Message) { plain++ })()

	f.Emit(Msg("PRIVMSG", "#a", "hi"))
	f.EmitBatch("chathistory", []ircmsg.Message{Msg("PRIVMSG", "#a", "hi")})

	if plain != 1 || batch != 1 {
		t.Fatalf("plain = %d, batch = %d, want 1 and 1: the two paths are separate", plain, batch)
	}
}

func TestReconnectFiresCallbacksInOrder(t *testing.T) {
	f := New()

	var order []string
	f.OnReconnect(func() { order = append(order, "first") })
	f.OnReconnect(func() { order = append(order, "second") })

	f.Reconnect()
	f.Reconnect()

	want := []string{"first", "second", "first", "second"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestNamesReturnsMultiCharPrefixes(t *testing.T) {
	f := New("multi-prefix")
	// Straight from the live ergo 2.19.1 probe: the founder carries two
	// prefix chars, so the table's values are runs, not single characters.
	f.SetNames(map[string]string{
		"botty": "~@",
		"carol": "@",
		"bob":   "%",
		"alice": "+",
		"dave":  "",
	})

	names, err := f.Names(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if names["botty"] != "~@" {
		t.Fatalf("botty = %q, want %q", names["botty"], "~@")
	}
	if names["dave"] != "" {
		t.Fatalf("dave = %q, want empty", names["dave"])
	}

	// The returned map is a copy: mutating it must not poison the table.
	names["botty"] = "mutated"
	again, err := f.Names(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if again["botty"] != "~@" {
		t.Fatalf("table mutated through returned map: %q", again["botty"])
	}

	if got := f.SentLines(); len(got) != 2 || got[0] != "NAMES #multi" {
		t.Fatalf("Sent = %q, want two NAMES lines", got)
	}
}

func TestNegotiatedAndOper(t *testing.T) {
	f := New("multi-prefix", "labeled-response")

	caps := f.Negotiated()
	if !caps.Has("multi-prefix") || caps.Has("sasl") {
		t.Fatalf("caps = %v, want exactly the two set", caps.Names())
	}
	// Names is sorted, so a test may diff it directly.
	got := caps.Names()
	if len(got) != 2 || got[0] != "labeled-response" || got[1] != "multi-prefix" {
		t.Fatalf("Names() = %v, want sorted pair", got)
	}

	if f.IsOper() {
		t.Fatal("IsOper = true on a fresh transport, want false")
	}
	f.SetOper(true)
	if !f.IsOper() {
		t.Fatal("IsOper = false after SetOper(true)")
	}
}

func TestCommandHelpersRecordRawLines(t *testing.T) {
	f := New()
	ctx := context.Background()

	if err := f.Join(ctx, "#a"); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if err := f.Mode(ctx, "#a", "+o", "botty"); err != nil {
		t.Fatalf("Mode: %v", err)
	}
	if err := f.Topic(ctx, "#a", "hello world"); err != nil {
		t.Fatalf("Topic: %v", err)
	}
	if err := f.Part(ctx, "#a"); err != nil {
		t.Fatalf("Part: %v", err)
	}

	want := []string{"JOIN #a", "MODE #a +o botty", "TOPIC #a :hello world", "PART #a"}
	got := f.SentLines()
	if len(got) != len(want) {
		t.Fatalf("Sent = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Sent = %q, want %q", got, want)
		}
	}

	// Recorded lines are real wire lines, so a test may re-parse them.
	msg, err := ircmsg.ParseLine(got[2])
	if err != nil {
		t.Fatalf("recorded TOPIC does not parse: %v", err)
	}
	if msg.Params[1] != "hello world" {
		t.Fatalf("topic text = %q, want %q", msg.Params[1], "hello world")
	}
}

func TestSendErrPropagates(t *testing.T) {
	f := New()
	f.Script = map[string][]ircmsg.Message{"NAMES": {Msg("353")}}
	boom := errors.New("boom")
	f.SendErr = boom

	if err := f.Send(context.Background(), "JOIN #a"); !errors.Is(err, boom) {
		t.Fatalf("Send err = %v, want boom", err)
	}
	// SendErr beats the script, so a test can fail a command that would
	// otherwise have matched.
	if _, err := f.SendLabeled(context.Background(), "NAMES #a"); !errors.Is(err, boom) {
		t.Fatalf("SendLabeled err = %v, want boom", err)
	}
	if got := f.SentLines(); len(got) != 2 {
		t.Fatalf("Sent = %q, want both failed lines recorded", got)
	}
}

func TestCloseRejectsFurtherUse(t *testing.T) {
	f := New()
	f.Close()
	f.Close() // idempotent

	if err := f.Send(context.Background(), "JOIN #a"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Send after Close = %v, want ErrClosed", err)
	}
	if _, err := f.Names(context.Background(), "#a"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Names after Close = %v, want ErrClosed", err)
	}
	if err := f.Connect(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Connect after Close = %v, want ErrClosed", err)
	}
}

func TestCancelledContextIsHonoured(t *testing.T) {
	f := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := f.Send(ctx, "JOIN #a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send = %v, want context.Canceled", err)
	}
	if _, err := f.SendLabeled(ctx, "NAMES #a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("SendLabeled = %v, want context.Canceled", err)
	}
	if _, err := f.Names(ctx, "#a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Names = %v, want context.Canceled", err)
	}
	if got := f.SentLines(); len(got) != 0 {
		t.Fatalf("Sent = %q, want nothing recorded for a dead context", got)
	}
}

func TestLinesLoadsNamesFixture(t *testing.T) {
	msgs, err := Lines(filepath.Join(probes, "names-multi-prefix.txt"))
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(msgs) != 10 {
		t.Fatalf("parsed %d messages, want 10 (comments and blanks skipped)", len(msgs))
	}

	// The second 353 is the multi-prefix listing; the founder carries two
	// prefix chars there, which is the whole point of the fixture.
	var found bool
	for _, msg := range msgs {
		if msg.Command != "353" {
			continue
		}
		trailing := msg.Params[len(msg.Params)-1]
		if trailing == "@carol %bob +alice ~@botty" {
			found = true
		}
	}
	if !found {
		t.Fatal("multi-prefix 353 with the ~@botty run not found in fixture")
	}

	// Tags survive parsing and are addressed by name, never by position.
	if ok, _ := msgs[0].GetTag("time"); !ok {
		t.Fatalf("first message lost its time tag: %+v", msgs[0])
	}
}

func TestLinesParsesTagsByNameNotPosition(t *testing.T) {
	msgs, err := Lines(filepath.Join(probes, "privmsg-tags.txt"))
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}

	// The fixture deliberately carries the same message twice with the tags
	// in a different order; both must yield identical tag values by name.
	var relay []ircmsg.Message
	for _, msg := range msgs {
		if ok, _ := msg.GetTag("draft/relaymsg"); ok {
			relay = append(relay, msg)
		}
	}
	if len(relay) != 2 {
		t.Fatalf("found %d relaymsg lines, want 2", len(relay))
	}
	for _, msg := range relay {
		if _, got := msg.GetTag("draft/relaymsg"); got != "botty" {
			t.Fatalf("relaymsg tag = %q, want botty", got)
		}
	}
	if _, a := relay[0].GetTag("msgid"); a != mustTag(t, relay[1], "msgid") {
		t.Fatal("same message parsed to different msgids across tag orders")
	}
}

func mustTag(t *testing.T, msg ircmsg.Message, name string) string {
	t.Helper()
	ok, value := msg.GetTag(name)
	if !ok {
		t.Fatalf("message lacks tag %q: %+v", name, msg)
	}
	return value
}

func TestLinesRejectsNonIRCFixture(t *testing.T) {
	// api-http.txt holds JSON bodies, not protocol lines. Returning a useful
	// error beats handing back garbage that looks like messages.
	msgs, err := Lines(filepath.Join(probes, "api-http.txt"))
	if err == nil {
		t.Fatalf("Lines on a non-IRC fixture returned %d messages, want an error", len(msgs))
	}
	if msgs != nil {
		t.Fatalf("msgs = %+v, want nil alongside the error", msgs)
	}
}

func TestLinesMissingFile(t *testing.T) {
	if _, err := Lines(filepath.Join(t.TempDir(), "absent.txt")); err == nil {
		t.Fatal("Lines on a missing file returned no error")
	}
}

func TestConcurrentEmitAndSubscribe(t *testing.T) {
	f := New()

	var (
		mu    sync.Mutex
		count int
	)
	defer f.Subscribe("PRIVMSG", func(ircmsg.Message) {
		mu.Lock()
		count++
		mu.Unlock()
	})()

	var wg sync.WaitGroup
	// Emitters, subscribers churning, and senders all at once: under -race
	// this is what catches an unguarded field.
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				f.Emit(Msg("PRIVMSG", "#a", "hi"))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				f.Subscribe("NOTICE", func(ircmsg.Message) {})()
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = f.Send(context.Background(), "PING :x")
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	got := count
	mu.Unlock()
	if got != 8*50 {
		t.Fatalf("callback fired %d times, want %d", got, 8*50)
	}
	if len(f.SentLines()) != 8*50 {
		t.Fatalf("Sent has %d lines, want %d", len(f.SentLines()), 8*50)
	}
}

func TestMustParseAndMsg(t *testing.T) {
	msg := MustParse("@time=2026-09-11T10:24:40.248Z :ergo.test 353 botty = #multi :~@botty")
	if msg.Command != "353" {
		t.Fatalf("command = %q, want 353", msg.Command)
	}
	if _, got := msg.GetTag("time"); got == "" {
		t.Fatal("time tag lost")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MustParse on a malformed line did not panic")
		}
	}()
	MustParse("")
}
