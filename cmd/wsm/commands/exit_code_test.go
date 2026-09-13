package commands

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
	// The code and the exit code must name the same thing. Before the
	// renumber this envelope rendered UNAUTHORIZED — kit's name for 5 —
	// while exiting 6, so the two halves of one envelope contradicted
	// each other. Asserting both together is what keeps them agreeing.
	if oe.ExitCode != ExitForbidden {
		t.Errorf("forbidden exit code = %d; want %d (story 015 scenario 7)", oe.ExitCode, ExitForbidden)
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
	if oe.ExitCode != output.ExitPrerequisite {
		t.Errorf("prerequisite exit code = %d; want %d (story 015 scenario 8)",
			oe.ExitCode, output.ExitPrerequisite)
	}
	// Prerequisite now speaks kit's own code, not GENERIC on a
	// wsm-local number. The old 7 was kit's CONSENT_REFUSED, which kit
	// classifies transient — the reading an agent would have taken for
	// a missing binary no retry can fix.
	if oe.Code != output.CodePrerequisite {
		t.Errorf("prerequisite envelope code = %q; want %q", oe.Code, output.CodePrerequisite)
	}
	// The fix text must survive in Message: the framework-level renderer
	// prints Code + Message only, so guidance in SuggestedFix is dropped.
	if oe.Message != fix {
		t.Errorf("prerequisite message = %q; want %q", oe.Message, fix)
	}
}

// TestExitCode_BackendMapping pins the persistence slot. kit has no
// class for a store fault, so 8 is wsm-local — chosen low rather than
// above kit's contiguous, already-full 64-70 band, which kit grows
// upward and would collide with.
func TestExitCode_BackendMapping(t *testing.T) {
	err := mapPolicyDenialError(workspace.NewErrorBackend("open sqlite backend", errors.New("disk fault")))

	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("mapPolicyDenialError returned %T; want *output.Error", err)
	}
	if oe.ExitCode != ExitBackend {
		t.Errorf("backend exit code = %d; want %d (story 015 scenario 5)", oe.ExitCode, ExitBackend)
	}
}

// TestExitCode_NoKitCollision is the guard the old table lacked: every
// wsm-local number must stay off every code kit has defined. wsm's 5,
// 6 and 7 each collided with a kit code meaning something else, and
// nothing failed when they did.
func TestExitCode_NoKitCollision(t *testing.T) {
	// kit's standard table plus its reserved tool-specific band.
	kitCodes := map[int]string{
		1: "GENERIC", 2: "USAGE", 3: "NOT_FOUND", 4: "CONFLICT",
		5: "UNAUTHORIZED", 6: "TRANSIENT", 7: "CONSENT_REFUSED",
		64: "RATE_LIMITED", 65: "PROVENANCE_MISSING", 66: "LEAK_DETECTED",
		67: "CONFIG", 68: "GRADE_FAIL", 69: "GRADE_UNGRADABLE",
		70: "PREREQUISITE",
	}
	for name, code := range map[string]int{
		"backend":        ExitBackend,
		"not_configured": ExitNotConfigured,
	} {
		if kit, clash := kitCodes[code]; clash {
			t.Errorf("wsm's %s claims exit %d, which is kit's %s", name, code, kit)
		}
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
			code: output.ExitPrerequisite,
		},
		{
			name: "prerequisite_transient",
			err:  workspace.NewErrorPrerequisiteTransient("ports 6667-6669 are not all free: choose another --port base"),
			want: output.TransienceTransient,
			code: output.ExitPrerequisite,
		},
		{
			name: "prerequisite_unclassified_stays_unknown",
			err:  workspace.NewErrorPrerequisite("open provider \"ergo\": connection reset"),
			want: output.TransienceUnknown,
			code: output.ExitPrerequisite,
		},
		{
			name: "not_configured",
			err:  workspace.NewErrorNotConfigured(`IRC is not enabled for "demo": run ` + "`wsm irc enable demo`" + ` first`),
			want: output.TransiencePermanent,
			code: ExitNotConfigured,
		},
		{
			name: "backend",
			err:  workspace.NewErrorBackend("encode backfill request", errors.New("unsupported type")),
			want: output.TransiencePermanent,
			code: ExitBackend,
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

// NotConfigured and Prerequisite are distinct kinds with distinct
// fixes, and a caller must be able to tell them apart.
//
// This test previously asserted they shared exit 7 and diverged only in
// transience — the sharing was a stopgap its own failure message named
// as such ("the renumber is a follow-up, not this change"). This is
// that renumber, so the requirement is kept and the stopgap inverted:
// the two must now differ in the exit code as well. Transience is still
// asserted to differ, because the number alone was never the whole
// distinction and a regression that collapsed either half would leave
// an agent unable to decide whether to retry.
//
// Following prerequisite onto kit's 70 was the alternative, and worse:
// 70 means PREREQUISITE to every kit consumer, and a workspace with IRC
// switched off is not one.
func TestNotConfigured_IsDistinctFromPrerequisite(t *testing.T) {
	var notCfg, unclassified *output.Error
	if !errors.As(mapPolicyDenialError(workspace.NewErrorNotConfigured("run wsm irc enable first")), &notCfg) {
		t.Fatal("not-configured did not map to *output.Error")
	}
	if !errors.As(mapPolicyDenialError(workspace.NewErrorPrerequisite("something environmental")), &unclassified) {
		t.Fatal("prerequisite did not map to *output.Error")
	}
	if notCfg.ExitCode == unclassified.ExitCode {
		t.Errorf("both kinds exit %d; they carry different fixes and must be told apart by code",
			notCfg.ExitCode)
	}
	if notCfg.ExitCode != ExitNotConfigured {
		t.Errorf("not-configured exit code = %d; want %d", notCfg.ExitCode, ExitNotConfigured)
	}
	if unclassified.ExitCode != output.ExitPrerequisite {
		t.Errorf("prerequisite exit code = %d; want %d", unclassified.ExitCode, output.ExitPrerequisite)
	}
	if notCfg.Transience == unclassified.Transience {
		t.Errorf("both kinds report transience %q; the distinction must not collapse",
			notCfg.Transience)
	}
}

// TestCapabilitySurface_PublishesCodesAboveSix pins the 12fcc
// obligation: a code above 6 MUST appear in the capability surface.
// wsm has three (8, 9, 70), and before this change `wsm capabilities`
// published no exit codes at all.
func TestCapabilitySurface_PublishesCodesAboveSix(t *testing.T) {
	published := map[int]string{}
	for _, e := range wsmExitCodes {
		if prev, dup := published[e.Code]; dup {
			t.Errorf("exit code %d published twice: %q and %q", e.Code, prev, e.Symbol)
		}
		published[e.Code] = e.Symbol
		if e.Symbol == "" || e.Meaning == "" {
			t.Errorf("exit code %d published without a symbol or meaning", e.Code)
		}
	}
	for _, code := range []int{ExitBackend, ExitNotConfigured, output.ExitPrerequisite} {
		if code <= 6 {
			continue
		}
		if _, ok := published[code]; !ok {
			t.Errorf("exit code %d is above 6 and is not in the capability surface", code)
		}
	}
	// The table must agree with the mapper it describes, or the
	// surface documents a contract the binary does not implement.
	for kind, want := range map[string]int{
		"forbidden":      ExitForbidden,
		"backend":        ExitBackend,
		"not_configured": ExitNotConfigured,
		"prerequisite":   output.ExitPrerequisite,
	} {
		if _, ok := published[want]; !ok {
			t.Errorf("%s exits %d, which the capability surface does not list", kind, want)
		}
	}
}

// TestExitCodeAnnotations_UseKnownClasses guards the annotation
// vocabulary: a symbol the top-level table does not define would reach
// an agent as an unresolvable string, and kit's own resolver silently
// turns anything it does not recognize into 1.
func TestExitCodeAnnotations_UseKnownClasses(t *testing.T) {
	known := map[string]bool{}
	for _, e := range wsmExitCodes {
		known[e.Symbol] = true
	}
	root := NewRootCmd()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if raw := c.Annotations["kit/exit-codes"]; raw != "" {
			for _, sym := range strings.Split(raw, ",") {
				if sym = strings.TrimSpace(sym); sym != "" && !known[sym] {
					t.Errorf("%s declares unknown exit class %q", c.CommandPath(), sym)
				}
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
