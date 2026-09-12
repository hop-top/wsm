package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/irc"
	ircdaemon "hop.top/wsm/pkg/irc/daemon"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/workspace"
)

// newIRCChannelCmd builds the `wsm irc channel` group.
func newIRCChannelCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Manage a workspace's IRC channels",
		Long: `Add, remove and list the channels wsm maintains for a workspace.

add and rm edit the workspace-local irc.channels override in the
workspace root's config, then ask the running daemon to reconcile. The
daemon is the only thing that ever joins or parts a channel, so these
verbs change intent and let the daemon act on it.

adopt and release say the same thing about a channel wsm did not
author: they record the request in the override and signal the daemon,
whose discovery pass performs the join or part. Neither verb joins
anything itself.

list merges the configured templates with what the daemon last observed,
so a channel that is configured but not yet registered is visible as
exactly that.`,
		RunE: groupOnlyRunE,
	}
	// Depth-3 leaves (wsm irc channel add) require kit/hierarchical on
	// every intermediate ancestor; without it kit's shape validator
	// refuses the whole tree at startup.
	cli.SetHierarchical(cmd)
	cmd.AddCommand(
		newIRCChannelAddCmd(opts),
		newIRCChannelRmCmd(opts),
		newIRCChannelListCmd(opts),
		newIRCChannelAdoptCmd(opts),
		newIRCChannelReleaseCmd(opts),
		newIRCChannelPurgeCmd(opts),
	)
	return cmd
}

// ircChannelFlags is the flag surface shared by add and rm, lifted into
// a struct so the mapping from flags to a template is unit-testable
// without constructing a cobra command.
type ircChannelFlags struct {
	Members    string
	MemberList []string
	Owner      string
	Modes      string
	Topic      string
	Guests     []string
	Borrowed   string
	Purge      bool
	Force      bool
}

// newIRCChannelAddCmd builds `wsm irc channel add`.
func newIRCChannelAddCmd(opts *globalOptions) *cobra.Command {
	var flags ircChannelFlags

	cmd := &cobra.Command{
		Use:   "add <workspace> <#channel>",
		Short: "Add a channel to a workspace's IRC configuration",
		Long: `Declare a channel the workspace's bot should maintain.

The channel is written to the workspace-local irc.channels override and
the running daemon is signalled to reconcile, which is what actually
registers it and applies roles. Other keys in that config file are left
byte-for-byte alone.

A name that falls inside a channel creator's namespace, or that the
daemon has already adopted through discovery, is refused: those channels
belong to a member's own bot, and declaring one here would make wsm
seize it on the next reconcile. --force overrides that when the seizure
is what you actually want.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCChannelAdd(ctx, cmd, app, opts, args[0], args[1], flags)
			})
		},
	}

	cmd.Flags().StringVar(&flags.Members, "members", "", "Membership rule: all|principals|list")
	cmd.Flags().StringSliceVar(&flags.MemberList, "member-list", nil, "Profile ids when --members list")
	cmd.Flags().StringVar(&flags.Owner, "owner", "", "Profile id given +o for an owner-managed channel")
	cmd.Flags().StringVar(&flags.Modes, "modes", "", "Channel modes applied on ensure, e.g. +ntR")
	cmd.Flags().StringVar(&flags.Topic, "topic", "", "Channel topic")
	cmd.Flags().StringArrayVar(&flags.Guests, "guest", nil, "Guest allow-list entry <profile>[:voice|op]")
	cmd.Flags().StringVar(&flags.Borrowed, "borrowed", "", "Borrow a hosted channel: <host-ws>:<#channel>")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Add even inside a creator namespace or discovered set")

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	return cmd
}

// newIRCChannelRmCmd builds `wsm irc channel rm`.
func newIRCChannelRmCmd(opts *globalOptions) *cobra.Command {
	var flags ircChannelFlags

	cmd := &cobra.Command{
		Use:   "rm <workspace> <#channel>",
		Short: "Remove a channel from a workspace's IRC configuration",
		Long: `Stop maintaining a channel.

The template is dropped from the workspace-local override and the daemon
reconciles, which clears the roles wsm authored there and, where the
server supports it, makes the channel invite-only so it stops accepting
new arrivals.

--purge additionally drops the channel registration on the server. That
is an ergo capability (CS PURGE, a two-step confirmation); on a provider
without it the purge is reported as unsupported rather than silently
skipped. Purging is not reversible through wsm: there is no unpurge
verb here, so it is gated behind the same --confirm as any other
destructive operation.`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// --force stands in for --confirm=yes on the destructive
			// half, the way it does on `wsm irc disable --purge`.
			if flags.Purge {
				return bridgeForceToConfirm(cmd, flags.Force)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCChannelRm(ctx, cmd, app, opts, args[0], args[1], flags)
			})
		},
	}

	cmd.Flags().BoolVar(&flags.Purge, "purge", false, "Also drop the channel registration on the server")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Proceed without the destructive confirmation")

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	return cmd
}

// newIRCChannelListCmd builds `wsm irc channel list`.
func newIRCChannelListCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <workspace>",
		Short: "List a workspace's IRC channels",
		Long: `Show the channels this workspace maintains and what became of them.

The list merges two sources: the configured templates, which say what
should exist, and the daemon's last status, which says what does. A
configured channel the daemon has not registered yet appears with
registered=false rather than being omitted, because "declared but not
yet applied" is the state an operator most often needs to see.

Adopted, peer and unmanaged channels are listed with their source, so a
channel wsm captures but does not author is never mistaken for one it
maintains.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCChannelList(ctx, app, opts, args[0])
			})
		},
	}
	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// newIRCChannelAdoptCmd builds `wsm irc channel adopt`.
func newIRCChannelAdoptCmd(opts *globalOptions) *cobra.Command {
	var owner string

	cmd := &cobra.Command{
		Use:   "adopt <workspace> <#channel>",
		Short: "Ask the daemon to adopt a channel it did not create",
		Long: `Record a channel the daemon should join and capture.

The request is written to irc.adopted in the workspace-local override
and the running daemon is signalled. The daemon's discovery pass is what
joins the channel and records it under discovered; this command never
joins anything, so a channel is captured by the long-lived bot session
rather than by whatever connection the CLI happened to open.

A channel previously released is un-released by adopting it again: the
name is dropped from irc.released, which otherwise outranks adoption.

The daemon must be running. Adoption that only reached a config file
would look identical to adoption that took effect, so a stopped daemon
is an error rather than a deferred write.`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCChannelAdopt(ctx, app, opts, args[0], args[1], owner)
			})
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "Account credited with the channel")

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// newIRCChannelReleaseCmd builds `wsm irc channel release`.
func newIRCChannelReleaseCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <workspace> <#channel>",
		Short: "Ask the daemon to drop a channel it adopted",
		Long: `Stop capturing a channel wsm does not own.

The name is written to irc.released in the workspace-local override and
any matching irc.adopted entry is dropped, so the request cannot be
undone by the adoption that preceded it. The running daemon is then
signalled and its discovery pass parts the channel.

Releasing does not touch the channel itself: wsm never owned it, so
there is nothing to unregister. Only wsm's own participation ends.

The daemon must be running, for the same reason adopt requires it.`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCChannelRelease(ctx, app, opts, args[0], args[1])
			})
		},
	}

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// newIRCChannelPurgeCmd builds `wsm irc channel purge`.
func newIRCChannelPurgeCmd(opts *globalOptions) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "purge <workspace> <#channel>",
		Short: "Drop a channel registration on the server",
		Long: `Remove a channel's registration entirely.

This is a server operation, not a configuration edit: the registration
is dropped and the channel emptied. On ergo that is CS PURGE, a two-step
confirmation the provider completes on wsm's behalf. A provider without
the capability reports the operation as unsupported (exit 7) rather than
appearing to succeed.

There is no unpurge. The purge is gated behind --confirm like every
other destructive verb, and --force stands in for --confirm=yes.`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return bridgeForceToConfirm(cmd, force)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCChannelPurge(ctx, cmd, app, opts, args[0], args[1])
			})
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Proceed without the destructive confirmation")

	cli.SetSideEffect(cmd, cli.SideEffectDestructive)
	cli.SetIdempotency(cmd, cli.IdempotencyNo)
	return cmd
}

// ircChannelDoc is the `wsm irc channel add|rm --json` document.
type ircChannelDoc struct {
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Action    string `json:"action"`
	// Config is the file the override was written to.
	Config string `json:"config"`
	// Synced reports whether a running daemon was asked to reconcile.
	Synced bool `json:"synced"`
	// Purged reports the registration drop, with a reason when the
	// provider could not do it.
	Purged bool `json:"purged"`
	// Reason explains a purge that did not happen.
	Reason string `json:"reason,omitempty"`
}

// runIRCChannelAdd declares a channel in the workspace override.
func runIRCChannelAdd(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	channel string,
	flags ircChannelFlags,
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

	name := strings.TrimSpace(channel)
	if err := validateChannelName(name); err != nil {
		return err
	}

	template, err := flags.template(name)
	if err != nil {
		return err
	}

	if !flags.Force {
		if err := refuseSeizure(rt, profile, name); err != nil {
			return err
		}
	}

	configPath := workspaceConfigPathFor(profile)
	if done, derr := previewDryRun(cmd,
		map[string]any{"workspace": rt.ws.Name, "channel": name},
		cli.Effect{
			Kind:       "create",
			Target:     "irc-channel:" + name,
			Reversible: true,
			Detail:     fmt.Sprintf("declare %s in %s", name, configPath),
		},
	); done {
		return derr
	}

	if err := upsertChannelTemplate(configPath, template); err != nil {
		return err
	}

	doc := ircChannelDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Channel:   name,
		Action:    "add",
		Config:    configPath,
		Synced:    signalReconcile(rt),
	}
	return renderIRCChannelResult(opts, doc)
}

// runIRCChannelRm drops a channel from the workspace override.
func runIRCChannelRm(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	channel string,
	flags ircChannelFlags,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if err := requireCapability(app, workspace.CapIRCEnable); err != nil {
		return err
	}
	if flags.Purge {
		if err := requireCapability(app, workspace.CapIRCPurge); err != nil {
			return err
		}
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	name := strings.TrimSpace(channel)
	if err := validateChannelName(name); err != nil {
		return err
	}

	configPath := workspaceConfigPathFor(profile)
	if flags.Purge {
		if done, derr := previewDryRun(cmd,
			map[string]any{"workspace": rt.ws.Name, "channel": name, "purge": true},
			cli.Effect{
				Kind:       "delete",
				Target:     "irc-channel:" + name,
				Reversible: false,
				Detail:     fmt.Sprintf("drop %s and purge its registration", name),
			},
		); done {
			return derr
		}
		// Gated after the preview, which destroys nothing.
		if gerr := gatePurge(cmd); gerr != nil {
			return gerr
		}
	} else if done, derr := previewDryRun(cmd,
		map[string]any{"workspace": rt.ws.Name, "channel": name},
		cli.Effect{
			Kind:       "delete",
			Target:     "irc-channel:" + name,
			Reversible: true,
			Detail:     fmt.Sprintf("stop maintaining %s", name),
		},
	); done {
		return derr
	}

	if err := removeChannelTemplate(configPath, name); err != nil {
		return err
	}

	doc := ircChannelDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Channel:   name,
		Action:    "rm",
		Config:    configPath,
	}

	if flags.Purge {
		doc.Purged, doc.Reason = purgeChannel(ctx, rt, profile, name)
		if !doc.Purged && doc.Reason == "" {
			doc.Reason = "unsupported"
		}
	}

	// Reconcile last: the template is gone and, on a purge, so is the
	// registration, so the daemon's next pass sees the world it must
	// converge to rather than one this command is still changing.
	doc.Synced = signalReconcile(rt)
	return renderIRCChannelResult(opts, doc)
}

// ircAdoptDoc is the `wsm irc channel adopt|release --json` document.
type ircAdoptDoc struct {
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Action    string `json:"action"`
	// Owner is the account credited with an adopted channel.
	Owner string `json:"owner,omitempty"`
	// Config is the file the request was written to.
	Config string `json:"config"`
	// Synced reports that the daemon was signalled. It is always true in
	// a successful run: a stopped daemon fails the command outright.
	Synced bool `json:"synced"`
}

// runIRCChannelAdopt records an adoption request and signals the daemon.
//
// Nothing here joins the channel. The write says what the operator
// wants and the SIGHUP asks the daemon to act on it; the daemon's
// discovery pass does the joining and the Discovered bookkeeping. That
// division is the point: the CLI's connection dies with the process, so
// a channel it joined would be captured by nothing.
func runIRCChannelAdopt(
	ctx context.Context,
	app *appContext,
	opts *globalOptions,
	ref string,
	channel string,
	owner string,
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

	name := strings.TrimSpace(channel)
	if err := validateChannelName(name); err != nil {
		return err
	}
	if err := requireRunningDaemon(rt, "adopt"); err != nil {
		return err
	}

	configPath := workspaceConfigPathFor(profile)
	if err := adoptChannelRequest(configPath, name, strings.TrimSpace(owner)); err != nil {
		return err
	}

	doc := ircAdoptDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Channel:   name,
		Action:    "adopt",
		Owner:     strings.TrimSpace(owner),
		Config:    configPath,
		Synced:    signalReconcile(rt),
	}
	return renderIRCAdoptResult(opts, doc)
}

// runIRCChannelRelease records a release request and signals the daemon.
func runIRCChannelRelease(
	ctx context.Context,
	app *appContext,
	opts *globalOptions,
	ref string,
	channel string,
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

	name := strings.TrimSpace(channel)
	if err := validateChannelName(name); err != nil {
		return err
	}
	if err := requireRunningDaemon(rt, "release"); err != nil {
		return err
	}

	configPath := workspaceConfigPathFor(profile)
	if err := releaseChannelRequest(configPath, name); err != nil {
		return err
	}

	doc := ircAdoptDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Channel:   name,
		Action:    "release",
		Config:    configPath,
		Synced:    signalReconcile(rt),
	}
	return renderIRCAdoptResult(opts, doc)
}

// runIRCChannelPurge drops a channel registration on the server.
func runIRCChannelPurge(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	channel string,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if err := requireCapability(app, workspace.CapIRCEnable); err != nil {
		return err
	}
	if err := requireCapability(app, workspace.CapIRCPurge); err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	name := strings.TrimSpace(channel)
	if err := validateChannelName(name); err != nil {
		return err
	}

	if done, derr := previewDryRun(cmd,
		map[string]any{"workspace": rt.ws.Name, "channel": name, "purge": true},
		cli.Effect{
			Kind:       "delete",
			Target:     "irc-channel:" + name,
			Reversible: false,
			Detail:     fmt.Sprintf("drop the registration for %s", name),
		},
	); done {
		return derr
	}
	// Gated after the preview, which destroys nothing.
	if gerr := gatePurge(cmd); gerr != nil {
		return gerr
	}

	purged, reason := purgeChannel(ctx, rt, profile, name)
	if !purged {
		// A provider that cannot purge is a prerequisite failure, not a
		// runtime one: the operator fixes it by using a server that
		// supports the operation, which is exactly what exit 7 means.
		if reason == "" || reason == "unsupported" {
			return workspace.NewErrorPrerequisitePermanent(fmt.Sprintf(
				"provider %q cannot purge channel registrations: "+
					"drop %s on the server directly",
				profile.Provider, name))
		}
		return &output.Error{
			Code:         output.CodeGeneric,
			Message:      fmt.Sprintf("purge %s: %s", name, reason),
			SuggestedFix: "check the server is reachable and the bot is opered up",
			ExitCode:     1,
		}
	}

	doc := ircChannelDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Channel:   name,
		Action:    "purge",
		Config:    workspaceConfigPathFor(profile),
		Purged:    true,
		Synced:    signalReconcile(rt),
	}
	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}
	fmt.Fprintf(os.Stderr, "Purged the registration for %s\n", doc.Channel)
	return nil
}

// requireRunningDaemon refuses adopt and release without a live daemon.
//
// Both verbs are requests to a daemon rather than edits that stand on
// their own: the join and the part happen in the discovery pass, so a
// write with nothing running would report success for work that nobody
// is going to do. add and rm can defer to the next start because their
// templates are read at startup; an adoption request is not.
func requireRunningDaemon(rt *ircRuntime, verb string) error {
	if _, running := daemonState(rt.paths); running {
		return nil
	}
	return &output.Error{
		Code: output.CodeGeneric,
		Message: fmt.Sprintf(
			"no irc daemon is running for %q: %s needs one to join or part the channel",
			rt.ws.Name, verb),
		SuggestedFix: fmt.Sprintf("run `wsm irc start %s` first", rt.ws.Name),
		ExitCode:     1,
	}
}

// renderIRCAdoptResult renders an adopt or release outcome.
func renderIRCAdoptResult(opts *globalOptions, doc ircAdoptDoc) error {
	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}

	// §6.3: acknowledgements are non-data → stderr.
	switch doc.Action {
	case "adopt":
		fmt.Fprintf(os.Stderr,
			"Asked the daemon to adopt %s; it joins on the next discovery pass\n",
			doc.Channel)
	default:
		fmt.Fprintf(os.Stderr,
			"Asked the daemon to release %s; it parts on the next discovery pass\n",
			doc.Channel)
	}
	return nil
}

// adoptChannelRequest records an adoption in the workspace override.
//
// The name is also dropped from irc.released, which outranks adoption in
// the daemon's classifier: leaving it there would make the adoption a
// no-op the operator could not see the cause of.
func adoptChannelRequest(path, name, owner string) error {
	return editIRCBlock(path, func(ircNode *yaml.Node) error {
		adopted, err := readAdopted(ircNode)
		if err != nil {
			return err
		}

		replaced := false
		for i := range adopted {
			if adopted[i].Name == name {
				adopted[i].Owner = owner
				replaced = true
				break
			}
		}
		if !replaced {
			adopted = append(adopted, irc.AdoptedChannel{Name: name, Owner: owner})
		}

		released, err := readReleased(ircNode)
		if err != nil {
			return err
		}
		kept := make([]string, 0, len(released))
		for _, entry := range released {
			if entry != name {
				kept = append(kept, entry)
			}
		}

		if err := setAdopted(ircNode, adopted); err != nil {
			return err
		}
		return setReleased(ircNode, kept)
	})
}

// releaseChannelRequest records a release in the workspace override.
//
// The matching adoption is dropped rather than left to be outranked.
// Both would produce the same behaviour today, but a config that still
// asks for a channel it also releases is a contradiction an operator
// would have to reason about; removing it keeps the file readable.
func releaseChannelRequest(path, name string) error {
	return editIRCBlock(path, func(ircNode *yaml.Node) error {
		released, err := readReleased(ircNode)
		if err != nil {
			return err
		}
		found := false
		for _, entry := range released {
			if entry == name {
				found = true
				break
			}
		}
		if !found {
			released = append(released, name)
		}

		adopted, err := readAdopted(ircNode)
		if err != nil {
			return err
		}
		kept := make([]irc.AdoptedChannel, 0, len(adopted))
		for _, entry := range adopted {
			if entry.Name != name {
				kept = append(kept, entry)
			}
		}

		if err := setReleased(ircNode, released); err != nil {
			return err
		}
		return setAdopted(ircNode, kept)
	})
}

// editIRCBlock applies edit to the irc block of the config at path.
//
// This is editChannelTemplates' sibling for the adoption lists, and it
// goes through the same node tree for the same reason: a typed
// round-trip would serialise only the fields wsm models and delete
// every unrelated key, comment and ordering in the operator's file. The
// write is atomic so an interrupted adopt cannot leave a truncated
// config the workspace no longer loads.
func editIRCBlock(path string, edit func(ircNode *yaml.Node) error) error {
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspace.NewErrorBackend(fmt.Sprintf("read %s", path), err)
	}

	var doc yaml.Node
	if len(raw) > 0 {
		if uerr := yaml.Unmarshal(raw, &doc); uerr != nil {
			return workspace.NewErrorBackend(fmt.Sprintf("parse %s", path), uerr)
		}
	}
	root := documentRoot(&doc)

	ircNode := mappingValue(root, "irc")
	if ircNode == nil {
		ircNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "irc"},
			ircNode)
	}

	if eerr := edit(ircNode); eerr != nil {
		return eerr
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("encode %s", path), err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("create %s", dir), err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("create temp file in %s", dir), err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return workspace.NewErrorBackend("chmod temp file", err)
	}
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return workspace.NewErrorBackend(fmt.Sprintf("write %s", path), err)
	}
	if err := tmp.Close(); err != nil {
		return workspace.NewErrorBackend("close temp file", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("install %s", path), err)
	}
	return nil
}

// readAdopted decodes irc.adopted from the irc block.
func readAdopted(ircNode *yaml.Node) ([]irc.AdoptedChannel, error) {
	node := mappingValue(ircNode, "adopted")
	if node == nil {
		return nil, nil
	}
	var out []irc.AdoptedChannel
	if err := node.Decode(&out); err != nil {
		return nil, workspace.NewErrorBackend("parse irc.adopted", err)
	}
	return out, nil
}

// readReleased decodes irc.released from the irc block.
func readReleased(ircNode *yaml.Node) ([]string, error) {
	node := mappingValue(ircNode, "released")
	if node == nil {
		return nil, nil
	}
	var out []string
	if err := node.Decode(&out); err != nil {
		return nil, workspace.NewErrorBackend("parse irc.released", err)
	}
	return out, nil
}

// setAdopted writes irc.adopted back, dropping the key when empty.
//
// An empty list is removed rather than written as `adopted: []` for the
// same reason the channel list is: the workspace override replaces the
// list wholesale, so an empty one is a statement about this workspace
// rather than the absence of a statement.
func setAdopted(ircNode *yaml.Node, entries []irc.AdoptedChannel) error {
	if len(entries) == 0 {
		removeMappingKey(ircNode, "adopted")
		return nil
	}
	return setSequence(ircNode, "adopted", entries)
}

// setReleased writes irc.released back, dropping the key when empty.
func setReleased(ircNode *yaml.Node, entries []string) error {
	if len(entries) == 0 {
		removeMappingKey(ircNode, "released")
		return nil
	}
	return setSequence(ircNode, "released", entries)
}

// setSequence encodes value into key on the irc block.
func setSequence(ircNode *yaml.Node, key string, value any) error {
	var encoded yaml.Node
	if err := encoded.Encode(value); err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("encode irc.%s", key), err)
	}
	if existing := mappingValue(ircNode, key); existing != nil {
		*existing = encoded
		return nil
	}
	ircNode.Content = append(ircNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&encoded)
	return nil
}

// purgeChannel drops a channel registration through the provider.
//
// A provider without the capability is not a failure: the channel has
// already been dropped from the configuration, which is the part wsm
// owns. The outcome is reported as {purged:false, reason:"unsupported"}
// so a caller can tell "we could not" from "we did not try".
func purgeChannel(
	ctx context.Context,
	rt *ircRuntime,
	profile irc.Profile,
	channel string,
) (bool, string) {
	session, err := openIRCSession(ctx, rt, profile)
	if err != nil {
		return false, fmt.Sprintf("session unavailable: %v", err)
	}
	defer session.Close()

	directory := session.providers.Directory
	if !directory.Caps().Has(provider.CapPurgeChannel) {
		return false, "unsupported"
	}
	if perr := directory.Purge(ctx, channel); perr != nil {
		if errors.Is(perr, provider.ErrUnsupported) {
			return false, "unsupported"
		}
		return false, perr.Error()
	}
	return true, ""
}

// signalReconcile asks a running daemon to reconcile.
//
// A stopped daemon is not an error here, unlike `wsm irc sync`: the
// configuration edit has already landed and is what the next start will
// read. Reporting synced=false says the edit is pending rather than
// lost.
func signalReconcile(rt *ircRuntime) bool {
	status, running := daemonState(rt.paths)
	if !running {
		return false
	}
	return signalDaemon(status.PID, syscall.SIGHUP) == nil
}

// template builds the channel template these flags describe.
func (f ircChannelFlags) template(name string) (irc.ChannelTemplate, error) {
	tmpl := irc.ChannelTemplate{
		Name:       name,
		MemberList: f.MemberList,
		Owner:      strings.TrimSpace(f.Owner),
		Modes:      strings.TrimSpace(f.Modes),
		Topic:      f.Topic,
	}

	if members := strings.TrimSpace(f.Members); members != "" {
		switch irc.Members(members) {
		case irc.MembersAll, irc.MembersPrincipals, irc.MembersList:
			tmpl.Members = irc.Members(members)
		default:
			return irc.ChannelTemplate{}, &output.Error{
				Code:         output.CodeUsage,
				Message:      fmt.Sprintf("--members %q: must be all|principals|list", members),
				SuggestedFix: "pass --members all, principals or list",
				ExitCode:     2,
			}
		}
	}
	if tmpl.Members == irc.MembersList && len(tmpl.MemberList) == 0 {
		return irc.ChannelTemplate{}, &output.Error{
			Code:         output.CodeUsage,
			Message:      "--members list needs --member-list <profile,...>",
			SuggestedFix: "pass --member-list with at least one profile id",
			ExitCode:     2,
		}
	}

	guests, err := parseGuests(f.Guests)
	if err != nil {
		return irc.ChannelTemplate{}, err
	}
	tmpl.Guests = guests

	if borrowed := strings.TrimSpace(f.Borrowed); borrowed != "" {
		b, berr := parseBorrowed(borrowed, tmpl.Members)
		if berr != nil {
			return irc.ChannelTemplate{}, berr
		}
		tmpl.Borrowed = b
	}

	return tmpl, nil
}

// parseGuests parses --guest <profile>[:role] entries.
//
// The role defaults to voice rather than op: a guest allow-list entry
// grants a persistent privilege to somebody outside this workspace, and
// the safe default for that is the one that lets them speak, not the one
// that lets them reconfigure the channel.
func parseGuests(raw []string) ([]irc.Guest, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	guests := make([]irc.Guest, 0, len(raw))
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		profileID, roleName, hasRole := strings.Cut(entry, ":")
		profileID = strings.TrimSpace(profileID)
		if profileID == "" {
			return nil, &output.Error{
				Code:         output.CodeUsage,
				Message:      fmt.Sprintf("--guest %q: missing profile id", entry),
				SuggestedFix: "pass --guest <profile>[:voice|op]",
				ExitCode:     2,
			}
		}

		role := irc.RoleVoice
		if hasRole {
			switch irc.Role(strings.TrimSpace(roleName)) {
			case irc.RoleVoice:
				role = irc.RoleVoice
			case irc.RoleOp:
				role = irc.RoleOp
			default:
				return nil, &output.Error{
					Code: output.CodeUsage,
					Message: fmt.Sprintf(
						"--guest %q: role must be voice or op", entry),
					SuggestedFix: "pass --guest <profile>:voice or <profile>:op",
					ExitCode:     2,
				}
			}
		}
		guests = append(guests, irc.Guest{Profile: profileID, Role: role})
	}
	return guests, nil
}

// parseBorrowed parses --borrowed <host-ws>:<#channel>.
func parseBorrowed(raw string, members irc.Members) (*irc.Borrowed, error) {
	host, channel, ok := strings.Cut(raw, ":")
	host = strings.TrimSpace(host)
	channel = strings.TrimSpace(channel)
	if !ok || host == "" || channel == "" {
		return nil, &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("--borrowed %q: expected <host-workspace>:<#channel>", raw),
			SuggestedFix: "pass --borrowed my-host:#their-channel",
			ExitCode:     2,
		}
	}
	if err := validateChannelName(channel); err != nil {
		return nil, err
	}
	if members == "" {
		members = irc.MembersAll
	}
	return &irc.Borrowed{Host: host, Channel: channel, Members: members}, nil
}

// validateChannelName refuses a name that is not a channel.
func validateChannelName(name string) error {
	if name == "" {
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      "channel name is required",
			SuggestedFix: "pass a channel, e.g. #team-ops",
			ExitCode:     2,
		}
	}
	if !strings.HasPrefix(name, "#") && !strings.HasPrefix(name, "&") {
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("channel %q must start with # or &", name),
			SuggestedFix: "prefix the name with #",
			ExitCode:     2,
		}
	}
	if strings.ContainsAny(name, " ,\x07") {
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("channel %q contains a character IRC forbids", name),
			SuggestedFix: "remove spaces and commas from the name",
			ExitCode:     2,
		}
	}
	return nil
}

// refuseSeizure blocks a channel that belongs to a member's own bot.
//
// Two sources, because they catch the same mistake at different times:
// Profile.Discovered is what the daemon has already adopted, and the
// creator globs are what it would adopt next. Declaring either as a
// template makes the next reconcile register it to the wsm bot and
// author roles in it — that is, take a channel away from the member who
// created it. A conflict rather than a usage error: the invocation is
// well-formed and collides with existing state.
func refuseSeizure(rt *ircRuntime, profile irc.Profile, name string) error {
	if _, adopted := profile.Discovered[name]; adopted {
		return workspace.NewErrorConflict(fmt.Sprintf(
			"channel %q was discovered and adopted from a channel creator: "+
				"declaring it would transfer it to the wsm bot; pass --force to do that deliberately",
			name,
		))
	}

	members, err := loadWorkspaceMembers(profile)
	if err != nil {
		// A members file that cannot be read means the namespaces
		// cannot be computed. Refusing here would block every add on a
		// workspace with a typo'd members.yaml, so the discovered-set
		// check above stands alone and this one is skipped.
		return nil
	}

	namespaces := creatorNamespaces(rt.cfg, rt.ws.Name, members)
	for _, label := range sortedKeys(namespaces) {
		glob := namespaces[label]
		if matchesGlob(glob, name) {
			return workspace.NewErrorConflict(fmt.Sprintf(
				"channel %q falls inside %s's creator namespace (%s): "+
					"declaring it would let wsm seize a channel that member registers; "+
					"pass --force to do that deliberately",
				name, label, glob,
			))
		}
	}
	return nil
}

// matchesGlob reports whether name matches a creator glob.
//
// The glob shape is fixed by irc.CreatorGlob: a literal stem followed by
// a single trailing '*'. Matching it with a prefix comparison rather
// than a pattern library keeps this honest about what the sweeper
// actually does; a general globber would accept patterns the sweeper
// would never produce and silently diverge from it.
func matchesGlob(glob, name string) bool {
	stem, ok := strings.CutSuffix(glob, "*")
	if !ok {
		return glob == name
	}
	return strings.HasPrefix(name, stem) && len(name) > len(stem)
}

// workspaceConfigPathFor reports the workspace-local config file to
// edit, whether or not it exists yet.
func workspaceConfigPathFor(profile irc.Profile) string {
	if existing := irc.WorkspaceConfigPath(profile.Root); existing != "" {
		return existing
	}
	return workspaceConfigFallback(profile.Root)
}

// workspaceConfigFallback is where a workspace override is created when
// the root has none yet.
//
// It matches what the daemon assumes when it looks for members.yaml, so
// a file created here is found by the daemon without a second
// convention.
func workspaceConfigFallback(root string) string {
	return filepath.Join(root, ".wsm", "config.yaml")
}

// upsertChannelTemplate writes one channel template into the workspace
// override, replacing an entry of the same name.
func upsertChannelTemplate(path string, tmpl irc.ChannelTemplate) error {
	return editChannelTemplates(path, func(existing []irc.ChannelTemplate) ([]irc.ChannelTemplate, error) {
		for i := range existing {
			if existing[i].Name == tmpl.Name {
				existing[i] = tmpl
				return existing, nil
			}
		}
		return append(existing, tmpl), nil
	})
}

// removeChannelTemplate drops a channel template from the override.
func removeChannelTemplate(path, name string) error {
	return editChannelTemplates(path, func(existing []irc.ChannelTemplate) ([]irc.ChannelTemplate, error) {
		out := make([]irc.ChannelTemplate, 0, len(existing))
		found := false
		for _, tmpl := range existing {
			if tmpl.Name == name {
				found = true
				continue
			}
			out = append(out, tmpl)
		}
		if !found {
			return nil, workspace.NewErrorNotFound(fmt.Sprintf(
				"channel %q is not declared in %s", name, path))
		}
		return out, nil
	})
}

// editChannelTemplates applies edit to the irc.channels list in the
// workspace config at path.
//
// The document is round-tripped through yaml.Node rather than through a
// typed struct. A typed round-trip would serialise only the fields wsm
// models, so every unrelated key in the operator's config file — other
// tools' blocks, comments, anything wsm's own Config does not declare —
// would be silently deleted by an edit to one channel list. The node
// tree preserves the rest of the document, including comments, and this
// function rewrites exactly one nested sequence.
func editChannelTemplates(
	path string,
	edit func([]irc.ChannelTemplate) ([]irc.ChannelTemplate, error),
) error {
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return workspace.NewErrorBackend(fmt.Sprintf("read %s", path), err)
	}

	var doc yaml.Node
	if len(raw) > 0 {
		if uerr := yaml.Unmarshal(raw, &doc); uerr != nil {
			return workspace.NewErrorBackend(fmt.Sprintf("parse %s", path), uerr)
		}
	}
	root := documentRoot(&doc)

	existing, err := readChannelTemplates(root)
	if err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("parse %s", path), err)
	}

	updated, err := edit(existing)
	if err != nil {
		return err
	}

	if err := setChannelTemplates(root, updated); err != nil {
		return workspace.NewErrorBackend("set irc.channels", err)
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("encode %s", path), err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("create %s", filepath.Dir(path)), err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return workspace.NewErrorBackend(fmt.Sprintf("write %s", path), err)
	}
	return nil
}

// documentRoot returns the mapping node an edit applies to, creating an
// empty one when the file was absent or held nothing.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

// readChannelTemplates decodes irc.channels from the document.
func readChannelTemplates(root *yaml.Node) ([]irc.ChannelTemplate, error) {
	ircNode := mappingValue(root, "irc")
	if ircNode == nil {
		return nil, nil
	}
	channels := mappingValue(ircNode, "channels")
	if channels == nil {
		return nil, nil
	}
	var out []irc.ChannelTemplate
	if err := channels.Decode(&out); err != nil {
		return nil, fmt.Errorf("irc.channels: %w", err)
	}
	return out, nil
}

// setChannelTemplates writes the channel list back into the document,
// creating the irc block when it is absent.
//
// An empty list removes the key rather than writing `channels: []`. The
// merge rule is that a workspace override replaces the global channel
// list wholesale when it declares one, so an empty list would mean "this
// workspace has no channels" — which is a different statement from "this
// workspace does not override the channel list", and the wrong one to
// leave behind after removing the last channel.
func setChannelTemplates(root *yaml.Node, templates []irc.ChannelTemplate) error {
	ircNode := mappingValue(root, "irc")
	if ircNode == nil {
		if len(templates) == 0 {
			return nil
		}
		ircNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "irc"},
			ircNode)
	}

	if len(templates) == 0 {
		removeMappingKey(ircNode, "channels")
		return nil
	}

	var encoded yaml.Node
	if err := encoded.Encode(templates); err != nil {
		return fmt.Errorf("encode irc.channels: %w", err)
	}
	if existing := mappingValue(ircNode, "channels"); existing != nil {
		*existing = encoded
		return nil
	}
	ircNode.Content = append(ircNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "channels"},
		&encoded)
	return nil
}

// mappingValue returns the value node for key in a mapping node.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// removeMappingKey drops a key and its value from a mapping node.
func removeMappingKey(node *yaml.Node, key string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}

// ircChannelRow is one row of `wsm irc channel list`.
type ircChannelRow struct {
	Name       string `table:"CHANNEL,priority=9"  json:"name"`
	Source     string `table:"SOURCE,priority=8"   json:"source"`
	Registered bool   `table:"REGISTERED,priority=7" json:"registered"`
	Members    int    `table:"MEMBERS,priority=6"  json:"members,omitempty"`
	Owner      string `table:"OWNER,priority=5"    json:"owner,omitempty"`
}

// ircChannelSections is the `wsm irc channel list --json` document.
//
// The sections answer four different questions an operator has about a
// channel, which a single flat list runs together: what wsm maintains
// here, what it borrows from another workspace's server, what it
// captures without owning, and what it has found and does not manage at
// all. Each is a separate array so a consumer can read one without
// filtering on a source string.
type ircChannelSections struct {
	// Managed are the channels this workspace's templates declare.
	Managed []ircChannelRow `json:"managed"`
	// Borrowed are templates pointing at another workspace's channel.
	Borrowed []ircChannelRow `json:"borrowed"`
	// Discovered are channels the daemon adopted, including peers it
	// lists but never touches.
	Discovered []ircChannelRow `json:"discovered"`
	// Unmanaged are registrations wsm found and does not own.
	Unmanaged []ircChannelRow `json:"unmanaged"`
}

// runIRCChannelList reports the workspace's channels.
func runIRCChannelList(ctx context.Context, app *appContext, opts *globalOptions, ref string) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	status, _ := daemonState(rt.paths)
	rows := mergeChannelRows(rt.cfg, rt.ws.Name, profile, status)

	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), sectionChannelRows(rows))
	}
	// The table stays one flat list carrying the source column: a
	// terminal reader scans rows, and four separate tables would cost
	// the alignment that makes them scannable.
	return output.Render(os.Stdout, output.Table, rows)
}

// sectionChannelRows partitions the merged rows into the four sections.
//
// The partition is driven by the source each row already carries, so
// the machine shape and the table cannot disagree about what a channel
// is. A peer channel belongs with discovered rather than unmanaged: wsm
// captures neither, but a peer has a known wsm founder and is never
// purge-eligible, which is the distinction unmanaged exists to draw.
func sectionChannelRows(rows []ircChannelRow) ircChannelSections {
	// Non-nil empties so the JSON shape carries four arrays whatever the
	// workspace holds; a consumer indexing a null would break on a
	// workspace that simply has no borrowed channels.
	out := ircChannelSections{
		Managed:    []ircChannelRow{},
		Borrowed:   []ircChannelRow{},
		Discovered: []ircChannelRow{},
		Unmanaged:  []ircChannelRow{},
	}
	for _, row := range rows {
		switch row.Source {
		case "borrowed":
			out.Borrowed = append(out.Borrowed, row)
		case "discovered", "peer":
			out.Discovered = append(out.Discovered, row)
		case "unmanaged":
			out.Unmanaged = append(out.Unmanaged, row)
		default:
			out.Managed = append(out.Managed, row)
		}
	}
	return out
}

// mergeChannelRows merges the configured templates with what the daemon
// observed.
//
// Configuration is the spine: a template with no runtime counterpart is
// still listed, as unregistered, because "declared but not yet applied"
// is a state an operator needs to see rather than a row to hide. The
// daemon's own view then fills in registration and occupancy, and the
// discovered, peer and unmanaged sets are appended with their source so
// a captured channel is never mistaken for a maintained one.
func mergeChannelRows(
	cfg irc.Config,
	wsName string,
	profile irc.Profile,
	status ircdaemon.Status,
) []ircChannelRow {
	registered := map[string]bool{}
	for _, ch := range channelReports(status) {
		registered[ch.Name] = ch.Registered
	}

	rows := make([]ircChannelRow, 0, len(cfg.Channels))
	seen := map[string]bool{}
	for _, tmpl := range cfg.Channels {
		name := tmpl.Name
		source := "template"
		if tmpl.Borrowed != nil {
			name = tmpl.Borrowed.Channel
			source = "borrowed"
		} else {
			name = irc.ChannelName(cfg.ChannelPrefix, tmpl.Name, wsName)
		}
		seen[name] = true
		rows = append(rows, ircChannelRow{
			Name:       name,
			Source:     source,
			Registered: registered[name],
			Owner:      tmpl.Owner,
		})
	}

	for _, ch := range status.Discovered {
		if seen[ch.Name] {
			continue
		}
		seen[ch.Name] = true
		rows = append(rows, ircChannelRow{
			Name:       ch.Name,
			Source:     "discovered",
			Registered: true,
			Owner:      ch.Owner,
		})
	}
	for _, ch := range status.Peer {
		if seen[ch.Name] {
			continue
		}
		seen[ch.Name] = true
		rows = append(rows, ircChannelRow{
			Name:       ch.Name,
			Source:     "peer",
			Registered: true,
			Owner:      ch.Founder,
		})
	}
	for _, ch := range status.Unmanaged {
		if seen[ch.Name] {
			continue
		}
		seen[ch.Name] = true
		rows = append(rows, ircChannelRow{
			Name:       ch.Name,
			Source:     "unmanaged",
			Registered: true,
			Owner:      ch.Founder,
		})
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// channelReports returns the last reconcile's channel reports, or none.
func channelReports(status ircdaemon.Status) []provisionChannelReport {
	if status.LastReconcile == nil {
		return nil
	}
	out := make([]provisionChannelReport, 0, len(status.LastReconcile.Channels))
	for _, ch := range status.LastReconcile.Channels {
		out = append(out, provisionChannelReport{Name: ch.Name, Registered: ch.Registered})
	}
	return out
}

// provisionChannelReport is the narrow slice of a reconcile channel
// report this listing needs.
type provisionChannelReport struct {
	Name       string
	Registered bool
}

// renderIRCChannelResult renders an add or rm outcome.
func renderIRCChannelResult(opts *globalOptions, doc ircChannelDoc) error {
	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}

	// §6.3: acknowledgements are non-data → stderr.
	switch doc.Action {
	case "add":
		fmt.Fprintf(os.Stderr, "Declared %s in %s\n", doc.Channel, doc.Config)
	default:
		fmt.Fprintf(os.Stderr, "Removed %s from %s\n", doc.Channel, doc.Config)
	}
	if doc.Purged {
		fmt.Fprintf(os.Stderr, "Purged the registration for %s\n", doc.Channel)
	} else if doc.Reason != "" {
		fmt.Fprintf(os.Stderr, "Registration not purged: %s\n", doc.Reason)
	}
	if !doc.Synced {
		fmt.Fprintln(os.Stderr,
			"No daemon running: the change applies on the next `wsm irc start`.")
	}
	return nil
}
