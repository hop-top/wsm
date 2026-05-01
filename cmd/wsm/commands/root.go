package commands

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	kitlog "hop.top/kit/go/console/log"
	"hop.top/kit/go/core/upgrade"
	"hop.top/wsm/pkg/backend"
	backendsqlite "hop.top/wsm/pkg/backend/sqlite"
	"hop.top/wsm/pkg/config"
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

// commandGroups maps top-level subcommand name to its kit group ID per
// CLI conventions §4. management is auto-hidden by kit unless --help-all.
var commandGroups = map[string]string{
	"init":       "interact",
	"tui":        "interact",
	"workspace":  "organize",
	"space":      "organize",
	"device":     "organize",
	"config":     "management",
	"event":      "capture",
	"snapshot":   "capture",
	"completion": "management",
	"version":    "management",
	"upgrade":    "management",
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
}

type appContext struct {
	cfg     config.Config
	backend backend.Backend
	manager *workspace.Manager
}

func (a *appContext) Close() error {
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
		return nil, workspace.NewErrorUsage("only sqlite backend is currently implemented")
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
	mgr, err := workspace.NewManager(b,
		workspace.WithSessionID(opts.SessionID),
		workspace.WithAgentID(opts.AgentID),
	)
	if err != nil {
		_ = b.Close()
		return nil, workspace.NewErrorBackend("create manager", err)
	}

	return &appContext{cfg: cfg, backend: b, manager: mgr}, nil
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
		Help: cli.HelpConfig{
			Disclaimer: "Workspace & Session Manager (wsm) for IDE-agnostic session storage.",
			// management is pre-registered + auto-hidden by cli.New;
			// adding it here would duplicate the rendered section.
			Groups: []cli.GroupConfig{
				{ID: "organize", Title: "ORGANIZE"},
				{ID: "capture", Title: "CAPTURE"},
				{ID: "interact", Title: "INTERACT"},
			},
		},
		Globals: []cli.Flag{
			{Name: "backend", Usage: "Backend: sqlite"},
			{Name: "backend-url", Usage: "Backend URL (reserved for future backends)"},
			{Name: "config", Usage: "Path to config file (overrides XDG location)"},
			{Name: "data-dir", Usage: "Data directory"},
			{Name: "session-id", Usage: "Session ID for event tracking"},
			{Name: "agent-id", Default: "user", Usage: "Agent ID for event tracking"},
		},
	})
	root := r.Cmd

	// Bridge kit-bound globals into opts so subcommands keep reading
	// opts.X. Existing PersistentPreRunE chain (set by cli.New for
	// chdir + identity) is preserved.
	prev := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
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
		opts.Format = r.Viper.GetString("format")
		opts.Quiet = r.Viper.GetBool("quiet")
		opts.NoColor = r.Viper.GetBool("no-color")
		// CLI conv §6.2: install kit/log as default slog handler.
		// -V (verbose count) raises level: 0=info, 1=debug, 2+=trace.
		slog.SetDefault(slog.New(kitlog.WithVerbose(r.Viper, r.VerboseCount())))
		if cmd.Name() != "upgrade" {
			upgrade.NotifyIfAvailable(cmd.Context(), newChecker(), os.Stderr)
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
	// cli.New sets Args: cobra.NoArgs; relax for the bare-invocation hint.
	root.Args = cobra.NoArgs

	root.AddCommand(
		newInitCmd(&opts),
		newWorkspaceGroupCmd(&opts),
		newSpaceGroupCmd(&opts),
		newEventGroupCmd(&opts),
		newDeviceGroupCmd(&opts),
		newSnapshotGroupCmd(&opts),
		newConfigCmd(&opts),
		newCompletionCmd(root, &opts),
		newTUICmd(&opts),
		newUpgradeCmd(),
		newVersionCmd(),
	)

	applyCommandGroups(root)
	registerWorkspaceCompletion(root, &opts)
	kitRootInstance = r
	return root
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
		return err
	}
	defer func() { _ = app.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return fn(ctx, app)
}
