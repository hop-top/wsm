package cli

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// SafetyLevel classifies the risk of a CLI operation.
type SafetyLevel string

const (
	SafetyRead      SafetyLevel = "read"
	SafetyCaution   SafetyLevel = "caution"
	SafetyDangerous SafetyLevel = "dangerous"
)

// SafetyGuard checks safety level and enforces confirmation.
// Returns nil if safe to proceed, error if blocked.
func SafetyGuard(cmd *cobra.Command, level SafetyLevel) error {
	if level == SafetyRead {
		return nil
	}
	if f := cmd.Flags().Lookup("force"); f != nil && f.Value.String() == "true" {
		return nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf(
			"%s operation requires --force in non-interactive mode",
			level,
		)
	}
	// Interactive confirmation.
	fmt.Fprintf(os.Stderr, "This is a %s operation. Continue? [y/N] ", level)
	var answer string
	fmt.Fscanln(os.Stdin, &answer)
	if answer != "y" && answer != "Y" {
		return fmt.Errorf("aborted by user")
	}
	return nil
}
