package provider

import (
	"errors"
	"strings"
	"testing"
)

// stubFactory returns a factory yielding empty providers, for registry tests
// that only care about lookup.
func stubFactory() Factory {
	return func(Deps) (Providers, error) { return Providers{}, nil }
}

// withCleanRegistry swaps in an empty registry for the duration of a test, so
// tests neither see each other's names nor leak into the real one.
func withCleanRegistry(t *testing.T) {
	t.Helper()

	registryMu.Lock()
	saved := registry
	registry = map[string]Factory{}
	registryMu.Unlock()

	t.Cleanup(func() {
		registryMu.Lock()
		registry = saved
		registryMu.Unlock()
	})
}

func TestOpen_Registered(t *testing.T) {
	withCleanRegistry(t)
	Register("ergo", stubFactory())

	if _, err := Open("ergo", Deps{}); err != nil {
		t.Fatalf("open registered provider: %v", err)
	}
}

func TestOpen_UnknownNameListsAlternatives(t *testing.T) {
	withCleanRegistry(t)
	Register("ergo", stubFactory())
	Register("plain", stubFactory())

	_, err := Open("ircd-seven", Deps{})
	if err == nil {
		t.Fatal("expected an error for an unknown provider")
	}

	// The fix is always to correct the config, so the message must name both
	// the bad value and the valid ones.
	msg := err.Error()
	for _, want := range []string{"ircd-seven", "ergo", "plain"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}

func TestOpen_PropagatesFactoryError(t *testing.T) {
	withCleanRegistry(t)

	sentinel := errors.New("dial failed")
	Register("ergo", func(Deps) (Providers, error) { return Providers{}, sentinel })

	if _, err := Open("ergo", Deps{}); !errors.Is(err, sentinel) {
		t.Fatalf("factory error should reach the caller, got %v", err)
	}
}

func TestRegister_DuplicatePanics(t *testing.T) {
	withCleanRegistry(t)
	Register("ergo", stubFactory())

	defer func() {
		if recover() == nil {
			t.Error("registering a duplicate name should panic")
		}
	}()
	Register("ergo", stubFactory())
}

func TestRegister_NilFactoryPanics(t *testing.T) {
	withCleanRegistry(t)

	defer func() {
		if recover() == nil {
			t.Error("registering a nil factory should panic")
		}
	}()
	Register("ergo", nil)
}

func TestRegistered_SortedNames(t *testing.T) {
	withCleanRegistry(t)
	Register("plain", stubFactory())
	Register("ergo", stubFactory())

	got := Registered()
	want := []string{"ergo", "plain"}

	if len(got) != len(want) {
		t.Fatalf("Registered() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Registered() = %v, want %v", got, want)
		}
	}
}

func TestDeps_LoggerNeverNil(t *testing.T) {
	// Factories log without nil-checking, so an unset Log must still yield a
	// usable logger.
	if got := (Deps{}).Logger(); got == nil {
		t.Fatal("Logger() must not return nil")
	}
	// Exercising it must not panic.
	(Deps{}).Logger().Info("discarded")
}
