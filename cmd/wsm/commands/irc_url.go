package commands

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/workspace"
)

// newIRCURLCmd builds `wsm irc url`.
func newIRCURLCmd(opts *globalOptions) *cobra.Command {
	var web bool

	cmd := &cobra.Command{
		Use:   "url <workspace>",
		Short: "Print the URL a client uses to reach this workspace's IRC server",
		Long: `Report where an IRC client connects for this workspace.

Managed mode answers with the loopback plaintext listener, which is the
socket a client on this machine uses. --web answers with the remote
WebSocket listener instead, for a browser client; it requires that the
workspace was enabled with the remote listener on, because without it
there is no such socket to name.

External mode answers with the configured server, ircs:// when the
connection is encrypted and irc:// when the operator turned TLS off.
--web is refused there: the WebSocket endpoint of a server wsm does not
own is not wsm's to advertise.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCURL(ctx, app, opts, args[0], web)
			})
		},
	}

	cmd.Flags().BoolVar(&web, "web", false, "Print the WebSocket URL for a browser client")

	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// ircURLDoc is the `wsm irc url --json` document.
type ircURLDoc struct {
	Workspace string   `json:"workspace"`
	ID        string   `json:"id"`
	Mode      irc.Mode `json:"mode"`
	URL       string   `json:"url"`
	Scheme    string   `json:"scheme"`
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	TLS       bool     `json:"tls"`
}

// runIRCURL reports the client URL for a workspace.
func runIRCURL(
	ctx context.Context,
	app *appContext,
	opts *globalOptions,
	ref string,
	web bool,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	doc, err := ircClientURL(rt, profile, web)
	if err != nil {
		return err
	}

	if opts.Quiet {
		// The URL is the whole answer, so --quiet still prints it:
		// suppressing it would leave the command with nothing to say.
		fmt.Fprintln(os.Stdout, doc.URL)
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}
	fmt.Fprintln(os.Stdout, doc.URL)
	return nil
}

// ircClientURL derives the client URL for one workspace and mode.
//
// Every branch reads Profile.Mode rather than Config.Mode: the profile
// records what this workspace was actually enabled against, and a config
// edit must never change the address of a running server behind the
// operator's back.
func ircClientURL(rt *ircRuntime, profile irc.Profile, web bool) (ircURLDoc, error) {
	doc := ircURLDoc{
		Workspace: rt.ws.Name,
		ID:        rt.ws.ID,
		Mode:      profile.Mode,
	}

	switch profile.Mode {
	case irc.ModeManaged:
		if !profile.Ports.Valid() {
			return ircURLDoc{}, workspace.NewErrorPrerequisite(
				"this managed workspace has no port triple recorded: re-enable it")
		}
		if web {
			host := strings.TrimSpace(rt.cfg.Remote.Host)
			if !profile.Remote || host == "" {
				return ircURLDoc{}, &output.Error{
					Code: output.CodeUsage,
					Message: fmt.Sprintf(
						"workspace %q has no remote listener: --web has no endpoint to name",
						rt.ws.Name),
					SuggestedFix: "re-enable the workspace with --remote and set irc.remote.host",
					ExitCode:     2,
				}
			}
			doc.Scheme, doc.Host, doc.Port, doc.TLS = "wss", host, profile.Ports.Remote, true
		} else {
			doc.Scheme, doc.Host, doc.Port, doc.TLS = "irc", irc.DefaultBind, profile.Ports.Plain, false
		}

	case irc.ModeExternal:
		if web {
			return ircURLDoc{}, &output.Error{
				Code:         output.CodeUsage,
				Message:      "--web is managed mode only: wsm does not own this server's WebSocket endpoint",
				SuggestedFix: "ask the server's operator for its web client address",
				ExitCode:     2,
			}
		}
		host := strings.TrimSpace(rt.cfg.Server.Host)
		if host == "" {
			return ircURLDoc{}, workspace.NewErrorPrerequisite(
				"irc.server.host is empty: set the server this workspace connects to")
		}
		doc.Host, doc.Port, doc.TLS = host, rt.cfg.Server.Port, rt.cfg.Server.TLSEnabled()
		doc.Scheme = "irc"
		if doc.TLS {
			doc.Scheme = "ircs"
		}

	default:
		return ircURLDoc{}, &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("irc mode %q: must be managed|external", profile.Mode),
			SuggestedFix: "re-enable the workspace",
			ExitCode:     2,
		}
	}

	doc.URL = doc.Scheme + "://" + net.JoinHostPort(doc.Host, strconv.Itoa(doc.Port))
	return doc, nil
}
