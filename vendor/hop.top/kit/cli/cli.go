// Package cli provides a cobra+fang+viper root command factory that
// implements the hop-top CLI contract.
//
// Every tool built with this package gets the following automatically:
//
//   - -v / --version  prints "<name> v<version>" (handled by fang)
//   - --quiet         persistent flag, suppresses non-essential output (opt-out via Config.Disable.Quiet)
//   - --no-color      persistent flag, disables ANSI color (opt-out via Config.Disable.NoColor)
//   - help subcommand hidden; -h / --help flag retained
//   - completion subcommand in management group (hidden from default --help)
//   - styled help, errors, and man pages via fang
//   - a Theme built from CharmTone palette + optional accent
//
// Flags suppressed via Config.Disable are not registered and their viper keys
// will not be present. Subcommands that read Root.Viper should check
// Config.Disable before assuming a key exists.
//
// Typical usage:
//
//	root := cli.New(cli.Config{Name: "mytool", Version: "1.2.3", Short: "..."})
//	root.Cmd.AddCommand(doSomethingCmd())
//	if err := root.Execute(context.Background()); err != nil {
//		os.Exit(1)
//	}
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/output"
	"hop.top/kit/tui/parity"
)

// Disable controls which built-in global flags are suppressed.
// Zero value enables all built-ins.
type Disable struct {
	Format  bool // suppress --format flag
	Quiet   bool // suppress --quiet flag
	NoColor bool // suppress --no-color flag
	Hints   bool // suppress --no-hints flag
}

// Flag defines a tool-specific global persistent flag.
type Flag struct {
	Name    string // long name without -- (e.g. "verbose")
	Short   string // single char, optional (e.g. "v")
	Usage   string // description shown in --help
	Default string // string default; empty = no default
}

// GroupConfig defines a command group for the root help layout.
type GroupConfig struct {
	ID     string // cobra group ID (e.g. "management")
	Title  string // section header (e.g. "MANAGEMENT")
	Hidden bool   // hidden from default --help; shown with --help-all
}

// HelpConfig controls the root --help output layout.
// Zero value uses kit defaults loaded from tui/parity/parity.json.
type HelpConfig struct {
	// Disclaimer is appended to Short as the Long description when non-empty.
	// Empty = no disclaimer block.
	Disclaimer string
	// SectionOrder overrides the section rendering order (e.g. ["commands","options"]).
	// Empty = use parity.json default.
	SectionOrder []string
	// ShowAliases displays command aliases in help output (e.g. "deploy  d, dp").
	// Default false — aliases work for dispatch but are hidden from help.
	ShowAliases bool
	// Groups registers additional command groups beyond the built-in
	// "COMMANDS" (default, GroupID="") and "MANAGEMENT" (GroupID="management", hidden).
	Groups []GroupConfig
}

// Config holds the tool identity for root command construction.
type Config struct {
	// Name is the binary name as invoked by the user (e.g. "mytool").
	Name string
	// Version is the semver string printed by --version (e.g. "1.2.3").
	Version string
	// Short is the one-line description shown in help output.
	Short string
	// Accent is an optional hex color string (e.g. "#FF0000") used as the
	// theme accent. Zero value falls back to CharmTone Charple.
	Accent string
	// Disable opts out of specific built-in global flags. Zero value enables all.
	Disable Disable
	// Globals registers extra persistent flags on the root command.
	Globals []Flag
	// Help controls root --help layout. Zero value uses parity.json defaults.
	Help HelpConfig
}

// Root wraps the cobra root command, viper instance, theme, and hint
// registry.
type Root struct {
	// Cmd is the configured cobra root command. Add subcommands to it,
	// then call Execute(ctx) to run the CLI.
	Cmd *cobra.Command
	// Viper is the viper instance. Flags not suppressed by Config.Disable are
	// bound here. Subcommands should check Config.Disable before reading keys.
	Viper *viper.Viper
	// Config is the identity provided to New; retained for subcommands
	// that need the tool name or version at runtime.
	Config Config
	// Theme holds semantic colors and styles built from CharmTone +
	// the optional accent.
	Theme Theme
	// Hints is the per-command hint registry. Commands register
	// next-step hints here; the output pipeline renders them after
	// primary output when enabled.
	Hints *output.HintSet
	// Streams enforces stdout=data, stderr=human convention.
	Streams *StreamWriter
	// Auth provides credential introspection. Defaults to NoAuth.
	Auth AuthIntrospector

	aliases            map[string]string
	aliasCompletionSet bool              // guards single ValidArgsFunction wrap
	hiddenGroups       map[string]bool   // group IDs hidden from default --help
	groupTitles        map[string]string // group ID → display title
	overrideArgs       []string          // captured from SetArgs for pre-parse inspection
}

// New returns a Root pre-configured to the hop-top CLI contract:
//   - no help subcommand (only -h/--help flag)
//   - completion subcommand in management group (hidden from default --help)
//   - version handled by fang (-v/--version)
//   - persistent global flags: --quiet, --no-color
//   - styled help/errors via fang
func New(cfg Config) *Root {
	v := viper.New()

	long := cfg.Short
	if cfg.Help.Disclaimer != "" {
		long += "\n\n" + cfg.Help.Disclaimer
	}

	cmd := &cobra.Command{
		Use:          cfg.Name,
		Short:        cfg.Short,
		Long:         long,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
	}

	// Override cobra's default version template to print "<name> v<version>"
	// instead of "<name> version <version>".
	cmd.SetVersionTemplate(
		`{{with .DisplayName}}{{printf "%s " .}}{{end}}{{printf "v%s" .Version}}` + "\n")

	// Normalize: strip leading "v" so the template's "v%s" doesn't double it.
	cfg.Version = strings.TrimPrefix(cfg.Version, "v")

	// SectionOrder from Config or parity defaults — for documentation / cross-lang parity.
	// Go's section order is enforced by fang (COMMANDS before FLAGS); this field
	// is validated but not re-applied since fang owns the template.
	_ = cfg.Help.SectionOrder // consumed by TS/Python; Go relies on fang defaults
	_ = parity.Values.Help.SectionOrder

	// Built-in command groups: default "COMMANDS" (empty ID) + "MANAGEMENT" (hidden).
	cmd.AddGroup(
		&cobra.Group{ID: "management", Title: "MANAGEMENT"},
	)
	// Custom groups from config.
	for _, g := range cfg.Help.Groups {
		cmd.AddGroup(&cobra.Group{ID: g.ID, Title: g.Title})
	}

	// Hide the default help command; -h/--help flag remains.
	cmd.SetHelpCommand(&cobra.Command{Hidden: true})

	// Eagerly register -h/--help so it is available before Execute().
	cmd.InitDefaultHelpFlag()

	// Completion subcommand: let cobra register it (not disabled),
	// but place it in the management group so it's hidden from default --help.

	// --help-all: show all groups including hidden ones.
	// Stored on Root; checked in Execute before fang runs.
	cmd.Flags().Bool("help-all", false, "Show all commands including management")
	cmd.Flags().Lookup("help-all").NoOptDefVal = "true"

	// Per-group help flags: --help-<id> for each registered group.
	groupTitles := map[string]string{
		"management": "MANAGEMENT",
	}
	for _, g := range cfg.Help.Groups {
		groupTitles[g.ID] = g.Title
	}
	for id := range groupTitles {
		flagName := "help-" + id
		cmd.Flags().Bool(flagName, false, "Show only "+id+" commands")
		cmd.Flags().Lookup(flagName).NoOptDefVal = "true"
	}

	// Hidden "help" subcommand: accepts group ID or "all".
	helpSub := &cobra.Command{
		Use:    "help [group]",
		Short:  "Show help for a command group",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
	}
	cmd.AddCommand(helpSub)

	// Global persistent flags bound to viper.
	pf := cmd.PersistentFlags()
	if !cfg.Disable.Quiet {
		pf.Bool("quiet", false, "Suppress non-essential output")
		_ = v.BindPFlag("quiet", pf.Lookup("quiet"))
	}
	if !cfg.Disable.NoColor {
		pf.Bool("no-color", false, "Disable ANSI color")
		_ = v.BindPFlag("no-color", pf.Lookup("no-color"))
	}

	if !cfg.Disable.Format {
		output.RegisterFlags(cmd, v)
	}
	if !cfg.Disable.Hints {
		output.RegisterHintFlags(cmd, v)
	}

	// Tool-specific extra persistent flags.
	for _, g := range cfg.Globals {
		if g.Short != "" {
			pf.StringP(g.Name, g.Short, g.Default, g.Usage)
		} else {
			pf.String(g.Name, g.Default, g.Usage)
		}
		_ = v.BindPFlag(g.Name, pf.Lookup(g.Name))
	}

	theme := buildTheme(cfg.Accent)

	// Built-in "management" is always hidden; custom groups opt in via Hidden.
	hidden := map[string]bool{"management": true}
	for _, g := range cfg.Help.Groups {
		if g.Hidden {
			hidden[g.ID] = true
		}
	}

	return &Root{
		Cmd:          cmd,
		Viper:        v,
		Config:       cfg,
		Theme:        theme,
		Hints:        output.NewHintSet(),
		Streams:      NewStreamWriter(),
		Auth:         NoAuth{},
		aliases:      make(map[string]string),
		hiddenGroups: hidden,
		groupTitles:  groupTitles,
	}
}

// Execute runs the root command through fang, which provides styled help,
// version output, error rendering, and man page generation.
func (r *Root) Execute(ctx context.Context) error {
	if r.Config.Help.ShowAliases {
		annotateAliases(r.Cmd)
	}

	// Eagerly register cobra's completion command and place it in the
	// management group so it's hidden from default --help.
	r.Cmd.InitDefaultCompletionCmd()
	for _, c := range r.Cmd.Commands() {
		if c.Name() == "completion" {
			c.GroupID = "management"
			break
		}
	}

	r.applyGroupVisibility()
	return fang.Execute(ctx, r.Cmd,
		fang.WithVersion(r.Config.Version),
		fang.WithColorSchemeFunc(brandColorScheme),
	)
}

// applyGroupVisibility hides commands in hidden groups unless --help-all is
// present in the args. When --help-all is detected, args are rewritten to
// --help so fang renders the full help.
//
// Per-group help: --help-<id> or "help <id>" renders only that group's
// commands. "help all" is equivalent to --help-all.
func (r *Root) applyGroupVisibility() {
	args := r.resolveArgs()

	// Check for "help <id>" subcommand form.
	if len(args) >= 2 && args[0] == "help" {
		groupID := args[1]
		if groupID == "all" {
			r.Cmd.SetArgs([]string{"--help"})
			return
		}
		if _, ok := r.groupTitles[groupID]; !ok {
			// Wire up RunE to return an error for unknown group.
			helpCmd := r.findHelpSubcommand()
			if helpCmd != nil {
				helpCmd.RunE = func(cmd *cobra.Command, _ []string) error {
					return fmt.Errorf("unknown help group %q", groupID)
				}
			}
			return
		}
		r.installGroupHelp(groupID)
		return
	}

	// Check for --help-<id> flag form.
	for id := range r.groupTitles {
		flag := "--help-" + id
		for _, a := range args {
			if a == flag {
				r.installGroupHelp(id)
				return
			}
		}
	}

	// Check for --help-all.
	helpAll := false
	for _, a := range args {
		if a == "--help-all" {
			helpAll = true
			break
		}
	}
	if helpAll {
		cleaned := make([]string, 0, len(args))
		for _, a := range args {
			if a == "--help-all" {
				cleaned = append(cleaned, "--help")
			} else {
				cleaned = append(cleaned, a)
			}
		}
		r.Cmd.SetArgs(cleaned)
		return
	}
	// Default: hide commands in hidden groups.
	for _, c := range r.Cmd.Commands() {
		if r.hiddenGroups[c.GroupID] {
			c.Hidden = true
		}
	}
}

// findHelpSubcommand returns the hidden "help" subcommand.
func (r *Root) findHelpSubcommand() *cobra.Command {
	for _, c := range r.Cmd.Commands() {
		if c.Name() == "help" {
			return c
		}
	}
	return nil
}

// installGroupHelp rewrites the command tree so only the target group's
// commands are visible, then triggers --help rendering.
func (r *Root) installGroupHelp(groupID string) {
	// Hide all commands not in the target group.
	for _, c := range r.Cmd.Commands() {
		if c.GroupID != groupID {
			c.Hidden = true
		}
	}

	r.Cmd.SetArgs([]string{"--help"})
}

// SetArgs stores args for pre-parse inspection and passes them to cobra.
func (r *Root) SetArgs(args []string) {
	r.overrideArgs = args
	r.Cmd.SetArgs(args)
}

// resolveArgs returns the args that will be used by cobra's Execute.
// If SetArgs was called, those are returned; otherwise os.Args[1:].
func (r *Root) resolveArgs() []string {
	if r.overrideArgs != nil {
		return r.overrideArgs
	}
	if len(os.Args) > 1 {
		return os.Args[1:]
	}
	return nil
}

// annotateAliases appends "(aliases: x, y)" to Short for commands with aliases.
func annotateAliases(root *cobra.Command) {
	for _, c := range root.Commands() {
		if len(c.Aliases) > 0 {
			c.Short += " (aliases: " + strings.Join(c.Aliases, ", ") + ")"
		}
		annotateAliases(c)
	}
}

// brandColorScheme returns a fang ColorScheme with hop.top brand accents.
func brandColorScheme(c lipgloss.LightDarkFunc) fang.ColorScheme {
	cs := fang.DefaultColorScheme(c)
	cs.Title = lipgloss.Color("#FFFFFF")
	cs.Command = Neon.Command
	cs.Flag = Neon.Flag
	cs.Program = Neon.Command
	cs.Argument = lipgloss.Color("#B5E89B")
	cs.DimmedArgument = lipgloss.Color("#8ABF6E")
	return cs
}
