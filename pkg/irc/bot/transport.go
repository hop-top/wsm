// Package bot is the IRC transport the wsm daemon drives.
//
// One Transport owns one bot session: it dials the server, authenticates with
// SASL PLAIN, negotiates the IRCv3 capabilities the capture and provisioning
// paths want, optionally takes operator privilege, and then exposes the
// connection as the provider.Transport the providers consume. It reconnects on
// the shared irc.Backoff curve and re-establishes authentication and operator
// privilege before telling anyone the session is back.
//
// The package holds no policy of its own beyond the protocol: no services
// commands (CS/NS spellings belong to the concrete providers), no capture
// rules, no provisioning. It also holds no UI dependency, which CI enforces
// through `make verify-no-ui-leak`.
package bot

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ergochat/irc-go/ircevent"
	"github.com/ergochat/irc-go/ircmsg"

	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
)

// Numerics the transport reacts to by hand.
const (
	// rplWelcome (001) ends registration.
	rplWelcome = "001"
	// rplYoureOper (381) confirms an OPER succeeded.
	rplYoureOper = "381"
	// rplNamReply (353) carries a page of channel occupants.
	rplNamReply = "353"
	// rplEndOfNames (366) terminates a NAMES listing.
	rplEndOfNames = "366"
)

// LabelTimeout caps how long SendLabeled waits for a correlated reply. Ten
// seconds is far longer than any healthy round trip and still short enough
// that a wedged server cannot stall a reconcile indefinitely.
const LabelTimeout = 10 * time.Second

// dialTimeout caps a single connection attempt, including registration. It
// also bounds ircevent's own CAP and SASL waits, which are derived from it.
const dialTimeout = 30 * time.Second

// keepAlive is how often the connection is pinged when otherwise idle. It must
// be at least dialTimeout or ircevent refuses the configuration.
const keepAlive = 2 * time.Minute

// ErrNotConnected is returned by every operation attempted before Connect
// succeeded, or after Close.
var ErrNotConnected = errors.New("irc/bot: not connected")

// ErrNoLabeledReply reports that a labeled command produced no response at
// all. Callers that tolerate a partial set still need to distinguish silence
// from an empty answer.
var ErrNoLabeledReply = errors.New("irc/bot: no labeled reply")

// Options configures a Transport. Addr, Nick and Account are required; the
// rest have workable defaults.
type Options struct {
	// Addr is the server's host:port.
	Addr string
	// TLS configures a TLS connection. Nil means plaintext, which is only
	// appropriate for a loopback listener.
	TLS *tls.Config
	// Account is the SASL account the bot logs in as.
	Account string
	// Password is the SASL passphrase.
	Password string
	// OperName is the operator block to OPER into. Empty means the bot
	// stays an ordinary user and IsOper reports false.
	OperName string
	// OperPass is the operator passphrase.
	OperPass string
	// Nick is the nickname to register. Empty falls back to Account, which
	// is what a wsm-provisioned bot wants: ergo reserves the nick to the
	// account anyway.
	Nick string
	// Realname is the GECOS field. Empty falls back to Nick.
	Realname string
	// Backoff is the reconnect schedule. The zero value takes the shared
	// wsm defaults, so a caller that does not care can leave it unset.
	Backoff irc.Backoff
	// WantCaps are the capabilities to request. Nil means DefaultCaps.
	WantCaps []string
	// Log receives transport diagnostics. Nil means discard.
	Log *slog.Logger
}

// nick resolves the nickname to register, falling back to the account.
func (o Options) nick() string {
	if o.Nick != "" {
		return o.Nick
	}
	return o.Account
}

// realname resolves the GECOS field, falling back to the nick.
func (o Options) realname() string {
	if o.Realname != "" {
		return o.Realname
	}
	return o.nick()
}

// caps resolves the capabilities to request.
func (o Options) caps() []string {
	if len(o.WantCaps) > 0 {
		return o.WantCaps
	}
	return DefaultCaps
}

// backoff resolves the reconnect schedule, substituting the shared wsm curve
// for an unset one. A Backoff with a nil Clock still works: Backoff itself
// falls back to the system clock.
func (o Options) backoff() irc.Backoff {
	b := o.Backoff
	if b.Min <= 0 && b.Max <= 0 && b.Attempts == 0 && b.Clock == nil {
		return irc.NewBackoff()
	}
	return b
}

// logger returns the configured logger, or one that discards everything.
func (o Options) logger() *slog.Logger {
	if o.Log != nil {
		return o.Log
	}
	return slog.New(slog.DiscardHandler)
}

// Transport is a live bot session implementing provider.Transport.
//
// It is safe for concurrent use: the connection pointer and the session-scoped
// state behind it are guarded by mu, while the callbacks ircevent invokes run
// on its own reader goroutine and reach subscribers directly.
type Transport struct {
	opts Options
	log  *slog.Logger

	// dialContext opens the underlying connection. Nil means the ordinary
	// TCP dialer; tests substitute an in-memory pipe so the whole session
	// runs without a socket.
	dialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	// mu guards every field below it.
	mu sync.Mutex
	// conn is the live connection, nil before Connect and after Close.
	conn *ircevent.Connection
	// caps is the capability set the current session negotiated.
	caps capSet
	// prefixes is the membership prefix run parsed from ISUPPORT.
	prefixes string
	// oper records whether this session received RPL_YOUREOPER.
	oper bool
	// closed is set by Close so the reconnect loop stops and every
	// operation reports ErrNotConnected.
	closed bool
	// subs are the caller's subscriptions, re-registered on each new
	// connection because ircevent callbacks do not survive a reconnect
	// that builds a fresh Connection.
	subs map[int]*subscription
	// nextSub numbers subscriptions.
	nextSub int
	// reconnectFns run after each successful reconnect, once
	// authentication and operator privilege have been re-established.
	reconnectFns []func()
	// done is closed by Close to interrupt a backoff wait in flight.
	done chan struct{}
	// loopDone is closed when the reconnect loop has exited, so Close can
	// return only once nothing else will touch the connection.
	loopDone chan struct{}
	// supervising reports whether the reconnect loop was ever started.
	// Close waits on loopDone only when it was, because a Transport that
	// never connected has nobody to close that channel.
	supervising bool
}

// subscription is one caller-registered callback, held so it can be
// re-registered against a replacement connection after a reconnect.
//
// id and registered are guarded by the Transport's mutex, because a reconnect
// re-registers a subscription while its owner may be cancelling it.
type subscription struct {
	// command is the IRC command or numeric, empty for a batch
	// subscription.
	command string
	// batchType is the batch type, empty for a command subscription.
	batchType string
	// fn receives single messages for a command subscription.
	fn func(ircmsg.Message)
	// batchFn receives the flattened contents of a completed batch.
	batchFn func([]ircmsg.Message)
	// id identifies the callback on the connection it was registered
	// against, so cancel can remove it.
	id ircevent.CallbackID
	// registered reports whether id refers to a live registration.
	registered bool
}

// discardLogger returns a logger that throws everything away. ircevent wants a
// *log.Logger and defaults to stdout, which would scribble protocol chatter
// over the CLI; transport diagnostics go through the slog logger instead.
func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// New builds a Transport from opts without contacting the server.
func New(opts Options) *Transport {
	return &Transport{
		opts:     opts,
		log:      opts.logger(),
		prefixes: DefaultPrefixes,
		subs:     map[int]*subscription{},
		done:     make(chan struct{}),
		loopDone: make(chan struct{}),
	}
}

// Transport implements the interface the providers consume.
var _ provider.Transport = (*Transport)(nil)

// Connect dials, authenticates, negotiates capabilities and, when an operator
// block is configured, takes operator privilege. On success it starts the
// reconnect loop that keeps the session alive for the life of the Transport.
//
// ctx bounds the initial attempt only: once Connect returns, the session
// outlives it and is torn down by Close.
func (t *Transport) Connect(ctx context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrNotConnected
	}
	if t.conn != nil {
		t.mu.Unlock()
		return errors.New("irc/bot: already connected")
	}
	t.mu.Unlock()

	conn, err := t.dial(ctx)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.conn = conn
	t.supervising = true
	t.mu.Unlock()

	go t.supervise(conn)
	return nil
}

// dial builds a connection, registers the library-level callbacks, performs
// the handshake and then applies the post-registration steps every session
// needs: ISUPPORT-derived prefixes, operator privilege, and the caller's
// subscriptions.
func (t *Transport) dial(ctx context.Context) (*ircevent.Connection, error) {
	conn := &ircevent.Connection{
		Server:      t.opts.Addr,
		Nick:        t.opts.nick(),
		User:        t.opts.nick(),
		RealName:    t.opts.realname(),
		RequestCaps: t.opts.caps(),
		Timeout:     dialTimeout,
		KeepAlive:   keepAlive,
		// The reconnect curve is ours, not ircevent's: we drive
		// reconnection ourselves so OnReconnect can fire after re-auth.
		ReconnectFreq: time.Second,
		Log:           discardLogger(),
	}
	if t.dialContext != nil {
		conn.DialContext = t.dialContext
	}
	if t.opts.TLS != nil {
		conn.UseTLS = true
		conn.TLSConfig = t.opts.TLS
	}
	if t.opts.Account != "" {
		conn.UseSASL = true
		conn.SASLLogin = t.opts.Account
		conn.SASLPassword = t.opts.Password
		conn.SASLMech = "PLAIN"
	}

	// Operator privilege is per-session state: a fresh connection has none
	// until its own 381 arrives.
	operCh := make(chan struct{}, 1)
	conn.AddCallback(rplYoureOper, func(ircmsg.Message) {
		t.mu.Lock()
		if t.conn == conn || t.conn == nil {
			t.oper = true
		}
		t.mu.Unlock()
		select {
		case operCh <- struct{}{}:
		default:
		}
	})

	if err := connectWithContext(ctx, conn); err != nil {
		return nil, fmt.Errorf("irc/bot: connect %s: %w", t.opts.Addr, err)
	}

	t.mu.Lock()
	t.caps = newCapSet(conn.AcknowledgedCaps())
	t.oper = false
	t.mu.Unlock()

	// ISUPPORT arrives during registration, so PREFIX is readable now.
	if token := conn.ISupport()["PREFIX"]; token != "" {
		t.mu.Lock()
		t.prefixes = parsePrefixes(token)
		t.mu.Unlock()
	}

	t.resubscribe(conn)

	if err := t.takeOper(ctx, conn, operCh); err != nil {
		conn.Quit()
		return nil, err
	}

	return conn, nil
}

// connectWithContext runs conn.Connect while honouring ctx. ircevent's Connect
// is synchronous and bounded by its own Timeout, so a cancelled context tears
// the half-open connection down rather than waiting that timeout out.
func connectWithContext(ctx context.Context, conn *ircevent.Connection) error {
	errCh := make(chan error, 1)
	go func() { errCh <- conn.Connect() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		conn.Quit()
		// Drain so the goroutine cannot outlive us holding the socket.
		<-errCh
		return ctx.Err()
	}
}

// takeOper sends OPER and waits for RPL_YOUREOPER when an operator block is
// configured. A configured operator that never confirms is a hard failure:
// every privileged provider path would otherwise silently degrade.
func (t *Transport) takeOper(ctx context.Context, conn *ircevent.Connection, operCh <-chan struct{}) error {
	if t.opts.OperName == "" {
		return nil
	}
	if err := conn.Send("OPER", t.opts.OperName, t.opts.OperPass); err != nil {
		return fmt.Errorf("irc/bot: oper: %w", err)
	}

	select {
	case <-operCh:
		return nil
	case <-time.After(LabelTimeout):
		return errors.New("irc/bot: oper: no RPL_YOUREOPER")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// supervise runs the connection's read loop and reconnects it on the
// configured backoff curve until Close.
//
// ircevent's own Loop reconnects too, but it re-runs only the handshake: it
// knows nothing about OPER or about the callbacks we re-register, and it gives
// no hook that runs after both. Driving the curve here is what lets
// OnReconnect mean "authenticated, opered, subscribed and ready".
func (t *Transport) supervise(conn *ircevent.Connection) {
	defer close(t.loopDone)

	backoff := t.opts.backoff()
	current := conn

	for {
		// Block until this connection stops for any reason, or until
		// Close asks us to stop watching it.
		t.waitForDisconnect(current)

		if t.isClosed() {
			return
		}

		t.mu.Lock()
		t.conn = nil
		t.oper = false
		t.mu.Unlock()

		next, ok := t.redial(backoff)
		if !ok {
			return
		}
		current = next
	}
}

// redial retries the connection on the backoff curve. It reports false when
// the Transport was closed or the attempt budget ran out, in which case the
// session stays down and every operation reports ErrNotConnected.
func (t *Transport) redial(backoff irc.Backoff) (*ircevent.Connection, bool) {
	for attempt := 1; !backoff.Done(attempt); attempt++ {
		if !backoff.Wait(attempt, t.done) {
			return nil, false
		}
		if t.isClosed() {
			return nil, false
		}

		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		conn, err := t.dial(ctx)
		cancel()
		if err != nil {
			t.log.Warn("irc bot reconnect failed", "attempt", attempt, "error", err)
			continue
		}

		t.mu.Lock()
		t.conn = conn
		fns := make([]func(), len(t.reconnectFns))
		copy(fns, t.reconnectFns)
		t.mu.Unlock()

		// Authentication, capabilities and operator privilege are all
		// re-established by dial, so callers can safely reassert channel
		// state from here.
		for _, fn := range fns {
			fn()
		}
		return conn, true
	}

	t.log.Error("irc bot reconnect budget exhausted", "addr", t.opts.Addr)
	return nil, false
}

// waitForDisconnect blocks until conn is no longer connected, or until Close
// signals that the session is being torn down.
//
// ircevent exposes no completion channel for a connection it is not looping
// over, so this polls the connection's own state. It must also watch done: a
// server that acknowledges QUIT without hanging up would otherwise leave this
// polling forever, and Close waits on the loop it runs in.
func (t *Transport) waitForDisconnect(conn *ircevent.Connection) {
	const pollInterval = 10 * time.Millisecond
	for conn.Connected() {
		select {
		case <-t.done:
			return
		case <-time.After(pollInterval):
		}
	}
}

// Close tears the session down and stops reconnecting.
func (t *Transport) Close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	conn := t.conn
	supervising := t.supervising
	t.conn = nil
	close(t.done)
	t.mu.Unlock()

	if conn != nil {
		conn.Quit()
	}
	// A Transport that never connected has no reconnect loop to drain, so
	// waiting on loopDone would block forever.
	if supervising {
		<-t.loopDone
	}
}

// isClosed reports whether Close has been called.
func (t *Transport) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// Negotiated reports the capabilities the server granted.
func (t *Transport) Negotiated() provider.CapSet {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.caps
}

// IsOper reports whether this session holds operator privilege.
func (t *Transport) IsOper() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.oper
}

// connection returns the live connection, or an error when there is none.
func (t *Transport) connection() (*ircevent.Connection, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.conn == nil {
		return nil, ErrNotConnected
	}
	return t.conn, nil
}

// Send writes a raw line without waiting for a reply.
func (t *Transport) Send(ctx context.Context, raw string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := t.connection()
	if err != nil {
		return err
	}
	if err := conn.SendRaw(raw); err != nil {
		return fmt.Errorf("irc/bot: send: %w", err)
	}
	return nil
}

// SendLabeled writes a raw line and collects the replies correlated to it.
//
// With labeled-response the correlation is exact. Without it there is nothing
// to correlate on, so the fallback collects whatever arrives during a short
// window and the caller tolerates a partial set, as the interface documents.
func (t *Transport) SendLabeled(ctx context.Context, raw string) ([]ircmsg.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := t.connection()
	if err != nil {
		return nil, err
	}

	msg, err := ircmsg.ParseLine(raw)
	if err != nil {
		return nil, fmt.Errorf("irc/bot: parse %q: %w", raw, err)
	}

	t.mu.Lock()
	labeled := t.caps.Has(CapLabeledResponse) && t.caps.Has(CapBatch)
	t.mu.Unlock()

	if !labeled {
		return t.collectUnlabeled(ctx, conn, msg)
	}

	ctx, cancel := context.WithTimeout(ctx, LabelTimeout)
	defer cancel()

	type result struct {
		batch *ircevent.Batch
		err   error
	}
	resCh := make(chan result, 1)
	go func() {
		batch, err := conn.GetLabeledResponse(msg.AllTags(), msg.Command, msg.Params...)
		resCh <- result{batch: batch, err: err}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("irc/bot: labeled send: %w", res.err)
		}
		return flatten(res.batch), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// unlabeledWindow is how long the labeled-response fallback keeps collecting
// replies. It is a compromise with no protocol backing, which is exactly why
// the interface tells callers to tolerate a partial set.
const unlabeledWindow = 2 * time.Second

// collectUnlabeled is the best-effort path for a server without
// labeled-response: send the command, then gather every line that arrives in a
// short window. It cannot distinguish our replies from unrelated traffic, so
// it filters to the numerics and commands servers actually reply with.
func (t *Transport) collectUnlabeled(ctx context.Context, conn *ircevent.Connection, msg ircmsg.Message) ([]ircmsg.Message, error) {
	var (
		mu        sync.Mutex
		collected []ircmsg.Message
	)

	id := conn.AddRawCallback(func(_ string, parsed ircmsg.Message, err error) {
		if err != nil {
			return
		}
		mu.Lock()
		collected = append(collected, parsed)
		mu.Unlock()
	})
	defer conn.RemoveCallback(id)

	if err := conn.SendWithTags(msg.AllTags(), msg.Command, msg.Params...); err != nil {
		return nil, fmt.Errorf("irc/bot: send: %w", err)
	}

	timer := time.NewTimer(unlabeledWindow)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	mu.Lock()
	out := make([]ircmsg.Message, len(collected))
	copy(out, collected)
	mu.Unlock()

	if len(out) == 0 {
		return nil, ErrNoLabeledReply
	}
	return out, nil
}

// flatten renders a batch as the flat sequence of its non-BATCH lines, in
// order, descending into nested batches. The enclosing BATCH markers carry no
// payload a caller wants.
func flatten(batch *ircevent.Batch) []ircmsg.Message {
	if batch == nil {
		return nil
	}
	if batch.Command != "BATCH" {
		return []ircmsg.Message{batch.Message}
	}
	var out []ircmsg.Message
	for _, item := range batch.Items {
		out = append(out, flatten(item)...)
	}
	return out
}

// Subscribe calls fn for every inbound message with the given command. The
// returned function cancels the subscription, including across reconnects.
func (t *Transport) Subscribe(cmd string, fn func(ircmsg.Message)) (cancel func()) {
	return t.addSubscription(&subscription{command: cmd, fn: fn})
}

// SubscribeBatch calls fn once per completed batch of the given type, with the
// batch's lines flattened in order. The returned function cancels the
// subscription.
func (t *Transport) SubscribeBatch(typ string, fn func([]ircmsg.Message)) (cancel func()) {
	return t.addSubscription(&subscription{batchType: typ, batchFn: fn})
}

// addSubscription records sub, registers it against the live connection if
// there is one, and returns a cancel that both forgets it and unregisters it.
func (t *Transport) addSubscription(sub *subscription) func() {
	t.mu.Lock()
	t.nextSub++
	key := t.nextSub
	t.subs[key] = sub
	conn := t.conn
	t.mu.Unlock()

	if conn != nil {
		t.register(conn, sub)
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			delete(t.subs, key)
			live := t.conn
			id, registered := sub.id, sub.registered
			t.mu.Unlock()

			if live != nil && registered {
				live.RemoveCallback(id)
			}
		})
	}
}

// register attaches one subscription to a connection.
func (t *Transport) register(conn *ircevent.Connection, sub *subscription) {
	var id ircevent.CallbackID
	if sub.batchFn != nil {
		wanted := sub.batchType
		batchFn := sub.batchFn
		id = conn.AddBatchCallback(func(batch *ircevent.Batch) bool {
			if !batchMatches(batch, wanted) {
				// Returning false lets other handlers see it,
				// and leaves an unclaimed batch to be flattened
				// into the ordinary callbacks.
				return false
			}
			batchFn(flatten(batch))
			return true
		})
	} else {
		id = conn.AddCallback(sub.command, sub.fn)
	}

	t.mu.Lock()
	sub.id = id
	sub.registered = true
	t.mu.Unlock()
}

// resubscribe registers every live subscription against a new connection.
func (t *Transport) resubscribe(conn *ircevent.Connection) {
	t.mu.Lock()
	subs := make([]*subscription, 0, len(t.subs))
	for _, sub := range t.subs {
		subs = append(subs, sub)
	}
	t.mu.Unlock()

	for _, sub := range subs {
		t.register(conn, sub)
	}
}

// batchMatches reports whether a completed batch is of the wanted type. The
// BATCH start line is `BATCH +<id> <type> [params...]`, so the type is the
// second parameter. An empty wanted type matches every batch.
func batchMatches(batch *ircevent.Batch, wanted string) bool {
	if batch == nil || batch.Command != "BATCH" {
		return false
	}
	if wanted == "" {
		return true
	}
	if len(batch.Params) < 2 {
		return false
	}
	return strings.EqualFold(batch.Params[1], wanted)
}

// Join enters a channel.
func (t *Transport) Join(ctx context.Context, ch string) error {
	return t.command(ctx, "JOIN", ch)
}

// Part leaves a channel.
func (t *Transport) Part(ctx context.Context, ch string) error {
	return t.command(ctx, "PART", ch)
}

// Mode applies channel modes.
func (t *Transport) Mode(ctx context.Context, ch, modes string, args ...string) error {
	params := append([]string{ch, modes}, args...)
	return t.command(ctx, "MODE", params...)
}

// Topic sets a channel topic.
func (t *Transport) Topic(ctx context.Context, ch, text string) error {
	return t.command(ctx, "TOPIC", ch, text)
}

// command sends one structured command, letting ircmsg handle the escaping a
// hand-built raw line would get wrong.
func (t *Transport) command(ctx context.Context, cmd string, params ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := t.connection()
	if err != nil {
		return err
	}
	if err := conn.Send(cmd, params...); err != nil {
		return fmt.Errorf("irc/bot: %s: %w", strings.ToLower(cmd), err)
	}
	return nil
}

// Names lists channel occupants as nick to mode prefixes.
//
// With multi-prefix the value carries every prefix the user holds, so a
// founder who is also opped reads as "~@". The listing is taken from the
// labeled response when the server supports one, and otherwise by watching the
// 353/366 pair the plain NAMES produces.
func (t *Transport) Names(ctx context.Context, ch string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := t.connection()
	if err != nil {
		return nil, err
	}

	t.mu.Lock()
	labeled := t.caps.Has(CapLabeledResponse) && t.caps.Has(CapBatch)
	prefixes := t.prefixes
	t.mu.Unlock()

	if labeled {
		replies, err := t.SendLabeled(ctx, "NAMES "+ch)
		if err != nil {
			return nil, err
		}
		return foldNames(replies, ch, prefixes), nil
	}
	return t.namesUnlabeled(ctx, conn, ch, prefixes)
}

// namesUnlabeled collects a NAMES listing without labeled-response by
// subscribing to 353 and 366 for the duration of the call.
func (t *Transport) namesUnlabeled(ctx context.Context, conn *ircevent.Connection, ch, prefixes string) (map[string]string, error) {
	var (
		mu    sync.Mutex
		names = map[string]string{}
	)
	done := make(chan struct{})
	var closeOnce sync.Once

	replyID := conn.AddCallback(rplNamReply, func(msg ircmsg.Message) {
		if !strings.EqualFold(namesChannel(msg), ch) {
			return
		}
		mu.Lock()
		parseNamesReply(msg, prefixes, names)
		mu.Unlock()
	})
	defer conn.RemoveCallback(replyID)

	endID := conn.AddCallback(rplEndOfNames, func(msg ircmsg.Message) {
		if !strings.EqualFold(namesChannel(msg), ch) {
			return
		}
		closeOnce.Do(func() { close(done) })
	})
	defer conn.RemoveCallback(endID)

	if err := conn.Send("NAMES", ch); err != nil {
		return nil, fmt.Errorf("irc/bot: names: %w", err)
	}

	timer := time.NewTimer(LabelTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		return nil, fmt.Errorf("irc/bot: names %s: %w", ch, ErrNoLabeledReply)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]string, len(names))
	for nick, held := range names {
		out[nick] = held
	}
	return out, nil
}

// foldNames reduces a set of replies to the nick-to-prefixes map, keeping only
// the 353 lines that name the requested channel.
func foldNames(replies []ircmsg.Message, ch, prefixes string) map[string]string {
	names := map[string]string{}
	for _, msg := range replies {
		if msg.Command != rplNamReply {
			continue
		}
		if !strings.EqualFold(namesChannel(msg), ch) {
			continue
		}
		parseNamesReply(msg, prefixes, names)
	}
	return names
}

// OnReconnect registers fn to run after each successful reconnect, once
// authentication, capabilities and operator privilege are back.
func (t *Transport) OnReconnect(fn func()) {
	if fn == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reconnectFns = append(t.reconnectFns, fn)
}
