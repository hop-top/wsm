package output

import "time"

// Provenance metadata attached to structured output.
type Provenance struct {
	Source    string    // data source (e.g. "local", "api", "cache")
	Timestamp time.Time // when data was retrieved
	Method    string    // retrieval method (e.g. "static", "query", "computed")
}

// WithProvenance wraps output data with _meta provenance field
// when --format is json/yaml.
func WithProvenance(data any, p Provenance) map[string]any {
	return map[string]any{
		"data": data,
		"_meta": map[string]any{
			"source":    p.Source,
			"timestamp": p.Timestamp.Format(time.RFC3339),
			"method":    p.Method,
		},
	}
}
