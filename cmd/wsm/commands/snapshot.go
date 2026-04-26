package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/workspace"
)

func newSnapshotGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "snapshot",
		Short:   "Snapshot operations",
		Aliases: []string{"snap", "s"},
	}
	cmd.AddCommand(
		newSnapshotListCmd(opts),
		newSnapshotGenerateCmd(opts),
		newSnapshotShowCmd(opts),
		newSnapshotUpdateCmd(opts),
		newSnapshotRemoveCmd(opts),
		newSnapshotContextCmd(opts),
	)
	return cmd
}

func newSnapshotListCmd(opts *globalOptions) *cobra.Command {
	var workspaceRefs []string
	var sessions []string
	var after, before, sortDir string
	var limit int
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "all", "l"},
		Short:   "List snapshots with filtering",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				var wsIDs []string
				for _, ref := range workspaceRefs {
					ws, err := app.manager.ResolveWorkspaceRef(ctx, ref)
					if err != nil {
						return err
					}
					wsIDs = append(wsIDs, ws.ID)
				}

				listOpts := backend.SnapshotListOptions{
					WorkspaceIDs: wsIDs,
					Sessions:     sessions,
					Limit:        limit,
					SortDir:      sortDir,
				}
				if after != "" {
					t, err := time.Parse(time.RFC3339, after)
					if err != nil {
						return workspace.NewErrorUsage("invalid after time format (RFC3339 required)")
					}
					listOpts.After = t
				}
				if before != "" {
					t, err := time.Parse(time.RFC3339, before)
					if err != nil {
						return workspace.NewErrorUsage("invalid before time format (RFC3339 required)")
					}
					listOpts.Before = t
				}

				snaps, err := app.manager.ListSnapshots(ctx, listOpts)
				if err != nil {
					return err
				}

				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, snaps)
				}

				if len(snaps) == 0 {
					if !opts.Quiet {
						fmt.Fprintln(os.Stdout, "No snapshots found.")
					}
					return nil
				}

				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tWORKSPACE\tSEQ\tNAME\tGENERATED")
				for _, s := range snaps {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
						shortID(s.ID), shortID(s.WorkspaceID), s.AtSeqNum, s.Name, humanAgo(s.GeneratedAt))
				}
				_ = tw.Flush()
				return nil
			})
		},
	}
	cmd.Flags().StringArrayVarP(&workspaceRefs, "workspace", "w", nil, "Filter by workspace ID or name (repeatable)")
	cmd.Flags().StringArrayVarP(&sessions, "session", "s", nil, "Filter by session ID (repeatable)")
	cmd.Flags().StringVar(&after, "after", "", "Snapshots after this time (RFC3339)")
	cmd.Flags().StringVar(&before, "before", "", "Snapshots before this time (RFC3339)")
	cmd.Flags().StringVar(&sortDir, "sort", "desc", "Sort direction: asc|desc")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "Maximum number of snapshots")
	_ = cmd.RegisterFlagCompletionFunc("sort", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"asc", "desc"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func newSnapshotGenerateCmd(opts *globalOptions) *cobra.Command {
	var sessionID, name string
	var isSeed bool
	cmd := &cobra.Command{
		Use:     "generate [workspace]",
		Short:   "Generate and persist a workspace snapshot",
		Aliases: []string{"gen", "g"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				var ws workspace.Workspace
				if len(args) == 0 {
					items, err := app.manager.List(ctx, backend.ListOptions{
						SortBy: "updated", SortDir: "desc", Limit: 1,
					})
					if err != nil {
						return err
					}
					if len(items) == 0 {
						return workspace.NewErrorNotFound("no workspaces available")
					}
					ws = items[0]
				} else {
					resolved, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
					if err != nil {
						return err
					}
					ws = *resolved
				}

				snap, err := app.manager.Snapshot(ctx, ws.ID, workspace.SnapshotOptions{
					SessionID: sessionID,
					Name:      name,
					IsSeed:    isSeed,
				})
				if err != nil {
					return err
				}
				if opts.Quiet {
					fmt.Fprintln(os.Stdout, snap.ID)
					return nil
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, snap)
				}
				fmt.Fprintf(os.Stdout, "Snapshot %s\n", snap.ID)
				if snap.Name != "" {
					fmt.Fprintf(os.Stdout, "  name:          %s\n", snap.Name)
				}
				fmt.Fprintf(os.Stdout, "  workspace:     %s (%s)\n", ws.Name, ws.ID)
				fmt.Fprintf(os.Stdout, "  at_seq_num:    %d\n", snap.AtSeqNum)
				fmt.Fprintf(os.Stdout, "  message_count: %d\n", snap.MessageCount)
				fmt.Fprintf(os.Stdout, "  tool_calls:    %d\n", snap.ToolCalls)
				fmt.Fprintf(os.Stdout, "  files_changed: %d\n", len(snap.FilesChanged))
				fmt.Fprintf(os.Stdout, "  devices:       %d\n", len(snap.Devices))
				fmt.Fprintf(os.Stdout, "  sessions:      %d\n", len(snap.Sessions))
				fmt.Fprintf(os.Stdout, "  is_seed:       %v\n", snap.IsSeed)
				fmt.Fprintf(os.Stdout, "  generated_at:  %s\n",
					snap.GeneratedAt.Format(time.RFC3339))
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "Filter by session ID")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Snapshot name/alias")
	cmd.Flags().BoolVar(&isSeed, "seed", false, "Mark as seed snapshot")
	return cmd
}

func newSnapshotShowCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "show <workspace>",
		Short:   "Show latest snapshot",
		Aliases: []string{"get", "s"},
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				snap, err := app.backend.GetLatestSnapshot(ctx, ws.ID)
				if err != nil {
					return err
				}
				if snap == nil {
					return workspace.NewErrorNotFound("no snapshots for this workspace")
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, snap)
				}
				fmt.Fprintf(os.Stdout, "Snapshot: %s (at seq %d)\n", snap.ID, snap.AtSeqNum)
				fmt.Fprintf(os.Stdout, "  Generated: %s\n", snap.GeneratedAt.Format(time.RFC3339))
				fmt.Fprintf(os.Stdout, "  Messages:  %d\n", snap.MessageCount)
				fmt.Fprintf(os.Stdout, "  Tools:     %d\n", snap.ToolCalls)
				fmt.Fprintf(os.Stdout, "  Files:     %d\n", len(snap.FilesChanged))
				return nil
			})
		},
	}
}

func newSnapshotContextCmd(opts *globalOptions) *cobra.Command {
	var recentEvents int
	cmd := &cobra.Command{
		Use:   "context <workspace>",
		Short: "Output markdown summary context for LLM",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				// Use FormatContext logic but wrapped in Manager if possible,
				// otherwise reconstruct here.
				out, err := app.manager.GenerateMarkdownContext(ctx, ws.ID)
				if err != nil {
					return err
				}
				fmt.Fprint(os.Stdout, out)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&recentEvents, "events", 5, "Number of recent events to include")
	return cmd
}

func newSnapshotUpdateCmd(opts *globalOptions) *cobra.Command {
	var name string
	var isSeed bool
	cmd := &cobra.Command{
		Use:     "update <snapshot-id-or-name>",
		Short:   "Update snapshot metadata",
		Aliases: []string{"rename"},
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				uOpts := workspace.UpdateSnapshotOptions{}
				if c.Flags().Changed("name") {
					uOpts.Name = &name
				}
				if c.Flags().Changed("seed") {
					uOpts.IsSeed = &isSeed
				}
				snap, err := app.manager.UpdateSnapshot(ctx, args[0], uOpts)
				if err != nil {
					return err
				}
				if opts.Quiet {
					fmt.Fprintln(os.Stdout, snap.ID)
					return nil
				}
				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, snap)
				}
				fmt.Fprintf(os.Stdout, "Updated snapshot %s\n", snap.ID)
				if snap.Name != "" {
					fmt.Fprintf(os.Stdout, "  name:    %s\n", snap.Name)
				}
				fmt.Fprintf(os.Stdout, "  is_seed: %v\n", snap.IsSeed)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Snapshot name/alias")
	cmd.Flags().BoolVar(&isSeed, "seed", false, "Mark as seed snapshot")
	return cmd
}

func newSnapshotRemoveCmd(opts *globalOptions) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "remove <snapshot-id-or-name>",
		Short:   "Remove a snapshot",
		Aliases: []string{"delete", "rm", "del"},
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				if !force {
					snap, err := app.manager.GetSnapshot(ctx, args[0])
					if err != nil {
						return err
					}
					if !stdoutIsTTY() {
						return workspace.NewErrorUsage("remove in non-TTY requires --force")
					}
					fmt.Fprintf(os.Stdout, "Remove snapshot %q? [y/N]: ", snap.ID)
					reader := bufio.NewReader(os.Stdin)
					line, _ := reader.ReadString('\n')
					line = strings.TrimSpace(strings.ToLower(line))
					if line != "y" && line != "yes" {
						fmt.Fprintln(os.Stdout, "Aborted.")
						return nil
					}
				}
				if err := app.manager.DeleteSnapshot(ctx, args[0]); err != nil {
					return err
				}
				if opts.Quiet {
					return nil
				}
				if shouldJSON(*opts) || !stdoutIsTTY() {
					return writeJSON(os.Stdout, map[string]any{"id": args[0], "deleted": true})
				}
				fmt.Fprintf(os.Stdout, "Removed snapshot %q\n", args[0])
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	return cmd
}
