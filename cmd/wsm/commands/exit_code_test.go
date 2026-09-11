package commands

import (
	"errors"
	"testing"

	"hop.top/kit/go/console/output"

	"hop.top/wsm/pkg/workspace"
)

// The capability gate that produces ErrorKindForbidden lives behind
// workspace.WithCapabilities(DenyAll()), which no CLI flag exposes, and
// ErrorKindPrerequisite has no producer until the irc commands land.
// Neither kind is reachable from an e2e invocation today, so the
// mapping function is asserted directly. These are the regression
// guards for story 015 scenarios 5, 7 and 8.
func TestExitCode_Forbidden(t *testing.T) {
	err := mapPolicyDenialError(workspace.NewErrorForbidden("workspace archive not permitted"))

	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("mapPolicyDenialError returned %T; want *output.Error", err)
	}
	if oe.ExitCode != 6 {
		t.Errorf("forbidden exit code = %d; want 6 (story 015 scenario 7)", oe.ExitCode)
	}
	if oe.Code != output.CodeUnauthorized {
		t.Errorf("forbidden envelope code = %q; want %q", oe.Code, output.CodeUnauthorized)
	}
}

func TestExitCode_Prerequisite(t *testing.T) {
	const fix = "ergo not installed: brew install ergo"
	err := mapPolicyDenialError(workspace.NewErrorPrerequisite(fix))

	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("mapPolicyDenialError returned %T; want *output.Error", err)
	}
	if oe.ExitCode != 7 {
		t.Errorf("prerequisite exit code = %d; want 7 (story 015 scenario 8)", oe.ExitCode)
	}
	// The fix text must survive in Message: the framework-level renderer
	// prints Code + Message only, so guidance in SuggestedFix is dropped.
	if oe.Message != fix {
		t.Errorf("prerequisite message = %q; want %q", oe.Message, fix)
	}
}

// TestExitCode_BackendMapping pins the other half of the realignment:
// backend faults claim 5, the slot forbidden used to occupy.
func TestExitCode_BackendMapping(t *testing.T) {
	err := mapPolicyDenialError(workspace.NewErrorBackend("open sqlite backend", errors.New("disk fault")))

	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("mapPolicyDenialError returned %T; want *output.Error", err)
	}
	if oe.ExitCode != 5 {
		t.Errorf("backend exit code = %d; want 5 (story 015 scenario 5)", oe.ExitCode)
	}
}

// Policy denials keep exit 4 and must not drift onto a neighbouring
// slot while the codes around them move.
func TestExitCode_PolicyDenialUnchanged(t *testing.T) {
	denial := &workspace.Error{
		Kind:    workspace.ErrorKindConflict,
		Message: `policy "archive-requires-note" denied: note required`,
		Policy:  true,
	}

	var oe *output.Error
	if !errors.As(mapPolicyDenialError(denial), &oe) {
		t.Fatal("policy denial did not map to *output.Error")
	}
	if oe.ExitCode != 4 {
		t.Errorf("policy denial exit code = %d; want 4", oe.ExitCode)
	}
}
