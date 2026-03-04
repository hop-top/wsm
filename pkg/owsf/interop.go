package owsf

import (
	"fmt"
	"strings"

	"hop.top/wsm/pkg/model"
)

// OASF taxonomy and URI prefixes
const (
	OASFAgentURIPrefix = "oasf:agent:"
	OASFCapabilityPrefix = "oasf:cap:"
)

// ValidateAgentIdentity ensures writer_id is a valid OASF Agent Record URI.
func ValidateAgentIdentity(writerID string) error {
	if writerID == "" {
		return fmt.Errorf("writer_id is required")
	}
	if !strings.HasPrefix(writerID, OASFAgentURIPrefix) {
		return fmt.Errorf("writer_id must be an OASF Agent Record URI (prefix %s)", OASFAgentURIPrefix)
	}
	return nil
}

// ValidateSpaceCapabilities ensures all capabilities follow OASF taxonomies.
func ValidateSpaceCapabilities(caps []string) error {
	for _, c := range caps {
		if !strings.HasPrefix(c, OASFCapabilityPrefix) {
			return fmt.Errorf("capability %q must follow OASF taxonomy (prefix %s)", c, OASFCapabilityPrefix)
		}
	}
	return nil
}

// AlignEventToOCSF performs semantic checks on event payloads to ensure OCSF compliance.
func AlignEventToOCSF(ev model.Event) error {
	switch ev.Type {
	case model.EventInteractionMessage:
		// OCSF Chat Message alignment check
		if ev.WriterID == "" {
			return fmt.Errorf("OCSF Chat Message requires a writer (actor)")
		}
	case model.EventInteractionToolCall:
		// OCSF API Call alignment check
		if ev.SpaceID == "" {
			return fmt.Errorf("OCSF API Call requires a space (target environment)")
		}
	case model.EventMutationFilePatch:
		// OCSF File System Activity alignment check
		if ev.SpaceID == "" {
			return fmt.Errorf("OCSF File System Activity requires a space context")
		}
	}
	return nil
}

// DefaultAndAlign performs OCSF alignment and applies default values for missing required fields.
func DefaultAndAlign(ev *model.Event) error {
	// Apply defaults for mandatory OCSF fields if missing
	if ev.WriterID == "" {
		ev.WriterID = OASFAgentURIPrefix + "wsm-internal"
	}
	if ev.SpaceID == "" && (ev.Type == model.EventInteractionToolCall || ev.Type == model.EventMutationFilePatch || ev.Type == model.EventTopologySpaceCreate || ev.Type == model.EventTopologySpaceDestroy) {
		ev.SpaceID = "sp-default"
	}

	return AlignEventToOCSF(*ev)
}

// MapBoundaryToObservables ensures space boundary is correctly mapped to OCSF observables.
func MapBoundaryToObservables(s *model.Space) {
	if len(s.Observables) > 0 {
		return
	}

	// Heuristic mapping if observables are missing
	if strings.HasPrefix(s.Boundary, "/") || strings.HasPrefix(s.Boundary, "./") {
		s.Observables = append(s.Observables, model.Observable{
			Type:  model.ObservableFile,
			Value: s.Boundary,
		})
	} else if strings.Contains(s.Boundary, "://") {
		s.Observables = append(s.Observables, model.Observable{
			Type:  model.ObservableURL,
			Value: s.Boundary,
		})
	}
}
