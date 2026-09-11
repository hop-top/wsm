package commands

import (
	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
)

// newIRCGroupCmd builds the `wsm irc` parent.
//
// The lifecycle verbs (enable, start, stop, status, sync and the rest) land
// with the IRC CLI work; this parent exists so the daemon the rest of the
// feature is built on is reachable and testable in the meantime. Adding verbs
// here is additive: each is its own constructor in the AddCommand call below.
func newIRCGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "irc",
		Short: "Workspace IRC operations",
		Long: `Run and inspect a workspace's IRC presence.

wsm can manage its own ergo server for a workspace or connect to one it does
not own. Either way a per-workspace daemon runs the bot, keeps channels and
roles in line with the workspace's members, and captures channel traffic into
the event log.`,
		RunE: groupOnlyRunE,
	}
	cli.SetTopLevelVerb(cmd)
	cmd.AddCommand(
		newIRCDaemonCmd(opts),
	)
	return cmd
}
