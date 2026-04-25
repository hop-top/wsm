package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// CorrectedError is a structured error with guidance for recovery.
// Agents and humans get classification, cause, fix, and alternatives.
type CorrectedError struct {
	Code         string   `json:"code"`
	Message      string   `json:"message"`
	Cause        string   `json:"cause"`
	Fix          string   `json:"fix"`
	Alternatives []string `json:"alternatives"`
	Retryable    bool     `json:"retryable"`
}

func (e *CorrectedError) Error() string { return e.Message }

// MarshalJSON renders all fields for agent consumption.
func (e *CorrectedError) MarshalJSON() ([]byte, error) {
	type raw CorrectedError // avoid recursion
	return json.Marshal((*raw)(e))
}

// FormatError renders the error for terminal output:
//
//	ERROR  mission not found
//	Cause: no mission matches "bogux"
//	Fix:   spaced mission list
//	Try:   spaced mission search <partial>
func FormatError(err error, w io.Writer, noColor bool) {
	if err == nil {
		return
	}
	var ce *CorrectedError
	if !errors.As(err, &ce) {
		fmt.Fprintf(w, "ERROR  %s\n", err.Error())
		return
	}
	fmt.Fprintf(w, "ERROR  %s\n", ce.Message)
	if ce.Cause != "" {
		fmt.Fprintf(w, "Cause: %s\n", ce.Cause)
	}
	if ce.Fix != "" {
		fmt.Fprintf(w, "Fix:   %s\n", ce.Fix)
	}
	for _, alt := range ce.Alternatives {
		fmt.Fprintf(w, "Try:   %s\n", alt)
	}
}
