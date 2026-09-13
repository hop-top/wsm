package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/irc"
	ircdaemon "hop.top/wsm/pkg/irc/daemon"
	"hop.top/wsm/pkg/irc/share"
	"hop.top/wsm/pkg/workspace"
)

// newIRCDelegateCmd builds the `wsm irc delegate` group.
func newIRCDelegateCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delegate",
		Short: "Let another workspace's bot hand out roles on this one's channels",
		Long: `Grant, revoke and list delegations on a host workspace's channels.

A delegation gives a guest workspace's bot a privilege on one of this
workspace's channels, and the server then caps what that bot may hand
out. add writes the entry into the workspace-local irc.channels
override and asks the running daemon to reconcile; rm takes it away,
which is what makes the host's next reconcile revoke the bot and strip
the grants it can prove that bot made.

Delegation is a managed-mode arrangement: it rests on server-side
privilege ceilings, which only hold on a server wsm controls.`,
		RunE: groupOnlyRunE,
	}
	// Depth-3 leaves (wsm irc delegate add) need kit/hierarchical on every
	// intermediate ancestor or kit's shape validator refuses the tree.
	cli.SetHierarchical(cmd)
	cmd.AddCommand(
		newIRCDelegateAddCmd(opts),
		newIRCDelegateRmCmd(opts),
		newIRCDelegateListCmd(opts),
	)
	return cmd
}

// ircDelegateFlags is the flag surface of add and rm.
type ircDelegateFlags struct {
	Channel string
	MaxRole string
	Bot     string
}

// newIRCDelegateAddCmd builds `wsm irc delegate add`.
func newIRCDelegateAddCmd(opts *globalOptions) *cobra.Command {
	var flags ircDelegateFlags

	cmd := &cobra.Command{
		Use:   "add <host-workspace> <guest>",
		Short: "Let a guest workspace's bot hand out roles here",
		Long: `Grant a guest workspace's bot a capped privilege on this host's channels.

--max-role names the ceiling the guest's bot may redistribute, and wsm
grants the privilege that makes that ceiling hold on the server:
--max-role voice grants the bot halfop, because a non-founder halfop may
set a persistent +v but is refused +o, while --max-role op grants chanop.

The cap is enforced by IRC privilege rules, not by wsm. A delegate at
--max-role op is a channel operator: it can op others, and nothing in
wsm prevents that. Grant it only where that is the intent.

The guest must already share this host's server, or --bot must name the
account the host operator provisioned for it with
` + "`wsm irc account create <host> --raw <name>`" + ` — that is the
cross-machine case, where the guest attaches over the remote listener
rather than through the loopback share.

Without --channel the delegation is granted on every channel this
workspace declares.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCDelegateAdd(ctx, cmd, app, opts, args[0], args[1], flags)
			})
		},
	}

	cmd.Flags().StringVar(&flags.Channel, "channel", "",
		"Channel to grant on; every declared channel when omitted")
	cmd.Flags().StringVar(&flags.MaxRole, "max-role", string(irc.RoleVoice),
		"Ceiling the delegate may hand out: voice|op")
	cmd.Flags().StringVar(&flags.Bot, "bot", "",
		"Guest bot account on this server, for a guest that is not sharing locally")

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	setExitCodes(cmd, ClassOK, ClassUsage, ClassNotFound, ClassConflict, ClassUnauthorized, ClassNotConfigured, ClassBackend)
	return cmd
}

// newIRCDelegateRmCmd builds `wsm irc delegate rm`.
func newIRCDelegateRmCmd(opts *globalOptions) *cobra.Command {
	var flags ircDelegateFlags

	cmd := &cobra.Command{
		Use:   "rm <host-workspace> <guest>",
		Short: "Take back a guest workspace's delegation",
		Long: `Revoke a delegation.

The entry is dropped from the workspace-local override, which is the
whole of what this does. The host's next reconcile finds a delegate role
it authored that the config no longer asks for, drops the bot's
privilege, and sweeps the roles it can prove that bot handed out. A role
wsm cannot attribute to the departing bot is reported as unverified and
left where it is, rather than stripped on suspicion.

Without --channel the delegation is revoked on every channel.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCDelegateRm(ctx, cmd, app, opts, args[0], args[1], flags)
			})
		},
	}

	cmd.Flags().StringVar(&flags.Channel, "channel", "",
		"Channel to revoke on; every declared channel when omitted")

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	setExitCodes(cmd, ClassOK, ClassUsage, ClassNotFound, ClassUnauthorized, ClassNotConfigured, ClassBackend)
	return cmd
}

// newIRCDelegateListCmd builds `wsm irc delegate list`.
func newIRCDelegateListCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <host-workspace>",
		Short: "List the delegations this workspace grants",
		Long: `Report every delegation this workspace authored.

Each row carries the configured ceiling, the role the daemon has
actually authored for that bot, and the number of roles in the channel
that wsm neither authored nor desires. A foreign count is not a fault:
it is what a delegate bot handing out roles looks like from the host's
side, and it is the figure a revoke sweep would have to account for.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCDelegateList(ctx, app, opts, args[0])
			})
		},
	}
	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	setExitCodes(cmd, ClassOK, ClassUsage, ClassNotFound, ClassNotConfigured, ClassBackend)
	return cmd
}

// ircDelegateDoc is the `wsm irc delegate add|rm --json` document.
type ircDelegateDoc struct {
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
	Guest     string `json:"guest"`
	Action    string `json:"action"`
	// Bot is the guest's bot account on this server.
	Bot string `json:"bot,omitempty"`
	// MaxRole is the privilege granted to that bot, after the ceiling
	// named by --max-role is translated into the rung that enforces it.
	MaxRole irc.Role `json:"max_role,omitempty"`
	// Channels counts the templates amended.
	Channels int `json:"channels"`
	// Config is the file the override was written to.
	Config string `json:"config"`
	// Synced reports whether a running daemon was asked to reconcile.
	Synced bool `json:"synced"`
}

// runIRCDelegateAdd authors a delegation in the host's override.
func runIRCDelegateAdd(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	guest string,
	flags ircDelegateFlags,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if err := requireCapability(app, workspace.CapIRCEnable); err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	guestRef := strings.TrimSpace(guest)
	if guestRef == "" {
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      "guest workspace is required",
			SuggestedFix: "name the workspace whose bot should be delegated to",
			ExitCode:     2,
		}
	}

	channel := strings.TrimSpace(flags.Channel)
	if channel != "" {
		if err := validateChannelName(channel); err != nil {
			return err
		}
	}

	ceiling, err := parseCeiling(flags.MaxRole)
	if err != nil {
		return err
	}

	bot, err := resolveDelegateBot(ctx, app, rt, profile, guestRef, flags.Bot)
	if err != nil {
		return err
	}

	configPath := workspaceConfigPathFor(profile)
	if done, derr := previewDryRun(cmd,
		map[string]any{
			"workspace": rt.ws.Name,
			"guest":     guestRef,
			"channel":   channel,
			"max_role":  string(ceiling),
		},
		cli.Effect{
			Kind:       "update",
			Target:     "irc-delegate:" + bot,
			Reversible: true,
			Detail: fmt.Sprintf("grant %s %s on %s in %s",
				bot, ceiling, channelScope(channel), configPath),
		},
	); done {
		return derr
	}

	amended, err := share.AddDelegate(profile.Root, channel, irc.Delegate{
		Workspace: guestRef,
		Bot:       bot,
		MaxRole:   ceiling,
	})
	if err != nil {
		return workspace.NewErrorBackend("record delegation", err)
	}
	if amended == 0 {
		return noSuchTemplateError(rt.ws.Name, channel)
	}

	return renderIRCDelegateResult(opts, ircDelegateDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Guest:     guestRef,
		Action:    "add",
		Bot:       bot,
		MaxRole:   ceiling,
		Channels:  amended,
		Config:    configPath,
		Synced:    signalReconcile(rt),
	})
}

// runIRCDelegateRm drops a delegation from the host's override.
func runIRCDelegateRm(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	guest string,
	flags ircDelegateFlags,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if err := requireCapability(app, workspace.CapIRCEnable); err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	guestRef := strings.TrimSpace(guest)
	if guestRef == "" {
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      "guest workspace is required",
			SuggestedFix: "name the workspace whose delegation should be revoked",
			ExitCode:     2,
		}
	}

	channel := strings.TrimSpace(flags.Channel)
	if channel != "" {
		if err := validateChannelName(channel); err != nil {
			return err
		}
	}

	// The entry may name the guest by id or by name, so both are offered.
	// A guest workspace that no longer resolves is not a reason to refuse:
	// revoking a delegation to something that has been deleted is exactly
	// when an operator most needs this to work.
	guestID, guestName := guestRef, guestRef
	if ws, rerr := app.manager.ResolveWorkspaceRef(ctx, guestRef); rerr == nil {
		guestID, guestName = ws.ID, ws.Name
	}

	configPath := workspaceConfigPathFor(profile)
	if done, derr := previewDryRun(cmd,
		map[string]any{
			"workspace": rt.ws.Name,
			"guest":     guestRef,
			"channel":   channel,
		},
		cli.Effect{
			Kind:       "delete",
			Target:     "irc-delegate:" + guestRef,
			Reversible: true,
			Detail: fmt.Sprintf("revoke %s on %s in %s",
				guestRef, channelScope(channel), configPath),
		},
	); done {
		return derr
	}

	removed, err := share.RemoveDelegate(profile.Root, channel, guestID, guestName)
	if err != nil {
		return workspace.NewErrorBackend("remove delegation", err)
	}
	if removed == 0 {
		return workspace.NewErrorNotFound(fmt.Sprintf(
			"%q holds no delegation on %s in %s",
			guestRef, channelScope(channel), configPath))
	}

	return renderIRCDelegateResult(opts, ircDelegateDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Guest:     guestRef,
		Action:    "rm",
		Channels:  removed,
		Config:    configPath,
		Synced:    signalReconcile(rt),
	})
}

// ircDelegateRow is one row of `wsm irc delegate list`.
type ircDelegateRow struct {
	Channel  string   `table:"CHANNEL,priority=9"   json:"channel"`
	Guest    string   `table:"GUEST,priority=8"     json:"workspace"`
	Bot      string   `table:"BOT,priority=7"       json:"bot"`
	MaxRole  irc.Role `table:"MAX_ROLE,priority=6"  json:"max_role"`
	Authored irc.Role `table:"AUTHORED,priority=5"  json:"authored,omitempty"`
	Foreign  int      `table:"FOREIGN,priority=4"   json:"foreign"`
	Attached bool     `table:"ATTACHED,priority=3"  json:"attached"`
}

// runIRCDelegateList reports the delegations this workspace grants.
func runIRCDelegateList(
	ctx context.Context,
	app *appContext,
	opts *globalOptions,
	ref string,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	delegations, err := share.ListDelegates(profile.Root)
	if err != nil {
		return workspace.NewErrorBackend("read delegations", err)
	}

	status, _ := daemonState(rt.paths)
	foreign := foreignRoleCounts(status)
	shared := sharedSet(profile)

	rows := make([]ircDelegateRow, 0, len(delegations))
	for _, d := range delegations {
		rows = append(rows, ircDelegateRow{
			Channel:  d.Channel,
			Guest:    d.Workspace,
			Bot:      d.Bot,
			MaxRole:  d.MaxRole,
			Authored: profile.AuthoredRoles[d.Channel][d.Bot].Role,
			Foreign:  foreign[d.Channel],
			Attached: shared[d.Workspace],
		})
	}

	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), rows)
	}
	return output.Render(os.Stdout, output.Table, rows)
}

// parseCeiling maps --max-role onto the privilege wsm grants the delegate bot.
//
// voice becomes halfop and op stays op: the flag names what the delegate may
// hand out, and the value stored is the rung that makes the server enforce it.
// Halfop is deliberately not accepted from the operator — it is the mechanism,
// not the intent, and offering it would invite "--max-role halfop" to be read as
// "may hand out halfop", which is not what it does.
func parseCeiling(raw string) (irc.Role, error) {
	role, ok := share.CeilingFor(irc.Role(strings.TrimSpace(raw)))
	if !ok {
		return "", &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("--max-role %q: must be voice or op", raw),
			SuggestedFix: "pass --max-role voice or --max-role op",
			ExitCode:     2,
		}
	}
	return role, nil
}

// resolveDelegateBot decides which account the delegation names, and refuses
// the invocation when the guest has no standing on this server.
//
// A guest already sharing the host's server has a bot account the host created
// for it, derived from its name. A guest that is not sharing has none, and wsm
// cannot invent one: off the machine the host operator provisions the account
// by hand with `wsm irc account create --raw`, so --bot is how that name
// reaches the delegation. Without either, there is nothing to delegate to, and
// authoring an entry naming an account that does not exist would leave the host
// reconciling against a phantom.
//
// A conflict rather than a usage error: the invocation is well-formed and
// collides with the state of the world, which is exit 4.
func resolveDelegateBot(
	ctx context.Context,
	app *appContext,
	rt *ircRuntime,
	profile irc.Profile,
	guestRef string,
	explicitBot string,
) (string, error) {
	if bot := strings.TrimSpace(explicitBot); bot != "" {
		return bot, nil
	}

	guestWS, err := app.manager.ResolveWorkspaceRef(ctx, guestRef)
	if err == nil {
		for _, id := range profile.SharedWith {
			if id == guestWS.ID {
				return share.BotAccount(guestWS.Name), nil
			}
		}
	}

	return "", workspace.NewErrorConflict(fmt.Sprintf(
		"%q does not share %q's server, so it has no bot account here: "+
			"attach it with `wsm irc enable %s --shared-with %s`, or pass "+
			"--bot <account> for a guest the host provisioned with "+
			"`wsm irc account create %s --raw <account>`",
		guestRef, rt.ws.Name, guestRef, rt.ws.Name, rt.ws.Name,
	))
}

// sharedSet indexes the guests this workspace hosts.
func sharedSet(profile irc.Profile) map[string]bool {
	out := make(map[string]bool, len(profile.SharedWith))
	for _, id := range profile.SharedWith {
		out[id] = true
	}
	return out
}

// foreignRoleCounts reads per-channel foreign role counts from the daemon's
// last reconcile.
//
// Foreign means a role wsm neither authored nor desires, which is what a
// delegate bot's own grants look like to the host. Reported rather than acted
// on, here as in the reconcile that recorded them.
func foreignRoleCounts(status ircdaemon.Status) map[string]int {
	out := map[string]int{}
	if status.LastReconcile == nil {
		return out
	}
	for _, ch := range status.LastReconcile.Channels {
		if n := len(ch.ForeignRoles); n > 0 {
			out[ch.Name] = n
		}
	}
	return out
}

// channelScope renders the reach of an edit for a message.
func channelScope(channel string) string {
	if channel == "" {
		return "every declared channel"
	}
	return channel
}

// noSuchTemplateError reports an edit that matched no channel template.
func noSuchTemplateError(wsName, channel string) error {
	if channel == "" {
		return workspace.NewErrorNotFound(fmt.Sprintf(
			"%q declares no channels to delegate on: add one with "+
				"`wsm irc channel add %s <#channel>` first", wsName, wsName))
	}
	return workspace.NewErrorNotFound(fmt.Sprintf(
		"channel %q is not declared for %q", channel, wsName))
}

// renderIRCDelegateResult renders an add or rm outcome.
func renderIRCDelegateResult(opts *globalOptions, doc ircDelegateDoc) error {
	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}

	// §6.3: acknowledgements are non-data → stderr.
	if doc.Action == "add" {
		fmt.Fprintf(os.Stderr, "Delegated %s to %s on %d channel(s) in %s\n",
			doc.MaxRole, doc.Bot, doc.Channels, doc.Config)
		if doc.MaxRole == irc.RoleOp {
			fmt.Fprintln(os.Stderr,
				"A chanop delegate can op others: the cap is enforced by IRC "+
					"privilege rules, not by wsm.")
		}
	} else {
		fmt.Fprintf(os.Stderr, "Revoked %s on %d channel(s) in %s\n",
			doc.Guest, doc.Channels, doc.Config)
	}
	if !doc.Synced {
		fmt.Fprintln(os.Stderr,
			"No daemon running: the change applies on the next `wsm irc start`.")
	}
	return nil
}
