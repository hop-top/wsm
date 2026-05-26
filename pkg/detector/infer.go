package detector

import (
	"context"
	"strings"

	"hop.top/wsm/pkg/model"
)

// SpaceInference holds inferred Space fields from detector results.
type SpaceInference struct {
	Type         model.SpaceType  `json:"type"`
	TrustLevel   model.TrustLevel `json:"trust_level"`
	Capabilities []string         `json:"capabilities,omitempty"`
}

// InferSpaceFromBoundary runs detectors against a boundary path and infers
// Space fields. Returns defaults if the boundary is not a local path or
// no detectors match.
func InferSpaceFromBoundary(ctx context.Context, boundary string) SpaceInference {
	inf := SpaceInference{
		Type:       model.SpaceFilesystem,
		TrustLevel: model.TrustLocal,
	}

	// Non-filesystem boundaries: infer from scheme.
	lower := strings.ToLower(boundary)
	switch {
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		inf.Type = model.SpaceAPI
		inf.TrustLevel = model.TrustExternal
		return inf
	case strings.HasPrefix(lower, "docker://") || strings.HasPrefix(lower, "container://"):
		inf.Type = model.SpaceContainer
		inf.TrustLevel = model.TrustVerified
		return inf
	case strings.HasPrefix(lower, "ssh://"):
		inf.Type = model.SpaceRemote
		inf.TrustLevel = model.TrustExternal
		return inf
	case strings.HasPrefix(lower, "db://") || strings.HasPrefix(lower, "postgres://") ||
		strings.HasPrefix(lower, "mysql://") || strings.HasPrefix(lower, "sqlite://"):
		inf.Type = model.SpaceDatabase
		inf.TrustLevel = model.TrustVerified
		return inf
	}

	// Filesystem boundary: run detectors.
	reg := NewRegistry()
	RegisterBuiltins(reg)
	results, err := reg.RunAll(ctx, boundary)
	if err != nil {
		return inf
	}

	for _, r := range results {
		if !r.Detected {
			continue
		}
		switch r.DetectorName {
		case "docker-compose", "devcontainer":
			inf.Type = model.SpaceContainer
			inf.TrustLevel = model.TrustVerified
		}
		inf.Capabilities = append(inf.Capabilities, r.DetectorName)
	}

	return inf
}
