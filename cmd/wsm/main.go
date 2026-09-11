package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"hop.top/kit/go/console/output"
	"hop.top/wsm/cmd/wsm/commands"
	"hop.top/wsm/pkg/workspace"
)

var (
	Version   = ""
	CommitSHA = ""
)

func main() {
	commands.Version = Version
	commands.CommitSHA = CommitSHA

	// Construct the root once so kit's Execute path can find the
	// instance; commands.Execute then routes through fang.
	_ = commands.NewRootCmd()
	if err := commands.Execute(context.Background()); err != nil {
		// Kit's RunE wrapper renders *output.Error envelopes (with
		// SuggestedFix) to stderr already. Re-print plain errors
		// (PersistentPreRunE failures, framework errors) here.
		var oe *output.Error
		if !errors.As(err, &oe) {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		// Context cancellation is the one signal not modeled as an
		// envelope (it comes from the cobra/fang plumbing).
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		os.Exit(exitCodeForError(err))
	}
}

// exitCodeForError maps a wsm error to a CLI exit code per
// docs/stories/015-exit-code-contract.md:
//
//	1 = generic error
//	2 = usage error
//	3 = not found
//	4 = conflict (including policy denial)
//	5 = backend / persistence failure   (scenario 5)
//	6 = forbidden / capability denied   (scenario 7)
//	7 = prerequisite not satisfied      (scenario 8)
//
// Must stay in lockstep with mapPolicyDenialError in
// cmd/wsm/commands/policy.go: that one renders the envelope for errors
// reaching kit's RunE wrapper, this one is the fallback for errors
// that arrive unwrapped (PersistentPreRunE, framework paths).
func exitCodeForError(err error) int {
	// kit/runtime/policy vetoes (T-1288) are surfaced via the kit cli
	// RunE middleware as *output.Error envelopes. Honour the embedded
	// ExitCode (4 for CONFLICT) so a policy denial yields exit 4.
	var oe *output.Error
	if errors.As(err, &oe) && oe.ExitCode != 0 {
		return oe.ExitCode
	}
	switch {
	case workspace.IsKind(err, workspace.ErrorKindUsage):
		return 2
	case workspace.IsKind(err, workspace.ErrorKindNotFound):
		return 3
	case workspace.IsKind(err, workspace.ErrorKindConflict):
		return 4
	case workspace.IsKind(err, workspace.ErrorKindBackend):
		return 5
	case workspace.IsKind(err, workspace.ErrorKindForbidden):
		return 6
	case workspace.IsKind(err, workspace.ErrorKindPrerequisite):
		return 7
	case isCobraUsageError(err):
		return 2
	default:
		return 1
	}
}

func isCobraUsageError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "accepts") ||
		strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "required flag")
}
