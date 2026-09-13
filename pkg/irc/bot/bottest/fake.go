// Package bottest provides a scriptable fake implementation of
// provider.Transport for testing providers and the capture path.
//
// It is a normal (non-test) package so packages outside pkg/irc/bot can import
// it. It implements provider.Transport directly rather than wrapping
// bot.Transport, which cannot be driven without a real connection.
//
// A test drives a FakeTransport from two directions. Outbound: it scripts
// replies per command prefix and afterwards asserts on the raw lines the code
// under test issued. Inbound: it pushes messages in with Emit and EmitBatch,
// which run the subscriptions the code under test registered.
//
// Like the rest of pkg/*, this package must stay free of any UI dependency.
package bottest

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/ergochat/irc-go/ircmsg"
	"hop.top/wsm/pkg/irc/provider"
)

// FakeTransport implements provider.Transport against a script instead of a
// server. The zero value is usable: it has no capabilities, is not oper, has an
// empty NAMES table, and errors on every SendLabeled until a script is set.
//
// Every exported field may be set before the code under test runs, and may be
// set concurrently afterwards through the setters, which take the same lock the
// transport's own methods do. Reading a field directly while another goroutine
// emits is a race; use the accessors (SentLines, Names) instead.
type FakeTransport struct {
	// Script maps a command prefix to the replies SendLabeled returns for a
	// raw line starting with that prefix. Matching is by literal string
	// prefix and the longest matching key wins, so "NAMES #a" can override a
	// general "NAMES" entry. A line matching no key is an error: silently
	// returning nothing would let a wrong command pass a test.
	Script map[string][]ircmsg.Message

	// Sent logs every raw line passed to Send, SendLabeled or the command
	// helpers (Join, Part, Mode, Topic, Names), in order. Read it with
	// SentLines when anything might still be running.
	Sent []string

	// SendErr, when non-nil, is returned by Send and SendLabeled instead of
	// consulting the script. The line is still appended to Sent, so a test
	// can assert the failing command was the one it expected.
	SendErr error

	mu        sync.Mutex
	caps      capSet
	oper      bool
	names     map[string]string
	namesErr  error
	subs      map[int]*subscription
	nextSub   int
	reconnect []func()
	closed    bool
}

// ErrNoScript reports a SendLabeled whose raw line matched no Script key.
// Callers match it with errors.Is rather than on the message text.
var ErrNoScript = errors.New("irc/bottest: no script entry")

// ErrClosed reports use of a transport after Close.
var ErrClosed = errors.New("irc/bottest: transport closed")

// FakeTransport must satisfy the interface providers consume; a change to
// provider.Transport should break this line, not each provider's tests.
var _ provider.Transport = (*FakeTransport)(nil)

// subscription is one registered callback, either per-command or per-batch.
type subscription struct {
	command   string
	fn        func(ircmsg.Message)
	batchType string
	batchFn   func([]ircmsg.Message)
}

// capSet is an immutable provider.CapSet built from a list of names.
type capSet struct {
	caps map[string]struct{}
}

// Has reports whether name was negotiated.
func (c capSet) Has(name string) bool {
	_, ok := c.caps[name]
	return ok
}

// Names lists the negotiated capabilities in sorted order, so a test diffing
// them gets a stable sequence.
func (c capSet) Names() []string {
	out := make([]string, 0, len(c.caps))
	for name := range c.caps {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// New returns a FakeTransport with the given capabilities negotiated and an
// empty script. Pass the caps the code under test is expected to degrade on,
// e.g. New("multi-prefix", "labeled-response").
func New(caps ...string) *FakeTransport {
	f := &FakeTransport{}
	f.SetCaps(caps...)
	return f
}

// SetCaps replaces the negotiated capability set.
func (f *FakeTransport) SetCaps(caps ...string) {
	set := make(map[string]struct{}, len(caps))
	for _, name := range caps {
		set[name] = struct{}{}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.caps = capSet{caps: set}
}

// SetOper sets what IsOper reports.
func (f *FakeTransport) SetOper(oper bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.oper = oper
}

// SetNames replaces the table Names returns, for every channel. Values are
// whole prefix runs, not single characters: with multi-prefix a founder who is
// also opped reads as "~@", so SetNames(map[string]string{"botty": "~@"}) is
// the faithful shape.
func (f *FakeTransport) SetNames(names map[string]string) {
	copied := make(map[string]string, len(names))
	for nick, held := range names {
		copied[nick] = held
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names = copied
}

// SetNamesErr makes Names fail with err, for exercising the error path.
func (f *FakeTransport) SetNamesErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.namesErr = err
}

// SetScript replaces the script. Equivalent to assigning Script before the code
// under test runs, but safe once it is running.
func (f *FakeTransport) SetScript(script map[string][]ircmsg.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Script = script
}

// SentLines returns a copy of the raw lines sent so far.
func (f *FakeTransport) SentLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.Sent...)
}

// Connect satisfies the interface and does nothing: there is no server. It
// errors only after Close, so a test cannot accidentally reuse a torn-down
// transport.
func (f *FakeTransport) Connect(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return ErrClosed
	}
	return nil
}

// Close marks the transport closed and drops every subscription, mirroring a
// real teardown. It is safe to call more than once.
func (f *FakeTransport) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.subs = nil
}

// Negotiated reports the capabilities set by New or SetCaps.
func (f *FakeTransport) Negotiated() provider.CapSet {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.caps
}

// IsOper reports the flag set by SetOper.
func (f *FakeTransport) IsOper() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.oper
}

// Send records raw and returns SendErr. Unlike SendLabeled it does not consult
// the script: a fire-and-forget line has no reply to script, and the assertion
// surface for it is Sent.
func (f *FakeTransport) Send(ctx context.Context, raw string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return ErrClosed
	}
	f.Sent = append(f.Sent, raw)
	return f.SendErr
}

// SendLabeled records raw and returns the replies scripted for the longest
// matching command prefix. A line matching no key returns ErrNoScript, so a
// wrong command fails the test at the point it was issued.
func (f *FakeTransport) SendLabeled(ctx context.Context, raw string) ([]ircmsg.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, ErrClosed
	}
	f.Sent = append(f.Sent, raw)
	if f.SendErr != nil {
		return nil, f.SendErr
	}
	replies, ok := f.lookupLocked(raw)
	if !ok {
		return nil, fmt.Errorf("%w for %q", ErrNoScript, raw)
	}
	return append([]ircmsg.Message(nil), replies...), nil
}

// lookupLocked finds the longest Script key that prefixes raw. Longest wins so
// a specific entry ("NAMES #a") beats a general one ("NAMES") regardless of map
// iteration order. f.mu must be held.
func (f *FakeTransport) lookupLocked(raw string) ([]ircmsg.Message, bool) {
	var (
		best   []ircmsg.Message
		bestN  = -1
		found  bool
		script = f.Script
	)
	for prefix, replies := range script {
		if !strings.HasPrefix(raw, prefix) {
			continue
		}
		if len(prefix) > bestN {
			best, bestN, found = replies, len(prefix), true
		}
	}
	return best, found
}

// Subscribe registers fn for every message Emit delivers with the given
// command. The returned cancel is idempotent.
func (f *FakeTransport) Subscribe(cmd string, fn func(ircmsg.Message)) (cancel func()) {
	return f.add(&subscription{command: cmd, fn: fn})
}

// SubscribeBatch registers fn for every batch EmitBatch delivers with the given
// type. The returned cancel is idempotent.
func (f *FakeTransport) SubscribeBatch(typ string, fn func([]ircmsg.Message)) (cancel func()) {
	return f.add(&subscription{batchType: typ, batchFn: fn})
}

// add records sub and returns a cancel that forgets it exactly once.
func (f *FakeTransport) add(sub *subscription) func() {
	f.mu.Lock()
	if f.subs == nil {
		f.subs = map[int]*subscription{}
	}
	f.nextSub++
	key := f.nextSub
	f.subs[key] = sub
	f.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			delete(f.subs, key)
			f.mu.Unlock()
		})
	}
}

// Emit delivers msg to every Subscribe callback registered for its command.
// Callbacks run outside the lock, so one may cancel itself or subscribe again
// without deadlocking.
func (f *FakeTransport) Emit(msg ircmsg.Message) {
	f.mu.Lock()
	var fns []func(ircmsg.Message)
	for _, sub := range f.subs {
		if sub.fn != nil && sub.command == msg.Command {
			fns = append(fns, sub.fn)
		}
	}
	f.mu.Unlock()

	for _, fn := range fns {
		fn(msg)
	}
}

// EmitBatch delivers msgs to every SubscribeBatch callback registered for typ.
func (f *FakeTransport) EmitBatch(typ string, msgs []ircmsg.Message) {
	f.mu.Lock()
	var fns []func([]ircmsg.Message)
	for _, sub := range f.subs {
		if sub.batchFn != nil && sub.batchType == typ {
			fns = append(fns, sub.batchFn)
		}
	}
	f.mu.Unlock()

	for _, fn := range fns {
		fn(append([]ircmsg.Message(nil), msgs...))
	}
}

// Join records a JOIN line.
func (f *FakeTransport) Join(ctx context.Context, ch string) error {
	return f.Send(ctx, "JOIN "+ch)
}

// Part records a PART line.
func (f *FakeTransport) Part(ctx context.Context, ch string) error {
	return f.Send(ctx, "PART "+ch)
}

// Mode records a MODE line with its arguments appended in order.
func (f *FakeTransport) Mode(ctx context.Context, ch, modes string, args ...string) error {
	parts := append([]string{"MODE", ch, modes}, args...)
	return f.Send(ctx, strings.Join(parts, " "))
}

// Topic records a TOPIC line, with the text as a trailing parameter so the
// recorded line round-trips through ircmsg.ParseLine.
func (f *FakeTransport) Topic(ctx context.Context, ch, text string) error {
	return f.Send(ctx, "TOPIC "+ch+" :"+text)
}

// Names records a NAMES line and returns the table set by SetNames. Values are
// whole prefix runs: a founder who is also opped reads as "~@".
func (f *FakeTransport) Names(ctx context.Context, ch string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, ErrClosed
	}
	f.Sent = append(f.Sent, "NAMES "+ch)
	if f.namesErr != nil {
		return nil, f.namesErr
	}
	out := make(map[string]string, len(f.names))
	for nick, held := range f.names {
		out[nick] = held
	}
	return out, nil
}

// OnReconnect registers fn to run on each Reconnect.
func (f *FakeTransport) OnReconnect(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconnect = append(f.reconnect, fn)
}

// Reconnect runs every callback registered with OnReconnect, in registration
// order, simulating a reconnect the provider must recover from. Callbacks run
// outside the lock so one may use the transport.
func (f *FakeTransport) Reconnect() {
	f.mu.Lock()
	fns := append([]func(){}, f.reconnect...)
	f.mu.Unlock()

	for _, fn := range fns {
		fn()
	}
}

// Lines parses a probe fixture into messages, skipping blank lines and the
// leading-# comments the fixtures use to annotate each capture.
//
// The IRC-line fixtures under
// docs/plans/2026-09-10-irc-workspace-chat-design/probes/ are
// sasl-caps-oper.txt, names-multi-prefix.txt, privmsg-tags.txt,
// chathistory-paging.txt, chanserv-amode.txt, chanserv-purge.txt and
// delegation.txt. The remaining two, api-http.txt and ergo-cli.txt, hold HTTP
// and CLI output rather than protocol lines; pointing Lines at those returns an
// error rather than garbage.
//
// That guard is this function's own, not ircmsg's: ircmsg.ParseLine is
// deliberately permissive and accepts almost any non-empty line, upcasing its
// first token into a "command", so `{"success":true}` parses without complaint.
// Lines therefore additionally requires a well-formed command — a three-digit
// numeric or an all-alphabetic verb — which is what separates a real capture
// from a JSON body. A line failing either check returns an error naming the
// file and the 1-based line number, so a fixture edit that breaks a test says
// where.
//
// The check is a filter, not a validator: a CLI line whose first word happens
// to be alphabetic (`mkcerts -d ...`) still parses. Point Lines at the IRC
// fixtures above.
func Lines(path string) ([]ircmsg.Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("irc/bottest: open fixture: %w", err)
	}
	defer file.Close()

	var (
		out     []ircmsg.Message
		scanner = bufio.NewScanner(file)
		lineNo  int
	)
	// Fixture lines carry tags up to 8191 bytes plus the message itself, so
	// bufio's default 64KiB limit is ample; raise nothing, but do report a
	// scan failure rather than truncating silently.
	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		msg, err := ircmsg.ParseLine(line)
		if err != nil {
			return nil, fmt.Errorf("irc/bottest: %s:%d: parse %q: %w", path, lineNo, line, err)
		}
		if !validCommand(msg.Command) {
			return nil, fmt.Errorf("irc/bottest: %s:%d: %q is not an IRC line (command %q)", path, lineNo, line, msg.Command)
		}
		out = append(out, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("irc/bottest: read fixture %s: %w", path, err)
	}
	return out, nil
}

// validCommand reports whether cmd is shaped like an IRC command: a three-digit
// numeric reply, or a verb of ASCII letters. It exists because ircmsg.ParseLine
// accepts non-IRC text and turns its first token into a command, which would
// otherwise let Lines hand back convincing-looking garbage.
func validCommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	if len(cmd) == 3 && cmd[0] >= '0' && cmd[0] <= '9' {
		for i := 0; i < 3; i++ {
			if cmd[i] < '0' || cmd[i] > '9' {
				return false
			}
		}
		return true
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

// Msg builds a message, a shorthand for scripting replies inline without
// spelling out ircmsg.MakeMessage and its nil tag map at every call site.
func Msg(command string, params ...string) ircmsg.Message {
	return ircmsg.MakeMessage(nil, "", command, params...)
}

// MustParse parses a raw IRC line or panics. It is for scripting replies from
// literal wire lines in a test, where a malformed literal is a bug in the test
// itself rather than a condition to handle.
func MustParse(raw string) ircmsg.Message {
	msg, err := ircmsg.ParseLine(raw)
	if err != nil {
		panic(fmt.Sprintf("irc/bottest: parse %q: %v", raw, err))
	}
	return msg
}
