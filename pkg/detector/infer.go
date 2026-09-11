package detector

import (
	"context"
	"strings"

	"hop.top/wsm/pkg/model"
)

// InferenceSource names how a single inferred field acquired its value.
// The three states are deliberately distinct: an agent chaining wsm
// output must be able to tell a positive determination from a fallback
// that fired because nothing matched.
type InferenceSource string

const (
	// SourceBoundaryScheme means the value was determined from the URI
	// scheme of the boundary (http://, docker://, ssh://, postgres://…).
	SourceBoundaryScheme InferenceSource = "boundary-scheme"

	// SourceDetector means the value was determined by a filesystem
	// detector that positively matched a file on disk.
	SourceDetector InferenceSource = "detector"

	// SourceDefault means nothing matched and the zero-knowledge
	// fallback was applied. A field carrying this source is a guess,
	// not a finding.
	SourceDefault InferenceSource = "default"
)

// FieldInference discloses the provenance of one inferred field. It
// follows the `_inferred` shape of 12-factor factor 11: what produced
// the value, which resource was consulted, and why this value rather
// than another.
//
// Path is populated only when a concrete resource was consulted (the
// file a detector matched). It stays empty for scheme- and
// default-sourced fields, which consult no path — an empty string is
// omitted rather than fabricated.
type FieldInference struct {
	Source InferenceSource `json:"source"`
	Path   string          `json:"path,omitempty"`
	Reason string          `json:"reason"`
}

// Determined reports whether the field was positively determined by
// evidence, as opposed to defaulted because nothing matched.
func (f FieldInference) Determined() bool {
	return f.Source != SourceDefault
}

// SpaceInference holds inferred Space fields from detector results,
// together with the per-field provenance of how each was obtained.
//
// Inferred is keyed by the JSON field name it describes ("type",
// "trust_level", "capabilities"). Only fields the inference actually
// produced appear; wsm does not emit provenance entries it has no
// real source for.
type SpaceInference struct {
	Type         model.SpaceType           `json:"type"`
	TrustLevel   model.TrustLevel          `json:"trust_level"`
	Capabilities []string                  `json:"capabilities,omitempty"`
	Inferred     map[string]FieldInference `json:"_inferred,omitempty"`
}

// Determined reports whether every disclosed field was positively
// determined. False when any field fell back to a default.
func (s SpaceInference) Determined() bool {
	for _, f := range s.Inferred {
		if !f.Determined() {
			return false
		}
	}
	return len(s.Inferred) > 0
}

// schemeRule maps a set of boundary URI prefixes to the Space fields
// they determine.
type schemeRule struct {
	prefixes []string
	typ      model.SpaceType
	trust    model.TrustLevel
}

// schemeRules is the boundary-scheme inference table. Order matters
// only in that the first matching rule wins; the prefix sets are
// disjoint.
var schemeRules = []schemeRule{
	{[]string{"http://", "https://"}, model.SpaceAPI, model.TrustExternal},
	{[]string{"docker://", "container://"}, model.SpaceContainer, model.TrustVerified},
	{[]string{"ssh://"}, model.SpaceRemote, model.TrustExternal},
	{
		[]string{"db://", "postgres://", "mysql://", "sqlite://"},
		model.SpaceDatabase, model.TrustVerified,
	},
}

// containerDetectors are the filesystem detectors whose presence
// upgrades a space from filesystem/local to container/verified.
var containerDetectors = map[string]bool{
	"docker-compose": true,
	"devcontainer":   true,
}

// InferSpaceFromBoundary runs detectors against a boundary path and infers
// Space fields. Returns defaults if the boundary is not a local path or
// no detectors match.
//
// Every returned field carries provenance in Inferred: whether it was
// determined from the boundary scheme, determined by a detector that
// matched a file on disk, or defaulted because nothing matched.
func InferSpaceFromBoundary(ctx context.Context, boundary string) SpaceInference {
	inf := SpaceInference{
		Type:       model.SpaceFilesystem,
		TrustLevel: model.TrustLocal,
		Inferred:   map[string]FieldInference{},
	}

	// Non-filesystem boundaries: infer from scheme.
	lower := strings.ToLower(boundary)
	for _, rule := range schemeRules {
		for _, p := range rule.prefixes {
			if !strings.HasPrefix(lower, p) {
				continue
			}
			inf.Type = rule.typ
			inf.TrustLevel = rule.trust
			reason := "boundary scheme " + p + " matched"
			inf.Inferred["type"] = FieldInference{
				Source: SourceBoundaryScheme,
				Reason: reason,
			}
			inf.Inferred["trust_level"] = FieldInference{
				Source: SourceBoundaryScheme,
				Reason: reason,
			}
			return inf
		}
	}

	// Filesystem boundary: run detectors.
	reg := NewRegistry()
	RegisterBuiltins(reg)
	results, err := reg.RunAll(ctx, boundary)
	if err != nil {
		inf.Inferred["type"] = defaultedField(
			"detectors failed to run; no scheme matched")
		inf.Inferred["trust_level"] = defaultedField(
			"detectors failed to run; no scheme matched")
		return inf
	}

	// containerEvidence records the detector and path that upgraded the
	// space to container, so the disclosure can name what decided it.
	var containerDetector, containerPath string
	var capPaths []string

	for _, r := range results {
		if !r.Detected {
			continue
		}
		if containerDetectors[r.DetectorName] && containerDetector == "" {
			inf.Type = model.SpaceContainer
			inf.TrustLevel = model.TrustVerified
			containerDetector = r.DetectorName
			containerPath = r.Details["path"]
		}
		inf.Capabilities = append(inf.Capabilities, r.DetectorName)
		if p := r.Details["path"]; p != "" {
			capPaths = append(capPaths, p)
		}
	}

	if containerDetector != "" {
		f := FieldInference{
			Source: SourceDetector,
			Path:   containerPath,
			Reason: "detector " + containerDetector + " matched",
		}
		inf.Inferred["type"] = f
		inf.Inferred["trust_level"] = f
	} else {
		reason := "no boundary scheme matched and no container detector matched"
		inf.Inferred["type"] = defaultedField(reason)
		inf.Inferred["trust_level"] = defaultedField(reason)
	}

	if len(inf.Capabilities) > 0 {
		inf.Inferred["capabilities"] = FieldInference{
			Source: SourceDetector,
			Path:   strings.Join(capPaths, ", "),
			Reason: "detectors matched: " + strings.Join(inf.Capabilities, ", "),
		}
	}

	return inf
}

// defaultedField builds the provenance entry for a field that fell back
// to its zero-knowledge default.
func defaultedField(reason string) FieldInference {
	return FieldInference{Source: SourceDefault, Reason: reason}
}
