package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func newDeviceGroupCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "device",
		Aliases: []string{"devices", "dev", "d"},
		Short:   "Device registry operations",
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

				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, devs)
				}

				if len(devs) == 0 {
					if !opts.Quiet {
						fmt.Fprintln(os.Stdout, "No devices registered.")
					}
					return nil
				}

				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tOS/ARCH\tFIRST SEEN\tLAST SEEN")
				for _, d := range devs {
					fmt.Fprintf(tw, "%s\t%s\t%s/%s\t%s\t%s\n",
						shortID(d.ID), d.Name, d.OS, d.Arch, humanAgo(d.FirstSeen), humanAgo(d.LastSeen))
				}
				_ = tw.Flush()
				return nil
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

				if shouldJSON(*opts) {
					return writeJSON(os.Stdout, map[string]any{
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
					tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
					fmt.Fprintln(tw, "SESSION\tAGENT\tWORKSPACE\tEVENTS\tLAST ACTIVITY")
					for _, s := range sessions {
						fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
							shortID(s.SessionID), s.AgentID,
							s.WorkspaceName, s.EventCount,
							humanAgo(s.LastEventAt))
					}
					_ = tw.Flush()
				}
				return nil
			})
		},
	}
}
