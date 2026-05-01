package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/model"
	"hop.top/wsm/pkg/workspace"
)

func newWorkspaceGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"ws", "w"},
		Short:   "Workspace operations",
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
		Use:     "add <name>",
		Aliases: []string{"create", "new", "a", "+"},
		Short:   "Create a workspace",
		Args:    cobra.ExactArgs(1),
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

				asJSON := shouldJSON(*opts) || !stdoutIsTTY()
				if asJSON {
					payload := map[string]any{"id": ws.ID, "created_at": ws.CreatedAt}
					return writeJSON(os.Stdout, payload)
				}
				fmt.Fprintf(os.Stdout, "Created workspace %q (%s)\n", ws.Name, ws.ID)
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
		Use:     "list",
		Aliases: []string{"ls", "all", "l"},
		Short:   "List workspaces",
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

				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, items)
				}

				if len(items) == 0 {
					if !opts.Quiet {
						fmt.Fprintln(os.Stdout, "No workspaces yet.")
					fmt.Fprintln(os.Stdout)
					fmt.Fprintln(os.Stdout, "  Create your first workspace:")
					fmt.Fprintln(os.Stdout, "    wsm workspace add my-project")
					}
					return nil
				}

				if opts.Quiet {
					for _, ws := range items {
						fmt.Fprintln(os.Stdout, ws.ID)
					}
					return nil
				}

				printWorkspaceTable(ctx, app, items)
				return nil
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

func printWorkspaceTable(ctx context.Context, app *appContext, items []workspace.Workspace) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tEVENTS\tSNAPS\tLAST ACTIVITY")
	for _, ws := range items {
		ec, _ := app.manager.EventCount(ctx, ws.ID)
		sc, _ := app.manager.SnapshotCount(ctx, ws.ID)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\n",
			shortID(ws.ID), ws.Name, ws.Status, ec, sc, humanAgo(ws.UpdatedAt))
	}
	_ = tw.Flush()
}

func newWorkspaceShowCmd(opts *globalOptions) *cobra.Command {
	var eventsAll, showDevices bool
	cmd := &cobra.Command{
		Use:     "show <id-or-name>",
		Aliases: []string{"get", "inspect", "info", "i"},
		Short:   "Show workspace details",
		Args:    cobra.ExactArgs(1),
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

				if shouldJSON(*opts) {
					ec, _ := app.manager.EventCount(ctx, ws.ID)
					sc, _ := app.manager.SnapshotCount(ctx, ws.ID)
					payload := map[string]any{
						"workspace":      ws,
						"event_count":    ec,
						"snapshot_count": sc,
						"events":         events,
						"snapshot":       latestSnap,
					}
					return writeJSON(os.Stdout, payload)
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
					tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
					fmt.Fprintln(tw, "SEQ\tTYPE\tTIMESTAMP\tDEVICE")
					for _, ev := range events {
						devID := ev.DeviceID
						if devID == "" {
							devID = "unknown"
						}
						fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n",
							ev.SeqNum, ev.Type, ev.Timestamp.Format("15:04:05"), devID)
					}
					_ = tw.Flush()
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
	return &cobra.Command{
		Use:     "archive <workspace>",
		Aliases: []string{"arch"},
		Short:   "Mark a workspace as archived (read-only)",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				if err := app.manager.Archive(ctx, ws.ID); err != nil {
					return err
				}
				if opts.Quiet {
					return nil
				}
				if shouldJSON(*opts) || !stdoutIsTTY() {
					return writeJSON(os.Stdout, map[string]any{"id": ws.ID, "status": "archived"})
				}
				fmt.Fprintf(os.Stdout, "Archived workspace %q\n", ws.Name)
				return nil
			})
		},
	}
}

func newWorkspaceUnarchiveCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "unarchive <workspace>",
		Aliases: []string{"unarch"},
		Short:   "Restore an archived workspace to active status",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				if err := app.manager.Unarchive(ctx, ws.ID); err != nil {
					return err
				}
				if opts.Quiet {
					return nil
				}
				if shouldJSON(*opts) || !stdoutIsTTY() {
					return writeJSON(os.Stdout, map[string]any{"id": ws.ID, "status": "active"})
				}
				fmt.Fprintf(os.Stdout, "Unarchived workspace %q\n", ws.Name)
				return nil
			})
		},
	}
}

func newWorkspaceDeleteCmd(opts *globalOptions) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:     "delete <workspace>",
		Aliases: []string{"rm", "del"},
		Short:   "Permanently delete a workspace and its history",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				preview, err := app.manager.DeletePreview(ctx, args[0])
				if err != nil {
					return err
				}

				if dryRun {
					if shouldJSON(*opts) {
						return writeJSON(os.Stdout, map[string]any{"dry_run": true, "preview": preview})
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
						fmt.Fprintf(os.Stdout, "Remove workspace %q? [y/N]: ", preview.WorkspaceName)
						reader := bufio.NewReader(os.Stdin)
						line, _ = reader.ReadString('\n')
						line = strings.TrimSpace(strings.ToLower(line))
					}
					if line != "y" && line != "yes" {
						fmt.Fprintln(os.Stdout, "Aborted.")
						return fmt.Errorf("deletion cancelled")
					}
				}

				if err := app.manager.Delete(ctx, preview.WorkspaceID); err != nil {
					return err
				}

				if opts.Quiet {
					return nil
				}
				if shouldJSON(*opts) || !stdoutIsTTY() {
					return writeJSON(os.Stdout, map[string]any{"id": preview.WorkspaceID, "deleted": true})
				}
				fmt.Fprintf(os.Stdout, "Removed workspace %q\n", preview.WorkspaceName)
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview removal without deleting data")
	return cmd
}

func newWorkspaceConfigCmd(opts *globalOptions) *cobra.Command {
	showCmd := &cobra.Command{
		Use:     "show",
		Aliases: []string{"cfg"},
		Short:   "Show resolved configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.LoadOptions{FlagBackend: opts.Backend, FlagDataDir: opts.DataDir, FlagBackendURL: opts.BackendURL})
			if err != nil {
				return workspace.NewErrorBackend("load config", err)
			}
			if shouldJSON(*opts) {
				return writeJSON(os.Stdout, cfg)
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
		Use:     "config",
		Aliases: []string{"cfg"},
		Short:   "Workspace configuration",
	}
	cmd.AddCommand(showCmd)
	return cmd
}
