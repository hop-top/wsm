package detector

import (
	"testing"
)

func TestGenerateStrategyNoDetections(t *testing.T) {
	results := []Result{
		{DetectorName: "a", Detected: false, Kind: "build"},
		{DetectorName: "b", Detected: false, Kind: "runtime"},
	}

	s := GenerateStrategy(results)
	if s.Name != "default" {
		t.Errorf("expected name=default, got %s", s.Name)
	}
	if len(s.Detectors) != 0 {
		t.Errorf("expected no detectors, got %d", len(s.Detectors))
	}
	if s.Recommendations["note"] == "" {
		t.Error("expected default note recommendation")
	}
}

func TestGenerateStrategyWithDetections(t *testing.T) {
	results := []Result{
		{
			DetectorName: "makefile",
			Detected:     true,
			Kind:         "build",
			Details:      map[string]string{"file": "Makefile"},
			Confidence:   1.0,
		},
		{
			DetectorName: "docker-compose",
			Detected:     true,
			Kind:         "runtime",
			Details:      map[string]string{"file": "docker-compose.yml"},
			Confidence:   1.0,
		},
		{
			DetectorName: "not-found",
			Detected:     false,
			Kind:         "build",
		},
	}

	s := GenerateStrategy(results)
	if s.Name != "auto" {
		t.Errorf("expected name=auto, got %s", s.Name)
	}
	if len(s.Detectors) != 2 {
		t.Fatalf("expected 2 detectors, got %d", len(s.Detectors))
	}
	if s.Recommendations["build_tool"] != "Makefile" {
		t.Errorf("expected build_tool=Makefile, got %s",
			s.Recommendations["build_tool"])
	}
	if s.Recommendations["runtime_docker-compose"] != "docker-compose.yml" {
		t.Errorf("expected runtime recommendation, got %s",
			s.Recommendations["runtime_docker-compose"])
	}
}

func TestGenerateStrategyHealth(t *testing.T) {
	results := []Result{
		{
			DetectorName: "dependency-health",
			Detected:     true,
			Kind:         "health",
			Details: map[string]string{
				"total_deps":  "5",
				"total_files": "2",
			},
			Confidence: 1.0,
		},
	}

	s := GenerateStrategy(results)
	if s.Recommendations["dependency_count"] != "5" {
		t.Errorf("expected dependency_count=5, got %s",
			s.Recommendations["dependency_count"])
	}
	if s.Recommendations["dependency_files"] != "2" {
		t.Errorf("expected dependency_files=2, got %s",
			s.Recommendations["dependency_files"])
	}
}

func TestGenerateStrategyKnowledge(t *testing.T) {
	results := []Result{
		{
			DetectorName: "local-docs",
			Detected:     true,
			Kind:         "knowledge",
			Details: map[string]string{
				"file_readme": "README.md",
				"dir_docs":    "true",
			},
			Confidence: 0.5,
		},
	}

	s := GenerateStrategy(results)
	if s.Recommendations["knowledge_file_readme"] != "README.md" {
		t.Errorf("expected knowledge recommendation for readme")
	}
	if s.Recommendations["knowledge_dir_docs"] != "true" {
		t.Errorf("expected knowledge recommendation for docs dir")
	}
}

func TestGenerateStrategyEmpty(t *testing.T) {
	s := GenerateStrategy(nil)
	if s.Name != "default" {
		t.Errorf("expected name=default for nil input, got %s", s.Name)
	}
}
