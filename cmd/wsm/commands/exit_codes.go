package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/output"
)

// Exit codes for the error kinds kit's standard table has no class for.
//
// kit publishes 0 OK, 1 GENERIC, 2 USAGE, 3 NOT_FOUND, 4 CONFLICT, 5
// UNAUTHORIZED, 6 TRANSIENT, 7 CONSENT_REFUSED, 64 RATE_LIMITED, 65
// PROVENANCE_MISSING and 70 PREREQUISITE. wsm maps every kind it can
// onto that table — forbidden onto 5, prerequisite onto 70 via
// output.ExitPrerequisite — and only the two kinds with no kit
// counterpart are numbered here.
//
// Both sit low, at 8 and 9, rather than extending kit's 64-70 band.
// That band is contiguous and already full (64 RATE_LIMITED, 65
// PROVENANCE_MISSING, 66 LEAK_DETECTED, 67 CONFIG, 68 GRADE_FAIL, 69
// GRADE_UNGRADABLE, 70 PREREQUISITE) and kit grows it upward, so the
// next code kit defines claims 71 and would collide with anything wsm
// parked immediately above the band. 8-63 carries no kit meaning now
// and none is planned.
const (
	// ExitBackend is a persistence fault: the store could not be
	// opened, read or written. Not a statement about the request —
	// the same command may succeed once the backend is healthy.
	ExitBackend = 8

	// ExitNotConfigured is wsm refusing because its own configuration
	// or recorded state does not yet say enough to run the command.
	// Distinct from ExitBackend (the store broke) and from
	// output.ExitPrerequisite (the environment around wsm is not
	// ready): here the invocation and the environment are both fine
	// and wsm has simply not been told something yet. Always
	// permanent — waiting never clears it.
	ExitNotConfigured = 9

	// ExitForbidden is a capability denial. It is kit's UNAUTHORIZED
	// slot, named here only so call sites read as a contract rather
	// than a literal; kit declares no exported constant for it.
	ExitForbidden = 5
)

// Compile-time proof that the kit-aligned slots really are kit's. A
// kit upgrade that moved PREREQUISITE off 70 breaks the build here
// rather than silently renumbering wsm's contract.
var _ = [1]struct{}{}[output.ExitPrerequisite-70]

// Exit-code class symbols for the kit/exit-codes annotation. The
// 12fcc spec requires any code above 6 to be declared in the
// capability surface, and this annotation is the mechanism kit
// provides: `wsm capabilities --format json` projects it per leaf, and
// the document's top-level exit_codes table resolves each symbol to
// its number and meaning.
//
// The three wsm-specific symbols are spelled here rather than reused
// from kit because kit names only the classes it defines. Note that
// kit's own conformance harness resolves unknown symbols to 1, so the
// numbers are published alongside the symbols rather than left to a
// consumer to look up.
const (
	ClassOK            = "OK"
	ClassUsage         = "USAGE"
	ClassNotFound      = "NOT_FOUND"
	ClassConflict      = "CONFLICT"
	ClassUnauthorized  = "UNAUTHORIZED"
	ClassBackend       = "BACKEND"
	ClassNotConfigured = "NOT_CONFIGURED"
	ClassPrerequisite  = "PREREQUISITE"
	ClassGeneric       = "GENERIC"
)

// setExitCodes declares the exit-code classes a leaf may produce.
//
// Every leaf can exit 1 and 2, so those are left implicit; what is
// worth declaring is which of the classified failures this particular
// command can reach. Declaring a class the leaf cannot produce is as
// much a defect as omitting one it can.
func setExitCodes(cmd *cobra.Command, classes ...string) {
	if cmd == nil || len(classes) == 0 {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["kit/exit-codes"] = strings.Join(classes, ",")
}
