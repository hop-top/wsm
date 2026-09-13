package main

import (
	"errors"
	"testing"

	"hop.top/kit/go/console/output"

	"hop.top/wsm/pkg/workspace"
)

// exitCodeForError is the fallback mapper for errors that reach main
// unwrapped (PersistentPreRunE, framework paths). It must agree with
// mapPolicyDenialError in cmd/wsm/commands/policy.go — a drift between
// the two means the same fault exits differently depending on where it
// was raised.
func TestExitCodeForError_Story015(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"usage", workspace.NewErrorUsage("missing name"), 2},
		{"not_found", workspace.NewErrorNotFound("no such workspace"), 3},
		{"conflict", workspace.NewErrorConflict("already exists"), 4},
		{"forbidden", workspace.NewErrorForbidden("not permitted"), 5},
		{"backend", workspace.NewErrorBackend("open sqlite backend", errors.New("disk fault")), 8},
		{"not_configured", workspace.NewErrorNotConfigured("run `wsm irc enable demo` first"), 9},
		{"prerequisite", workspace.NewErrorPrerequisite("ergo not installed: brew install ergo"), 70},
		{"generic", errors.New("boom"), 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCodeForError(tc.err); got != tc.want {
				t.Errorf("exitCodeForError(%s) = %d; want %d", tc.name, got, tc.want)
			}
		})
	}
}

// An embedded envelope wins over kind inspection, which is how kit's
// own confirm gate keeps its UNAUTHORIZED exit 5 refusal.
func TestExitCodeForError_EnvelopeWins(t *testing.T) {
	oe := &output.Error{Code: output.CodeUnauthorized, Message: "--confirm=no", ExitCode: 5}
	if got := exitCodeForError(oe); got != 5 {
		t.Errorf("envelope exit code = %d; want 5", got)
	}
}
