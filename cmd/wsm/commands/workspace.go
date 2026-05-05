package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/output"
	"hop.top/kit/go/runtime/policy"
	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/model"
	"hop.top/wsm/pkg/workspace"
)

// groupOnlyRunE makes a group command (parent of subcommands) error on
// unknown args while still showing help for bare invocation.
//
// Why a RunE + manual arg check (not cobra.NoArgs alone): without a
// RunE, cobra short-circuits to flag.ErrHelp before Args validation
// (cobra/command.go: !c.Runnable() → ErrHelp) — so the bug repros even
// with cobra.NoArgs set. Returning the unknown-command error as
// workspace.NewErrorUsage routes through main's exitCodeForError to
// produce exit 2, per CLI conventions §8.1.
func groupOnlyRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return workspace.NewErrorUsage(
			fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath()))
	}
	return cmd.Help()
}

// workspaceRow is the table projection for `workspace list`. The model
// Workspace tags don't include event/snapshot counts (computed per-row),
// so we build a row struct here.
type workspaceRow struct {
	ID           string `table:"ID,priority=9"           json:"id"`
	Name         string `table:"NAME,priority=9"         json:"name"`
	Status       string `table:"STATUS,priority=7"       json:"status"`
	Events       uint64 `table:"EVENTS,priority=5"       json:"events"`
	Snaps        uint64 `table:"SNAPS,priority=4"        json:"snaps"`
	LastActivity string `table:"LAST ACTIVITY,priority=4" json:"last_activity"`
}

// eventRow is the table projection for `workspace show --events` and
// `event list`. Mirrors model.Event's table-relevant fields with shortID
// applied to DeviceID/SessionID for table mode.
type eventRow struct {
	SeqNum    uint64 `table:"SEQ,priority=9"       json:"seq_num"`
	Type      string `table:"TYPE,priority=8"      json:"type"`
	Timestamp string `table:"TIMESTAMP,priority=6" json:"timestamp"`
	Device    string `table:"DEVICE,priority=4"    json:"device"`
	Session   string `table:"SESSION,priority=3"   json:"session"`
}

func newWorkspaceGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Workspace operations",
		RunE:  groupOnlyRunE,
	}
	cmd.AddCommand(
		newWorkspaceAddCmd(opts),
		newWorkspaceListCmd(opts),
		newWorkspaceShowCmd(opts),
		newWorkspaceArchiveCmd(opts),
		newWorkspaceUnarchiveCmd(opts),
		newWorkspaceDeleteCmd(opts),
		newWorkspaceConfigCmd(opts),
	)
	return cmd
}

func newWorkspaceAddCmd(opts *globalOptions) *cobra.Command {
	var description string
	var tags []string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.Create(ctx, args[0], workspace.CreateOptions{Description: description, Tags: tags})
				if err != nil {
					return err
				}
				if opts.Quiet {
					fmt.Fprintln(os.Stdout, ws.ID)
					return nil
				}

				if opts.Format == output.JSON || opts.Format == output.YAML || !stdoutIsTTY() {
					format := opts.Format
					if format == "" || format == output.Table {
						format = output.JSON
					}
					payload := map[string]any{"id": ws.ID, "created_at": ws.CreatedAt}
					return output.Render(os.Stdout, format, payload)
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Created workspace %q (%s)\n", ws.Name, ws.ID)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "Workspace description")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "Tags (repeatable)")
	return cmd
}

func newWorkspaceListCmd(opts *globalOptions) *cobra.Command {
	var status, sortBy string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				sortDir := "desc"
				if strings.EqualFold(sortBy, "name") {
					sortDir = "asc"
				}
				listOpts := backend.ListOptions{
					SortBy:  sortBy,
					SortDir: sortDir,
				}
				if status != "" {
					s := model.Status(status)
					listOpts.Status = &s
				}
				items, err := app.manager.List(ctx, listOpts)
				if err != nil {
					return err
				}

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, items)
				}

				if len(items) == 0 {
					if !opts.Quiet {
						// §6.3: empty-state hint is non-data → stderr.
						fmt.Fprintln(os.Stderr, "No workspaces yet.")
						fmt.Fprintln(os.Stderr)
						fmt.Fprintln(os.Stderr, "  Create your first workspace:")
						fmt.Fprintln(os.Stderr, "    wsm workspace add my-project")
					}
					return nil
				}

				if opts.Quiet {
					for _, ws := range items {
						fmt.Fprintln(os.Stdout, ws.ID)
					}
					return nil
				}

				rows := make([]workspaceRow, len(items))
				for i, ws := range items {
					ec, _ := app.manager.EventCount(ctx, ws.ID)
					sc, _ := app.manager.SnapshotCount(ctx, ws.ID)
					rows[i] = workspaceRow{
						ID:           shortID(ws.ID),
						Name:         ws.Name,
						Status:       string(ws.Status),
						Events:       ec,
						Snaps:        sc,
						LastActivity: humanAgo(ws.UpdatedAt),
					}
				}
				return output.Render(os.Stdout, output.Table, rows)
			})
		},
	}
	cmd.Flags().StringVarP(&status, "status", "s", "", "Filter by status: active|archived")
	cmd.Flags().StringVar(&sortBy, "sort", "updated", "Sort by: name|created|updated")
	_ = cmd.RegisterFlagCompletionFunc("status", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"active", "archived"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("sort", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"created", "updated", "name"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func newWorkspaceShowCmd(opts *globalOptions) *cobra.Command {
	var eventsAll, showDevices bool
	cmd := &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show workspace details",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}

				limit := 5
				if eventsAll {
					limit = 0
				}
				events, _ := app.manager.Events(ctx, ws.ID, backend.EventListOptions{Limit: limit})
				latestSnap, _ := app.backend.GetLatestSnapshot(ctx, ws.ID)

				if opts.Format == output.JSON || opts.Format == output.YAML {
					ec, _ := app.manager.EventCount(ctx, ws.ID)
					sc, _ := app.manager.SnapshotCount(ctx, ws.ID)
					payload := map[string]any{
						"workspace":      ws,
						"event_count":    ec,
						"snapshot_count": sc,
						"events":         events,
						"snapshot":       latestSnap,
					}
					return output.Render(os.Stdout, opts.Format, payload)
				}

				fmt.Fprintf(os.Stdout, "Workspace: %s\n", ws.Name)
				fmt.Fprintf(os.Stdout, "  ID:          %s\n", ws.ID)
				fmt.Fprintf(os.Stdout, "  Status:      %s\n", ws.Status)
				if ws.Description != "" {
					fmt.Fprintf(os.Stdout, "  Description: %s\n", ws.Description)
				}
				if len(ws.Tags) > 0 {
					fmt.Fprintf(os.Stdout, "  Tags:        %s\n", strings.Join(ws.Tags, ", "))
				}
				fmt.Fprintf(os.Stdout, "  Created:     %s\n", ws.CreatedAt.Format(time.RFC3339))
				fmt.Fprintf(os.Stdout, "  Updated:     %s\n", ws.UpdatedAt.Format(time.RFC3339))
				if ws.ArchivedAt != nil {
					fmt.Fprintf(os.Stdout, "  Archived:    %s\n", ws.ArchivedAt.Format(time.RFC3339))
				}

				ec, _ := app.manager.EventCount(ctx, ws.ID)
				sc, _ := app.manager.SnapshotCount(ctx, ws.ID)
				fmt.Fprintf(os.Stdout, "\nStatistics:\n")
				fmt.Fprintf(os.Stdout, "  Total Events:    %d\n", ec)
				fmt.Fprintf(os.Stdout, "  Total Snapshots: %d\n", sc)

				var devices, sessions []string
				if latestSnap != nil {
					fmt.Fprintf(os.Stdout, "\nLatest Snapshot: %s\n", latestSnap.ID)
					fmt.Fprintf(os.Stdout, "  At Seq:        %d\n", latestSnap.AtSeqNum)
					fmt.Fprintf(os.Stdout, "  Messages:      %d\n", latestSnap.MessageCount)
					fmt.Fprintf(os.Stdout, "  Files Changed: %d\n", len(latestSnap.FilesChanged))
					devices = latestSnap.Devices
					sessions = latestSnap.Sessions
				} else if ec > 0 {
					// Fallback: extract from replayed events if no snapshot exists
					allEvents, _ := app.manager.Events(ctx, ws.ID, backend.EventListOptions{})
					tempSnap := model.GenerateSnapshot(*ws, allEvents, 0)
					devices = tempSnap.Devices
					sessions = tempSnap.Sessions
				}

				if len(devices) > 0 {
					fmt.Fprintf(os.Stdout, "  Devices:         %d\n", len(devices))
				}

				if showDevices && len(devices) > 0 {
					fmt.Fprintln(os.Stdout, "\nDEVICES")
					for _, d := range devices {
						fmt.Fprintln(os.Stdout, " -", d)
					}
				}
				if showDevices && len(sessions) > 0 {
					fmt.Fprintln(os.Stdout, "\nSESSIONS")
					for _, s := range sessions {
						fmt.Fprintln(os.Stdout, " -", s)
					}
				}

				if len(events) > 0 {
					fmt.Fprintf(os.Stdout, "\nEVENTS:\n")
					rows := make([]eventRow, len(events))
					for i, ev := range events {
						devID := ev.DeviceID
						if devID == "" {
							devID = "unknown"
						}
						rows[i] = eventRow{
							SeqNum:    ev.SeqNum,
							Type:      string(ev.Type),
							Timestamp: ev.Timestamp.Format("15:04:05"),
							Device:    shortID(devID),
							Session:   shortID(ev.SessionID),
						}
					}
					_ = output.Render(os.Stdout, output.Table, rows)
				}

				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&eventsAll, "events", false, "Show all events instead of last 5")
	// Support both flags for backward compatibility/tests
	cmd.Flags().BoolVar(&eventsAll, "events-all", false, "Show all events instead of last 5")
	cmd.Flags().BoolVar(&showDevices, "devices", false, "Show attached devices")
	return cmd
}

func newWorkspaceArchiveCmd(opts *globalOptions) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "archive <workspace>",
		Short: "Mark a workspace as archived (read-only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
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
				if err := app.manager.Archive(ctx, ws.ID); err != nil {
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
						map[string]any{"id": ws.ID, "status": "archived"})
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Archived workspace %q\n", ws.Name)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&note, "note", "n", "", "Reason note (recorded for audit; future policies may require it)")
	return cmd
}

func newWorkspaceUnarchiveCmd(opts *globalOptions) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "unarchive <workspace>",
		Short: "Restore an archived workspace to active status",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
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
				if err := app.manager.Unarchive(ctx, ws.ID); err != nil {
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
						map[string]any{"id": ws.ID, "status": "active"})
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Unarchived workspace %q\n", ws.Name)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&note, "note", "n", "", "Reason note (recorded for audit; future policies may require it)")
	return cmd
}

func newWorkspaceDeleteCmd(opts *globalOptions) *cobra.Command {
	var force, dryRun bool
	var note string
	cmd := &cobra.Command{
		Use:   "delete <workspace>",
		Short: "Permanently delete a workspace and its history",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				preview, err := app.manager.DeletePreview(ctx, args[0])
				if err != nil {
					return err
				}

				if dryRun {
					if opts.Format == output.JSON || opts.Format == output.YAML {
						return output.Render(os.Stdout, opts.Format,
							map[string]any{"dry_run": true, "preview": preview})
					}
					fmt.Fprintf(os.Stdout, "Would remove workspace %q (%s):\n", preview.WorkspaceName, preview.WorkspaceID)
					fmt.Fprintf(os.Stdout, "  - %d events\n", preview.EventCount)
					fmt.Fprintf(os.Stdout, "  - %d snapshots\n", preview.SnapshotCount)
					return nil
				}

				if !force {
					line := ""
					if runtime.GOOS == "darwin" {
						line = strings.TrimSpace(strings.ToLower(os.Getenv("WSM_DELETE_CONFIRM")))
					}
					if !stdoutIsTTY() && line == "" {
						return workspace.NewErrorUsage("remove in non-TTY requires --force")
					}
					if line == "" {
						// §6.3: prompt is non-data → stderr.
						fmt.Fprintf(os.Stderr, "Remove workspace %q? [y/N]: ", preview.WorkspaceName)
						reader := bufio.NewReader(os.Stdin)
						line, _ = reader.ReadString('\n')
						line = strings.TrimSpace(strings.ToLower(line))
					}
					if line != "y" && line != "yes" {
						fmt.Fprintln(os.Stderr, "Aborted.")
						return fmt.Errorf("deletion cancelled")
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
				if err := app.manager.Delete(ctx, preview.WorkspaceID); err != nil {
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
						map[string]any{"id": preview.WorkspaceID, "deleted": true})
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Removed workspace %q\n", preview.WorkspaceName)
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview removal without deleting data")
	cmd.Flags().StringVarP(&note, "note", "n", "", "Reason note (recorded for audit; future policies may require it)")
	return cmd
}

func newWorkspaceConfigCmd(opts *globalOptions) *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show resolved configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.LoadOptions{FlagBackend: opts.Backend, FlagDataDir: opts.DataDir, FlagBackendURL: opts.BackendURL})
			if err != nil {
				return workspace.NewErrorBackend("load config", err)
			}
			if opts.Format == output.JSON || opts.Format == output.YAML {
				return output.Render(os.Stdout, opts.Format, cfg)
			}
			fmt.Fprintf(os.Stdout, "backend:     %s\n", cfg.Backend)
			fmt.Fprintf(os.Stdout, "data_dir:    %s\n", cfg.DataDir)
			if cfg.ConfigFileFound {
				fmt.Fprintf(os.Stdout, "config_file: %s\n", cfg.ConfigFile)
			} else {
				fmt.Fprintf(os.Stdout, "config_file: %s (not found)\n", cfg.ConfigFile)
			}
			fmt.Fprintf(os.Stdout, "source:      %s\n", cfg.Source)
			return nil
		},
	}
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Workspace configuration",
		RunE:  groupOnlyRunE,
	}
	cmd.AddCommand(showCmd)
	return cmd
}
