package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"

	"hop.top/upgrade"
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
)

type globalOptions struct {
	Backend    string
	DataDir    string
	BackendURL string
	JSON       bool
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

// NewRootCmd creates the root command for wsm.
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

	root := &cobra.Command{
		Use:           "wsm",
		Short:         "Workspace & Session Manager",
		Long:          "Workspace & Session Manager (wsm) for IDE-agnostic session storage.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Enhance version template with commit SHA if available
	if len(CommitSHA) >= 7 {
		vt := root.VersionTemplate()
		root.SetVersionTemplate(vt[:len(vt)-1] + " (" + CommitSHA[0:7] + ")\n")
	}

	root.PersistentFlags().StringVar(&opts.Backend, "backend", "", "Backend: sqlite")
	root.PersistentFlags().StringVar(&opts.DataDir, "data-dir", "", "Data directory")
	root.PersistentFlags().StringVar(&opts.BackendURL, "backend-url", "", "Backend URL (reserved for future backends)")
	root.PersistentFlags().BoolVar(&opts.JSON, "json", false, "Output JSON")
	root.PersistentFlags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Minimal output")
	root.PersistentFlags().BoolVar(&opts.NoColor, "no-color", false, "Disable styled output")
	root.PersistentFlags().StringVar(&opts.SessionID, "session-id", "", "Session ID for event tracking")
	root.PersistentFlags().StringVar(&opts.AgentID, "agent-id", "user", "Agent ID for event tracking")

	root.RunE = func(cmd *cobra.Command, _ []string) error {
		cmd.Printf("Workspace & Session Manager\n\n")
		cmd.Printf("Quick start:\n")
		cmd.Printf("  wsm workspace add my-project\n")
		cmd.Printf("  wsm workspace list\n")
		cmd.Printf("  wsm workspace show <id-or-name>\n")
		cmd.Printf("\nRun 'wsm --help' for full command reference.\n")
		return nil
	}

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

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Name() != "upgrade" {
			upgrade.NotifyIfAvailable(cmd.Context(), newChecker(), os.Stderr)
		}
		return nil
	}

	registerWorkspaceCompletion(root, &opts)
	return root
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
