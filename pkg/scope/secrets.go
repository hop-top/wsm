package scope

import (
	"fmt"

	"hop.top/wsm/pkg/model"
)

// SecretStore is the interface wsm expects from an aps-compatible
// secret backend. Implementations live in aps (age, env, sops, etc.)
// and are injected at runtime.
type SecretStore interface {
	Get(key string) (string, error)
	List() ([]string, error)
}

// ScopedSecretStore wraps a SecretStore and filters access based on
// a workspace member's secrets_scope and capacity.
type ScopedSecretStore struct {
	inner    SecretStore
	member   model.Member
	policies []SecretPolicy
}

// SecretPolicy defines which secret keys a scope grants access to.
type SecretPolicy struct {
	Scope     string   `json:"scope" yaml:"scope"`
	AllowKeys []string `json:"allow_keys,omitempty" yaml:"allow_keys,omitempty"`
	DenyKeys  []string `json:"deny_keys,omitempty" yaml:"deny_keys,omitempty"`
	AllowAll  bool     `json:"allow_all,omitempty" yaml:"allow_all,omitempty"`
}

// NewScopedSecretStore wraps a SecretStore with member-aware access control.
func NewScopedSecretStore(
	inner SecretStore,
	member model.Member,
	policies []SecretPolicy,
) *ScopedSecretStore {
	return &ScopedSecretStore{
		inner:    inner,
		member:   member,
		policies: policies,
	}
}

// Get retrieves a secret, enforcing scope policy.
func (s *ScopedSecretStore) Get(key string) (string, error) {
	if !s.isAllowed(key) {
		return "", fmt.Errorf(
			"secret %q denied for member %q (scope=%q, capacity=%s)",
			key, s.member.ProfileID, s.member.SecretsScope, s.member.Capacity,
		)
	}
	return s.inner.Get(key)
}

// List returns only the keys the member is allowed to see.
func (s *ScopedSecretStore) List() ([]string, error) {
	all, err := s.inner.List()
	if err != nil {
		return nil, err
	}
	var filtered []string
	for _, k := range all {
		if s.isAllowed(k) {
			filtered = append(filtered, k)
		}
	}
	return filtered, nil
}

// isAllowed checks whether the member's scope + capacity permits
// access to the given key.
func (s *ScopedSecretStore) isAllowed(key string) bool {
	// No scope = no secret access.
	if s.member.SecretsScope == "" {
		return false
	}

	// Delegates inherit scope from their principal's policy,
	// but never gain broader access than the scope string grants.
	policy := s.findPolicy(s.member.SecretsScope)
	if policy == nil {
		return false
	}

	if policy.AllowAll {
		return !s.isDenied(policy, key)
	}

	return s.isInAllowList(policy, key) && !s.isDenied(policy, key)
}

func (s *ScopedSecretStore) findPolicy(scope string) *SecretPolicy {
	for i := range s.policies {
		if s.policies[i].Scope == scope {
			return &s.policies[i]
		}
	}
	return nil
}

func (s *ScopedSecretStore) isInAllowList(p *SecretPolicy, key string) bool {
	for _, k := range p.AllowKeys {
		if k == key {
			return true
		}
	}
	return false
}

func (s *ScopedSecretStore) isDenied(p *SecretPolicy, key string) bool {
	for _, k := range p.DenyKeys {
		if k == key {
			return true
		}
	}
	return false
}
