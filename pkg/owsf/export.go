package owsf

import "errors"

type ExportKind string

const (
	ExportFull    ExportKind = "full"
	ExportPartial ExportKind = "partial"
)

type DeterminismLevel string

const (
	DeterminismFull       DeterminismLevel = "full"
	DeterminismBestEffort DeterminismLevel = "best_effort"
	DeterminismNone       DeterminismLevel = "none"
)

type SliceScope struct {
	SpaceIDs []string `json:"space_ids"`
}

type MissingDependency struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

type Export struct {
	Kind           ExportKind          `json:"kind"`
	SliceScope     *SliceScope         `json:"slice_scope,omitempty"`
	Determinism    DeterminismLevel    `json:"determinism"`
	MaxConformance int                 `json:"max_conformance"`
	MissingDeps    []MissingDependency `json:"missing_deps,omitempty"`
}

func NewExport(kind ExportKind) Export {
	return Export{Kind: kind}
}

// Validate checks export constraints.
// I2 fix: both full and partial exports must declare determinism + max_conformance.
func (e Export) Validate() error {
	if e.Determinism == "" {
		return errors.New("export requires determinism declaration")
	}
	if e.MaxConformance < 1 || e.MaxConformance > 3 {
		return errors.New("max_conformance must be 1-3")
	}
	if e.Kind == ExportPartial {
		if e.SliceScope == nil {
			return errors.New("partial export requires scope")
		}
	}
	return nil
}
