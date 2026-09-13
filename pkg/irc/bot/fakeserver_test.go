package bot

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ergochat/irc-go/ircmsg"
)

// The lines the fake server speaks are transcribed from real ergo 2.19.1
// output captured in
// docs/plans/2026-09-10-irc-workspace-chat-design/probes/, so the parsers are
// exercised against the bytes a live server actually sends rather than against
// a convenient approximation.
const (
	// capLSFirst is the first of the two CAP LS 302 lines. The trailing '*'
	// parameter marks it as continued, which is the shape ergo uses when
	// the advertisement does not fit one line.
	capLSFirst = ":ergo.test CAP * LS * :account-notify account-tag away-notify batch cap-notify chghost draft/chathistory draft/multiline=max-bytes=4096,max-lines=100"
	// capLSSecond terminates the advertisement.
	capLSSecond = ":ergo.test CAP * LS :draft/read-marker echo-message extended-join labeled-response message-tags multi-prefix sasl=PLAIN,EXTERNAL,SCRAM-SHA-256 server-time setname"

	// saslLoggedIn and saslSuccess are the SASL PLAIN success pair.
	saslLoggedIn = "@time=2026-09-11T10:22:00.407Z :ergo.test 900 * * botty :You are now logged in as botty"
	saslSuccess  = "@time=2026-09-11T10:22:00.407Z :ergo.test 903 * :Authentication successful"

	// isupportPrefix carries the PREFIX token the prefix parser reads.
	isupportPrefix = "@time=2026-09-11T10:22:00.408Z :ergo.test 005 botty CASEMAPPING=ascii CHANTYPES=# CHATHISTORY=1000 NICKLEN=32 PREFIX=(qaohv)~&@%+ STATUSMSG=~&@%+ :are supported by this server"

	// youreOper is RPL_YOUREOPER, the only proof an OPER succeeded.
	youreOper = "@time=2026-09-11T10:22:01.613Z :ergo.test 381 botty :You are now an IRC operator"

	// namesMulti is the multi-prefix NAMES reply. The founder carries TWO
	// prefix characters, which is the case a single-character parser gets
	// wrong.
	namesMulti = "@time=2026-09-11T10:24:40.248Z :ergo.test 353 botty = #multi :@carol %bob +alice ~@botty"
	// namesEnd terminates the listing.
	namesEnd = "@time=2026-09-11T10:24:40.249Z :ergo.test 366 botty #multi :End of NAMES list"
)

// fakeServer is a scripted IRC server over net.Pipe. It answers registration
// faithfully enough for ircevent to complete a handshake, and lets a test
// script the rest.
type fakeServer struct {
	t *testing.T

	// mu guards every field below it.
	mu sync.Mutex
	// received records every line the client sent, for assertions.
	received []string
	// conns counts accepted connections, so a reconnect is observable.
	conns int
	// requestedCaps records the caps the client asked for, in order.
	requestedCaps []string
	// ackCaps limits which caps are ACKed; nil ACKs everything requested.
	ackCaps map[string]bool
	// operFails suppresses RPL_YOUREOPER so the oper path can be tested
	// failing.
	operFails bool
	// current is the live server-side connection, so a test can drop it.
	current net.Conn
	// onLine lets a test react to a client line beyond the scripted
	// replies.
	onLine func(s *fakeServer, w func(string), msg ircmsg.Message)
	// lingerAfterQuit keeps the connection open after acknowledging QUIT,
	// standing in for a server that never hangs up. A client that waits
	// for the socket to drop would wait forever.
	lingerAfterQuit bool
}

// newFakeServer returns a server that has accepted no connections yet.
func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	return &fakeServer{t: t}
}

// dial is the DialContext the Transport uses: every call produces a fresh pipe
// and a fresh scripted session, which is what makes a reconnect observable.
func (s *fakeServer) dial(context.Context, string, string) (net.Conn, error) {
	client, server := net.Pipe()

	s.mu.Lock()
	s.conns++
	s.current = server
	s.mu.Unlock()

	go s.serve(server)
	return client, nil
}

// drop closes the live server-side connection, which the client sees as EOF.
func (s *fakeServer) drop() {
	s.mu.Lock()
	conn := s.current
	s.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// connections reports how many sessions have been accepted.
func (s *fakeServer) connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}

// lines returns a copy of every line the client sent.
func (s *fakeServer) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.received))
	copy(out, s.received)
	return out
}

// sent reports whether the client sent a line containing want.
func (s *fakeServer) sent(want string) bool {
	for _, line := range s.lines() {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

// serve reads client lines and replies per the script until the pipe closes.
func (s *fakeServer) serve(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	write := func(line string) {
		// A closed pipe is the normal end of a session, not a failure:
		// the client hangs up on Close and on every reconnect.
		_, _ = conn.Write([]byte(line + "\r\n"))
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		s.mu.Lock()
		s.received = append(s.received, line)
		s.mu.Unlock()

		msg, err := ircmsg.ParseLine(line)
		if err != nil {
			continue
		}
		s.handle(write, msg)

		// A real server hangs up after acknowledging QUIT; keeping the
		// pipe open would leave the client believing it is connected.
		s.mu.Lock()
		linger := s.lingerAfterQuit
		s.mu.Unlock()
		if strings.EqualFold(msg.Command, "QUIT") && !linger {
			return
		}
	}
}

// handle scripts one client line.
func (s *fakeServer) handle(write func(string), msg ircmsg.Message) {
	switch strings.ToUpper(msg.Command) {
	case "CAP":
		s.handleCap(write, msg)
	case "AUTHENTICATE":
		// The first AUTHENTICATE names the mechanism and the server
		// answers with a bare '+'; the second carries the credentials.
		if len(msg.Params) > 0 && msg.Params[0] == "PLAIN" {
			write("AUTHENTICATE +")
			return
		}
		write(saslLoggedIn)
		write(saslSuccess)
	case "OPER":
		s.mu.Lock()
		fails := s.operFails
		s.mu.Unlock()
		if fails {
			write(":ergo.test 464 botty :Password incorrect")
			return
		}
		write(youreOper)
	case "NAMES":
		s.handleNames(write, msg)
	case "PING":
		write(":ergo.test PONG ergo.test :" + lastParam(msg))
	case "QUIT":
		write(":ergo.test ERROR :Bye")
	}

	s.mu.Lock()
	hook := s.onLine
	s.mu.Unlock()
	if hook != nil {
		hook(s, write, msg)
	}
}

// handleCap scripts capability negotiation: the two-line LS advertisement, one
// ACK per requested cap, and the 001/005/376 burst that ends registration.
func (s *fakeServer) handleCap(write func(string), msg ircmsg.Message) {
	if len(msg.Params) == 0 {
		return
	}
	switch strings.ToUpper(msg.Params[0]) {
	case "LS":
		write(capLSFirst)
		write(capLSSecond)
	case "REQ":
		if len(msg.Params) < 2 {
			return
		}
		name := msg.Params[1]

		s.mu.Lock()
		s.requestedCaps = append(s.requestedCaps, name)
		allowed := s.ackCaps == nil || s.ackCaps[name]
		s.mu.Unlock()

		if allowed {
			write(":ergo.test CAP * ACK " + name)
			return
		}
		write(":ergo.test CAP * NAK " + name)
	case "END":
		write(":ergo.test 001 botty :Welcome to the ErgoTest IRC Network botty")
		write(isupportPrefix)
		write(":ergo.test 376 botty :End of MOTD command")
	}
}

// handleNames answers a NAMES, wrapping the reply in a labeled batch when the
// client correlated the command with a label.
func (s *fakeServer) handleNames(write func(string), msg ircmsg.Message) {
	hasLabel, label := msg.GetTag("label")
	if !hasLabel {
		write(namesMulti)
		write(namesEnd)
		return
	}

	write("@label=" + label + " :ergo.test BATCH +nms labeled-response")
	write("@batch=nms;time=2026-09-11T10:24:40.248Z :ergo.test 353 botty = #multi :@carol %bob +alice ~@botty")
	write("@batch=nms;time=2026-09-11T10:24:40.249Z :ergo.test 366 botty #multi :End of NAMES list")
	write(":ergo.test BATCH -nms")
}

// lastParam returns a message's trailing parameter, or an empty string.
func lastParam(msg ircmsg.Message) string {
	if len(msg.Params) == 0 {
		return ""
	}
	return msg.Params[len(msg.Params)-1]
}

// testClock is an irc.Clock that fires every wait immediately and records the
// delay it was asked for, so a reconnect test asserts the backoff curve
// without any real sleeping.
type testClock struct {
	// mu guards waited.
	mu sync.Mutex
	// waited is every delay After was asked for, in order.
	waited []time.Duration
}

// Now returns a fixed instant; nothing under test measures elapsed time.
func (c *testClock) Now() time.Time { return time.Unix(0, 0) }

// After records d and fires at once.
func (c *testClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.waited = append(c.waited, d)
	c.mu.Unlock()

	ch := make(chan time.Time, 1)
	ch <- time.Unix(0, 0)
	return ch
}

// delays returns a copy of the recorded schedule.
func (c *testClock) delays() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]time.Duration, len(c.waited))
	copy(out, c.waited)
	return out
}

// waitFor polls cond until it holds or the deadline passes, so a test can wait
// on the reader goroutine without sleeping a fixed amount.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
