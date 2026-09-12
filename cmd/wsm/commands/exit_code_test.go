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

// The split of the old over-broad prerequisite kind is only worth
// anything if each resulting kind reports an honest transience. These
// pin that: kit's TransienceForCode classifies only kit's own standard
// codes, so a GENERIC envelope that leaves Transience unset renders
// "unknown" — which for a kind wsm has classified is a lie of omission.
func TestTransience_ByKind(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
		code int
	}{
		{
			name: "prerequisite_permanent",
			err:  workspace.NewErrorPrerequisitePermanent(`ergo binary "ergo" not found: install it`),
			want: output.TransiencePermanent,
			code: 7,
		},
		{
			name: "prerequisite_transient",
			err:  workspace.NewErrorPrerequisiteTransient("ports 6667-6669 are not all free: choose another --port base"),
			want: output.TransienceTransient,
			code: 7,
		},
		{
			name: "prerequisite_unclassified_stays_unknown",
			err:  workspace.NewErrorPrerequisite("open provider \"ergo\": connection reset"),
			want: output.TransienceUnknown,
			code: 7,
		},
		{
			name: "not_configured",
			err:  workspace.NewErrorNotConfigured(`IRC is not enabled for "demo": run ` + "`wsm irc enable demo`" + ` first`),
			want: output.TransiencePermanent,
			code: 7,
		},
		{
			name: "backend",
			err:  workspace.NewErrorBackend("encode backfill request", errors.New("unsupported type")),
			want: output.TransiencePermanent,
			code: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var oe *output.Error
			if !errors.As(mapPolicyDenialError(tc.err), &oe) {
				t.Fatalf("%s did not map to *output.Error", tc.name)
			}
			if oe.Transience != tc.want {
				t.Errorf("transience = %q; want %q", oe.Transience, tc.want)
			}
			if oe.ExitCode != tc.code {
				t.Errorf("exit code = %d; want %d", oe.ExitCode, tc.code)
			}
		})
	}
}

// NotConfigured shares prerequisite's exit 7 while the exit-code table
// is renumbered elsewhere. It is still a distinct kind, and the two
// must stay distinguishable by transience so the shared number costs
// no information.
func TestNotConfigured_SharesExitSevenButNotTransience(t *testing.T) {
	var notCfg, unclassified *output.Error
	if !errors.As(mapPolicyDenialError(workspace.NewErrorNotConfigured("run wsm irc enable first")), &notCfg) {
		t.Fatal("not-configured did not map to *output.Error")
	}
	if !errors.As(mapPolicyDenialError(workspace.NewErrorPrerequisite("something environmental")), &unclassified) {
		t.Fatal("prerequisite did not map to *output.Error")
	}
	if notCfg.ExitCode != unclassified.ExitCode {
		t.Fatalf("exit codes diverged: %d vs %d; the renumber is a follow-up, not this change",
			notCfg.ExitCode, unclassified.ExitCode)
	}
	if notCfg.Transience == unclassified.Transience {
		t.Errorf("both kinds report transience %q; the shared exit code must not also collapse transience",
			notCfg.Transience)
	}
}
