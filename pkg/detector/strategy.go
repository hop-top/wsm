package detector

// Strategy aggregates detector results into a recommendation profile.
type Strategy struct {
	// Name is a human-readable label for this strategy.
	Name string `json:"name"`

	// Detectors lists the names of detectors that contributed.
	Detectors []string `json:"detectors"`

	// Recommendations holds key-value suggestions derived from results.
	Recommendations map[string]string `json:"recommendations"`
}

// GenerateStrategy aggregates positive detector results into a strategy.
func GenerateStrategy(results []Result) Strategy {
	s := Strategy{
		Name:            "auto",
		Recommendations: make(map[string]string),
	}

	for _, r := range results {
		if !r.Detected {
			continue
		}
		s.Detectors = append(s.Detectors, r.DetectorName)
		applyRecommendations(&s, r)
	}

	if len(s.Detectors) == 0 {
		s.Name = "default"
		s.Recommendations["note"] = "no detections; using defaults"
	}

	return s
}

func applyRecommendations(s *Strategy, r Result) {
	switch r.Kind {
	case "build":
		if file, ok := r.Details["file"]; ok {
			s.Recommendations["build_tool"] = file
		}
	case "runtime":
		if file, ok := r.Details["file"]; ok {
			s.Recommendations["runtime_"+r.DetectorName] = file
		}
	case "knowledge":
		for k, v := range r.Details {
			s.Recommendations["knowledge_"+k] = v
		}
	case "health":
		if total, ok := r.Details["total_deps"]; ok {
			s.Recommendations["dependency_count"] = total
		}
		if files, ok := r.Details["total_files"]; ok {
			s.Recommendations["dependency_files"] = files
		}
	}
}
