package access

import "errors"

// EnforcementLevel indicates how strictly a prerequisite must be met.
type EnforcementLevel string

const (
	LevelRequired    EnforcementLevel = "required"
	LevelRecommended EnforcementLevel = "recommended"
	LevelOptional    EnforcementLevel = "optional"
)

// Prerequisite is a named check with an enforcement level.
type Prerequisite struct {
	Name    string
	Level   EnforcementLevel
	Check   func() error
	Message string
}

// EnforcementResult captures the outcome of a single prerequisite check.
type EnforcementResult struct {
	Prerequisite Prerequisite
	Passed       bool
	Error        error
}

// Enforce runs all prerequisites and returns their results.
// It returns a non-nil error only when a required prerequisite fails.
func Enforce(prereqs []Prerequisite) ([]EnforcementResult, error) {
	results := make([]EnforcementResult, 0, len(prereqs))
	var requiredErrs []error

	for _, p := range prereqs {
		r := EnforcementResult{Prerequisite: p}
		if err := p.Check(); err != nil {
			r.Passed = false
			r.Error = err
			if p.Level == LevelRequired {
				requiredErrs = append(requiredErrs, err)
			}
		} else {
			r.Passed = true
		}
		results = append(results, r)
	}

	if len(requiredErrs) > 0 {
		return results, errors.Join(requiredErrs...)
	}
	return results, nil
}
