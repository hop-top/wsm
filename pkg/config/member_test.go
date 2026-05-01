package config

import (
	"os"
	"path/filepath"
	"testing"

	"hop.top/wsm/pkg/model"
)

func TestLoadMemberConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	mc, err := LoadMemberConfig(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("expected no error for missing file: %v", err)
	}
	if len(mc.Members) != 0 {
		t.Errorf("expected empty members, got %d", len(mc.Members))
	}
}

func TestMemberConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	mc := &MemberConfig{
		Members: []model.Member{
			{
				ProfileID:    "sami",
				Facet:        "eng",
				Capacity:     model.CapacityPrincipal,
				AuditLabel:   "sami-eng",
				SecretsScope: "dev",
			},
			{
				ProfileID:   "noor",
				Facet:       "ops",
				Capacity:    model.CapacityDelegate,
				DelegateFor: "sami",
				AuditLabel:  "noor-ops",
			},
		},
	}

	if err := SaveMemberConfig(configPath, mc); err != nil {
		t.Fatalf("SaveMemberConfig: %v", err)
	}

	loaded, err := LoadMemberConfig(configPath)
	if err != nil {
		t.Fatalf("LoadMemberConfig: %v", err)
	}
	if len(loaded.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(loaded.Members))
	}

	m := loaded.Members[0]
	if m.ProfileID != "sami" || m.Facet != "eng" || m.Capacity != model.CapacityPrincipal {
		t.Errorf("member 0 mismatch: %+v", m)
	}
	if m.SecretsScope != "dev" {
		t.Errorf("expected secrets_scope=dev, got %q", m.SecretsScope)
	}

	m2 := loaded.Members[1]
	if m2.DelegateFor != "sami" {
		t.Errorf("expected delegate_for=sami, got %q", m2.DelegateFor)
	}
}

func TestMemberConfig_InvalidMember(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	// Write invalid config (delegate without delegate_for).
	content := `members:
  - profile_id: bad
    capacity: delegate
`
	if err := os.WriteFile(
		filepath.Join(dir, "members.yaml"),
		[]byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadMemberConfig(configPath)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMemberConfig_FindMember(t *testing.T) {
	mc := &MemberConfig{
		Members: []model.Member{
			{ProfileID: "a", Capacity: model.CapacityPrincipal},
			{ProfileID: "b", Capacity: model.CapacityPrincipal, Facet: "ops"},
		},
	}

	m := mc.FindMember("b")
	if m == nil || m.Facet != "ops" {
		t.Errorf("expected to find member b with facet ops")
	}
	if mc.FindMember("z") != nil {
		t.Error("expected nil for unknown member")
	}
}

func TestMemberConfig_ActiveFacet(t *testing.T) {
	mc := &MemberConfig{
		Members: []model.Member{
			{ProfileID: "x", Capacity: model.CapacityPrincipal, Facet: "dev"},
		},
	}

	if f := mc.ActiveFacet("x"); f != "dev" {
		t.Errorf("expected dev, got %q", f)
	}
	if f := mc.ActiveFacet("unknown"); f != "" {
		t.Errorf("expected empty for unknown, got %q", f)
	}
}

func TestMemberConfig_ActiveCapacity(t *testing.T) {
	mc := &MemberConfig{
		Members: []model.Member{
			{ProfileID: "x", Capacity: model.CapacityDelegate, DelegateFor: "y"},
		},
	}

	if c := mc.ActiveCapacity("x"); c != model.CapacityDelegate {
		t.Errorf("expected delegate, got %q", c)
	}
	// Unknown defaults to principal.
	if c := mc.ActiveCapacity("unknown"); c != model.CapacityPrincipal {
		t.Errorf("expected principal default, got %q", c)
	}
}
