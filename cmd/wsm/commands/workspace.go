package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
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
// with cobra.NoArgs set. The unknown positional returns a structured
// usage envelope (exit 2, per CLI conventions §8.1) whose Message
// carries the corrective --help pointer: kit's framework renderer
// prints only Code + Message, so guidance parked in SuggestedFix alone
// would never reach stderr.
func groupOnlyRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		fix := "run '" + cmd.CommandPath() + " --help' to see available commands"
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("unknown command %q for %q — %s", args[0], cmd.CommandPath(), fix),
			SuggestedFix: fix,
			ExitCode:     2,
		}
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
		Long: `Manage workspaces — the top-level container for sessions, snapshots, and
events. Use this group to create, list, show, archive, unarchive, or delete
workspaces, and to inspect per-workspace configuration via 'workspace config'.`,
		RunE: groupOnlyRunE,
	}
	cli.SetTopLevelVerb(cmd)
	// Intermediate ancestor of the depth-3 leaf `wsm workspace config show`;
	// kit requires kit/hierarchical on every non-root, non-leaf ancestor.
	cli.SetHierarchical(cmd)
	cmd.AddCommand(
		newWorkspaceAddCmd(opts),
		newWorkspaceUpdateCmd(opts),
		newWorkspaceListCmd(opts),
		newWorkspaceShowCmd(opts),
		newWorkspaceArchiveCmd(opts),
		newWorkspaceUnarchiveCmd(opts),
		newWorkspaceDeleteCmd(opts),
		newWorkspaceConfigCmd(opts),
	)
	return cmd
}

// orgPackageMarker is the only file wsm inspects to detect an
// organizational layout kind. Layout content is otherwise opaque
// (interpreted by external tooling), so detection stays limited to
// this single marker — never content heuristics.
const orgPackageMarker = "COMPANY.md"

// detectOrgPackageKind maps a layout directory to a layout kind using
// only the marker-file check: <dir>/COMPANY.md → "agentcompanies/v1",
// anything else → "unknown".
func detectOrgPackageKind(dir string) string {
	if fi, err := os.Stat(filepath.Join(dir, orgPackageMarker)); err == nil && fi.Mode().IsRegular() {
		return "agentcompanies/v1"
	}
	return "unknown"
}

// resolveOrgPackageLayout builds the Layout for --org-package and
// --org-package-kind flag values. Path is stored as the user passed it
// (cleaned, never absolutized): layout paths are metadata resolved by
// external tooling relative to its own boundary.
func resolveOrgPackageLayout(orgPackage, kindOverride string) *model.Layout {
	kind := kindOverride
	if kind == "" {
		kind = detectOrgPackageKind(orgPackage)
	}
	return &model.Layout{Kind: kind, Path: filepath.Clean(orgPackage)}
}

func newWorkspaceAddCmd(opts *globalOptions) *cobra.Command {
	var description string
	var tags []string
	var orgPackage, orgPackageKind string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a workspace",
		Long: `Create a new workspace under the active backend. The workspace becomes the
container for sessions, events, and snapshots captured against it. Use
--description and --tag to attach metadata at creation time, and
--org-package to attach an organizational layout (kind auto-detected from
a COMPANY.md marker, overridable via --org-package-kind).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if orgPackage == "-" {
				return workspace.NewErrorUsage("--org-package -: nothing to detach on add")
			}
			if done, err := previewDryRun(cmd,
				map[string]any{"name": args[0]},
				cli.Effect{
					Kind:       "create",
					Target:     "workspace:" + args[0],
					Reversible: true,
					Detail:     "create workspace " + args[0] + " in the active backend",
				},
			); done {
				return err
			}
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.Create(ctx, args[0], workspace.CreateOptions{Description: description, Tags: tags})
				if err != nil {
					return err
				}
				if orgPackage != "" {
					layout := resolveOrgPackageLayout(orgPackage, orgPackageKind)
					if ws, err = app.manager.SetLayout(ctx, ws.ID, layout); err != nil {
						return err
					}
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
	cmd.Flags().StringVar(&orgPackage, "org-package", "", "Attach organizational layout at <dir> (kind auto-detected)")
	cmd.Flags().StringVar(&orgPackageKind, "org-package-kind", "", "Override detected layout kind")
	cli.SetSideEffect(cmd, cli.SideEffectWrite)
	return cmd
}

func newWorkspaceUpdateCmd(opts *globalOptions) *cobra.Command {
	var orgPackage, orgPackageKind string
	cmd := &cobra.Command{
		Use:   "update <workspace>",
		Short: "Update workspace metadata",
		Long: `Update mutable workspace metadata. Currently supports the organizational
layout: --org-package <dir> attaches (kind auto-detected from a COMPANY.md
marker, overridable via --org-package-kind); '--org-package -' detaches.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if orgPackage == "" {
				return workspace.NewErrorUsage("nothing to update: pass --org-package <dir> (or '-' to detach)")
			}
			detail := "attach layout " + orgPackage
			if orgPackage == "-" {
				detail = "detach the attached layout"
			}
			if done, err := previewDryRun(cmd,
				map[string]any{"workspace": args[0], "org_package": orgPackage},
				cli.Effect{
					Kind:       "update",
					Target:     "workspace:" + args[0],
					Reversible: true,
					Detail:     detail,
				},
			); done {
				return err
			}
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				var layout *model.Layout
				if orgPackage != "-" {
					layout = resolveOrgPackageLayout(orgPackage, orgPackageKind)
				}
				ws, err := app.manager.SetLayout(ctx, args[0], layout)
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
					payload := map[string]any{"id": ws.ID, "updated_at": ws.UpdatedAt, "layout": ws.Layout}
					return output.Render(os.Stdout, format, payload)
				}
				// §6.3: ack message (non-data) → stderr.
				if layout == nil {
					fmt.Fprintf(os.Stderr, "Detached layout from workspace %q\n", ws.Name)
				} else {
					fmt.Fprintf(os.Stderr, "Updated workspace %q\n", ws.Name)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&orgPackage, "org-package", "", "Attach organizational layout at <dir>; '-' detaches")
	cmd.Flags().StringVar(&orgPackageKind, "org-package-kind", "", "Override detected layout kind")
	cli.SetSideEffect(cmd, cli.SideEffectWrite)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

func newWorkspaceListCmd(opts *globalOptions) *cobra.Command {
	var status, sortBy string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		Long: `List workspaces from the active backend, optionally filtered by --status and
sorted via --sort. Output renders as a table by default and as JSON or YAML
when --format is set; --quiet emits IDs only for scripting.`,
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
	cli.SetSideEffect(cmd, cli.SideEffectRead)
	return cmd
}

func newWorkspaceShowCmd(opts *globalOptions) *cobra.Command {
	var eventsAll, showDevices bool
	cmd := &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show workspace details",
		Long: `Show a workspace resolved by ID or name, including metadata, statistics,
the latest snapshot summary, and the most recent events. Use --events to
include the full event log and --devices to list attached devices and
sessions. Read-only: surfaces stored workspace state without mutating it.`,
		Args: cobra.ExactArgs(1),
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
				if ws.Layout != nil {
					layout := ws.Layout.Kind
					if ws.Layout.Path != "" {
						layout += " (" + ws.Layout.Path + ")"
					}
					fmt.Fprintf(os.Stdout, "  Layout:      %s\n", layout)
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
	cli.SetSideEffect(cmd, cli.SideEffectRead)
	return cmd
}

func newWorkspaceArchiveCmd(opts *globalOptions) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "archive <workspace>",
		Short: "Mark a workspace as archived (read-only)",
		Long: `Archive a workspace, flipping it to read-only and excluding it from default
list views. Idempotent: re-running on an already-archived workspace yields the
same state. Use 'workspace unarchive' to restore active status.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := previewDryRun(cmd,
				map[string]any{"workspace": args[0], "note": note},
				cli.Effect{
					Kind:       "update",
					Target:     "workspace:" + args[0],
					Reversible: true,
					Detail:     "flip status to archived (reverse with 'wsm workspace unarchive')",
				},
			); done {
				return err
			}
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				// Plumb --note into ctx via policy.ContextAttrsKey so
				// the runtime policy engine can read it during pre-event
				// veto.
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
	cli.SetSideEffect(cmd, cli.SideEffectWrite)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

func newWorkspaceUnarchiveCmd(opts *globalOptions) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "unarchive <workspace>",
		Short: "Restore an archived workspace to active status",
		Long: `Unarchive a workspace, flipping it back to active and restoring write access.
Idempotent: re-running on an already-active workspace yields the same state.
Inverse of 'workspace archive'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := previewDryRun(cmd,
				map[string]any{"workspace": args[0], "note": note},
				cli.Effect{
					Kind:       "update",
					Target:     "workspace:" + args[0],
					Reversible: true,
					Detail:     "flip status to active (reverse with 'wsm workspace archive')",
				},
			); done {
				return err
			}
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				ws, err := app.manager.ResolveWorkspaceRef(ctx, args[0])
				if err != nil {
					return err
				}
				// Plumb --note into ctx via policy.ContextAttrsKey so
				// the runtime policy engine can read it during pre-event
				// veto.
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
	cli.SetSideEffect(cmd, cli.SideEffectWrite)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

func newWorkspaceDeleteCmd(opts *globalOptions) *cobra.Command {
	var force bool
	var note string
	cmd := &cobra.Command{
		Use:   "delete <workspace>",
		Short: "Permanently delete a workspace and its history",
		Long: `Permanently delete a workspace and all of its events, snapshots, and metadata.

Confirmation is gated by the global --confirm flag (auto|yes|no|prompt):
prompts on a TTY, denies on non-TTY unless --confirm=yes. Use --dry-run to
preview what would be removed without applying. Destructive and not
recoverable.

Deprecated: --force is a backward-compat alias for --confirm=yes. New
scripts should pass --confirm=yes directly; --force will be removed in a
future release.`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// Bridge legacy --force to kit's global --confirm=yes so the
			// policy gate (wrapPolicyRunE → gateConfirm) accepts the
			// invocation. Emit a one-shot deprecation hint to stderr.
			return bridgeForceToConfirm(cmd, force)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				preview, err := app.manager.DeletePreview(ctx, args[0])
				if err != nil {
					return err
				}

				// --dry-run is kit's global flag (kit's confirm gate
				// short-circuits on it, so no confirmation is demanded
				// for a preview). The plan counts what the real run
				// would destroy — resolved from the backend because
				// blast radius is the whole point of previewing a
				// destructive verb.
				if done, err := previewDryRun(cmd,
					map[string]any{
						"workspace":     args[0],
						"events":        preview.EventCount,
						"snapshots":     preview.SnapshotCount,
						"resolved_id":   preview.WorkspaceID,
						"resolved_name": preview.WorkspaceName,
					},
					cli.Effect{
						Kind:       "delete",
						Target:     "workspace:" + preview.WorkspaceID,
						Reversible: false,
						Detail: fmt.Sprintf("remove workspace %q with %d events and %d snapshots",
							preview.WorkspaceName, preview.EventCount, preview.SnapshotCount),
					},
				); done {
					return err
				}

				// Plumb --note into ctx via policy.ContextAttrsKey so
				// the runtime policy engine can read it during pre-event
				// veto. T-1287 wires the flag; T-1288 adds the default
				// policy that enforces note presence. Note: kit's
				// --confirm gate fires BEFORE this RunE; the note policy
				// fires AFTER (inside manager.Delete via bus subscriber)
				// — both gates remain effective.
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
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Deprecated: alias for --confirm=yes (kept for backward compat)")
	// --dry-run is kit's global (registered on the root persistent flag
	// set); a local redeclaration would shadow it and break the plan
	// contract, so this leaf deliberately declares none.
	cmd.Flags().StringVarP(&note, "note", "n", "", "Reason note (recorded for audit; future policies may require it)")
	// idempotent: kit default (delete → Yes).
	cli.SetSideEffect(cmd, cli.SideEffectDestructive)
	return cmd
}

func newWorkspaceConfigCmd(opts *globalOptions) *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show resolved configuration",
		Long: `Show the workspace configuration as resolved by the layered loader (defaults,
config file, environment, flags). Reports the active backend, data directory,
and the source path of the config file actually used.`,
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
	cli.SetSideEffect(showCmd, cli.SideEffectRead)
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Workspace configuration",
		Long: `Inspect resolved workspace configuration (backend, data directory, config
file source). Read-only surface for confirming how wsm interpreted the layered
config sources (defaults, file, environment, flags).`,
		RunE: groupOnlyRunE,
	}
	// Depth-2 intermediate in the wsm → workspace → config → show chain.
	cli.SetHierarchical(cmd)
	cmd.AddCommand(showCmd)
	return cmd
}
