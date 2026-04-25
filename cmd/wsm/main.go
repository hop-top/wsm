package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"hop.top/wsm/pkg/workspace"
)

var (
	Version   = ""
	CommitSHA = ""
)

func main() {
	if err := newRoot().Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		if workspace.IsKind(err, workspace.ErrorKindNotFound) {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Run 'wsm workspace list' to see available workspaces.")
		}
		os.Exit(exitCodeForError(err))
	}
}

func exitCodeForError(err error) int {
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
