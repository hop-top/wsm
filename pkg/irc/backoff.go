package irc

import "time"

// Backoff default schedule: the supervisor restarts a dead ergo and the bot
// reconnects a dropped session on the same curve.
const (
	// DefaultBackoffMin is the first delay.
	DefaultBackoffMin = time.Second
	// DefaultBackoffMax caps the delay.
	DefaultBackoffMax = 30 * time.Second
	// DefaultBackoffAttempts is how many retries precede giving up and
	// reporting degraded.
	DefaultBackoffAttempts = 5
)

// Clock supplies the time source a Backoff sleeps against. Injectable so the
// supervisor and the bot can be tested without real delays.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// After returns a channel that receives after d elapses.
	After(d time.Duration) <-chan time.Time
}

// realClock is the production Clock backed by the time package.
type realClock struct{}

// Now returns the wall-clock time.
func (realClock) Now() time.Time { return time.Now() }

// After delegates to time.After.
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// SystemClock is the default Clock.
var SystemClock Clock = realClock{}

// Backoff is an exponential retry schedule shared by every wsm IRC component
// that supervises a connection or a child process.
//
// The zero value is not useful; call NewBackoff or set Min, Max and Attempts
// explicitly. A nil Clock falls back to SystemClock, so a Backoff built as a
// plain struct literal still works.
type Backoff struct {
	// Min is the delay before the first retry.
	Min time.Duration
	// Max caps the delay however many attempts have elapsed.
	Max time.Duration
	// Attempts is how many retries are allowed before Done reports true.
	// Zero or negative means unlimited.
	Attempts int
	// Clock is the time source; nil means SystemClock.
	Clock Clock
}

// NewBackoff returns the standard wsm IRC schedule: 1s doubling to 30s, five
// attempts, system clock.
func NewBackoff() Backoff {
	return Backoff{
		Min:      DefaultBackoffMin,
		Max:      DefaultBackoffMax,
		Attempts: DefaultBackoffAttempts,
		Clock:    SystemClock,
	}
}

// Next returns the delay before retry n, counting from 1. The delay doubles
// each attempt, starting at Min and clamped to Max. n below 1 is treated as 1.
//
// Next is pure: it holds no attempt counter, so one Backoff value can be
// shared by several independent retry loops.
func (b Backoff) Next(n int) time.Duration {
	min, max := b.bounds()
	if n < 1 {
		n = 1
	}

	d := min
	for i := 1; i < n; i++ {
		// Double with an overflow guard: a large n must saturate at max
		// rather than wrapping to a negative duration.
		if d > max/2 {
			return max
		}
		d *= 2
	}
	if d > max {
		return max
	}
	return d
}

// Done reports whether attempt n has exhausted the configured budget.
// Attempts <= 0 means unlimited, so Done is always false.
func (b Backoff) Done(n int) bool {
	if b.Attempts <= 0 {
		return false
	}
	return n > b.Attempts
}

// Wait blocks for Next(n), or until done is closed. It reports false when it
// returned early because done fired, so a caller can distinguish a completed
// wait from a cancelled one.
func (b Backoff) Wait(n int, done <-chan struct{}) bool {
	clock := b.Clock
	if clock == nil {
		clock = SystemClock
	}
	select {
	case <-clock.After(b.Next(n)):
		return true
	case <-done:
		return false
	}
}

// Schedule returns the first n delays, for logging a plan or asserting the
// curve in a test.
func (b Backoff) Schedule(n int) []time.Duration {
	if n < 0 {
		n = 0
	}
	out := make([]time.Duration, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, b.Next(i))
	}
	return out
}

// bounds resolves Min and Max, substituting defaults for unset or inverted
// values so a partially built Backoff still produces a sane curve.
func (b Backoff) bounds() (time.Duration, time.Duration) {
	min, max := b.Min, b.Max
	if min <= 0 {
		min = DefaultBackoffMin
	}
	if max <= 0 {
		max = DefaultBackoffMax
	}
	if max < min {
		max = min
	}
	return min, max
}
