package owsf

type ImportReport struct {
	RequestedConformance int              `json:"requested"`
	ActualConformance    int              `json:"actual"`
	Downgraded           bool             `json:"downgraded"`
	Determinism          DeterminismLevel `json:"determinism"`
	Warnings             []string         `json:"warnings,omitempty"`
}

func NewImportReport(exp Export, achievable int) ImportReport {
	downgraded := achievable < exp.MaxConformance
	r := ImportReport{
		RequestedConformance: exp.MaxConformance,
		ActualConformance:    achievable,
		Downgraded:           downgraded,
		Determinism:          exp.Determinism,
	}
	if downgraded {
		r.Warnings = append(r.Warnings, "conformance downgraded")
	}
	return r
}
