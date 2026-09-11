package provision

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/wsm/pkg/irc"
)

// fakeLifecycle records the calls the hook makes, in order.
type fakeLifecycle struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]error
}

func (f *fakeLifecycle) record(verb, wsID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, verb+":"+wsID)
	return f.fail[verb]
}

func (f *fakeLifecycle) Enable(_ context.Context, wsID string) error {
	return f.record("enable", wsID)
}
func (f *fakeLifecycle) Start(_ context.Context, wsID string) error {
	return f.record("start", wsID)
}
func (f *fakeLifecycle) Stop(_ context.Context, wsID string) error {
	return f.record("stop", wsID)
}
func (f *fakeLifecycle) Purge(_ context.Context, wsID string) error {
	return f.record("purge", wsID)
}

func (f *fakeLifecycle) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// managedAuto is the only configuration that subscribes.
func managedAuto() irc.Config {
	return irc.Config{Enabled: true, Mode: irc.ModeManaged, AutoProvision: true}
}

// newTestBus builds an in-memory bus closed at test end.
func newTestBus(t *testing.T) bus.Bus {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = b.Close(ctx)
	})
	return b
}

// publish emits one workspace topic for wsID.
func publish(t *testing.T, b bus.Bus, topic, wsID string) {
	t.Helper()
	ev := bus.NewEvent(bus.Topic(topic), "wsm", map[string]any{"id": wsID})
	ev.WorkspaceID = wsID
	if err := b.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish %s: %v", topic, err)
	}
}

// noGuests is a loader for workspaces that host nobody.
func noGuests(irc.Profile) ProfileLoader {
	return func(string) (irc.Profile, error) { return irc.Profile{}, nil }
}

func TestSubscribeRoutesTopicsToLifecycle(t *testing.T) {
	tests := []struct {
		name  string
		topic string
		want  []string
	}{
		{"created enables", "wsm.runtime.workspace.created", []string{"enable:ws1"}},
		{"archived stops", "wsm.runtime.workspace.archived", []string{"stop:ws1"}},
		{"unarchived starts", "wsm.runtime.workspace.unarchived", []string{"start:ws1"}},
		{"deleted stops then purges", "wsm.runtime.workspace.deleted", []string{"stop:ws1", "purge:ws1"}},
		{"updated is a no-op", "wsm.runtime.workspace.updated", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lc := &fakeLifecycle{}
			b := newTestBus(t)
			unsub := Subscribe(b, managedAuto(), HookOptions{
				Lifecycle: lc,
				Profiles:  noGuests(irc.Profile{}),
			})
			defer unsub()

			publish(t, b, tc.topic, "ws1")

			got := lc.seen()
			if len(got) != len(tc.want) {
				t.Fatalf("calls = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("calls = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestSubscribeOnlyInManagedAutoProvision(t *testing.T) {
	tests := []struct {
		name string
		cfg  irc.Config
	}{
		{
			name: "external mode does not subscribe",
			cfg:  irc.Config{Enabled: true, Mode: irc.ModeExternal, AutoProvision: true},
		},
		{
			name: "auto_provision off does not subscribe",
			cfg:  irc.Config{Enabled: true, Mode: irc.ModeManaged, AutoProvision: false},
		},
		{
			name: "external mode without auto_provision does not subscribe",
			cfg:  irc.Config{Enabled: true, Mode: irc.ModeExternal, AutoProvision: false},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lc := &fakeLifecycle{}
			b := newTestBus(t)
			unsub := Subscribe(b, tc.cfg, HookOptions{
				Lifecycle: lc,
				Profiles:  noGuests(irc.Profile{}),
			})
			defer unsub()

			for _, topic := range []string{
				"wsm.runtime.workspace.created",
				"wsm.runtime.workspace.archived",
				"wsm.runtime.workspace.unarchived",
				"wsm.runtime.workspace.deleted",
			} {
				publish(t, b, topic, "ws1")
			}

			if got := lc.seen(); len(got) != 0 {
				t.Fatalf("no subscription expected, got calls %v", got)
			}
		})
	}
}

// TestHostGuardBlocksTeardown pins the guard: a workspace whose profile
// lists guests must not be stopped or purged from a bus event. Removing
// the SharedWith check in hostGuarded turns both subtests red.
func TestHostGuardBlocksTeardown(t *testing.T) {
	tests := []struct {
		name  string
		topic string
	}{
		{"archived does not stop a host", "wsm.runtime.workspace.archived"},
		{"deleted does not stop or purge a host", "wsm.runtime.workspace.deleted"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lc := &fakeLifecycle{}
			var errs bytes.Buffer
			b := newTestBus(t)
			unsub := Subscribe(b, managedAuto(), HookOptions{
				Lifecycle: lc,
				Profiles: func(string) (irc.Profile, error) {
					return irc.Profile{SharedWith: []string{"guest-a", "guest-b"}}, nil
				},
				Errs: &errs,
			})
			defer unsub()

			publish(t, b, tc.topic, "host1")

			if got := lc.seen(); len(got) != 0 {
				t.Fatalf("host guard must block teardown, got calls %v", got)
			}
			log := errs.String()
			if !strings.Contains(log, "--force") {
				t.Errorf("refusal must name the --force hint, got %q", log)
			}
			if !strings.Contains(log, "guest-a") {
				t.Errorf("refusal must name the hosted guests, got %q", log)
			}
		})
	}
}

// TestHostGuardAllowsNonHost is the other half of the mutation pair: a
// workspace with no guests is torn down normally, so a guard that always
// refused would fail here.
func TestHostGuardAllowsNonHost(t *testing.T) {
	lc := &fakeLifecycle{}
	b := newTestBus(t)
	unsub := Subscribe(b, managedAuto(), HookOptions{
		Lifecycle: lc,
		Profiles: func(string) (irc.Profile, error) {
			return irc.Profile{SharedWith: nil}, nil
		},
	})
	defer unsub()

	publish(t, b, "wsm.runtime.workspace.deleted", "ws1")

	got := lc.seen()
	want := []string{"stop:ws1", "purge:ws1"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

// TestHostGuardFailsClosed covers the unreadable-profile and missing-loader
// paths: the hook cannot tell whether guests exist, so it declines.
func TestHostGuardFailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		opts  func(lc *fakeLifecycle, errs *bytes.Buffer) HookOptions
		wants string
	}{
		{
			name: "profile unreadable",
			opts: func(lc *fakeLifecycle, errs *bytes.Buffer) HookOptions {
				return HookOptions{
					Lifecycle: lc,
					Profiles: func(string) (irc.Profile, error) {
						return irc.Profile{}, errors.New("profile.yaml is corrupt")
					},
					Errs: errs,
				}
			},
			wants: "profile.yaml is corrupt",
		},
		{
			name: "no profile loader",
			opts: func(lc *fakeLifecycle, errs *bytes.Buffer) HookOptions {
				return HookOptions{Lifecycle: lc, Profiles: nil, Errs: errs}
			},
			wants: "no profile loader",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lc := &fakeLifecycle{}
			var errs bytes.Buffer
			b := newTestBus(t)
			unsub := Subscribe(b, managedAuto(), tc.opts(lc, &errs))
			defer unsub()

			publish(t, b, "wsm.runtime.workspace.archived", "ws1")

			if got := lc.seen(); len(got) != 0 {
				t.Fatalf("unverifiable host must not be torn down, got %v", got)
			}
			if log := errs.String(); !strings.Contains(log, tc.wants) {
				t.Errorf("refusal must explain %q, got %q", tc.wants, log)
			}
		})
	}
}

// TestFailureLogsRetryCommand pins the stderr contract: Manager ignores
// Publish errors, so a failed hook is invisible unless the hook says so.
func TestFailureLogsRetryCommand(t *testing.T) {
	lc := &fakeLifecycle{fail: map[string]error{"enable": errors.New("ergo binary not found")}}
	var errs bytes.Buffer
	b := newTestBus(t)
	unsub := Subscribe(b, managedAuto(), HookOptions{
		Lifecycle: lc,
		Profiles:  noGuests(irc.Profile{}),
		Errs:      &errs,
	})
	defer unsub()

	publish(t, b, "wsm.runtime.workspace.created", "ws1")

	log := errs.String()
	if !strings.Contains(log, "ergo binary not found") {
		t.Errorf("failure must report the cause, got %q", log)
	}
	if !strings.Contains(log, "wsm irc enable ws1") {
		t.Errorf("failure must name the retry command, got %q", log)
	}
}

// TestEventWithoutWorkspaceIDIsIgnored guards against acting on a global
// event that carries no workspace scope.
func TestEventWithoutWorkspaceIDIsIgnored(t *testing.T) {
	lc := &fakeLifecycle{}
	b := newTestBus(t)
	unsub := Subscribe(b, managedAuto(), HookOptions{
		Lifecycle: lc,
		Profiles:  noGuests(irc.Profile{}),
	})
	defer unsub()

	ev := bus.NewEvent(bus.Topic("wsm.runtime.workspace.created"), "wsm", nil)
	if err := b.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got := lc.seen(); len(got) != 0 {
		t.Fatalf("unscoped event must be ignored, got %v", got)
	}
}

// TestSubscribeWithoutLifecycleIsSafe covers the nil-collaborator guard.
func TestSubscribeWithoutLifecycleIsSafe(t *testing.T) {
	b := newTestBus(t)
	unsub := Subscribe(b, managedAuto(), HookOptions{Lifecycle: nil})
	defer unsub()

	publish(t, b, "wsm.runtime.workspace.created", "ws1")
}

// TestUnsubscribeStopsHandling pins that the caller can tear the hook down.
func TestUnsubscribeStopsHandling(t *testing.T) {
	lc := &fakeLifecycle{}
	b := newTestBus(t)
	unsub := Subscribe(b, managedAuto(), HookOptions{
		Lifecycle: lc,
		Profiles:  noGuests(irc.Profile{}),
	})
	unsub()

	publish(t, b, "wsm.runtime.workspace.created", "ws1")

	if got := lc.seen(); len(got) != 0 {
		t.Fatalf("unsubscribed hook must not fire, got %v", got)
	}
}
