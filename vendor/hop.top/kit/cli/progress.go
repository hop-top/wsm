package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// ProgressEvent is a structured progress update for long-running ops.
type ProgressEvent struct {
	Phase   string  `json:"phase"`
	Step    string  `json:"step"`
	Current int     `json:"current"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
	Message string  `json:"message,omitempty"`
}

// ProgressReporter emits structured progress to stderr.
type ProgressReporter struct {
	w      io.Writer
	format string // "json" for agents, "human" for TTY
}

// NewProgressReporter creates a reporter. Non-TTY gets JSON lines,
// TTY gets human-readable output.
func NewProgressReporter(w io.Writer, isTTY bool) *ProgressReporter {
	f := "json"
	if isTTY {
		f = "human"
	}
	return &ProgressReporter{w: w, format: f}
}

// Emit writes a progress event.
func (p *ProgressReporter) Emit(event ProgressEvent) {
	if p.format == "json" {
		_ = json.NewEncoder(p.w).Encode(event)
		return
	}
	fmt.Fprintf(p.w, "  [%s] %s  %d/%d (%.0f%%)",
		event.Phase, event.Step, event.Current, event.Total, event.Percent)
	if event.Message != "" {
		fmt.Fprintf(p.w, " — %s", event.Message)
	}
	fmt.Fprintln(p.w)
}

// Done emits a terminal progress event.
func (p *ProgressReporter) Done(message string) {
	p.Emit(ProgressEvent{
		Phase:   "done",
		Percent: 100,
		Message: message,
	})
}

// JobHandle represents an async operation that can be polled/canceled.
type JobHandle struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "running", "completed", "failed", "canceled"
}
