package detector

// Override customizes detector behavior per space or workspace-wide.
type Override struct {
	// SpaceID scopes the override to a specific space. Empty = all.
	SpaceID string `json:"space_id,omitempty"`

	// WorkspaceID scopes the override to a workspace. Empty = global.
	WorkspaceID string `json:"workspace_id,omitempty"`

	// DisabledDetectors lists detector names to exclude from results.
	DisabledDetectors []string `json:"disabled_detectors,omitempty"`

	// CustomSettings are key-value overrides applied to recommendations.
	CustomSettings map[string]string `json:"custom_settings,omitempty"`
}

// ApplyOverrides modifies a strategy in-place based on override rules.
// Disabled detectors are removed from the strategy's detector list,
// and custom settings are merged into recommendations (overwriting).
func ApplyOverrides(strategy *Strategy, overrides []Override) {
	if strategy == nil {
		return
	}

	disabled := make(map[string]bool)
	for _, o := range overrides {
		for _, name := range o.DisabledDetectors {
			disabled[name] = true
		}
	}

	// Remove disabled detectors
	if len(disabled) > 0 {
		filtered := strategy.Detectors[:0]
		for _, d := range strategy.Detectors {
			if !disabled[d] {
				filtered = append(filtered, d)
			}
		}
		strategy.Detectors = filtered
	}

	// Merge custom settings into recommendations
	for _, o := range overrides {
		for k, v := range o.CustomSettings {
			strategy.Recommendations[k] = v
		}
	}
}
