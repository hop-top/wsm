package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/output"
	"hop.top/kit/go/runtime/policy"
	"hop.top/wsm/pkg/detector"
	"hop.top/wsm/pkg/workspace"
)

func newSpaceGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "space",
		Short: "OWSF Space management",
		RunE:  groupOnlyRunE,
	}
	cmd.AddCommand(
		newSpaceAddCmd(opts),
		newSpaceListCmd(opts),
		newSpaceShowCmd(opts),
		newSpaceDeleteCmd(opts),
		newSpaceSyncCmd(opts),
	)
	return cmd
}

func newSpaceAddCmd(opts *globalOptions) *cobra.Command {
	var typ, path, trustLevel, startupCmd string
	var capabilities []string
	var autoDetect bool
	cmd := &cobra.Command{
		Use:   "add <workspace> <id>",
		Short: "Associate a compute/storage space with a workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				s := workspace.Space{
					ID:           args[1],
					Type:         workspace.SpaceType(typ),
					Boundary:     path,
					Capabilities: capabilities,
					TrustLevel:   workspace.TrustLevel(trustLevel),
				}

				if autoDetect {
					d := detector.NewRegistry()
					detector.RegisterBuiltins(d)
					results, err := d.RunAll(ctx, path)
					if err != nil {
						return fmt.Errorf("auto-detection failed: %w", err)
					}
					// Apply results to space (simple logic for now: first successful detection)
					for _, res := range results {
						if res.Detected {
							// NOTE: Space model doesn't have Kind/Metadata fields directly yet
							// but we can add them to the manager's CreateSpace logic if needed.
							// For now, we just log that we found something.
							if !opts.Quiet {
								fmt.Fprintf(os.Stderr, "Detected %s (%s) with confidence %.2f\n", res.DetectorName, res.Kind, res.Confidence)
							}
						}
					}
				}

				created, err := app.manager.AddSpaceToWorkspace(ctx, args[0], s)
				if err != nil {
					return err
				}

				// Write-through to rux projects.yaml. Failures here are
				// non-fatal: the wsm DB write already succeeded; recovery
				// path is `wsm space sync`.
				werr := writeProjectsEntry(created.ID, projectsEntry{
					Path:       path,
					StartupCmd: startupCmd,
					Source:     projectsSourceWSM,
				})
				if werr != nil {
					fmt.Fprintf(os.Stderr,
						"warning: wrote space %q to wsm DB but failed to update projects.yaml: %v\nhint: run `wsm space sync` to retry\n",
						created.ID, werr)
				}

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, created)
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Added space %s to workspace %s\n", created.ID, args[0])
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&typ, "type", string(workspace.SpaceFilesystem), "Space type (filesystem|container|vm|api|database|machine|remote)")
	cmd.Flags().StringVar(&path, "path", "", "Resource path (filesystem path, URL, etc.)")
	cmd.Flags().StringVar(&startupCmd, "startup-cmd", "", "Default command rux runs when connecting (empty = $SHELL)")
	cmd.Flags().StringSliceVar(&capabilities, "cap", nil, "Space capabilities (OASF URI, repeatble)")
	cmd.Flags().StringVar(&trustLevel, "trust", string(workspace.TrustLocal), "Trust level (local|verified|external|untrusted)")
	cmd.Flags().BoolVar(&autoDetect, "auto", false, "Attempt to auto-detect space attributes from path")
	return cmd
}

func newSpaceListCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <workspace>",
		Short: "List spaces associated with a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				spaces, err := app.manager.ListWorkspaceSpaces(ctx, args[0])
				if err != nil {
					return err
				}

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, spaces)
				}

				if len(spaces) == 0 {
					if !opts.Quiet {
						// §6.3: empty-state hint is non-data → stderr.
						fmt.Fprintln(os.Stderr, "No spaces associated with this workspace.")
					}
					return nil
				}

				return output.Render(os.Stdout, output.Table, spaces)
			})
		},
	}
	return cmd
}

func newSpaceShowCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "show <space-id>",
		Short: "Show space details",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				s, err := app.manager.GetSpace(ctx, args[0])
				if err != nil {
					return err
				}

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, s)
				}

				fmt.Fprintf(os.Stdout, "Space %s\n", s.ID)
				fmt.Fprintf(os.Stdout, "  Type:         %s\n", s.Type)
				fmt.Fprintf(os.Stdout, "  Trust Level:  %s\n", s.TrustLevel)
				fmt.Fprintf(os.Stdout, "  Boundary:     %s\n", s.Boundary)
				if s.Parent != "" {
					fmt.Fprintf(os.Stdout, "  Parent:       %s\n", s.Parent)
				}
				if len(s.Capabilities) > 0 {
					fmt.Fprintf(os.Stdout, "  Capabilities: %s\n", strings.Join(s.Capabilities, ", "))
				}
				if len(s.Observables) > 0 {
					fmt.Fprintln(os.Stdout, "  Observables:")
					for _, o := range s.Observables {
						fmt.Fprintf(os.Stdout, "    - %s: %s\n", o.Type, o.Value)
					}
				}
				return nil
			})
		},
	}
}

func newSpaceDeleteCmd(opts *globalOptions) *cobra.Command {
	var force bool
	var note string
	cmd := &cobra.Command{
		Use:   "delete <workspace> <space-id>",
		Short: "Dissociate a space from a workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				// Plumb --note into ctx via policy.ContextAttrsKey so
				// the runtime policy engine can read it during pre-event
				// veto. T-1287 wires the flag; T-1288 adds the default
				// policy that enforces note presence.
				if note != "" {
					ctx = context.WithValue(ctx, policy.ContextAttrsKey, map[string]any{
						"note": note,
					})
				}
				if err := app.manager.RemoveSpace(ctx, args[0], args[1], force); err != nil {
					return err
				}
				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format,
						map[string]any{"space_id": args[1], "removed": true})
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "Removed space %s\n", args[1])
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force removal (cascade child spaces)")
	cmd.Flags().StringVarP(&note, "note", "n", "", "Reason note (recorded for audit; future policies may require it)")
	return cmd
}

// newSpaceSyncCmd rebuilds rux/projects.yaml from the wsm DB. Useful
// after a partial-write failure (warning printed by `wsm space add`),
// after a fresh checkout where projects.yaml is missing, or to recover
// from drift. Manual entries written by hand or future
// `rux project add` are preserved by the kit projects.Write contract.
//
// Note: ListSpaces returns root spaces only. wsm currently does not
// surface nested spaces in CLI add flows — if/when nested spaces need
// to be synced, recurse via parent ID.
func newSpaceSyncCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Rewrite rux projects.yaml from wsm DB (recovery)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				spaces, err := app.manager.ListSpaces(ctx, "")
				if err != nil {
					return err
				}
				// Read current entries to preserve StartupCmd values
				// (wsm DB doesn't store them; the rux config is the
				// source of truth for that field).
				existing := readExistingStartupCmds()
				var written, failed int
				var firstErr error
				for _, s := range spaces {
					werr := writeProjectsEntry(s.ID, projectsEntry{
						Path:       s.Boundary,
						StartupCmd: existing[s.ID],
						Source:     projectsSourceWSM,
					})
					if werr != nil {
						failed++
						if firstErr == nil {
							firstErr = werr
						}
						fmt.Fprintf(os.Stderr, "warning: failed to write %q to projects.yaml: %v\n", s.ID, werr)
						continue
					}
					written++
				}
				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, map[string]any{
						"written": written,
						"failed":  failed,
					})
				}
				// §6.3: ack message (non-data) → stderr.
				fmt.Fprintf(os.Stderr, "synced %d spaces to projects.yaml", written)
				if failed > 0 {
					fmt.Fprintf(os.Stderr, " (%d failed)", failed)
				}
				fmt.Fprintln(os.Stderr)
				return nil
			})
		},
	}
}
