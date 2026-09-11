package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/alias"
	"hop.top/kit/go/console/cli"
	kitlog "hop.top/kit/go/console/log"
	"hop.top/kit/go/console/output"
	"hop.top/kit/go/core/upgrade"
	"hop.top/kit/go/core/xdg"
	"hop.top/kit/go/runtime/bus"
	"hop.top/wsm/pkg/backend"
	backendsqlite "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/irc"
	"hop.top/wsm/pkg/workspace"
)

var (
	// Version is the application version.
	Version = ""
	// CommitSHA is the git commit SHA.
	CommitSHA = ""

	// kitRootInstance holds the kit Root constructed by NewRootCmd so
	// Execute() can route through fang and apply group visibility.
	kitRootInstance *cli.Root
)

// Layer-A annotation conventions
//
// kit/side-effect:   set explicitly via cli.SetSideEffect on every leaf.
// kit/top-level-verb: set explicitly via cli.SetTopLevelVerb on depth-1 verbs.
// kit/hierarchical:  set explicitly via cli.SetHierarchical on intermediate
//                    ancestors of depth>=3 leaves (e.g., workspace, workspace
//                    config).
// kit/idempotent:    typically left implicit — kit auto-fills from its
//                    verb-name table during Root.Validate(). The table maps
//                    well-known verbs (list, show, get, find, search, info,
//                    delete, update, sync, create, add, ...) to their default
//                    classification. Adopters override explicitly only when
//                    the kit default is wrong for the verb's actual semantics
//                    (e.g., snapshot generate → IdempotencyNo, tui →
//                    IdempotencyConditional, init → IdempotencyYes).
//
// Per-site comments below repeat the rationale only where the implicit-default
// path is non-obvious (mutating verbs whose name happens to map to
// IdempotencyYes via the kit table). The annotation-coverage contract is
// regression-locked by TestLayerAAnnotationsPresent in root_test.go.

// aliasStorePath returns the path to the wsm alias YAML store. Falls
// back to a relative path when XDG resolution fails so tests/embedded
// runs still get a deterministic location.
func aliasStorePath() string {
	if p, err := xdg.ConfigDir("wsm"); err == nil {
		return filepath.Join(p, "aliases.yaml")
	}
	return filepath.Join(".config", "wsm", "aliases.yaml")
}

// commandGroups maps top-level subcommand name to its kit group ID per
// CLI conventions §4. management is auto-hidden by kit unless --help-all.
var commandGroups = map[string]string{
	"init":         "interact",
	"tui":          "interact",
	"workspace":    "organize",
	"space":        "organize",
	"device":       "organize",
	"config":       "management",
	"event":        "capture",
	"snapshot":     "capture",
	"irc":          "interact",
	"completion":   "management",
	"version":      "management",
	"upgrade":      "management",
	"alias":        "management",
	"capabilities": "management",
}

// applyCommandGroups assigns GroupID to each top-level subcommand based
// on the commandGroups map. Call after all root.AddCommand invocations.
func applyCommandGroups(root *cobra.Command) {
	for _, c := range root.Commands() {
		if g, ok := commandGroups[c.Name()]; ok {
			c.GroupID = g
		}
	}
}

// subcommandGroups defines how parent commands split their verbs between
// the KNOWLEDGE (read) and CURATE (mutate) help groups. Keyed by the
// parent command name; value maps verb name → group ID. Verbs absent
// from this map keep cobra's default (no group, "Available Commands").
var subcommandGroups = map[string]map[string]string{
	"workspace": {
		"list":      "knowledge",
		"show":      "knowledge",
		"add":       "curate",
		"archive":   "curate",
		"unarchive": "curate",
		"delete":    "curate",
		"config":    "curate",
	},
	"snapshot": {
		"list":    "knowledge",
		"show":    "knowledge",
		"context": "knowledge",
		// "generate" creates a snapshot; CURATE side.
		"generate": "curate",
		"create":   "curate",
		"update":   "curate",
		"delete":   "curate",
	},
	"event": {
		"list": "knowledge",
		"show": "knowledge",
		// CLI conv §4: appending an event is data CAPTURE.
		"add": "capture",
	},
}

// subcommandGroupTitles is the display title for each group ID used by
// subcommandGroups. Parents AddGroup these so cobra renders the
// section header in help output.
var subcommandGroupTitles = map[string]string{
	"knowledge": "KNOWLEDGE",
	"curate":    "CURATE",
	"capture":   "CAPTURE",
}

// applySubcommandGroups walks each parent that has an entry in
// subcommandGroups, registers the needed cobra groups on that parent,
// and assigns GroupID to its child verbs. Idempotent — safe to call
// after group commands have been added but before Execute.
func applySubcommandGroups(root *cobra.Command) {
	for _, parent := range root.Commands() {
		mapping, ok := subcommandGroups[parent.Name()]
		if !ok {
			continue
		}
		// Collect group IDs we'll need on this parent (deduped).
		needed := map[string]struct{}{}
		for _, gid := range mapping {
			needed[gid] = struct{}{}
		}
		for gid := range needed {
			if title, ok := subcommandGroupTitles[gid]; ok {
				parent.AddGroup(&cobra.Group{ID: gid, Title: title})
			}
		}
		for _, c := range parent.Commands() {
			if gid, ok := mapping[c.Name()]; ok {
				c.GroupID = gid
			}
		}
	}
}

type globalOptions struct {
	Backend    string
	DataDir    string
	BackendURL string
	ConfigFile string
	Format     string // from kit's --format (table|json|yaml)
	Quiet      bool
	NoColor    bool
	SessionID  string
	AgentID    string
	Offline    bool   // §5: skip network (sync, upgrade check, plugin downloads)
	Profile    string // §5: named credential / config profile
	Instance   string // §5: backend instance discriminator
}

type appContext struct {
	cfg          config.Config
	backend      backend.Backend
	manager      *workspace.Manager
	caps         *workspace.Capabilities
	bus          bus.Bus
	policyUnwire func()
	ircUnwire    func()
}

func (a *appContext) Close() error {
	if a.ircUnwire != nil {
		a.ircUnwire()
	}
	if a.policyUnwire != nil {
		a.policyUnwire()
	}
	if a.bus != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.bus.Close(ctx)
	}
	if a.backend != nil {
		return a.backend.Close()
	}
	return nil
}

func newAppContext(opts globalOptions) (*appContext, error) {
	cfg, err := config.Load(config.LoadOptions{
		FlagBackend:    opts.Backend,
		FlagDataDir:    opts.DataDir,
		FlagBackendURL: opts.BackendURL,
		FlagConfig:     opts.ConfigFile,
	})
	if err != nil {
		return nil, workspace.NewErrorBackend("load config", err)
	}

	if cfg.Backend == "" {
		cfg.Backend = "sqlite"
	}
	if cfg.Backend != "sqlite" {
		// An unrecognized --backend value is semantically a Backend
		// error, not a usage error. Routing through ErrorKindBackend
		// yields exit 5 (story 015 scenario 5) instead of 2.
		return nil, workspace.NewErrorBackend("unsupported backend "+cfg.Backend+" (only sqlite is currently implemented)", nil)
	}

	// Attempt legacy migration before opening the backend.
	if err := config.MigrateFromLegacy(cfg); err != nil {
		return nil, workspace.NewErrorBackend("migrate legacy data", err)
	}

	// Ensure XDG directories exist for first-run.
	for _, dir := range []string{cfg.DataDir, cfg.CacheDir, cfg.StateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, workspace.NewErrorBackend("create directory "+dir, err)
		}
	}

	dbPath := filepath.Join(cfg.DataDir, "wsm.db")
	b, err := backendsqlite.Open(dbPath)
	if err != nil {
		return nil, workspace.NewErrorBackend("open sqlite backend", err)
	}
	if err := b.Migrate(context.Background()); err != nil {
		_ = b.Close()
		return nil, workspace.NewErrorBackend("run sqlite migrations", err)
	}
	// kit/bus: in-memory bus for lifecycle events. Subscribers (aps,
	// ctxt, future plugins) can attach via Subscribe; cross-process
	// fan-out is the bus adapter's job, not ours.
	eventBus := bus.New()

	caps := resolveCapabilities()
	mgr, err := workspace.NewManager(b,
		workspace.WithSessionID(opts.SessionID),
		workspace.WithAgentID(opts.AgentID),
		workspace.WithBus(eventBus),
		workspace.WithCapabilities(caps),
	)
	if err != nil {
		_ = b.Close()
		return nil, workspace.NewErrorBackend("create manager", err)
	}

	// kit/runtime/policy: subscribe the engine to this bus so pre_*
	// veto seams enforce declared rules. Engine is a process-wide
	// singleton (built once on first call); each newAppContext gets a
	// fresh subscription that appContext.Close will tear down.
	unwire, err := wirePolicyToBus(eventBus)
	if err != nil {
		_ = b.Close()
		return nil, workspace.NewErrorBackend("wire policy engine", err)
	}

	app := &appContext{cfg: cfg, backend: b, manager: mgr, caps: caps, bus: eventBus, policyUnwire: unwire}

	// pkg/irc/provision: mirror workspace lifecycle onto IRC when the
	// operator asked for it (managed mode + auto_provision). Subscribe
	// decides that itself and returns a no-op unwire otherwise, so a
	// config failure here must not fail the command: IRC is an optional
	// feature and an unreadable irc block is not a reason to refuse
	// `wsm workspace list`.
	//
	// Provisioning runs inline on the publishing goroutine, so
	// `wsm workspace add` can block for up to the daemon readiness
	// timeout (~15s) while the new workspace's daemon comes up.
	if ircCfg, ircErr := irc.Load(cfg.ConfigFile); ircErr == nil && ircCfg.Enabled {
		app.ircUnwire = wireIRCToBus(eventBus, ircCfg, app)
	}

	return app, nil
}

// EnvDenyCapabilities names the capabilities to withhold from this
// invocation, comma-separated, or "all" to withhold every one.
//
// Capability denial is a library-level policy: the manager is
// constructed with AllCapabilities in production and no flag exposes
// anything else, which leaves the forbidden path (exit 6) unreachable
// from a real invocation and therefore untestable at the process
// boundary. This seam closes that gap the same way WSM_POLICY_FILE
// does for the policy engine — an environment variable a test sets,
// never a documented operator control.
const EnvDenyCapabilities = "WSM_DENY_CAPABILITIES"

// resolveCapabilities builds the capability policy for this process.
// The default is AllCapabilities; EnvDenyCapabilities narrows it.
func resolveCapabilities() *workspace.Capabilities {
	raw := strings.TrimSpace(os.Getenv(EnvDenyCapabilities))
	if raw == "" {
		return workspace.AllCapabilities()
	}
	if strings.EqualFold(raw, "all") {
		return workspace.DenyAll()
	}
	denied := map[string]bool{}
	for _, name := range strings.Split(raw, ",") {
		if name = strings.TrimSpace(name); name != "" {
			denied[name] = true
		}
	}
	return workspace.DenyCapabilities(denied)
}

// NewRootCmd creates the root command for wsm via kit/cli.
func NewRootCmd() *cobra.Command {
	opts := globalOptions{}

	// Set version from build info if not injected at build time
	if Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
			Version = info.Main.Version
		} else {
			Version = "unknown (built from source)"
		}
	}

	r := cli.New(cli.Config{
		Name:    "wsm",
		Version: Version,
		Short:   "Workspace & Session Manager",
		// kit v0.4 registers a built-in -c/--config StringArray for layered
		// overrides. wsm owns the --config name with single-path semantics
		// (override XDG config file location); suppress the built-in so the
		// custom global below remains the sole --config flag.
		Disable: cli.Disable{Config: true},
		// Three intentional additions over kit's default cap of 10:
		// `capabilities` (machine-readable command surface, F5), the
		// hardened `config` group (usage-erroring RunE makes it a
		// runnable depth-1 verb), and `irc` (workspace chat: its own
		// noun with a lifecycle of its own, so it groups under no
		// existing verb). Locked by TestTopLevelVerbCountLock.
		MaxTopLevelVerbs: 13,
		Help: cli.HelpConfig{
			Disclaimer: "Workspace & Session Manager (wsm) for IDE-agnostic session storage.",
			// management is pre-registered + auto-hidden by cli.New;
			// adding it here would duplicate the rendered section.
			Groups: []cli.GroupConfig{
				{ID: "organize", Title: "ORGANIZE"},
				{ID: "capture", Title: "CAPTURE"},
				{ID: "interact", Title: "INTERACT"},
				// CLI conv §4: KNOWLEDGE = read/inspect verbs;
				// CURATE = mutate/lifecycle verbs. Used by the
				// workspace, snapshot, and event subcommand
				// help layouts to split read vs write surface.
				{ID: "knowledge", Title: "KNOWLEDGE"},
				{ID: "curate", Title: "CURATE"},
			},
		},
		Globals: []cli.Flag{
			{Name: "backend", Usage: "Backend: sqlite"},
			{Name: "backend-url", Usage: "Backend URL (reserved for future backends)"},
			{Name: "config", Usage: "Path to config file (overrides XDG location)"},
			{Name: "data-dir", Usage: "Data directory"},
			{Name: "session-id", Usage: "Session ID for event tracking"},
			{Name: "agent-id", Default: "user", Usage: "Agent ID for event tracking"},
			// CLI conv §5 canonical globals. --offline is registered by
			// cli.New itself as a reserved session global; re-declaring it
			// here panics pflag with a redefinition. Read it off the Root
			// (r.Offline()) instead of a local binding.
			{Name: "profile", Usage: "Named profile (credentials/config)", StringVar: &opts.Profile},
			{Name: "instance", Usage: "Backend instance", StringVar: &opts.Instance},
		},
	},
		// kit/console/cli ships a fully-conforming `status` subcommand
		// (the validator's reserved depth-1 verb). Mount it so we satisfy
		// the reserved-status arm of Layer-A enforcement once
		// DisableValidate flips off. Kit's defaults cover profile, env,
		// auth, effective-config, and a workspace stub — wsm-specific
		// providers can be registered via r.RegisterStatusProvider in a
		// later slice without touching this wiring.
		cli.WithStatus(cli.StatusConfig{}),
	)
	root := r.Cmd

	// kit's effective-config section reports raw flag values, so the
	// backend and data dir wsm actually resolved stay invisible when
	// the flags are unset. Register a wsm section carrying the
	// resolved state the next command would read and write with.
	r.RegisterStatusProvider("wsm", wsmStatusProvider)

	// NO_COLOR honor: cli.New installs a process-wide default table style
	// (colors lifted from r.Theme) via output.SetDefaultTableStyle. The
	// styled-table renderer then routes through lipgloss v2's Style.Render,
	// which always emits full-fidelity 24-bit SGR even when NO_COLOR is set
	// (lipgloss v2 downsampling lives at the output writer layer, which
	// kit's renderer doesn't wrap). Clobbering the default with a zero
	// TableStyle restores plain box-drawing output for the NO_COLOR=1
	// env case at root construction time. The --no-color flag path is
	// handled in PersistentPreRunE below because viper only knows the
	// flag value after cobra has parsed args.
	//
	// Caveat (kit-side, not addressable here): lipgloss compat init runs
	// `HasDarkBackground(os.Stdin, os.Stdout)` at package load time,
	// emitting OSC 11 + CSI DA terminal probes to stdout. That happens
	// before any wsm code can run, so fully ANSI-free NO_COLOR output on
	// a TTY requires kit/lipgloss work. Tests asserting an ANSI-free TTY
	// capture must account for those probes.
	if os.Getenv("NO_COLOR") != "" {
		output.SetDefaultTableStyle(output.TableStyle{})
	}

	// Flag-parse failures are usage errors (exit 2), and the
	// corrective guidance must ride in Message: the framework-level
	// renderer prints only Code + Message, so guidance parked in
	// SuggestedFix alone would never reach stderr.
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		fix := "run '" + cmd.CommandPath() + " --help' for accepted flags"
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      err.Error() + " — " + fix,
			SuggestedFix: fix,
			ExitCode:     2,
		}
	})

	// Bridge kit-bound globals into opts so subcommands keep reading
	// opts.X. Existing PersistentPreRunE chain (set by cli.New for
	// chdir + identity) is preserved.
	prev := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// The declared output contract is closed: a --format value
		// outside kit's formatter registry is refused loudly as a
		// usage error, never silently coerced to the default.
		if err := validateFormatFlag(cmd); err != nil {
			return err
		}
		if prev != nil {
			if err := prev(cmd, args); err != nil {
				return err
			}
		}
		opts.Backend, _ = cmd.Flags().GetString("backend")
		opts.BackendURL, _ = cmd.Flags().GetString("backend-url")
		opts.ConfigFile, _ = cmd.Flags().GetString("config")
		opts.DataDir, _ = cmd.Flags().GetString("data-dir")
		opts.SessionID, _ = cmd.Flags().GetString("session-id")
		opts.AgentID, _ = cmd.Flags().GetString("agent-id")
		// opts.Offline / opts.Profile / opts.Instance are bound directly via
		// cli.Flag.{Bool,String}Var pointers, no manual lookup needed.
		opts.Format = r.Viper.GetString("format")
		opts.Quiet = r.Viper.GetBool("quiet")
		opts.NoColor = r.Viper.GetBool("no-color")
		// Mirror the NO_COLOR env handling in NewRootCmd for the
		// --no-color flag path: viper only knows the parsed flag here.
		if opts.NoColor {
			output.SetDefaultTableStyle(output.TableStyle{})
		}
		// CLI conv §6.2: install kit/log as default slog handler.
		// -V (verbose count) raises level: 0=info, 1=debug, 2+=trace.
		slog.SetDefault(slog.New(kitlog.WithVerbose(r.Viper, r.VerboseCount())))
		if cmd.Name() != "upgrade" && !opts.Offline {
			upgrade.NotifyIfAvailable(cmd.Context(), newChecker(), os.Stderr)
		}
		// Fail loud on policy misconfig (bad YAML, broken CEL, unknown
		// topic) at boot, before any subcommand RunE executes. The engine
		// itself is constructed lazily here; appContext.bus wires the
		// resulting engine via wirePolicyToBus on a per-command basis.
		if _, err := initPolicyEngine(); err != nil {
			return err
		}
		return nil
	}

	root.RunE = func(cmd *cobra.Command, _ []string) error {
		// CLI conv §6.3: quick-start hint is non-data → stderr.
		// Preserves `wsm | jq` (stdout stays empty for the bare invocation).
		w := cmd.ErrOrStderr()
		_, _ = w.Write([]byte("Workspace & Session Manager\n\n"))
		_, _ = w.Write([]byte("Quick start:\n"))
		_, _ = w.Write([]byte("  wsm workspace add my-project\n"))
		_, _ = w.Write([]byte("  wsm workspace list\n"))
		_, _ = w.Write([]byte("  wsm workspace show <id-or-name>\n"))
		_, _ = w.Write([]byte("\nRun 'wsm --help' for full command reference.\n"))
		return nil
	}
	// CLI conv §8.1: an unmatched top-level positional is a usage error
	// (exit 2) with a corrective envelope, never cobra.NoArgs' plain
	// error (which main can only map to the generic exit 1). The bare
	// invocation (len(args)==0) stays on the quick-start hint path.
	root.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		fix := "run 'wsm --help' to see available commands"
		return &output.Error{
			Code:         output.CodeUsage,
			Message:      fmt.Sprintf("unknown command %q for %q — %s", args[0], cmd.CommandPath(), fix),
			SuggestedFix: fix,
			ExitCode:     2,
		}
	}

	root.AddCommand(
		newInitCmd(&opts),
		newWorkspaceGroupCmd(&opts),
		newSpaceGroupCmd(&opts),
		newEventGroupCmd(&opts),
		newDeviceGroupCmd(&opts),
		newSnapshotGroupCmd(&opts),
		newIRCGroupCmd(&opts),
		newConfigCmd(&opts),
		newCompletionCmd(root, &opts),
		newTUICmd(&opts),
		newUpgradeCmd(),
		newVersionCmd(),
		newCapabilitiesCmd(r),
	)

	// kit/console/alias: YAML-backed user alias store at
	// $XDG_CONFIG_HOME/wsm/aliases.yaml. AliasCmd attaches add/remove/
	// list subcommands. Load failures are non-fatal — an unreadable
	// aliases file should not block the CLI.
	store := alias.NewStore(aliasStorePath())
	_ = store.Load()
	root.AddCommand(r.AliasCmd(store))

	applyCommandGroups(root)
	applySubcommandGroups(root)
	registerWorkspaceCompletion(root, &opts)
	kitRootInstance = r
	return root
}

// validateFormatFlag refuses --format values outside kit's formatter
// registry with a usage envelope. Only explicitly set flags are
// checked, so per-leaf defaults stay untouched.
func validateFormatFlag(cmd *cobra.Command) error {
	f := cmd.Flag("format")
	if f == nil || !f.Changed {
		return nil
	}
	val := f.Value.String()
	if val == "" {
		return nil
	}
	keys := output.Default.Keys()
	sort.Strings(keys)
	for _, k := range keys {
		if val == k {
			return nil
		}
	}
	valid := strings.Join(keys, ", ")
	return &output.Error{
		Code:         output.CodeUsage,
		Message:      fmt.Sprintf("invalid --format %q (valid: %s)", val, valid),
		SuggestedFix: "pass one of: " + valid,
		ExitCode:     2,
	}
}

// Execute runs the root command through kit/cli (fang-styled help, version,
// and group visibility for --help-all). Falls back to plain cobra Execute
// when NewRootCmd has not yet been invoked.
func Execute(ctx context.Context) error {
	if kitRootInstance == nil {
		return NewRootCmd().Execute()
	}
	return kitRootInstance.Execute(ctx)
}

func withApp(opts *globalOptions, fn func(ctx context.Context, app *appContext) error) error {
	app, err := newAppContext(*opts)
	if err != nil {
		return mapPolicyDenialError(err)
	}
	defer func() { _ = app.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return mapPolicyDenialError(fn(ctx, app))
}
