package commands

import (
	"fmt"
	"io"
	"sort"

	"hop.top/wsm/pkg/detector"
	"hop.top/wsm/pkg/workspace"
)

// spaceWithInference is the structured payload for `wsm space add
// --auto`. It embeds the Space so every existing field keeps its name
// and position, and adds one sibling key.
//
// The block is named _inferred after 12-factor factor 11: it describes
// what detection concluded, per field, and how. It is disclosure only —
// the embedded Space is what wsm actually persisted, which is whatever
// the user passed on the command line.
type spaceWithInference struct {
	workspace.Space
	Inference *inferenceReport `json:"_inferred,omitempty" yaml:"_inferred,omitempty"`
}

// inferenceReport is the disclosed inference: the values detection
// arrived at, the provenance of each, and whether the whole inference
// was positively determined or fell back to defaults.
//
// Applied is always false today: wsm discloses the inference without
// applying it to the persisted space. It is stated explicitly rather
// than left to be assumed, because an agent reading type/trust_level
// here must not mistake them for the space's stored values.
type inferenceReport struct {
	Type         string                             `json:"type"                   yaml:"type"`
	TrustLevel   string                             `json:"trust_level"            yaml:"trust_level"`
	Capabilities []string                           `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Determined   bool                               `json:"determined"             yaml:"determined"`
	Applied      bool                               `json:"applied"                yaml:"applied"`
	Fields       map[string]detector.FieldInference `json:"fields"                 yaml:"fields"`
}

// withInference builds the render payload for `space add`. When inf is
// nil (no --auto) it returns the bare Space, so the emitted document is
// exactly what prior releases emitted.
func withInference(s workspace.Space, inf *detector.SpaceInference) any {
	if inf == nil {
		return s
	}
	return spaceWithInference{
		Space: s,
		Inference: &inferenceReport{
			Type:         string(inf.Type),
			TrustLevel:   string(inf.TrustLevel),
			Capabilities: inf.Capabilities,
			Determined:   inf.Determined(),
			Applied:      false,
			Fields:       inf.Inferred,
		},
	}
}

// reportInference writes the human-readable disclosure to w. Table
// output keeps its existing columns; this goes to stderr alongside the
// other §6.3 non-data messages, so the default view is not cluttered
// for users who did not ask for detection.
//
// Each line names the field, the value, and whether that value was
// determined by evidence or defaulted because nothing matched — the
// distinction the JSON block carries, in one line per field.
func reportInference(w io.Writer, inf detector.SpaceInference) {
	fmt.Fprintln(w, "Inferred (not applied; space saved as specified):")
	values := map[string]string{
		"type":        string(inf.Type),
		"trust_level": string(inf.TrustLevel),
	}
	names := make([]string, 0, len(inf.Inferred))
	for name := range inf.Inferred {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f := inf.Inferred[name]
		verdict := "determined"
		if !f.Determined() {
			verdict = "defaulted"
		}
		value := values[name]
		if name == "capabilities" {
			value = fmt.Sprintf("%v", inf.Capabilities)
		}
		fmt.Fprintf(w, "  %s: %s (%s — %s)\n", name, value, verdict, f.Reason)
	}
}
