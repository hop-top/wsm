package detector

import (
	"testing"
)

func TestApplyOverridesNil(t *testing.T) {
	// Should not panic on nil strategy.
	ApplyOverrides(nil, []Override{{
		DisabledDetectors: []string{"x"},
	}})
}

func TestApplyOverridesDisableDetectors(t *testing.T) {
	s := &Strategy{
		Name:            "auto",
		Detectors:       []string{"makefile", "docker-compose", "env-example"},
		Recommendations: map[string]string{"build_tool": "Makefile"},
	}

	overrides := []Override{
		{
			SpaceID:           "space-1",
			DisabledDetectors: []string{"docker-compose"},
		},
	}

	ApplyOverrides(s, overrides)
	if len(s.Detectors) != 2 {
		t.Fatalf("expected 2 detectors after override, got %d", len(s.Detectors))
	}
	for _, d := range s.Detectors {
		if d == "docker-compose" {
			t.Error("docker-compose should have been removed")
		}
	}
}

func TestApplyOverridesCustomSettings(t *testing.T) {
	s := &Strategy{
		Name:      "auto",
		Detectors: []string{"makefile"},
		Recommendations: map[string]string{
			"build_tool": "Makefile",
		},
	}

	overrides := []Override{
		{
			WorkspaceID: "ws-1",
			CustomSettings: map[string]string{
				"build_tool": "just",
				"extra":      "value",
			},
		},
	}

	ApplyOverrides(s, overrides)
	if s.Recommendations["build_tool"] != "just" {
		t.Errorf("expected build_tool=just, got %s",
			s.Recommendations["build_tool"])
	}
	if s.Recommendations["extra"] != "value" {
		t.Errorf("expected extra=value, got %s",
			s.Recommendations["extra"])
	}
}

func TestApplyOverridesMultiple(t *testing.T) {
	s := &Strategy{
		Name:            "auto",
		Detectors:       []string{"a", "b", "c"},
		Recommendations: map[string]string{},
	}

	overrides := []Override{
		{DisabledDetectors: []string{"a"}},
		{
			DisabledDetectors: []string{"c"},
			CustomSettings:    map[string]string{"key": "val"},
		},
	}

	ApplyOverrides(s, overrides)
	if len(s.Detectors) != 1 || s.Detectors[0] != "b" {
		t.Errorf("expected only [b], got %v", s.Detectors)
	}
	if s.Recommendations["key"] != "val" {
		t.Error("expected custom setting applied")
	}
}

func TestApplyOverridesEmptyOverrides(t *testing.T) {
	s := &Strategy{
		Name:            "auto",
		Detectors:       []string{"a", "b"},
		Recommendations: map[string]string{"x": "y"},
	}

	ApplyOverrides(s, nil)
	if len(s.Detectors) != 2 {
		t.Error("expected no change with nil overrides")
	}
}
