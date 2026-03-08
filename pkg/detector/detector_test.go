package detector

import (
	"context"
	"errors"
	"testing"
)

// stubDetector is a test helper that returns a fixed result.
type stubDetector struct {
	name   string
	result *Result
	err    error
}

func (s *stubDetector) Name() string { return s.name }

func (s *stubDetector) Detect(_ context.Context, _ string) (*Result, error) {
	return s.result, s.err
}

func TestRegistryRegisterAndDetectors(t *testing.T) {
	reg := NewRegistry()
	if len(reg.Detectors()) != 0 {
		t.Fatal("expected empty registry")
	}

	reg.Register(&stubDetector{name: "a"})
	reg.Register(&stubDetector{name: "b"})

	dets := reg.Detectors()
	if len(dets) != 2 {
		t.Fatalf("expected 2 detectors, got %d", len(dets))
	}
	if dets[0].Name() != "a" || dets[1].Name() != "b" {
		t.Fatalf("unexpected detector names: %s, %s", dets[0].Name(), dets[1].Name())
	}
}

func TestRunAllSuccess(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubDetector{
		name: "found",
		result: &Result{
			Detected:   true,
			Kind:       "test",
			Confidence: 1.0,
		},
	})
	reg.Register(&stubDetector{
		name: "notfound",
		result: &Result{
			Detected:   false,
			Kind:       "test",
			Confidence: 0.0,
		},
	})

	results, err := reg.RunAll(context.Background(), "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].DetectorName != "found" {
		t.Errorf("expected DetectorName=found, got %s", results[0].DetectorName)
	}
	if !results[0].Detected {
		t.Error("expected first result to be detected")
	}
	if results[1].Detected {
		t.Error("expected second result to not be detected")
	}
}

func TestRunAllWithError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubDetector{
		name: "ok",
		result: &Result{
			Detected:   true,
			Kind:       "test",
			Confidence: 1.0,
		},
	})
	reg.Register(&stubDetector{
		name: "broken",
		err:  errors.New("boom"),
	})

	results, err := reg.RunAll(context.Background(), "/tmp")
	if err == nil {
		t.Fatal("expected error from broken detector")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (from ok detector), got %d", len(results))
	}
}

func TestRunAllCancelledContext(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubDetector{
		name:   "slow",
		result: &Result{Detected: true, Kind: "test"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := reg.RunAll(ctx, "/tmp")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRunAllNilResult(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubDetector{
		name:   "nil-result",
		result: nil,
	})

	results, err := reg.RunAll(context.Background(), "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for nil result, got %d", len(results))
	}
}
