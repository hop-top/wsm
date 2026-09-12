package access

import "errors"

// EnforcementLevel indicates how strictly a prerequisite must be met.
type EnforcementLevel string

const (
	LevelRequired    EnforcementLevel = "required"
	LevelRecommended EnforcementLevel = "recommended"
	LevelOptional    EnforcementLevel = "optional"
)

// Transience classes a prerequisite failure for retry decisions. The
// vocabulary matches the one the CLI renders; the strings are spelled
// here rather than imported so pkg/access stays free of the console
// stack.
type Transience string

const (
	// TransienceUnknown is the zero value: this check's author has not
	// said whether a bare retry could clear a failure. It renders as
	// "unknown", which is an honest report of an unclassified check.
	TransienceUnknown Transience = ""
	// TransienceTransient marks a check a later run may pass with no
	// action from the caller — a port another process holds right now,
	// a server that is down.
	TransienceTransient Transience = "transient"
	// TransiencePermanent marks a check that will keep failing until
	// the caller changes something — a binary that is not installed, a
	// version too old, a range with no room in it.
	TransiencePermanent Transience = "permanent"
)

// Prerequisite is a named check with an enforcement level.
type Prerequisite struct {
	Name    string
	Level   EnforcementLevel
	Check   func() error
	Message string
	// Transience says whether a failure of this check could clear on
	// its own. It belongs on the check rather than on the caller
	// because the author of the check is the only one who knows: a
	// missing binary and a busy port are both "environment not ready"
	// and they have opposite answers. Left unset, the failure reports
	// as unclassified rather than guessing.
	Transience Transience
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
