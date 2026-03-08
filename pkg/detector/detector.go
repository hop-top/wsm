package detector

import (
	"context"
	"fmt"
	"sync"
)

// Detector scans a space directory and reports what it finds.
type Detector interface {
	// Name returns a unique identifier for this detector.
	Name() string

	// Detect inspects spaceDir and returns a result.
	Detect(ctx context.Context, spaceDir string) (*Result, error)
}

// Result holds the outcome of a single detector run.
type Result struct {
	// DetectorName identifies which detector produced this result.
	DetectorName string `json:"detector_name"`

	// Detected is true when the detector found what it was looking for.
	Detected bool `json:"detected"`

	// Kind classifies the detection (e.g. "build", "runtime", "knowledge").
	Kind string `json:"kind"`

	// Details holds arbitrary key-value metadata about the detection.
	Details map[string]string `json:"details,omitempty"`

	// Confidence is a value between 0.0 and 1.0 indicating certainty.
	Confidence float64 `json:"confidence"`
}

// Registry holds registered detectors and runs them against a directory.
type Registry struct {
	mu        sync.RWMutex
	detectors []Detector
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a detector to the registry.
func (r *Registry) Register(d Detector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.detectors = append(r.detectors, d)
}

// Detectors returns a snapshot of all registered detectors.
func (r *Registry) Detectors() []Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Detector, len(r.detectors))
	copy(out, r.detectors)
	return out
}

// RunAll executes every registered detector against dir.
// It returns all results, including non-detections.
// Errors from individual detectors are wrapped and collected; RunAll
// itself only returns an error if the context is cancelled.
func (r *Registry) RunAll(ctx context.Context, dir string) ([]Result, error) {
	r.mu.RLock()
	dets := make([]Detector, len(r.detectors))
	copy(dets, r.detectors)
	r.mu.RUnlock()

	var results []Result
	var errs []error

	for _, d := range dets {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		res, err := d.Detect(ctx, dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("detector %s: %w", d.Name(), err))
			continue
		}
		if res != nil {
			res.DetectorName = d.Name()
			results = append(results, *res)
		}
	}

	if len(errs) > 0 {
		return results, fmt.Errorf("detector errors: %v", errs)
	}
	return results, nil
}
