package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/model"
	"hop.top/wsm/pkg/workspace"
)

// Client credential environment variables.
//
// These are deliberately NOT the WSM_IRC_* names the config layer reads
// (pkg/irc/env.go). A member sourcing `wsm irc creds --env` into a shell
// wants to point an IRC client at the server; reusing the config names
// would mean that same shell silently reconfigures every later `wsm`
// invocation — WSM_IRC_SERVER_HOST would re-point the workspace at
// whatever the last creds call printed. The CLIENT infix keeps the two
// namespaces from ever colliding.
const (
	// EnvClientServer is the host a client connects to.
	EnvClientServer = "WSM_IRC_CLIENT_SERVER"
	// EnvClientPort is the port a client connects to.
	EnvClientPort = "WSM_IRC_CLIENT_PORT"
	// EnvClientTLS reports whether that connection is encrypted.
	EnvClientTLS = "WSM_IRC_CLIENT_TLS"
	// EnvClientAccount is the account name to authenticate as.
	EnvClientAccount = "WSM_IRC_CLIENT_ACCOUNT"
	// EnvClientPassword is the passphrase, emitted only when wsm holds one.
	EnvClientPassword = "WSM_IRC_CLIENT_PASSWORD"
)

// newIRCCredsCmd builds `wsm irc creds`.
func newIRCCredsCmd(opts *globalOptions) *cobra.Command {
	var env bool

	cmd := &cobra.Command{
		Use:   "creds <workspace> <member>",
		Short: "Show the IRC credentials wsm holds for a member",
		Long: `Report how a member connects to this workspace's IRC server.

Managed mode: wsm created the account, so it holds the passphrase and
prints it. A member with no passphrase on file is an error naming the
fix, because the account exists on the server and only a rotation can
produce a credential wsm can hand out.

External mode: accounts are bring-your-own. The account name is resolved
through the same chain the daemon uses (the workspace profile, then aps,
then members.yaml) and printed with the server id, so the member knows
which aps entry to look under. A password is printed only when wsm
actually holds one; wsm never invents or guesses a credential it does
not own.

--env renders the result as shell assignments. They are WSM_IRC_CLIENT_*
rather than WSM_IRC_*: sourcing them configures a client, and must never
reconfigure wsm itself.`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCCreds(ctx, app, opts, args[0], args[1], env)
			})
		},
	}

	cmd.Flags().BoolVar(&env, "env", false, "Render as WSM_IRC_CLIENT_* shell assignments")

	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// ircCredsDoc is the `wsm irc creds --json` document.
//
// Password is omitempty and Sensitive is always true: the absence of a
// password is meaningful (wsm does not hold one), and a consumer that
// sees this document must treat it as secret-bearing whether or not a
// password happens to be present in this particular answer.
type ircCredsDoc struct {
	Account  string `json:"account"`
	Server   string `json:"server"`
	Password string `json:"password,omitempty"`

	// Sensitive marks the whole document as secret-bearing, so a
	// consumer does not have to infer it from the presence of a
	// password field that is sometimes absent.
	Sensitive bool `json:"sensitive"`

	// Host and Port address the server for a client; they are what
	// --env renders and are informational in the JSON shape.
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	TLS  bool   `json:"tls"`

	// SecretRef names the aps secret holding the password, when wsm
	// does not hold one itself. It is a hint, never a value.
	SecretRef string `json:"secret_ref,omitempty"`
}

// runIRCCreds reports the credentials wsm holds for one member.
func runIRCCreds(
	ctx context.Context,
	app *appContext,
	opts *globalOptions,
	ref string,
	member string,
	asEnv bool,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	account, err := resolveMemberAccount(ctx, rt, profile, member)
	if err != nil {
		return err
	}

	doc := ircCredsDoc{
		Account:   account,
		Server:    serverIDOf(rt, profile),
		Sensitive: true,
	}
	doc.Host, doc.Port, doc.TLS = clientEndpoint(rt, profile)

	secrets, err := irc.LoadSecrets(rt.paths.Secrets)
	if err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}

	if password, held := secrets.Get(account); held {
		doc.Password = password
	} else if profile.Mode == irc.ModeManaged {
		// Managed mode provisioned the account, so a missing passphrase
		// is a real gap rather than a bring-your-own situation: the
		// account exists on the server and nobody can authenticate as
		// it until wsm sets a credential it also records.
		return workspace.NewErrorPrerequisite(fmt.Sprintf(
			"no passphrase on file for account %q: run `wsm irc account rotate %s %s`",
			account, rt.ws.Name, member,
		))
	} else if ref := strings.TrimSpace(rt.cfg.Bot.SecretRef); ref != "" && account == botAccountOf(rt, profile) {
		// The bot's own credential may live in aps rather than in
		// secrets.yaml. Report where it lives; never resolve it here,
		// because printing it would copy a secret out of the store that
		// deliberately owns it.
		doc.SecretRef = ref
	}

	if opts.Quiet {
		return nil
	}
	if asEnv {
		return renderIRCCredsEnv(os.Stdout, doc)
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}
	return renderIRCCredsTable(doc)
}

// renderIRCCredsEnv writes the credentials as shell assignments.
//
// The password line is omitted entirely when wsm holds no password,
// rather than emitted empty: an empty assignment would override a value
// the member already had in their environment, turning "wsm has nothing
// for you" into "wsm just cleared your password".
func renderIRCCredsEnv(w *os.File, doc ircCredsDoc) error {
	lines := []string{
		EnvClientServer + "=" + shellQuote(doc.Host),
		EnvClientPort + "=" + strconv.Itoa(doc.Port),
		EnvClientTLS + "=" + strconv.FormatBool(doc.TLS),
		EnvClientAccount + "=" + shellQuote(doc.Account),
	}
	if doc.Password != "" {
		lines = append(lines, EnvClientPassword+"="+shellQuote(doc.Password))
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// shellQuote renders s as a single-quoted shell word.
//
// Single quotes rather than double: inside double quotes a passphrase
// containing $ or ` would be expanded by the sourcing shell, which turns
// a correct credential into a wrong one silently. The embedded-quote
// escape is the standard '\” dance.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ircCredsRow is the table projection.
type ircCredsRow struct {
	Field string `table:"FIELD,priority=9" json:"field"`
	Value string `table:"VALUE,priority=9" json:"value"`
}

// renderIRCCredsTable renders the human-readable form.
//
// The passphrase is shown, because the whole point of the command is to
// hand it to the member who needs it; what the table does not do is
// print it when there is none, which is why the row is conditional.
func renderIRCCredsTable(doc ircCredsDoc) error {
	rows := []ircCredsRow{
		{Field: "ACCOUNT", Value: doc.Account},
		{Field: "SERVER", Value: doc.Server},
	}
	if doc.Host != "" {
		rows = append(rows, ircCredsRow{
			Field: "ADDRESS",
			Value: fmt.Sprintf("%s:%d (tls=%t)", doc.Host, doc.Port, doc.TLS),
		})
	}
	if doc.Password != "" {
		rows = append(rows, ircCredsRow{Field: "PASSWORD", Value: doc.Password})
	}
	if doc.SecretRef != "" {
		rows = append(rows, ircCredsRow{Field: "SECRET REF", Value: doc.SecretRef})
	}
	if err := output.Render(os.Stdout, output.Table, rows); err != nil {
		return err
	}
	if doc.Password == "" && doc.SecretRef == "" {
		// §6.3: diagnostics are non-data → stderr.
		fmt.Fprintf(os.Stderr,
			"note: wsm holds no password for %q; it is the member's own credential\n",
			doc.Account)
	}
	return nil
}

// clientEndpoint reports where a client connects for this workspace.
//
// Managed mode answers with the loopback plaintext listener, which is
// the socket a local client uses; the remote TLS listener is what `wsm
// irc url --web` describes, and conflating the two here would hand a
// member a host they cannot reach from this machine's client.
func clientEndpoint(rt *ircRuntime, profile irc.Profile) (host string, port int, useTLS bool) {
	if profile.Mode == irc.ModeManaged {
		return irc.DefaultBind, profile.Ports.Plain, false
	}
	return rt.cfg.Server.Host, rt.cfg.Server.Port, rt.cfg.Server.TLSEnabled()
}

// serverIDOf reports the stable server id this workspace was enabled
// against, falling back to the config when the profile predates it.
func serverIDOf(rt *ircRuntime, profile irc.Profile) string {
	if profile.ServerID != "" {
		return profile.ServerID
	}
	return rt.cfg.ServerID()
}

// botAccountOf reports the bot's own account name.
func botAccountOf(rt *ircRuntime, profile irc.Profile) string {
	if profile.BotAccount != "" {
		return profile.BotAccount
	}
	if rt.cfg.Bot.Account != "" {
		return rt.cfg.Bot.Account
	}
	return irc.DefaultBotAccount
}

// resolveMemberAccount maps a member reference to its account on this
// server, through the same resolver chain the daemon uses.
//
// The member may be named by profile id or by audit label, because those
// are the two names an operator reads off members.yaml; the resolver
// chain keys on profile id, so a label is translated first. A member
// that resolves to nothing is a usage error rather than a prerequisite:
// the argument named somebody this workspace does not have, which is a
// mistake in the invocation, not a broken environment.
func resolveMemberAccount(
	ctx context.Context,
	rt *ircRuntime,
	profile irc.Profile,
	member string,
) (string, error) {
	members, err := loadWorkspaceMembers(profile)
	if err != nil {
		return "", workspace.NewErrorPrerequisite(err.Error())
	}

	profileID := member
	if m, found := findMember(members, member); found {
		profileID = m.ProfileID
	}

	resolver, _ := provider.ResolverFor(profile, members, rt.app.cfg.DataDir)
	account, found, err := resolver.AccountFor(ctx, profileID, serverIDOf(rt, profile))
	if err != nil {
		return "", workspace.NewErrorPrerequisite(fmt.Sprintf("resolve account for %q: %v", member, err))
	}
	if !found {
		return "", &output.Error{
			Code: output.CodeUsage,
			Message: fmt.Sprintf(
				"no IRC account for %q on %s", member, serverIDOf(rt, profile)),
			SuggestedFix: fmt.Sprintf(
				"add the member to members.yaml, or run `wsm irc account create %s %s`",
				rt.ws.Name, member),
			ExitCode: 2,
		}
	}
	return account, nil
}

// findMember locates a member by profile id or audit label.
func findMember(members []model.Member, ref string) (model.Member, bool) {
	for _, m := range members {
		if m.ProfileID == ref || m.AuditLabel == ref {
			return m, true
		}
	}
	return model.Member{}, false
}

// loadWorkspaceMembers reads members.yaml from the root the profile
// recorded at enable.
//
// The root is the profile's, never the working directory: these commands
// run from anywhere, and resolving members against the caller's cwd
// would silently answer about a different workspace's people.
func loadWorkspaceMembers(profile irc.Profile) ([]model.Member, error) {
	localPath := irc.WorkspaceConfigPath(profile.Root)
	if localPath == "" {
		// LoadMemberConfig reads members.yaml from the config file's own
		// directory, so the notional path must still sit under the root.
		localPath = workspaceConfigFallback(profile.Root)
	}
	mc, err := config.LoadMemberConfig(localPath)
	if err != nil {
		return nil, err
	}
	return mc.Members, nil
}

// creatorNamespaces maps each creator member's account to the channel
// glob it may register under.
//
// This mirrors provision.addNamespaces, which computes the same map for
// the daemon. It is recomputed here rather than read from the daemon
// because `channel add` has to refuse a name inside a namespace whether
// or not a daemon is running, and a stopped workspace publishes no such
// map.
func creatorNamespaces(cfg irc.Config, wsName string, members []model.Member) map[string]string {
	out := map[string]string{}
	for i := range members {
		m := &members[i]
		if !m.ChannelCreator {
			continue
		}
		ns := irc.CreatorNamespace(m.ChannelLabel(), m.ChannelNamespace)
		if ns == "" {
			continue
		}
		out[m.ChannelLabel()] = irc.CreatorGlob(cfg.ChannelPrefix, wsName, ns)
	}
	return out
}

// sortedKeys returns the map's keys in a stable order, so every listing
// this package renders is reproducible.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
