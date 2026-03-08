package access

import (
	"errors"
	"testing"
)

func TestRunVerification_NoVerify(t *testing.T) {
	called := false
	prereqs := []Prerequisite{
		{Name: "boom", Level: LevelRequired, Check: func() error {
			called = true
			return errors.New("should not run")
		}},
	}
	results, err := RunVerification(prereqs, VerifyOptions{NoVerify: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("check should not have been called with NoVerify")
	}
	if !results[0].Passed {
		t.Error("result should be passed when NoVerify is true")
	}
}

func TestRunVerification_SkipOptional(t *testing.T) {
	optCalled := false
	prereqs := []Prerequisite{
		{Name: "req", Level: LevelRequired, Check: func() error { return nil }},
		{Name: "opt", Level: LevelOptional, Check: func() error {
			optCalled = true
			return errors.New("should skip")
		}},
		{Name: "rec", Level: LevelRecommended, Check: func() error { return nil }},
	}
	results, err := RunVerification(prereqs, VerifyOptions{SkipOptional: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if optCalled {
		t.Error("optional check should not have been called")
	}

	// All should be marked passed.
	for _, r := range results {
		if !r.Passed {
			t.Errorf("prerequisite %q should have passed", r.Prerequisite.Name)
		}
	}
}

func TestRunVerification_SkipOptional_RequiredFails(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "req", Level: LevelRequired, Check: func() error {
			return errors.New("required fail")
		}},
		{Name: "opt", Level: LevelOptional, Check: func() error {
			return errors.New("should skip")
		}},
	}
	_, err := RunVerification(prereqs, VerifyOptions{SkipOptional: true})
	if err == nil {
		t.Fatal("expected error for required failure")
	}
}

func TestRunVerification_Default(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "a", Level: LevelRequired, Check: func() error { return nil }},
		{Name: "b", Level: LevelOptional, Check: func() error {
			return errors.New("opt fail")
		}},
	}
	results, err := RunVerification(prereqs, VerifyOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !results[0].Passed {
		t.Error("required check should pass")
	}
	if results[1].Passed {
		t.Error("optional check should fail")
	}
}

func TestRunVerification_Empty(t *testing.T) {
	results, err := RunVerification(nil, VerifyOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Error("expected empty results")
	}
}
