package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
	kitcliconfig "hop.top/kit/go/console/cli/config"
	"hop.top/kit/go/console/output"
	"hop.top/kit/go/core/xdg"
	"hop.top/wsm/pkg/backend"
	pkgcli "hop.top/wsm/pkg/cli"
	"hop.top/wsm/pkg/config"
	"hop.top/wsm/pkg/workspace"
)

func newInitCmd(opts *globalOptions) *cobra.Command {
	var hop, force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize local config directory",
		Long: `Create a local config directory (.wsm/ or .hop/wsm/) in the current
directory with a minimal config.yaml and a symlink from the other path.

By default creates .wsm/ and symlinks .hop/wsm/ -> .wsm/.
With --hop, creates .hop/wsm/ and symlinks .wsm/ -> .hop/wsm/.

Re-running on an already-initialized directory fails with an
"already exists" error unless --force is passed, in which case the
existing directory is replaced with the same minimal config.yaml.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			configPath, err := config.InitLocalConfig(dir, hop, force)
			if err != nil {
				return err
			}
			mode := ".wsm/"
			if hop {
				mode = ".hop/wsm/"
			}
			// §6.3: ack message (non-data) → stderr.
			fmt.Fprintf(os.Stderr, "Initialized %s in %s\n", mode, dir)
			if configPath != "" {
				fmt.Fprintf(os.Stderr, "Config file: %s\n", configPath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&hop, "hop", false, "Use .hop/wsm/ as primary (for hop-managed projects)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing config directories")
	cli.SetTopLevelVerb(cmd)
	cli.SetSideEffect(cmd, cli.SideEffectWriteLocal)
	// Idempotent under --force: re-running with the same flags produces the
	// same minimal config.yaml. Without --force, re-runs fail "already exists"
	// without mutating anything, which is also observably stable per §8.5.
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	return cmd
}

func newConfigCmd(opts *globalOptions) *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show resolved configuration",
		Long: `Print the effective wsm configuration after walking the precedence
chain — project (.wsm.yaml / .wsm/config.yaml / .hop/wsm/config.yaml),
user (XDG config dir), system (/etc/wsm/config.yaml) — and applying any
--config / --data-dir / --backend / --backend-url overrides. Honors the
global --format flag for JSON or YAML output.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.LoadOptions{
				FlagBackend:    opts.Backend,
				FlagDataDir:    opts.DataDir,
				FlagBackendURL: opts.BackendURL,
				FlagConfig:     opts.ConfigFile,
			})
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
	// Layer-A: pure read of the resolved config layers. Idempotency is
	// auto-applied by kit's defaultIdempotency table (show → yes).
	cli.SetSideEffect(showCmd, cli.SideEffectRead)
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration commands",
	}
	cmd.AddCommand(showCmd)
	// kit/cli/config: shared `path` and `paths` subcommands that walk
	// the precedence chain. wsmConfigResolver mirrors pkg/config.Load
	// rung order (project → user → system) so `wsm config paths`
	// agrees with `wsm config show`.
	kitcliconfig.RegisterPathSubcommands(cmd, "wsm",
		kitcliconfig.WithResolver(wsmConfigResolver))
	return cmd
}

// wsmConfigResolver returns the wsm config precedence chain at cwd,
// highest-precedence first. Mirrors pkg/config.Load: project ./.wsm.yaml
// or ./.wsm/config.yaml or ./.hop/wsm/config.yaml → user XDG → system
// /etc/wsm/config.yaml.
func wsmConfigResolver(cwd string) []kitcliconfig.ResolvedPath {
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	out := make([]kitcliconfig.ResolvedPath, 0, 5)
	for _, name := range []string{
		".wsm.yaml",
		filepath.Join(".wsm", "config.yaml"),
		filepath.Join(".hop", "wsm", "config.yaml"),
	} {
		p := filepath.Join(cwd, name)
		out = append(out, kitcliconfig.ResolvedPath{
			Path:   p,
			Source: "project",
			Scope:  "project",
			Exists: pathExists(p),
		})
	}
	if p, err := xdg.ConfigDir("wsm"); err == nil {
		userPath := filepath.Join(p, "config.yaml")
		out = append(out, kitcliconfig.ResolvedPath{
			Path:   userPath,
			Source: "user",
			Scope:  "user",
			Exists: pathExists(userPath),
		})
	}
	out = append(out, kitcliconfig.ResolvedPath{
		Path:   "/etc/wsm/config.yaml",
		Source: "system",
		Scope:  "system",
		Exists: pathExists("/etc/wsm/config.yaml"),
	})
	return out
}

// pathExists reports whether path stats successfully.
func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func newCompletionCmd(root *cobra.Command, _ *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			shell := args[0]
			switch shell {
			case "bash":
				return root.GenBashCompletion(os.Stdout)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return workspace.NewErrorUsage("unsupported shell")
			}
		},
	}
	return cmd
}

func newTUICmd(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "tui",
		Short:  "Launch terminal UI",
		Long: `Launch the bubbletea-driven workspace browser. Interactive surface
exposes the workspace list with inspect (enter), create (c), archive
toggle (a), and delete (d) keybindings dispatched through the same
backend as the CLI; mutations occur only on explicit user keypress.
Not suitable for batch / agent use.`,
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runTUI(ctx, app, *opts)
			})
		},
	}
	cli.SetTopLevelVerb(cmd)
	// Layer-A: long-running session-bound interactive surface. Idempotency
	// depends on which verbs the operator invokes inside the TUI, so the
	// conditional tier is the honest classification.
	cli.SetSideEffect(cmd, cli.SideEffectInteractive)
	cli.SetIdempotency(cmd, cli.IdempotencyConditional)
	return cmd
}

func registerWorkspaceCompletion(root *cobra.Command, _ *globalOptions) {
	completer := func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		// Read flags from root to get data-dir etc.
		flags := cmd.Root().PersistentFlags()
		dataDir, _ := flags.GetString("data-dir")
		backendFlag, _ := flags.GetString("backend")
		backendURL, _ := flags.GetString("backend-url")

		app, err := newAppContext(globalOptions{
			DataDir:    dataDir,
			Backend:    backendFlag,
			BackendURL: backendURL,
		})
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer func() { _ = app.Close() }()
		items, err := app.manager.List(context.Background(), backend.ListOptions{SortBy: "name", SortDir: "asc"})
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return pkgcli.WorkspaceCompletionValues(items), cobra.ShellCompDirectiveNoFileComp
	}

	// Manual walk to ensure we find commands across nested groups
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			use := c.Name()
			parent := cmd.Name()

			// workspace show, archive, unarchive, delete
			if parent == "workspace" {
				switch use {
				case "show", "archive", "unarchive", "delete":
					c.ValidArgsFunction = completer
				}
			}

			// snapshot generate, show, context, list (flag)
			if parent == "snapshot" {
				switch use {
				case "generate", "show", "context":
					c.ValidArgsFunction = completer
				case "list":
					_ = c.RegisterFlagCompletionFunc("workspace", completer)
				}
			}

			// event list, show, add
			if parent == "event" {
				switch use {
				case "list", "show":
					c.ValidArgsFunction = completer
				case "add":
					c.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
						if len(args) == 0 {
							return completer(cmd, args, toComplete)
						}
						if len(args) == 1 {
							out := make([]string, 0, len(manualAppendableEventTypes()))
							for _, v := range manualAppendableEventTypes() {
								if strings.HasPrefix(v, toComplete) {
									out = append(out, v)
								}
							}
							return out, cobra.ShellCompDirectiveNoFileComp
						}
						return nil, cobra.ShellCompDirectiveNoFileComp
					}
				}
			}

			walk(c)
		}
	}
	walk(root)
}
