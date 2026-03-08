// Package detector provides tool integration detection for workspace spaces.
package detector

import "context"

// Detector identifies tool integrations within a space directory.
type Detector interface {
	Name() string
	Detect(ctx context.Context, spaceDir string) (*Result, error)
}

// Result holds the outcome of a detection run.
type Result struct {
	Detected   bool              `json:"detected"`
	Kind       string            `json:"kind"`
	Details    map[string]string `json:"details,omitempty"`
	Confidence float64           `json:"confidence"`
}
