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
		os.Exit(exitCodeForError(err))
	}
}

// exitCodeForError maps a wsm error to a CLI exit code per CLI conv §8.1:
//
//	1   = generic error
//	2   = usage error
//	3   = not found
//	4   = conflict
//	5   = unauthorized / forbidden
//	64+ = tool-specific (we use 64 for backend / persistence failures)
//
// Historically Backend mapped to 5, colliding with the §8.1 reserved
// slot for unauthorized. Forbidden now claims 5; Backend moved to 64.
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
	case workspace.IsKind(err, workspace.ErrorKindForbidden):
		return 5
	case workspace.IsKind(err, workspace.ErrorKindBackend):
		return 64
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
		strings.Contains(msg, "required flag")
}
