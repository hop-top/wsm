package access

// VerifyOptions controls which prerequisites are evaluated.
type VerifyOptions struct {
	NoVerify     bool
	SkipOptional bool
}

// RunVerification evaluates prerequisites according to the given options.
// When NoVerify is true, all checks are skipped and every result is marked
// as passed. When SkipOptional is true, optional-level prerequisites are
// skipped (marked passed without running their check).
func RunVerification(
	prereqs []Prerequisite,
	opts VerifyOptions,
) ([]EnforcementResult, error) {
	if opts.NoVerify {
		results := make([]EnforcementResult, len(prereqs))
		for i, p := range prereqs {
			results[i] = EnforcementResult{Prerequisite: p, Passed: true}
		}
		return results, nil
	}

	if opts.SkipOptional {
		filtered := make([]Prerequisite, 0, len(prereqs))
		skipped := make([]EnforcementResult, 0)
		for _, p := range prereqs {
			if p.Level == LevelOptional {
				skipped = append(skipped, EnforcementResult{
					Prerequisite: p,
					Passed:       true,
				})
				continue
			}
			filtered = append(filtered, p)
		}
		results, err := Enforce(filtered)
		return append(skipped, results...), err
	}

	return Enforce(prereqs)
}
