package bot

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
)

// newTestTransport wires a Transport to a fake server and guarantees teardown.
func newTestTransport(t *testing.T, s *fakeServer, mutate func(*Options)) *Transport {
	t.Helper()

	opts := Options{
		Addr:     "ergo.test:6667",
		Account:  "botty",
		Password: "hunter2",
		Nick:     "botty",
		Realname: "wsm bot",
		Backoff: irc.Backoff{
			Min:      time.Second,
			Max:      30 * time.Second,
			Attempts: 5,
			Clock:    &testClock{},
		},
	}
	if mutate != nil {
		mutate(&opts)
	}

	tr := New(opts)
	tr.dialContext = s.dial
	t.Cleanup(tr.Close)
	return tr
}

func TestConnectNegotiatesCapsAndAuthenticates(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)

	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Every default cap is requested, one CAP REQ per cap, as ergo's
	// per-cap ACK fixture shows.
	for _, want := range DefaultCaps {
		if !s.sent("CAP REQ " + want) {
			t.Errorf("cap %q was never requested; sent %v", want, s.lines())
		}
	}

	caps := tr.Negotiated()
	for _, want := range DefaultCaps {
		if !caps.Has(want) {
			t.Errorf("cap %q should have been negotiated, got %v", want, caps.Names())
		}
	}

	// SASL PLAIN ran: the mechanism line and then the credentials.
	if !s.sent("AUTHENTICATE PLAIN") {
		t.Errorf("SASL never started; sent %v", s.lines())
	}

	// An account with no oper block stays an ordinary user.
	if tr.IsOper() {
		t.Error("IsOper should be false without an oper block")
	}
	if s.sent("OPER") {
		t.Error("OPER sent without an oper block configured")
	}
}

func TestConnectDegradesOnNakedCaps(t *testing.T) {
	s := newFakeServer(t)
	// A server that grants only the bare minimum: the transport must
	// record what it actually got rather than what it asked for.
	s.ackCaps = map[string]bool{"sasl": true, "message-tags": true}

	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	caps := tr.Negotiated()
	if !caps.Has("sasl") || !caps.Has("message-tags") {
		t.Errorf("granted caps missing from set: %v", caps.Names())
	}
	if caps.Has(CapLabeledResponse) || caps.Has(CapMultiPrefix) {
		t.Errorf("NAKed caps reported as granted: %v", caps.Names())
	}
}

func TestConnectTakesOper(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, func(o *Options) {
		o.OperName = "botty"
		o.OperPass = "operpass"
	})

	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if !s.sent("OPER botty operpass") {
		t.Errorf("OPER never sent; sent %v", s.lines())
	}
	// IsOper is true only because RPL_YOUREOPER arrived, never because
	// OPER was sent.
	if !tr.IsOper() {
		t.Error("IsOper should be true after RPL_YOUREOPER")
	}
}

func TestConnectFailsWhenOperRefused(t *testing.T) {
	s := newFakeServer(t)
	s.operFails = true

	tr := newTestTransport(t, s, func(o *Options) {
		o.OperName = "botty"
		o.OperPass = "wrong"
	})

	// A configured oper that never confirms must fail loudly: silently
	// continuing would leave every privileged provider path broken.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := tr.Connect(ctx)
	if err == nil {
		t.Fatal("Connect should fail when OPER is refused")
	}
	if tr.IsOper() {
		t.Error("IsOper should stay false when OPER was refused")
	}
}

func TestSendLabeledCollectsTheBatch(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	replies, err := tr.SendLabeled(context.Background(), "NAMES #multi")
	if err != nil {
		t.Fatalf("SendLabeled: %v", err)
	}

	// The enclosing BATCH markers are not payload; the 353/366 pair is.
	var commands []string
	for _, msg := range replies {
		commands = append(commands, msg.Command)
	}
	want := []string{rplNamReply, rplEndOfNames}
	if !reflect.DeepEqual(commands, want) {
		t.Errorf("labeled replies = %v, want %v", commands, want)
	}
}

func TestNamesParsesMultiPrefix(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	names, err := tr.Names(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("Names: %v", err)
	}

	// The founder holds two prefixes; a one-character parser would report
	// "~" and lose the op.
	want := map[string]string{
		"carol": "@",
		"bob":   "%",
		"alice": "+",
		"botty": "~@",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Names = %v, want %v", names, want)
	}
}

func TestNamesWithoutLabeledResponse(t *testing.T) {
	s := newFakeServer(t)
	// No labeled-response and no batch: Names must fall back to watching
	// the 353/366 pair.
	s.ackCaps = map[string]bool{"sasl": true, "multi-prefix": true, "message-tags": true}

	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	names, err := tr.Names(context.Background(), "#multi")
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if got := names["botty"]; got != "~@" {
		t.Errorf("founder prefixes = %q, want ~@", got)
	}
	if len(names) != 4 {
		t.Errorf("Names = %v, want four occupants", names)
	}
}

func TestSubscribeCancelUnsubscribes(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var (
		mu   sync.Mutex
		hits int
	)
	cancel := tr.Subscribe(rplEndOfNames, func(ircmsg.Message) {
		mu.Lock()
		hits++
		mu.Unlock()
	})

	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return hits
	}

	// An unlabeled NAMES produces a 366 through the ordinary callbacks.
	if err := tr.Send(context.Background(), "NAMES #multi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, 2*time.Second, "first 366", func() bool { return count() == 1 })

	cancel()

	if err := tr.Send(context.Background(), "NAMES #multi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Give the second reply time to arrive and be ignored.
	waitFor(t, 2*time.Second, "second NAMES round trip", func() bool {
		return len(s.lines()) > 0 && s.sent("NAMES #multi")
	})
	time.Sleep(100 * time.Millisecond)

	if got := count(); got != 1 {
		t.Errorf("callback fired %d times after cancel, want 1", got)
	}

	// Cancelling twice must not panic or remove someone else's callback.
	cancel()
}

func TestSubscribeBatchReceivesFlattenedBatch(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var (
		mu      sync.Mutex
		batches [][]ircmsg.Message
	)
	cancel := tr.SubscribeBatch("chathistory", func(msgs []ircmsg.Message) {
		mu.Lock()
		batches = append(batches, msgs)
		mu.Unlock()
	})
	defer cancel()

	// Script a chathistory batch, shaped as the paging fixture shows.
	s.mu.Lock()
	s.onLine = func(_ *fakeServer, write func(string), msg ircmsg.Message) {
		if !nameIs(msg, "CHATHISTORY") {
			return
		}
		write(":ergo.test BATCH +hst chathistory #hist")
		write("@batch=hst;account=alice;msgid=one :alice!~u@host PRIVMSG #hist :msg-1")
		write("@batch=hst;account=alice;msgid=two :alice!~u@host PRIVMSG #hist :msg-2")
		write(":ergo.test BATCH -hst")
	}
	s.mu.Unlock()

	if err := tr.Send(context.Background(), "CHATHISTORY AFTER #hist timestamp=2026-09-11T10:00:00.000Z 3"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	waitFor(t, 2*time.Second, "chathistory batch", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(batches) == 1
	})

	mu.Lock()
	got := batches[0]
	mu.Unlock()

	if len(got) != 2 {
		t.Fatalf("batch carried %d lines, want 2", len(got))
	}
	for _, msg := range got {
		if msg.Command != "PRIVMSG" {
			t.Errorf("unexpected line in batch: %s", msg.Command)
		}
	}
}

func TestReconnectReauthenticatesAndFiresHooks(t *testing.T) {
	clock := &testClock{}
	s := newFakeServer(t)
	tr := newTestTransport(t, s, func(o *Options) {
		o.OperName = "botty"
		o.OperPass = "operpass"
		o.Backoff = irc.Backoff{
			Min:      time.Second,
			Max:      30 * time.Second,
			Attempts: 5,
			Clock:    clock,
		}
	})

	var reconnects int
	var mu sync.Mutex
	tr.OnReconnect(func() {
		mu.Lock()
		reconnects++
		mu.Unlock()
	})

	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if s.connections() != 1 {
		t.Fatalf("connections = %d, want 1", s.connections())
	}

	// Drop the session: the client sees EOF and reconnects on the curve.
	s.drop()

	waitFor(t, 5*time.Second, "reconnect", func() bool { return s.connections() >= 2 })
	waitFor(t, 5*time.Second, "reconnect hook", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reconnects >= 1
	})

	// The hook runs only after re-auth and re-OPER, so both must already
	// be true by the time it fired.
	if !tr.IsOper() {
		t.Error("IsOper should be restored after reconnect")
	}
	caps := tr.Negotiated()
	if !caps.Has(CapMultiPrefix) {
		t.Errorf("caps should be renegotiated after reconnect, got %v", caps.Names())
	}

	// The wait came from the injected clock, so no wall-clock sleeping
	// happened and the curve starts at Min.
	delays := clock.delays()
	if len(delays) == 0 {
		t.Fatal("backoff clock was never consulted")
	}
	if delays[0] != time.Second {
		t.Errorf("first backoff delay = %v, want 1s", delays[0])
	}
}

func TestOperationsBeforeConnect(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)

	ctx := context.Background()
	if err := tr.Send(ctx, "PING x"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Send before Connect = %v, want ErrNotConnected", err)
	}
	if _, err := tr.SendLabeled(ctx, "NAMES #multi"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SendLabeled before Connect = %v, want ErrNotConnected", err)
	}
	if err := tr.Join(ctx, "#multi"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Join before Connect = %v, want ErrNotConnected", err)
	}
	if _, err := tr.Names(ctx, "#multi"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Names before Connect = %v, want ErrNotConnected", err)
	}
	if tr.IsOper() {
		t.Error("IsOper should be false before Connect")
	}
	if got := tr.Negotiated().Names(); len(got) != 0 {
		t.Errorf("Negotiated before Connect = %v, want empty", got)
	}
}

func TestChannelCommands(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	ctx := context.Background()
	if err := tr.Join(ctx, "#multi"); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if err := tr.Mode(ctx, "#multi", "+o", "alice"); err != nil {
		t.Fatalf("Mode: %v", err)
	}
	if err := tr.Topic(ctx, "#multi", "a topic with spaces"); err != nil {
		t.Fatalf("Topic: %v", err)
	}
	if err := tr.Part(ctx, "#multi"); err != nil {
		t.Fatalf("Part: %v", err)
	}

	waitFor(t, 2*time.Second, "channel commands", func() bool {
		return s.sent("PART #multi")
	})

	for _, want := range []string{
		"JOIN #multi",
		"MODE #multi +o alice",
		// A topic with spaces must be sent as a trailing parameter.
		"TOPIC #multi :a topic with spaces",
		"PART #multi",
	} {
		if !s.sent(want) {
			t.Errorf("never sent %q; sent %v", want, s.lines())
		}
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	tr.Close()
	// A second Close must not panic, block, or reconnect.
	tr.Close()

	if err := tr.Send(context.Background(), "PING x"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Send after Close = %v, want ErrNotConnected", err)
	}

	before := s.connections()
	time.Sleep(200 * time.Millisecond)
	if after := s.connections(); after != before {
		t.Errorf("reconnected after Close: %d -> %d", before, after)
	}
}

func TestCloseReturnsWhenServerNeverHangsUp(t *testing.T) {
	s := newFakeServer(t)
	// A server that acknowledges QUIT but keeps the socket open. The
	// session watcher polls the connection's own state, so without a stop
	// signal of its own it would poll forever and Close, which waits for
	// that watcher, would never return.
	s.lingerAfterQuit = true

	tr := newTestTransport(t, s, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	closed := make(chan struct{})
	go func() {
		tr.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked on a server that never hung up")
	}
}

func TestConnectRespectsContext(t *testing.T) {
	s := newFakeServer(t)
	tr := newTestTransport(t, s, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := tr.Connect(ctx); err == nil {
		t.Fatal("Connect with a cancelled context should fail")
	}
}

// nameIs reports whether msg is the named command, case-insensitively.
func nameIs(msg ircmsg.Message, command string) bool {
	return strings.EqualFold(msg.Command, command)
}
