package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/output"
)

// deviceRow is the table projection for `device list`. Pre-truncates the
// device ID via shortID and formats first/last seen via humanAgo.
type deviceRow struct {
	ID        string `table:"ID,priority=9"         json:"id"`
	Name      string `table:"NAME,priority=8"       json:"name"`
	OS        string `table:"OS,priority=5"         json:"os"`
	Arch      string `table:"ARCH,priority=5"       json:"arch"`
	FirstSeen string `table:"FIRST SEEN,priority=3" json:"first_seen"`
	LastSeen  string `table:"LAST SEEN,priority=4"  json:"last_seen"`
}

// deviceSessionRow is the table projection for the SESSIONS section of
// `device show`. Mirrors backend.DeviceSession with shortened IDs.
type deviceSessionRow struct {
	Session       string `table:"SESSION,priority=8"       json:"session"`
	Agent         string `table:"AGENT,priority=6"         json:"agent"`
	Workspace     string `table:"WORKSPACE,priority=7"     json:"workspace"`
	Events        int    `table:"EVENTS,priority=5"        json:"events"`
	LastActivity  string `table:"LAST ACTIVITY,priority=4" json:"last_activity"`
}

func newDeviceGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "device",
		Aliases: []string{"devices", "dev", "d"},
		Short:   "Device registry operations",
		RunE:    groupOnlyRunE,
	}
	cmd.AddCommand(
		newDeviceListCmd(opts),
		newDeviceShowCmd(opts),
	)
	return cmd
}

func newDeviceListCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "all", "l"},
		Short:   "List all registered devices",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				devs, err := app.manager.ListDevices(ctx)
				if err != nil {
					return err
				}

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, devs)
				}

				if len(devs) == 0 {
					if !opts.Quiet {
						// §6.3: empty-state hint is non-data → stderr.
						fmt.Fprintln(os.Stderr, "No devices registered.")
					}
					return nil
				}

				rows := make([]deviceRow, len(devs))
				for i, d := range devs {
					rows[i] = deviceRow{
						ID:        shortID(d.ID),
						Name:      d.Name,
						OS:        d.OS,
						Arch:      d.Arch,
						FirstSeen: humanAgo(d.FirstSeen),
						LastSeen:  humanAgo(d.LastSeen),
					}
				}
				return output.Render(os.Stdout, output.Table, rows)
			})
		},
	}
	return cmd
}

func newDeviceShowCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "show <id-or-name>",
		Aliases: []string{"get", "inspect", "info", "s"},
		Short:   "Show device details and activity",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				dev, err := app.manager.ResolveDeviceRef(ctx, args[0])
				if err != nil {
					return err
				}

				sessions, _ := app.manager.DeviceSessions(ctx, dev.ID)

				if opts.Format == output.JSON || opts.Format == output.YAML {
					return output.Render(os.Stdout, opts.Format, map[string]any{
						"device":   dev,
						"sessions": sessions,
					})
				}

				fmt.Fprintf(os.Stdout, "Device %s\n", dev.ID)
				fmt.Fprintf(os.Stdout, "Name:       %s\n", dev.Name)
				fmt.Fprintf(os.Stdout, "Hostname:   %s\n", dev.Hostname)
				fmt.Fprintf(os.Stdout, "Platform:   %s/%s\n", dev.OS, dev.Arch)
				fmt.Fprintf(os.Stdout, "First Seen: %s\n", dev.FirstSeen.Format(time.RFC3339))
				fmt.Fprintf(os.Stdout, "Last Seen:  %s\n", dev.LastSeen.Format(time.RFC3339))
				if len(sessions) > 0 {
					fmt.Fprintln(os.Stdout, "\nSESSIONS")
					rows := make([]deviceSessionRow, len(sessions))
					for i, s := range sessions {
						rows[i] = deviceSessionRow{
							Session:      shortID(s.SessionID),
							Agent:        s.AgentID,
							Workspace:    s.WorkspaceName,
							Events:       s.EventCount,
							LastActivity: humanAgo(s.LastEventAt),
						}
					}
					_ = output.Render(os.Stdout, output.Table, rows)
				}
				return nil
			})
		},
	}
}
