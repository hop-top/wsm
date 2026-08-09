package commands

import (
	"os"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// renderDryRunPlan emits the structured cli.Plan for a --dry-run
// invocation of cmd and applies nothing. Mutating RunEs call
// previewDryRun first; the plan renders on stdout in the active
// --format (JSON/YAML serialize the plan document, table renders the
// header block + effect rows via kit's RenderPlan dispatch).
func renderDryRunPlan(cmd *cobra.Command, args map[string]any, effects ...cli.Effect) error {
	plan := cli.Plan{
		Command:     cmd.CommandPath(),
		Args:        args,
		Effects:     effects,
		GeneratedAt: time.Now().UTC(),
	}
	format := output.Table
	if f := cmd.Flag("format"); f != nil && f.Value.String() != "" {
		format = f.Value.String()
	}
	return output.RenderPlan(os.Stdout, format, plan)
}

// previewDryRun reports whether cmd runs under --dry-run and, when it
// does, renders the structured plan of the effects the command WOULD
// apply. Call at the top of every mutating RunE:
//
//	if done, err := previewDryRun(cmd, args, effects...); done {
//		return err
//	}
//
// The plan is assembled from the invocation alone (no backend open, no
// reads) so previews stay side-effect-free even when the target does
// not exist yet.
func previewDryRun(cmd *cobra.Command, args map[string]any, effects ...cli.Effect) (bool, error) {
	if !cli.IsDryRun(cmd) {
		return false, nil
	}
	return true, renderDryRunPlan(cmd, args, effects...)
}
