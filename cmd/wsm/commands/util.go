package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

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
With --hop, creates .hop/wsm/ and symlinks .wsm/ -> .hop/wsm/.`,
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
			fmt.Fprintf(os.Stdout, "Initialized %s in %s\n", mode, dir)
			if configPath != "" {
				fmt.Fprintf(os.Stdout, "Config file: %s\n", configPath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&hop, "hop", false, "Use .hop/wsm/ as primary (for hop-managed projects)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing config directories")
	return cmd
}

func newConfigCmd(opts *globalOptions) *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show resolved configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.LoadOptions{FlagBackend: opts.Backend, FlagDataDir: opts.DataDir, FlagBackendURL: opts.BackendURL})
			if err != nil {
				return workspace.NewErrorBackend("load config", err)
			}
			if shouldJSON(*opts) {
				return writeJSON(os.Stdout, cfg)
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
	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"cfg"},
		Short:   "Configuration commands",
	}
	cmd.AddCommand(showCmd)
	return cmd
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
		Use:     "tui",
		Aliases: []string{"ui"},
		Short:   "Launch terminal UI",
		Hidden:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(opts, func(ctx context.Context, app *appContext) error {
				return runTUI(ctx, app, *opts)
			})
		},
	}
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

			// workspace show, archive, unarchive, remove
			if parent == "workspace" {
				switch use {
				case "show", "archive", "unarchive", "remove":
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
