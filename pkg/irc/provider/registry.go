package provider

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

// APIConfig addresses a provider's HTTP API. Nil in Deps when the backend has
// none, which is the normal case for a plain server.
type APIConfig struct {
	// BaseURL is the API root, e.g. http://127.0.0.1:8089.
	BaseURL string
	// Token is the bearer token.
	Token string
}

// Deps is what a provider factory needs to build itself.
type Deps struct {
	// Transport is the live connection. Always set.
	Transport Transport
	// API addresses an HTTP API, or is nil when the backend has none.
	API *APIConfig
	// Log receives provider diagnostics. Nil means discard; use Logger to
	// read it rather than dereferencing the field.
	Log *slog.Logger
}

// Logger returns Deps.Log, or a logger that discards everything when unset,
// so factories never nil-check before logging.
func (d Deps) Logger() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.New(slog.DiscardHandler)
}

// Factory builds a provider trio from its dependencies.
type Factory func(Deps) (Providers, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register adds a provider factory under name. It panics on a duplicate name
// or a nil factory: both are programming errors in package init, not runtime
// conditions a caller could handle.
func Register(name string, factory Factory) {
	if factory == nil {
		panic("irc/provider: Register called with nil factory for " + name)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, dup := registry[name]; dup {
		panic("irc/provider: duplicate Register for provider " + name)
	}
	registry[name] = factory
}

// Open builds the provider named name. An unknown name is a config error and
// lists what is available, because the fix is always to correct the config.
func Open(name string, deps Deps) (Providers, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()

	if !ok {
		return Providers{}, fmt.Errorf(
			"irc: unknown provider %q: must be one of %v", name, Registered(),
		)
	}
	return factory(deps)
}

// Registered lists the registered provider names in sorted order.
func Registered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
