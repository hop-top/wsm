package scope

import (
	"fmt"
	"testing"

	"hop.top/wsm/pkg/model"
)

// fakeStore satisfies SecretStore for testing.
type fakeStore struct {
	secrets map[string]string
}

func (f *fakeStore) Get(key string) (string, error) {
	v, ok := f.secrets[key]
	if !ok {
		return "", fmt.Errorf("not found: %s", key)
	}
	return v, nil
}

func (f *fakeStore) List() ([]string, error) {
	keys := make([]string, 0, len(f.secrets))
	for k := range f.secrets {
		keys = append(keys, k)
	}
	return keys, nil
}

func newFakeStore() *fakeStore {
	return &fakeStore{secrets: map[string]string{
		"DB_PASSWORD": "secret1",
		"API_KEY":     "secret2",
		"ADMIN_TOKEN": "secret3",
	}}
}

func TestScopedSecretStore_AllowAll(t *testing.T) {
	member := model.Member{
		ProfileID:    "sami",
		Capacity:     model.CapacityPrincipal,
		SecretsScope: "full",
	}
	policies := []SecretPolicy{
		{Scope: "full", AllowAll: true},
	}
	store := NewScopedSecretStore(newFakeStore(), member, policies)

	v, err := store.Get("DB_PASSWORD")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "secret1" {
		t.Errorf("expected secret1, got %q", v)
	}

	keys, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestScopedSecretStore_AllowList(t *testing.T) {
	member := model.Member{
		ProfileID:    "noor",
		Capacity:     model.CapacityPrincipal,
		SecretsScope: "dev",
	}
	policies := []SecretPolicy{
		{Scope: "dev", AllowKeys: []string{"DB_PASSWORD", "API_KEY"}},
	}
	store := NewScopedSecretStore(newFakeStore(), member, policies)

	// Allowed.
	v, err := store.Get("DB_PASSWORD")
	if err != nil {
		t.Fatalf("Get allowed: %v", err)
	}
	if v != "secret1" {
		t.Errorf("expected secret1, got %q", v)
	}

	// Denied (not in allow list).
	_, err = store.Get("ADMIN_TOKEN")
	if err == nil {
		t.Fatal("expected denial for ADMIN_TOKEN")
	}

	// List filters.
	keys, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestScopedSecretStore_DenyList(t *testing.T) {
	member := model.Member{
		ProfileID:    "kai",
		Capacity:     model.CapacityPrincipal,
		SecretsScope: "restricted",
	}
	policies := []SecretPolicy{
		{Scope: "restricted", AllowAll: true, DenyKeys: []string{"ADMIN_TOKEN"}},
	}
	store := NewScopedSecretStore(newFakeStore(), member, policies)

	// Allowed (not denied).
	if _, err := store.Get("DB_PASSWORD"); err != nil {
		t.Fatalf("Get allowed: %v", err)
	}

	// Denied explicitly.
	_, err := store.Get("ADMIN_TOKEN")
	if err == nil {
		t.Fatal("expected denial for ADMIN_TOKEN")
	}

	keys, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys (ADMIN_TOKEN denied), got %d", len(keys))
	}
}

func TestScopedSecretStore_NoScope(t *testing.T) {
	member := model.Member{
		ProfileID:    "guest",
		Capacity:     model.CapacityPrincipal,
		SecretsScope: "", // no scope
	}
	store := NewScopedSecretStore(newFakeStore(), member, nil)

	_, err := store.Get("DB_PASSWORD")
	if err == nil {
		t.Fatal("expected denial when no scope set")
	}

	keys, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestScopedSecretStore_UnknownScope(t *testing.T) {
	member := model.Member{
		ProfileID:    "unknown",
		Capacity:     model.CapacityPrincipal,
		SecretsScope: "nonexistent",
	}
	store := NewScopedSecretStore(newFakeStore(), member, []SecretPolicy{
		{Scope: "dev", AllowAll: true},
	})

	_, err := store.Get("DB_PASSWORD")
	if err == nil {
		t.Fatal("expected denial for unknown scope")
	}
}

func TestScopedSecretStore_Delegate(t *testing.T) {
	member := model.Member{
		ProfileID:    "noor",
		Capacity:     model.CapacityDelegate,
		DelegateFor:  "sami",
		SecretsScope: "dev",
	}
	policies := []SecretPolicy{
		{Scope: "dev", AllowKeys: []string{"API_KEY"}},
	}
	store := NewScopedSecretStore(newFakeStore(), member, policies)

	// Delegate uses their own secrets_scope, not principal's.
	if _, err := store.Get("API_KEY"); err != nil {
		t.Fatalf("delegate should access allowed key: %v", err)
	}
	if _, err := store.Get("ADMIN_TOKEN"); err == nil {
		t.Fatal("delegate should not access keys outside scope")
	}
}
