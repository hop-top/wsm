package cli

import (
	"context"
	"fmt"
	"time"
)

// Credential describes an active credential with its lifecycle.
type Credential struct {
	Source    string     `json:"source"`               // "env", "config", "keyring", "exchange"
	Identity  string     `json:"identity"`             // who am I acting as
	Scopes    []string   `json:"scopes"`               // what am I allowed to do
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // when does this expire
	Renewable bool       `json:"renewable"`            // can it be refreshed
}

// AuthIntrospector provides "who am I, what can I do" answers.
type AuthIntrospector interface {
	Inspect(ctx context.Context) (*Credential, error)
	Refresh(ctx context.Context) error
}

// NoAuth is the default — no auth configured.
type NoAuth struct{}

// Inspect returns an anonymous credential.
func (n NoAuth) Inspect(_ context.Context) (*Credential, error) {
	return &Credential{Source: "none", Identity: "anonymous"}, nil
}

// Refresh returns an error; nothing to refresh.
func (n NoAuth) Refresh(_ context.Context) error {
	return fmt.Errorf("no auth configured")
}
