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
	// `account` and `channel` group their own leaves, which puts those
	// leaves at depth 3. kit's signature validator walks every ancestor
	// between such a leaf and the root and demands kit/hierarchical on
	// each one — it exempts a depth-1 ancestor only when kit itself
	// reserved the name, which `irc` is not. So the annotation belongs
	// here as well as on the two sub-groups; without it kit refuses the
	// whole tree at startup rather than just the new verbs.
	cli.SetHierarchical(cmd)
	cmd.AddCommand(
		newIRCEnableCmd(opts),
		newIRCDisableCmd(opts),
		newIRCStartCmd(opts),
		newIRCStopCmd(opts),
		newIRCRestartCmd(opts),
		newIRCStatusCmd(opts),
		newIRCSyncCmd(opts),
		newIRCAccountCmd(opts),
		newIRCCredsCmd(opts),
		newIRCURLCmd(opts),
		newIRCChannelCmd(opts),
		newIRCBackfillCmd(opts),
		newIRCDaemonCmd(opts),
	)
	return cmd
}
