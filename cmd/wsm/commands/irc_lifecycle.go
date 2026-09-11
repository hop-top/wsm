package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
	"hop.top/wsm/pkg/access"
	"hop.top/wsm/pkg/irc"
	ircdaemon "hop.top/wsm/pkg/irc/daemon"
	"hop.top/wsm/pkg/irc/managed"
	"hop.top/wsm/pkg/irc/provider"
	"hop.top/wsm/pkg/irc/provider/ergo"
	"hop.top/wsm/pkg/irc/provision"
	"hop.top/wsm/pkg/model"
	"hop.top/wsm/pkg/workspace"
)

// daemonReadyTimeout bounds how long enable, start and restart wait for
// a freshly spawned daemon to report itself running.
//
// The daemon writes status.json once it has taken the workspace lock,
// started the managed child (if any) and connected the bot. Fifteen
// seconds covers an ergo cold start on a loaded machine; past that the
// spawn is reported as a prerequisite failure naming the daemon log,
// rather than left to hang a script forever.
const daemonReadyTimeout = 15 * time.Second

// daemonPollInterval is how often the readiness wait re-reads status.json.
const daemonPollInterval = 100 * time.Millisecond

// ircRuntime is everything the lifecycle verbs resolve before doing
// any work: the workspace, its paths, and the config in force for it.
//
// Every verb needs the same four things and each of them can fail with
// a different exit code, so resolving them in one place is what keeps
// the exit-code contract identical across the whole group rather than
// re-derived per verb.
type ircRuntime struct {
	ws    *model.Workspace
	paths irc.Paths
	cfg   irc.Config
	app   *appContext
}

// resolveIRCRuntime performs the checks every `wsm irc` verb shares:
// the feature gate, the workspace lookup and the path layout.
//
// The gate is checked before the workspace lookup deliberately. With
// the feature off, "which workspace?" is not yet a meaningful question,
// and reporting a missing workspace (exit 3) to someone who has not
// turned IRC on at all would send them looking in the wrong place.
func resolveIRCRuntime(ctx context.Context, app *appContext, ref string) (*ircRuntime, error) {
	cfg, err := irc.Load(app.cfg.ConfigFile)
	if err != nil {
		return nil, workspace.NewErrorPrerequisite(fmt.Sprintf("load irc config: %v", err))
	}
	if !cfg.Enabled {
		return nil, ircDisabledError()
	}

	ws, err := app.manager.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	paths, err := irc.ResolvePaths(irc.PathsInput{
		DataDir:  app.cfg.DataDir,
		StateDir: app.cfg.StateDir,
	}, ws.ID)
	if err != nil {
		return nil, workspace.NewErrorPrerequisite(err.Error())
	}

	// A workspace that has been enabled carries a Root, and the config
	// in force for it is the global file merged with that root's
	// override. Before enable there is no Root yet and the global file
	// is the whole story.
	if profile, perr := irc.LoadProfile(paths.Profile); perr == nil && profile.Root != "" {
		merged, mErr := irc.LoadWorkspace(app.cfg.ConfigFile, profile.Root)
		if mErr != nil {
			return nil, workspace.NewErrorPrerequisite(fmt.Sprintf("load irc config: %v", mErr))
		}
		if !merged.Enabled {
			return nil, ircDisabledError()
		}
		cfg = merged
	}

	return &ircRuntime{ws: ws, paths: paths, cfg: cfg, app: app}, nil
}

// ircDisabledError is the feature-gate refusal shared by every verb.
//
// Exit 2 rather than 7: the fix is a config edit the operator makes
// deliberately, not an environment problem to repair, and story 015
// classes an invocation that is not valid for the current mode as
// usage.
func ircDisabledError() error {
	const fix = "set irc.enabled: true"
	return &output.Error{
		Code:         output.CodeUsage,
		Message:      "irc is disabled: " + fix,
		SuggestedFix: fix,
		ExitCode:     2,
	}
}

// requireCapability refuses the verb when the capability is withheld.
func requireCapability(app *appContext, capability string) error {
	if app.caps != nil && !app.caps.Can(capability) {
		return workspace.NewErrorForbidden(fmt.Sprintf("%s not permitted", capability))
	}
	return nil
}

// loadEnabledProfile reads the profile of a workspace that must already
// be enabled.
func (rt *ircRuntime) loadEnabledProfile() (irc.Profile, error) {
	profile, err := irc.LoadProfile(rt.paths.Profile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return irc.Profile{}, workspace.NewErrorPrerequisite(fmt.Sprintf(
				"IRC is not enabled for %q: run `wsm irc enable %s` first",
				rt.ws.Name, rt.ws.Name,
			))
		}
		return irc.Profile{}, workspace.NewErrorPrerequisite(fmt.Sprintf("read irc profile: %v", err))
	}
	return profile, nil
}

// enforcePrereqs runs a prerequisite set and reports the first required
// failure as exit 7, carrying that check's fix text verbatim.
//
// Message rather than the underlying error is surfaced because the
// prerequisite authors wrote Message to be the corrective action; the
// error beneath it names what was observed, which is the wrong half to
// show someone who needs to know what to do next.
func enforcePrereqs(prereqs []access.Prerequisite) error {
	results, err := access.Enforce(prereqs)
	if err == nil {
		return nil
	}
	for _, r := range results {
		if !r.Passed && r.Prerequisite.Level == access.LevelRequired {
			return workspace.NewErrorPrerequisite(r.Prerequisite.Message)
		}
	}
	return workspace.NewErrorPrerequisite(err.Error())
}

// daemonState reports the live daemon's status and whether one is running.
//
// A pidfile is evidence a daemon was started, never that one still
// runs, so the recorded pid is probed before its status is trusted.
// A stale file therefore reads as stopped, which is what the operator
// means by "is it up?" and what start needs in order to be idempotent
// without racing the lock.
func daemonState(paths irc.Paths) (ircdaemon.Status, bool) {
	status, err := ircdaemon.ReadStatus(paths.Status)
	if err != nil {
		return ircdaemon.Status{State: ircdaemon.StateStopped}, false
	}
	if !processAlive(status.PID) {
		status.State = ircdaemon.StateStopped
		return status, false
	}
	return status, true
}

// awaitDaemon waits for a spawned daemon to report itself running.
func awaitDaemon(paths irc.Paths, pid int) (ircdaemon.Status, error) {
	deadline := time.Now().Add(daemonReadyTimeout)
	for {
		status, err := ircdaemon.ReadStatus(paths.Status)
		if err == nil && status.PID == pid && status.State != ircdaemon.StateStopped {
			return status, nil
		}
		// A child that has already exited will never write a status,
		// so waiting out the full timeout would only delay the report.
		if !processAlive(pid) {
			return ircdaemon.Status{}, workspace.NewErrorPrerequisite(fmt.Sprintf(
				"irc daemon exited before reporting ready: see %s", paths.DaemonLog,
			))
		}
		if time.Now().After(deadline) {
			return ircdaemon.Status{}, workspace.NewErrorPrerequisite(fmt.Sprintf(
				"irc daemon did not report ready within %s: see %s",
				daemonReadyTimeout, paths.DaemonLog,
			))
		}
		time.Sleep(daemonPollInterval)
	}
}

// startDaemon spawns the daemon unless one already holds the workspace.
//
// Idempotence is the point: the daemon takes an exclusive lock for its
// lifetime, so a second one would exit 1 on the lock rather than
// serving anything. Reporting the running daemon's status is both the
// honest answer to "start it" and what makes the verb safe to re-run
// from a script or a lifecycle hook.
func startDaemon(rt *ircRuntime) (ircdaemon.Status, bool, error) {
	if status, running := daemonState(rt.paths); running {
		return status, false, nil
	}
	pid, err := spawnDaemon(rt.paths, rt.ws.ID, nil)
	if err != nil {
		return ircdaemon.Status{}, false, workspace.NewErrorPrerequisite(err.Error())
	}
	status, err := awaitDaemon(rt.paths, pid)
	if err != nil {
		return ircdaemon.Status{}, false, err
	}
	return status, true, nil
}

// stopDaemon terminates a running daemon and waits for it to leave.
func stopDaemon(paths irc.Paths) (bool, error) {
	status, running := daemonState(paths)
	if !running {
		return false, nil
	}
	if err := signalDaemon(status.PID, syscall.SIGTERM); err != nil {
		return false, workspace.NewErrorPrerequisite(fmt.Sprintf("signal irc daemon: %v", err))
	}
	deadline := time.Now().Add(daemonReadyTimeout)
	for processAlive(status.PID) {
		if time.Now().After(deadline) {
			return false, &output.Error{
				Code: output.CodeGeneric,
				Message: fmt.Sprintf(
					"irc daemon %d did not exit within %s", status.PID, daemonReadyTimeout),
				ExitCode: 1,
			}
		}
		time.Sleep(daemonPollInterval)
	}
	return true, nil
}

// newIRCEnableCmd builds `wsm irc enable`.
func newIRCEnableCmd(opts *globalOptions) *cobra.Command {
	var (
		external   bool
		remote     bool
		sharedWith string
		port       int
		dir        string
		botStdin   bool
	)

	cmd := &cobra.Command{
		Use:   "enable <workspace>",
		Short: "Enable IRC for a workspace and start its daemon",
		Long: `Provision a workspace's IRC presence and start its daemon.

Managed mode (the default) checks prerequisites, allocates a consecutive
port triple, generates the bot account's secrets and the server's API
token, renders ergo's configuration, creates its database, and starts the
daemon that supervises it.

External mode (--external) verifies the configured server is reachable and
starts the bot against it; wsm never owns that server. The bot's password
is read from stdin with --bot-password-stdin, or resolved from the aps
secret named by irc.bot.secret_ref.

Re-running against a workspace whose daemon is already up reports that
daemon rather than starting a second one.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCEnable(ctx, cmd, app, opts, args[0], ircEnableFlags{
					External:   external,
					Remote:     remote,
					SharedWith: sharedWith,
					Port:       port,
					Dir:        dir,
					BotStdin:   botStdin,
				})
			})
		},
	}

	cmd.Flags().BoolVar(&external, "external", false, "Connect to a server wsm does not own")
	cmd.Flags().BoolVar(&remote, "remote", false, "Managed: enable the remote TLS/WebSocket listener")
	cmd.Flags().StringVar(&sharedWith, "shared-with", "", "Attach to another workspace's server as a guest")
	cmd.Flags().IntVar(&port, "port", 0, "Managed: base port of the triple to use instead of scanning")
	cmd.Flags().StringVar(&dir, "dir", "", "Workspace root holding .wsm/ (default: current directory)")
	cmd.Flags().BoolVar(&botStdin, "bot-password-stdin", false, "External: read the bot password from stdin")

	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	// Conditional rather than yes: a second enable against a running
	// daemon is a no-op, but the first one provisions ports, secrets
	// and a database that did not exist before.
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	return cmd
}

// ircEnableFlags is the flag surface of `wsm irc enable`, lifted into a
// struct so the mapping from flags to behaviour is unit-testable
// without constructing a cobra command.
type ircEnableFlags struct {
	External   bool
	Remote     bool
	SharedWith string
	Port       int
	Dir        string
	BotStdin   bool
}

// mode reports the mode these flags select against a resolved config.
//
// --external is a one-way override: it can select external mode when
// the config says managed, but there is no flag that forces managed,
// because wsm taking ownership of a server is not something a flag
// should decide behind the config's back.
func (f ircEnableFlags) mode(cfg irc.Config) irc.Mode {
	if f.External {
		return irc.ModeExternal
	}
	return cfg.Mode
}

// ports reports the port triple these flags select, and whether they
// select one at all. A --port value names the base of the triple; the
// remaining two are consecutive, matching what allocation produces.
func (f ircEnableFlags) ports() (irc.Ports, bool) {
	if f.Port <= 0 {
		return irc.Ports{}, false
	}
	return irc.Ports{Plain: f.Port, Remote: f.Port + 1, API: f.Port + 2}, true
}

// root reports the workspace root these flags select.
func (f ircEnableFlags) root() (string, error) {
	if f.Dir != "" {
		return filepath.Abs(f.Dir)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return cwd, nil
}

// wsmRootMarkers are the directories that mark a wsm workspace root.
var wsmRootMarkers = []string{
	filepath.Join(".wsm"),
	filepath.Join(".hop", "wsm"),
}

// validateRoot refuses a root that is not a wsm workspace directory.
//
// The root is where members.yaml and the per-workspace config override
// live, and the daemon reads both from the path recorded here long
// after this command has exited. Accepting a directory with neither
// marker would record a root that resolves to nothing, producing a
// daemon that reconciles an empty member list rather than failing here
// where the operator can still fix it.
func validateRoot(root string) error {
	for _, marker := range wsmRootMarkers {
		if info, err := os.Stat(filepath.Join(root, marker)); err == nil && info.IsDir() {
			return nil
		}
	}
	return &output.Error{
		Code: output.CodeUsage,
		Message: fmt.Sprintf(
			"%s is not a wsm workspace root: expected .wsm/ or .hop/wsm/", root),
		SuggestedFix: "run this from the workspace directory, or pass --dir <root>",
		ExitCode:     2,
	}
}

// runIRCEnable provisions a workspace's IRC presence and starts its daemon.
func runIRCEnable(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	flags ircEnableFlags,
) error {
	// Sharing is the subject of its own task: the host-side guards,
	// the delegate grants and the guest's borrowed channels are all
	// absent, and an enable that silently ignored the flag would
	// record a profile claiming a relationship nothing enforces.
	if flags.SharedWith != "" {
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      "--shared-with is not implemented",
			SuggestedFix: "enable this workspace on its own server for now",
			ExitCode:     2,
		}
	}

	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if err := requireCapability(app, workspace.CapIRCEnable); err != nil {
		return err
	}

	root, err := flags.root()
	if err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}
	if err := validateRoot(root); err != nil {
		return err
	}

	// The root's own override participates from here on: enable is
	// where the root becomes known, so every later decision (mode,
	// ports, channels) must see the merged config rather than the
	// global one resolveIRCRuntime started with.
	cfg, err := irc.LoadWorkspace(app.cfg.ConfigFile, root)
	if err != nil {
		return workspace.NewErrorPrerequisite(fmt.Sprintf("load irc config: %v", err))
	}
	if !cfg.Enabled {
		return ircDisabledError()
	}
	rt.cfg = cfg

	mode := flags.mode(cfg)
	cfg.Mode = mode
	if _, err := cfg.Validate(); err != nil {
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      err.Error(),
			SuggestedFix: "correct the irc block in your wsm config",
			ExitCode:     2,
		}
	}

	if done, err := previewDryRun(cmd,
		map[string]any{
			"workspace": rt.ws.Name,
			"mode":      string(mode),
			"root":      root,
			"remote":    flags.Remote,
		},
		cli.Effect{
			Kind:       "create",
			Target:     "irc:" + rt.ws.ID,
			Reversible: true,
			Detail: fmt.Sprintf("enable %s IRC for workspace %q and start its daemon",
				mode, rt.ws.Name),
		},
	); done {
		return err
	}

	// An existing profile means this workspace is already enabled. The
	// daemon may or may not be up; startDaemon settles that, and the
	// provisioning below is skipped rather than redone so a re-enable
	// cannot reallocate ports or regenerate credentials under a live
	// session.
	if existing, lerr := irc.LoadProfile(rt.paths.Profile); lerr == nil {
		status, _, serr := startDaemon(rt)
		if serr != nil {
			return serr
		}
		return renderIRCEnabled(rt, existing, status, opts)
	}

	profile := irc.Profile{
		Mode:       mode,
		Provider:   cfg.Provider,
		ServerID:   irc.DefaultServerID(cfg),
		Root:       root,
		Remote:     flags.Remote,
		Bind:       cfg.Bind,
		BotAccount: cfg.Bot.Account,
		CreatedAt:  time.Now().UTC(),
	}
	if mode == irc.ModeManaged {
		profile.Provider = irc.ProviderErgo
	}

	if err := rt.paths.EnsureDirs(); err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}

	switch mode {
	case irc.ModeManaged:
		if err := provisionManaged(ctx, rt, &profile, cfg, flags); err != nil {
			return err
		}
	case irc.ModeExternal:
		if err := provisionExternal(rt, &profile, cfg, flags); err != nil {
			return err
		}
	default:
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("irc.mode %q: must be managed|external", mode),
			SuggestedFix: "set irc.mode to managed or external",
			ExitCode:     2,
		}
	}

	if err := irc.SaveProfile(rt.paths.Profile, profile); err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}

	status, _, err := startDaemon(rt)
	if err != nil {
		return err
	}
	return renderIRCEnabled(rt, profile, status, opts)
}

// provisionManaged prepares everything a wsm-owned ergo needs.
//
// The order is forced by ergo itself: the config names the certificate
// files relatively, so mkcerts must run in the directory that holds
// them before initdb parses the document, and initdb needs the
// rendered config to exist before it can create the database the
// config points at.
func provisionManaged(
	ctx context.Context,
	rt *ircRuntime,
	profile *irc.Profile,
	cfg irc.Config,
	flags ircEnableFlags,
) error {
	managed.Binary = cfg.Binary

	if err := enforcePrereqs(managed.PreflightStatic(cfg)); err != nil {
		return err
	}

	if explicit, ok := flags.ports(); ok {
		if !irc.PortsFree(explicit) {
			return workspace.NewErrorPrerequisite(fmt.Sprintf(
				"ports %d-%d are not all free: choose another --port base",
				explicit.Plain, explicit.API,
			))
		}
		profile.Ports = explicit
	} else {
		allocated, err := irc.AllocatePorts(cfg.PortRange, nil)
		if err != nil {
			return workspace.NewErrorPrerequisite(err.Error())
		}
		profile.Ports = allocated
	}

	secrets, err := irc.LoadSecrets(rt.paths.Secrets)
	if err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}
	if _, held := secrets.Get(profile.BotAccount); !held {
		passphrase, gerr := irc.GeneratePassphrase()
		if gerr != nil {
			return workspace.NewErrorPrerequisite(gerr.Error())
		}
		secrets.Set(profile.BotAccount, passphrase)
	}
	if err := secrets.Save(rt.paths.Secrets); err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}

	if _, err := irc.LoadAPIToken(rt.paths.APIToken); err != nil {
		token, gerr := managed.GenToken(ctx)
		if gerr != nil {
			return workspace.NewErrorPrerequisite(gerr.Error())
		}
		if serr := irc.SaveAPIToken(rt.paths.APIToken, token); serr != nil {
			return workspace.NewErrorPrerequisite(serr.Error())
		}
	}

	// Self-signed material is generated only when the remote listener
	// is on and the operator has not supplied a certificate: an empty
	// cert path means "generate", never "fail".
	if profile.Remote && cfg.Remote.TLSCert == "" && cfg.Remote.TLSKey == "" {
		if err := os.MkdirAll(rt.paths.TLSDir, 0o700); err != nil {
			return workspace.NewErrorPrerequisite(err.Error())
		}
		if err := managed.Mkcerts(ctx, rt.paths.TLSDir); err != nil {
			return workspace.NewErrorPrerequisite(err.Error())
		}
	}

	rendered, err := managed.Renderer{Paths: rt.paths}.RenderConfig(cfg, *profile, secrets)
	if err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}
	if err := os.WriteFile(rt.paths.ErgoYAML, rendered, 0o600); err != nil {
		return workspace.NewErrorPrerequisite(fmt.Sprintf("write ergo config: %v", err))
	}

	if _, err := os.Stat(rt.paths.DB); errors.Is(err, os.ErrNotExist) {
		if ierr := managed.Initdb(ctx, rt.paths.ErgoYAML); ierr != nil {
			return workspace.NewErrorPrerequisite(ierr.Error())
		}
	}
	return nil
}

// provisionExternal verifies a server wsm does not own and records the
// bot's credential.
func provisionExternal(
	rt *ircRuntime,
	profile *irc.Profile,
	cfg irc.Config,
	flags ircEnableFlags,
) error {
	if err := enforcePrereqs(irc.PreflightExternal(cfg)); err != nil {
		return err
	}

	secrets, err := irc.LoadSecrets(rt.paths.Secrets)
	if err != nil {
		return workspace.NewErrorPrerequisite(err.Error())
	}

	if flags.BotStdin {
		password, rerr := readBotPasswordStdin()
		if rerr != nil {
			return rerr
		}
		secrets.Set(cfg.Bot.Account, password)
		if serr := secrets.Save(rt.paths.Secrets); serr != nil {
			return workspace.NewErrorPrerequisite(serr.Error())
		}
	}

	// The bot cannot authenticate without a password, and discovering
	// that at connect time would leave a workspace enabled against a
	// server it can never reach. Checking here keeps the failure at
	// the point where the fix (--bot-password-stdin, or an aps
	// secret_ref) is still in the operator's hands.
	if _, err := irc.ReadBotPassword(cfg, secrets); err != nil {
		if errors.Is(err, irc.ErrNoBotSecret) {
			return workspace.NewErrorPrerequisite(fmt.Sprintf(
				"no password for bot account %q: pass --bot-password-stdin, "+
					"or set irc.bot.secret_ref to an aps secret", cfg.Bot.Account,
			))
		}
		return workspace.NewErrorPrerequisite(err.Error())
	}
	return nil
}

// readBotPasswordStdin reads one password from stdin.
//
// The inherited descriptor is read directly rather than reopened by
// name: os.Stdin.Name() is the label "/dev/stdin", which on a pipe
// either names nothing openable or, worse, resolves to a different
// stream than the one the caller redirected.
func readBotPasswordStdin() (string, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", workspace.NewErrorPrerequisite(fmt.Sprintf("read bot password from stdin: %v", err))
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return "", workspace.NewErrorPrerequisite("bot password on stdin is empty")
	}
	return password, nil
}

// newIRCDisableCmd builds `wsm irc disable`.
func newIRCDisableCmd(opts *globalOptions) *cobra.Command {
	var (
		purge bool
		force bool
	)

	cmd := &cobra.Command{
		Use:   "disable <workspace>",
		Short: "Stop a workspace's IRC daemon",
		Long: `Stop the workspace's IRC daemon.

The workspace's data and state directories are left in place so the
daemon can be started again with everything it had. --purge removes
them instead, which destroys the server's database, its generated
certificates and every secret wsm holds for it; that is gated by the
global --confirm flag like any other destructive verb.

A workspace hosting guests is refused: disabling it takes the server
out from under workspaces that are not this one. --force overrides
that, leaving the guests to fail on their next reconnect.`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// --force carries two meanings here and only one of them
			// is kit's: it overrides the guest refusal (handled in the
			// RunE) and, on a purge, stands in for --confirm=yes the
			// way it does on every other destructive wsm verb.
			if purge {
				return bridgeForceToConfirm(cmd, force)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCDisable(ctx, cmd, app, opts, args[0], purge, force)
			})
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "Also remove the workspace's IRC data and state directories")
	cmd.Flags().BoolVar(&force, "force", false, "Disable even while guest workspaces are hosted here")

	// write-local rather than destructive, even though --purge destroys
	// data. kit's confirm gate keys on the verb's tag and fires before
	// this command's RunE, so a destructive tag would demand a
	// confirmation for the bare stop as well — and would mask the
	// feature gate, the workspace lookup and the shared-server refusal
	// behind a blanket UNAUTHORIZED. The destructive half is gated
	// below instead, where --purge is actually known.
	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	return cmd
}

// gatePurge enforces the confirmation contract for `disable --purge`.
//
// This reproduces kit's --confirm matrix for one flag rather than the
// whole verb: yes proceeds, no refuses, and auto resolves the way kit
// resolves it — prompt on a terminal, refuse off one, so a script that
// did not ask for destruction never gets it. The envelope matches
// kit's own so the refusal is indistinguishable from the gate it
// stands in for.
func gatePurge(cmd *cobra.Command) error {
	mode := strings.ToLower(strings.TrimSpace(confirmFlagValue(cmd)))
	switch mode {
	case "yes":
		return nil
	case "no":
		return purgeRefused(cmd)
	}
	// auto, prompt, or unset.
	if !stdoutIsTTY() {
		return purgeRefused(cmd)
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"This permanently removes the workspace's IRC data and state (%s). Continue? [y/N] ",
		cmd.CommandPath())
	var answer string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &answer); err != nil {
		return purgeRefused(cmd)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return purgeRefused(cmd)
	}
}

// purgeRefused is the envelope a declined purge produces.
func purgeRefused(cmd *cobra.Command) error {
	return &output.Error{
		Code: output.CodeUnauthorized,
		Message: "destructive command " + cmd.CommandPath() +
			" --purge refused: --confirm=no (or non-TTY default)",
		SuggestedFix: "pass --confirm=yes to remove the workspace's IRC data and state",
		ExitCode:     5,
	}
}

// confirmFlagValue reads the inherited --confirm flag.
//
// The flag is registered by kit on the root's persistent set, so it is
// found by walking up the parent chain rather than on this command.
func confirmFlagValue(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.PersistentFlags().Lookup("confirm"); f != nil {
			return f.Value.String()
		}
		if f := c.Flags().Lookup("confirm"); f != nil {
			return f.Value.String()
		}
	}
	return ""
}

// runIRCDisable stops the daemon and optionally removes its directories.
func runIRCDisable(
	ctx context.Context,
	cmd *cobra.Command,
	app *appContext,
	opts *globalOptions,
	ref string,
	purge bool,
	force bool,
) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if err := requireCapability(app, workspace.CapIRCDisable); err != nil {
		return err
	}
	if purge {
		if err := requireCapability(app, workspace.CapIRCPurge); err != nil {
			return err
		}
	}

	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	// Guests are other workspaces whose channels and accounts live on
	// this server. Stopping it is not this workspace's decision alone,
	// so the refusal is a conflict rather than a prerequisite: nothing
	// about the environment is broken, the operation simply collides
	// with someone else's state.
	if len(profile.SharedWith) > 0 && !force {
		return workspace.NewErrorConflict(fmt.Sprintf(
			"workspace %q hosts %d guest workspace(s) on its IRC server (%s): "+
				"disabling it would cut them off; pass --force to proceed anyway",
			rt.ws.Name, len(profile.SharedWith), strings.Join(profile.SharedWith, ", "),
		))
	}

	if purge {
		if done, derr := previewDryRun(cmd,
			map[string]any{"workspace": rt.ws.Name, "purge": true},
			cli.Effect{
				Kind:       "delete",
				Target:     "irc:" + rt.ws.ID,
				Reversible: false,
				Detail: fmt.Sprintf("remove %s and %s",
					rt.paths.DataDir, rt.paths.StateDir),
			},
		); done {
			return derr
		}
		// Gated after the dry-run preview, which destroys nothing, and
		// after the guest refusal above, so the operator is never asked
		// to confirm an operation that is going to be refused anyway.
		if gerr := gatePurge(cmd); gerr != nil {
			return gerr
		}
	}

	stopped, err := stopDaemon(rt.paths)
	if err != nil {
		return err
	}

	if purge {
		for _, dir := range []string{rt.paths.DataDir, rt.paths.StateDir} {
			if rerr := os.RemoveAll(dir); rerr != nil {
				return workspace.NewErrorPrerequisite(fmt.Sprintf("remove %s: %v", dir, rerr))
			}
		}
	}

	payload := map[string]any{
		"workspace": rt.ws.Name,
		"id":        rt.ws.ID,
		"stopped":   stopped,
		"purged":    purge,
	}
	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), payload)
	}
	// §6.3: acknowledgements are non-data → stderr.
	if stopped {
		fmt.Fprintf(os.Stderr, "Stopped IRC daemon for %q\n", rt.ws.Name)
	} else {
		fmt.Fprintf(os.Stderr, "No IRC daemon running for %q\n", rt.ws.Name)
	}
	if purge {
		fmt.Fprintf(os.Stderr, "Removed IRC data and state for %q\n", rt.ws.Name)
	}
	return nil
}

// newIRCStartCmd builds `wsm irc start`.
func newIRCStartCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <workspace>",
		Short: "Start a workspace's IRC daemon",
		Long: `Start the IRC daemon for a workspace that is already enabled.

In managed mode the recorded port triple is re-checked before the daemon
is spawned: a port that something else has taken since enable is reported
rather than silently reallocated, because a workspace that moves ports
behind the operator's back breaks every client configured for it.

Starting a workspace whose daemon is already up reports that daemon and
starts nothing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCStart(ctx, app, opts, args[0])
			})
		},
	}
	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	return cmd
}

// runIRCStart starts the daemon for an enabled workspace.
func runIRCStart(ctx context.Context, app *appContext, opts *globalOptions, ref string) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}
	if err := checkRecordedPorts(profile, rt.paths); err != nil {
		return err
	}
	status, _, err := startDaemon(rt)
	if err != nil {
		return err
	}
	return renderIRCEnabled(rt, profile, status, opts)
}

// checkRecordedPorts re-verifies a managed workspace's allocated ports.
//
// Skipped while a daemon is running, because the ports are then held by
// the very ergo this workspace owns: probing them would report the
// workspace's own server as a conflict with itself.
func checkRecordedPorts(profile irc.Profile, paths irc.Paths) error {
	if profile.Mode != irc.ModeManaged || !profile.Ports.Valid() {
		return nil
	}
	if _, running := daemonState(paths); running {
		return nil
	}
	if !irc.PortsFree(profile.Ports) {
		return workspace.NewErrorPrerequisite(fmt.Sprintf(
			"ports %d-%d recorded for this workspace are in use: "+
				"free them, or re-enable the workspace to allocate a new triple",
			profile.Ports.Plain, profile.Ports.API,
		))
	}
	return nil
}

// newIRCStopCmd builds `wsm irc stop`.
func newIRCStopCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <workspace>",
		Short: "Stop a workspace's IRC daemon",
		Long: `Stop the workspace's IRC daemon, leaving everything it owns in place.

The daemon shuts the bot down, stops the managed server if it supervises
one, and exits. Nothing is removed: start brings the workspace back with
the same ports, accounts and history. Stopping a workspace whose daemon
is already down succeeds and reports it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCStop(ctx, app, opts, args[0])
			})
		},
	}
	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// runIRCStop stops a workspace's daemon.
func runIRCStop(ctx context.Context, app *appContext, opts *globalOptions, ref string) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if _, err := rt.loadEnabledProfile(); err != nil {
		return err
	}
	stopped, err := stopDaemon(rt.paths)
	if err != nil {
		return err
	}

	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), map[string]any{
			"workspace": rt.ws.Name,
			"id":        rt.ws.ID,
			"stopped":   stopped,
		})
	}
	if stopped {
		fmt.Fprintf(os.Stderr, "Stopped IRC daemon for %q\n", rt.ws.Name)
	} else {
		fmt.Fprintf(os.Stderr, "No IRC daemon running for %q\n", rt.ws.Name)
	}
	return nil
}

// newIRCRestartCmd builds `wsm irc restart`.
func newIRCRestartCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart <workspace>",
		Short: "Restart a workspace's IRC daemon",
		Long: `Stop the workspace's IRC daemon if it is running, then start it again.

Managed mode re-checks the recorded port triple between the two halves,
after the old daemon has released it, so a port taken by something else
in the meantime is reported rather than inherited. Restarting a stopped
workspace simply starts it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCRestart(ctx, app, opts, args[0])
			})
		},
	}
	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	return cmd
}

// runIRCRestart stops then starts a workspace's daemon.
func runIRCRestart(ctx context.Context, app *appContext, opts *globalOptions, ref string) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}
	if _, err := stopDaemon(rt.paths); err != nil {
		return err
	}
	// Checked after the stop: the ports are only observable once the
	// daemon that held them has gone.
	if err := checkRecordedPorts(profile, rt.paths); err != nil {
		return err
	}
	status, _, err := startDaemon(rt)
	if err != nil {
		return err
	}
	return renderIRCEnabled(rt, profile, status, opts)
}

// newIRCSyncCmd builds `wsm irc sync`.
func newIRCSyncCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync <workspace>",
		Short: "Reconcile a workspace's IRC state now",
		Long: `Ask the running daemon to reconcile immediately.

Reconciliation creates missing accounts, registers missing channels and
brings roles in line with the workspace's members. The daemon does this
on start and whenever members.yaml changes; sync is for making it happen
now without waiting.

The request is a signal to the running daemon, so a workspace whose
daemon is stopped is an error rather than a silent no-op: nothing would
have reconciled.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCSync(ctx, app, opts, args[0])
			})
		},
	}
	cli.SetSideEffect(cmd, cli.SideEffectWriteShared)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// runIRCSync signals the running daemon to reconcile.
func runIRCSync(ctx context.Context, app *appContext, opts *globalOptions, ref string) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	if _, err := rt.loadEnabledProfile(); err != nil {
		return err
	}

	status, running := daemonState(rt.paths)
	if !running {
		return &output.Error{
			Code:         output.CodeGeneric,
			Message:      fmt.Sprintf("irc daemon not running for %q", rt.ws.Name),
			SuggestedFix: fmt.Sprintf("run `wsm irc start %s`", rt.ws.Name),
			ExitCode:     1,
		}
	}
	if err := signalDaemon(status.PID, syscall.SIGHUP); err != nil {
		return &output.Error{
			Code:     output.CodeGeneric,
			Message:  fmt.Sprintf("signal irc daemon %d: %v", status.PID, err),
			ExitCode: 1,
		}
	}

	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), map[string]any{
			"workspace": rt.ws.Name,
			"id":        rt.ws.ID,
			"synced":    true,
			"pid":       status.PID,
		})
	}
	fmt.Fprintf(os.Stderr, "Requested reconcile for %q\n", rt.ws.Name)
	return nil
}

// newIRCStatusCmd builds `wsm irc status`.
func newIRCStatusCmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <workspace>",
		Short: "Show a workspace's IRC state",
		Long: `Report what the workspace's IRC presence is doing.

The daemon maintains the machine-readable half of this (state, pids,
negotiated capabilities, the last reconcile); this command adds what only
it can know — the workspace's name, its configuration warnings, and, when
an ergo HTTP API is reachable, live channel occupancy.

A workspace whose daemon is not running reports state "stopped" and exits
0: "it is not running" is an answer, not a failure.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runIRCStatus(ctx, app, opts, args[0])
			})
		},
	}
	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

// ircStatusDoc is the `wsm irc status --json` document.
//
// It is the daemon's status.json plus the three things the daemon
// cannot supply: the workspace's human name, the warnings that need the
// config to spot, and live channel occupancy from the server's API.
type ircStatusDoc struct {
	Workspace   string             `json:"workspace"`
	ID          string             `json:"id"`
	Mode        irc.Mode           `json:"mode"`
	Provider    irc.ProviderName   `json:"provider"`
	State       ircdaemon.State    `json:"state"`
	PID         int                `json:"pid,omitempty"`
	ErgoPID     int                `json:"ergo_pid,omitempty"`
	Ports       *irc.Ports         `json:"ports,omitempty"`
	Remote      bool               `json:"remote"`
	Server      ircdaemon.Server   `json:"server"`
	Caps        ircdaemon.Caps     `json:"caps"`
	Since       *time.Time         `json:"since,omitempty"`
	Reasons     []string           `json:"reasons,omitempty"`
	Restarts    int                `json:"restarts,omitempty"`
	Warnings    []string           `json:"warnings,omitempty"`
	ErgoVersion string             `json:"ergo_version,omitempty"`
	Channels    []ircStatusChannel `json:"channels,omitempty"`
	SharedWith  []string           `json:"shared_with,omitempty"`
	Host        string             `json:"host_workspace,omitempty"`

	// Discovered lists the creator-namespace channels wsm has adopted.
	Discovered []provision.DiscoveredChannel `json:"discovered,omitempty"`
	// Peer lists channels founded by another wsm bot on this server.
	Peer []provision.PeerChannel `json:"peer,omitempty"`
	// Unmanaged lists registrations wsm does not own, per the configured
	// unmanaged_channels policy.
	Unmanaged []provision.UnmanagedChannel `json:"unmanaged,omitempty"`
	// LastDiscover is the most recent discovery pass, embedded verbatim.
	LastDiscover *provision.DiscoverReport `json:"last_discover,omitempty"`
}

// ircStatusChannel is one channel's live state.
type ircStatusChannel struct {
	Name       string `json:"name"`
	Registered bool   `json:"registered"`
	Members    int    `json:"members"`
}

// runIRCStatus reports a workspace's IRC state.
func runIRCStatus(ctx context.Context, app *appContext, opts *globalOptions, ref string) error {
	rt, err := resolveIRCRuntime(ctx, app, ref)
	if err != nil {
		return err
	}
	profile, err := rt.loadEnabledProfile()
	if err != nil {
		return err
	}

	status, running := daemonState(rt.paths)
	doc := ircStatusDoc{
		Workspace:   rt.ws.Name,
		ID:          rt.ws.ID,
		Mode:        profile.Mode,
		Provider:    profile.Provider,
		State:       status.State,
		Remote:      profile.Remote,
		Server:      status.Server,
		Caps:        status.Caps,
		Reasons:     status.Reasons,
		Restarts:    status.Restarts,
		ErgoVersion: status.ErgoVersion,
		SharedWith:  profile.SharedWith,
		Host:        profile.HostWorkspace,
		Warnings:    ircStatusWarnings(rt.cfg, status),

		// Discovery results come from the daemon's document rather than
		// from a live pass, and are reported whether or not a daemon is
		// running now: the last status.json still records what wsm
		// adopted, and dropping it on a stopped workspace would report
		// no adopted channels where there are several.
		Discovered:   status.Discovered,
		Peer:         status.Peer,
		Unmanaged:    status.Unmanaged,
		LastDiscover: status.LastDiscover,
	}
	if profile.Ports.Valid() {
		ports := profile.Ports
		doc.Ports = &ports
	}
	if running {
		doc.PID = status.PID
		doc.ErgoPID = status.ErgoPID
		since := status.Since
		doc.Since = &since
		doc.Channels = liveChannels(ctx, rt, profile)
	}

	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		return output.Render(os.Stdout, machineFormat(opts), doc)
	}
	return renderIRCStatusTable(doc)
}

// ircStatusWarnings collects the non-fatal findings status reports.
//
// Two sources, because neither sees the whole picture: Config.Validate
// judges the configuration alone, while the daemon's codes come from
// the live runtime. The creator-namespace overlap warning on
// provision.DesiredState is deliberately NOT covered here — it needs
// the member list, which only the daemon reads, and surfacing a stale
// copy of it would be worse than pointing at the reconcile report that
// carries the current one.
func ircStatusWarnings(cfg irc.Config, status ircdaemon.Status) []string {
	seen := map[string]bool{}
	var out []string
	add := func(msg string) {
		if msg != "" && !seen[msg] {
			seen[msg] = true
			out = append(out, msg)
		}
	}

	warns, _ := cfg.Validate()
	for _, w := range warns {
		add(w.String())
	}
	for _, code := range status.Warnings {
		add(code)
	}
	sort.Strings(out)
	return out
}

// liveChannels asks the server which channels exist and how many
// people are in them.
//
// Best-effort by design: the API is optional in external mode and
// unreachable whenever the server is down, and a status command that
// failed because an enrichment failed would be useless exactly when it
// is needed most. A failure yields no channel rows, and the state and
// reasons above already explain why.
func liveChannels(ctx context.Context, rt *ircRuntime, profile irc.Profile) []ircStatusChannel {
	api := ircAPIConfig(rt, profile)
	if api == nil {
		return nil
	}
	infos, err := ergo.NewClient(api).List(ctx)
	if err != nil {
		return nil
	}

	prefix := rt.cfg.ChannelPrefix
	rows := make([]ircStatusChannel, 0, len(infos))
	for _, info := range infos {
		if prefix != "" && !strings.Contains(info.Name, prefix) {
			continue
		}
		rows = append(rows, ircStatusChannel{
			Name:       info.Name,
			Registered: info.Registered,
			Members:    info.Users,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// ircAPIConfig addresses the ergo HTTP API for this workspace, or
// reports nil when there is none to address.
func ircAPIConfig(rt *ircRuntime, profile irc.Profile) *provider.APIConfig {
	switch profile.Mode {
	case irc.ModeManaged:
		if !profile.Ports.Valid() {
			return nil
		}
		token, err := irc.LoadAPIToken(rt.paths.APIToken)
		if err != nil {
			return nil
		}
		return &provider.APIConfig{
			BaseURL: "http://" + irc.DefaultBind + ":" + strconv.Itoa(profile.Ports.API),
			Token:   token,
		}
	case irc.ModeExternal:
		if rt.cfg.Server.ErgoAPI == "" {
			return nil
		}
		token, err := irc.ReadAPIToken(rt.cfg, rt.paths, nil)
		if err != nil {
			return nil
		}
		return &provider.APIConfig{
			BaseURL: strings.TrimSuffix(rt.cfg.Server.ErgoAPI, "/"),
			Token:   token,
		}
	default:
		return nil
	}
}

// ircStatusRow is the table projection of a status document.
type ircStatusRow struct {
	Field string `table:"FIELD,priority=9" json:"field"`
	Value string `table:"VALUE,priority=9" json:"value"`
}

// renderIRCStatusTable renders the human-readable status.
func renderIRCStatusTable(doc ircStatusDoc) error {
	rows := []ircStatusRow{
		{Field: "WORKSPACE", Value: doc.Workspace},
		{Field: "STATE", Value: string(doc.State)},
		{Field: "MODE", Value: string(doc.Mode)},
		{Field: "PROVIDER", Value: string(doc.Provider)},
	}
	if doc.PID > 0 {
		rows = append(rows, ircStatusRow{Field: "PID", Value: strconv.Itoa(doc.PID)})
	}
	if doc.ErgoPID > 0 {
		rows = append(rows, ircStatusRow{Field: "ERGO PID", Value: strconv.Itoa(doc.ErgoPID)})
	}
	if doc.Ports != nil {
		rows = append(rows, ircStatusRow{
			Field: "PORTS",
			Value: fmt.Sprintf("%d, %d, %d", doc.Ports.Plain, doc.Ports.Remote, doc.Ports.API),
		})
	}
	if doc.Server.Host != "" {
		rows = append(rows, ircStatusRow{
			Field: "SERVER",
			Value: fmt.Sprintf("%s:%d", doc.Server.Host, doc.Server.Port),
		})
	}
	for _, ch := range doc.Channels {
		rows = append(rows, ircStatusRow{
			Field: "CHANNEL",
			Value: fmt.Sprintf("%s (%d)", ch.Name, ch.Members),
		})
	}
	// Adopted, peer and unmanaged channels are listed after the owned
	// ones and kept distinct from them: they are the channels wsm
	// captures but does not author, and folding them into CHANNEL would
	// read as "wsm maintains these", which is exactly what it does not do.
	for _, ch := range doc.Discovered {
		value := fmt.Sprintf("%s (%s)", ch.Name, ch.Owner)
		if !ch.Joined {
			reason := ch.Reason
			if reason == "" {
				reason = "not joined"
			}
			value += " — " + reason
		}
		rows = append(rows, ircStatusRow{Field: "DISCOVERED", Value: value})
	}
	for _, ch := range doc.Peer {
		rows = append(rows, ircStatusRow{
			Field: "PEER",
			Value: fmt.Sprintf("%s (%s)", ch.Name, ch.Founder),
		})
	}
	for _, ch := range doc.Unmanaged {
		value := ch.Name
		if ch.Founder != "" {
			value += fmt.Sprintf(" (%s)", ch.Founder)
		}
		if ch.Purged {
			value += " — purged"
		}
		rows = append(rows, ircStatusRow{Field: "UNMANAGED", Value: value})
	}
	if err := output.Render(os.Stdout, output.Table, rows); err != nil {
		return err
	}

	// §6.3: diagnostics are non-data → stderr, so `wsm irc status |
	// column` still sees only the table.
	for _, reason := range doc.Reasons {
		fmt.Fprintf(os.Stderr, "degraded: %s\n", reason)
	}
	for _, warn := range doc.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
	}
	return nil
}

// renderIRCEnabled reports the outcome of a verb that leaves a daemon running.
func renderIRCEnabled(
	rt *ircRuntime,
	profile irc.Profile,
	status ircdaemon.Status,
	opts *globalOptions,
) error {
	if opts.Quiet {
		return nil
	}
	if isMachineFormat(opts) {
		payload := map[string]any{
			"workspace": rt.ws.Name,
			"id":        rt.ws.ID,
			"mode":      profile.Mode,
			"provider":  profile.Provider,
			"state":     status.State,
			"pid":       status.PID,
		}
		if profile.Ports.Valid() {
			payload["ports"] = profile.Ports
		}
		return output.Render(os.Stdout, machineFormat(opts), payload)
	}
	fmt.Fprintf(os.Stderr, "IRC %s for %q (daemon pid %d)\n",
		status.State, rt.ws.Name, status.PID)
	return nil
}

// isMachineFormat reports whether the caller asked for a structured
// document, either explicitly or by piping the output somewhere.
func isMachineFormat(opts *globalOptions) bool {
	if opts.Format == output.JSON || opts.Format == output.YAML {
		return true
	}
	return !stdoutIsTTY()
}

// machineFormat resolves which structured format to emit. A piped
// invocation that named no format gets JSON, the lingua franca of the
// tools on the other end of the pipe.
func machineFormat(opts *globalOptions) string {
	if opts.Format == output.JSON || opts.Format == output.YAML {
		return opts.Format
	}
	return output.JSON
}
