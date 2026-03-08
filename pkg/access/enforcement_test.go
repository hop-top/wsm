package access

import (
	"errors"
	"testing"
)

func TestEnforce_AllPass(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "a", Level: LevelRequired, Check: func() error { return nil }},
		{Name: "b", Level: LevelRecommended, Check: func() error { return nil }},
		{Name: "c", Level: LevelOptional, Check: func() error { return nil }},
	}
	results, err := Enforce(prereqs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if !r.Passed {
			t.Errorf("prerequisite %q should have passed", r.Prerequisite.Name)
		}
	}
}

func TestEnforce_RequiredFails(t *testing.T) {
	errAuth := errors.New("auth failed")
	prereqs := []Prerequisite{
		{Name: "auth", Level: LevelRequired, Check: func() error { return errAuth }},
		{Name: "opt", Level: LevelOptional, Check: func() error { return nil }},
	}
	results, err := Enforce(prereqs)
	if err == nil {
		t.Fatal("expected error for required failure")
	}
	if !errors.Is(err, errAuth) {
		t.Errorf("expected auth error, got: %v", err)
	}
	if results[0].Passed {
		t.Error("auth prerequisite should not have passed")
	}
	if !results[1].Passed {
		t.Error("opt prerequisite should have passed")
	}
}

func TestEnforce_RecommendedFails_NoError(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "lint", Level: LevelRecommended, Check: func() error {
			return errors.New("lint warning")
		}},
	}
	results, err := Enforce(prereqs)
	if err != nil {
		t.Fatalf("recommended failure should not return error, got: %v", err)
	}
	if results[0].Passed {
		t.Error("lint should not have passed")
	}
	if results[0].Error == nil {
		t.Error("expected error on failed result")
	}
}

func TestEnforce_OptionalFails_NoError(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "nice", Level: LevelOptional, Check: func() error {
			return errors.New("optional thing")
		}},
	}
	_, err := Enforce(prereqs)
	if err != nil {
		t.Fatalf("optional failure should not return error, got: %v", err)
	}
}

func TestEnforce_MultipleRequiredFails(t *testing.T) {
	e1 := errors.New("fail1")
	e2 := errors.New("fail2")
	prereqs := []Prerequisite{
		{Name: "r1", Level: LevelRequired, Check: func() error { return e1 }},
		{Name: "r2", Level: LevelRequired, Check: func() error { return e2 }},
	}
	_, err := Enforce(prereqs)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Errorf("expected both errors, got: %v", err)
	}
}

func TestEnforce_Empty(t *testing.T) {
	results, err := Enforce(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Error("expected empty results")
	}
}
