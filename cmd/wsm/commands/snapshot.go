package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/output"
	"hop.top/kit/go/runtime/policy"
	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/workspace"
)

// snapshotRow is the table projection for `snapshot list`. Pre-truncates
// ID + WorkspaceID via shortID and pre-formats GeneratedAt via humanAgo
// for table mode (JSON/YAML go through the model.Snapshot directly).
type snapshotRow struct {
	ID        string `table:"ID,priority=9"        json:"id"`
	Name      string `table:"NAME,priority=7"      json:"name"`
	Workspace string `table:"WORKSPACE,priority=8" json:"workspace"`
	Seq       uint64 `table:"SEQ,priority=6"       json:"seq"`
	Generated string `table:"GENERATED,priority=4" json:"generated"`
}

func newSnapshotGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Snapshot operations",
		RunE:  groupOnlyRunE,
	}
	cmd.AddCommand(
		newSnapshotListCmd(opts),
		newSnapshotGenerateCmd(opts),
		newSnapshotShowCmd(opts),
		newSnapshotUpdateCmd(opts),
		newSnapshotDeleteCmd(opts),
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
		Use:   "list",
		Short: "List snapshots with filtering",
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

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, snaps)
				}

				if len(snaps) == 0 {
					if !opts.Quiet {
						// §6.3: empty-state hint is non-data → stderr.
						fmt.Fprintln(os.Stderr, "No snapshots found.")
					}
					return nil
				}

				rows := make([]snapshotRow, len(snaps))
				for i, s := range snaps {
					rows[i] = snapshotRow{
						ID:        shortID(s.ID),
						Name:      s.Name,
						Workspace: shortID(s.WorkspaceID),
						Seq:       s.AtSeqNum,
						Generated: humanAgo(s.GeneratedAt),
					}
				}
				return output.Render(os.Stdout, output.Table, rows)
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
		Use:   "generate [workspace]",
		Short: "Generate and persist a workspace snapshot",
		Args:  cobra.MaximumNArgs(1),
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
				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, snap)
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
		Use:   "show <workspace>",
		Short: "Show latest snapshot",
		Args:  cobra.ExactArgs(1),
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
				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, snap)
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
		Use:   "update <snapshot-id-or-name>",
		Short: "Update snapshot metadata",
		Args:  cobra.ExactArgs(1),
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
				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, snap)
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

func newSnapshotDeleteCmd(opts *globalOptions) *cobra.Command {
	var force bool
	var note string
	cmd := &cobra.Command{
		Use:   "delete <snapshot-id-or-name>",
		Short: "Delete a snapshot",
		Args:  cobra.ExactArgs(1),
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
					// §6.3: prompt is non-data → stderr.
					fmt.Fprintf(os.Stderr, "Remove snapshot %q? [y/N]: ", snap.ID)
					reader := bufio.NewReader(os.Stdin)
					line, _ := reader.ReadString('\n')
					line = strings.TrimSpace(strings.ToLower(line))
					if line != "y" && line != "yes" {
						fmt.Fprintln(os.Stderr, "Aborted.")
						return nil
					}
				}
				// Plumb --note into ctx via policy.ContextAttrsKey so
				// the runtime policy engine can read it during pre-event
				// veto. T-1287 wires the flag; T-1288 adds the default
				// policy that enforces note presence.
				if note != "" {
					ctx = context.WithValue(ctx, policy.ContextAttrsKey, map[string]any{
						"note": note,
					})
				}
				if err := app.manager.DeleteSnapshot(ctx, args[0]); err != nil {
					return err
				}
				if opts.Quiet {
					return nil
				}
				if opts.Format == output.JSON || opts.Format == output.YAML || !stdoutIsTTY() {
					format := opts.Format
					if format == "" || format == output.Table {
						format = output.JSON
					}
					return output.Render(os.Stdout, format,
						map[string]any{"id": args[0], "deleted": true})
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Removed snapshot %q\n", args[0])
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	cmd.Flags().StringVarP(&note, "note", "n", "", "Reason note (recorded for audit; future policies may require it)")
	return cmd
}
