package irc

import (
	"testing"
	"time"
)

// fakeClock is a Clock that fires immediately and records what it was asked
// to wait for, so a retry loop can be tested without real delays.
type fakeClock struct {
	waited []time.Duration
}

// Now returns a fixed instant; nothing under test reads it for elapsed time.
func (f *fakeClock) Now() time.Time { return time.Unix(0, 0) }

// After records d and fires at once.
func (f *fakeClock) After(d time.Duration) <-chan time.Time {
	f.waited = append(f.waited, d)
	ch := make(chan time.Time, 1)
	ch <- time.Unix(0, 0)
	return ch
}

func TestBackoff_Schedule(t *testing.T) {
	// The documented curve: 1s doubling to a 30s ceiling.
	b := NewBackoff()

	want := []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // clamped, would be 32s
		30 * time.Second,
	}

	got := b.Schedule(len(want))
	if len(got) != len(want) {
		t.Fatalf("Schedule(%d) returned %d entries", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("attempt %d = %v, want %v", i+1, got[i], want[i])
		}
	}
}

func TestBackoff_Next(t *testing.T) {
	b := NewBackoff()

	tests := []struct {
		name string
		n    int
		want time.Duration
	}{
		{name: "first attempt", n: 1, want: time.Second},
		{name: "second attempt", n: 2, want: 2 * time.Second},
		{name: "clamped at the ceiling", n: 6, want: 30 * time.Second},
		{name: "stays clamped", n: 100, want: 30 * time.Second},
		{name: "zero treated as first", n: 0, want: time.Second},
		{name: "negative treated as first", n: -5, want: time.Second},
		{name: "huge n does not overflow", n: 1_000_000, want: 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.Next(tt.n)
			if got != tt.want {
				t.Errorf("Next(%d) = %v, want %v", tt.n, got, tt.want)
			}
			if got < 0 {
				t.Errorf("Next(%d) = %v, must never be negative", tt.n, got)
			}
		})
	}
}

func TestBackoff_CustomBounds(t *testing.T) {
	b := Backoff{Min: 100 * time.Millisecond, Max: time.Second}

	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		time.Second, // clamped
	}
	for i, w := range want {
		if got := b.Next(i + 1); got != w {
			t.Errorf("Next(%d) = %v, want %v", i+1, got, w)
		}
	}
}

func TestBackoff_ZeroValueUsesDefaults(t *testing.T) {
	// A Backoff built as a bare struct literal (no constructor) must still
	// produce the standard curve rather than a zero delay spin.
	var b Backoff

	if got := b.Next(1); got != DefaultBackoffMin {
		t.Errorf("Next(1) = %v, want the default min %v", got, DefaultBackoffMin)
	}
	if got := b.Next(50); got != DefaultBackoffMax {
		t.Errorf("Next(50) = %v, want the default max %v", got, DefaultBackoffMax)
	}
}

func TestBackoff_InvertedBounds(t *testing.T) {
	// Max below Min must not produce a negative or shrinking delay.
	b := Backoff{Min: 10 * time.Second, Max: time.Second}

	if got := b.Next(1); got != 10*time.Second {
		t.Errorf("Next(1) = %v, want the min %v", got, 10*time.Second)
	}
	if got := b.Next(5); got < b.Min {
		t.Errorf("Next(5) = %v, must not fall below Min %v", got, b.Min)
	}
}

func TestBackoff_Done(t *testing.T) {
	b := NewBackoff() // 5 attempts

	tests := []struct {
		n    int
		want bool
	}{
		{n: 1, want: false},
		{n: 5, want: false},
		{n: 6, want: true},
		{n: 99, want: true},
	}
	for _, tt := range tests {
		if got := b.Done(tt.n); got != tt.want {
			t.Errorf("Done(%d) = %v, want %v", tt.n, got, tt.want)
		}
	}

	// Unlimited budget never finishes.
	unlimited := Backoff{Min: time.Second, Max: time.Second}
	if unlimited.Done(1_000) {
		t.Error("Attempts <= 0 must mean unlimited")
	}
}

func TestBackoff_Wait(t *testing.T) {
	clock := &fakeClock{}
	b := Backoff{
		Min: time.Second, Max: 30 * time.Second, Attempts: 5, Clock: clock,
	}

	done := make(chan struct{})
	if !b.Wait(1, done) {
		t.Fatal("Wait must report true when the delay elapses")
	}
	if !b.Wait(2, done) {
		t.Fatal("Wait must report true when the delay elapses")
	}

	// The injected clock saw the real schedule, so a supervisor test can
	// assert the curve without sleeping.
	if len(clock.waited) != 2 {
		t.Fatalf("clock saw %d waits, want 2", len(clock.waited))
	}
	if clock.waited[0] != time.Second || clock.waited[1] != 2*time.Second {
		t.Errorf("clock saw %v, want [1s 2s]", clock.waited)
	}
}

func TestBackoff_WaitCancelled(t *testing.T) {
	// A closed done channel must win over a pending delay, so a shutting-down
	// supervisor is not held for the full backoff.
	b := Backoff{Min: time.Hour, Max: time.Hour, Clock: SystemClock}

	done := make(chan struct{})
	close(done)

	if b.Wait(1, done) {
		t.Error("Wait must report false when cancelled")
	}
}

func TestBackoff_WaitNilClockDoesNotPanic(t *testing.T) {
	b := Backoff{Min: time.Millisecond, Max: time.Millisecond}

	done := make(chan struct{})
	if !b.Wait(1, done) {
		t.Error("Wait with a nil Clock must fall back to the system clock")
	}
}

func TestBackoff_SharedAcrossLoops(t *testing.T) {
	// Next is pure, so one value can drive two independent retry loops
	// (supervisor and bot) without their attempt counts interfering.
	b := NewBackoff()

	if b.Next(1) != time.Second {
		t.Fatal("first loop attempt 1")
	}
	if b.Next(3) != 4*time.Second {
		t.Fatal("second loop attempt 3")
	}
	if b.Next(1) != time.Second {
		t.Fatal("first loop attempt 1 must be unaffected by the second loop")
	}
}

func TestBackoff_ScheduleEdgeCases(t *testing.T) {
	b := NewBackoff()

	if got := b.Schedule(0); len(got) != 0 {
		t.Errorf("Schedule(0) = %v, want empty", got)
	}
	if got := b.Schedule(-1); len(got) != 0 {
		t.Errorf("Schedule(-1) = %v, want empty", got)
	}
}
