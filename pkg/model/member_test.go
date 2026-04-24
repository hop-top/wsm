package model

import (
	"testing"
)

func TestMember_Validate_Principal(t *testing.T) {
	m := Member{
		ProfileID: "sami",
		Capacity:  CapacityPrincipal,
		Facet:     "ops",
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid principal should pass: %v", err)
	}
}

func TestMember_Validate_Delegate(t *testing.T) {
	m := Member{
		ProfileID:   "noor",
		Capacity:    CapacityDelegate,
		DelegateFor: "sami",
		Facet:       "ops",
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid delegate should pass: %v", err)
	}
}

func TestMember_Validate_MissingProfileID(t *testing.T) {
	m := Member{Capacity: CapacityPrincipal}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing profile_id")
	}
}

func TestMember_Validate_InvalidCapacity(t *testing.T) {
	m := Member{ProfileID: "x", Capacity: "observer"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for invalid capacity")
	}
}

func TestMember_Validate_DelegateMissingDelegateFor(t *testing.T) {
	m := Member{ProfileID: "x", Capacity: CapacityDelegate}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error: delegate without delegate_for")
	}
}

func TestMember_Validate_PrincipalWithDelegateFor(t *testing.T) {
	m := Member{
		ProfileID:   "x",
		Capacity:    CapacityPrincipal,
		DelegateFor: "someone",
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error: principal with delegate_for")
	}
}

func TestMember_FacetOptional(t *testing.T) {
	m := Member{ProfileID: "x", Capacity: CapacityPrincipal}
	if err := m.Validate(); err != nil {
		t.Fatalf("facet should be optional: %v", err)
	}
	if m.Facet != "" {
		t.Error("facet should default to empty")
	}
}

func TestValidateCapacity(t *testing.T) {
	for _, c := range []Capacity{CapacityPrincipal, CapacityDelegate} {
		if err := ValidateCapacity(c); err != nil {
			t.Errorf("expected %q valid: %v", c, err)
		}
	}
	if err := ValidateCapacity("admin"); err == nil {
		t.Error("expected error for invalid capacity")
	}
}
