package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/kit/alias"
	"hop.top/kit/output"
)

type aliasEntry struct {
	Alias  string `table:"ALIAS"  json:"alias"  yaml:"alias"`
	Target string `table:"TARGET" json:"target" yaml:"target"`
}

// AliasesCmd returns a hidden subcommand that lists active aliases.
func (r *Root) AliasesCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "aliases",
		Short:  "List active command aliases",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries := make([]aliasEntry, 0, len(r.aliases))
			for name, target := range r.aliases {
				entries = append(entries, aliasEntry{Alias: name, Target: target})
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Alias < entries[j].Alias
			})
			format := r.Viper.GetString("format")
			return output.Render(cmd.OutOrStdout(), format, entries)
		},
	}
}

// AliasCmd returns a command group for managing aliases backed by an
// alias.Store. Includes list (default), add, and remove subcommands.
func (r *Root) AliasCmd(store *alias.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage command aliases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return r.listAliases(cmd, store)
		},
	}

	cmd.AddCommand(r.aliasListCmd(store))
	cmd.AddCommand(r.aliasAddCmd(store))
	cmd.AddCommand(r.aliasRemoveCmd(store))
	return cmd
}

func (r *Root) aliasListCmd(store *alias.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List aliases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return r.listAliases(cmd, store)
		},
	}
}

func (r *Root) listAliases(cmd *cobra.Command, store *alias.Store) error {
	all := store.All()
	// merge runtime aliases from r.aliases
	for k, v := range r.aliases {
		if _, ok := all[k]; !ok {
			all[k] = v
		}
	}
	entries := make([]aliasEntry, 0, len(all))
	for name, target := range all {
		entries = append(entries, aliasEntry{Alias: name, Target: target})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Alias < entries[j].Alias
	})
	format := r.Viper.GetString("format")
	return output.Render(cmd.OutOrStdout(), format, entries)
}

func (r *Root) aliasAddCmd(store *alias.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <target...>",
		Short: "Add or update an alias",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			target := strings.Join(args[1:], " ")
			if err := store.Set(name, target); err != nil {
				return err
			}
			if err := store.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "alias %s → %s\n", name, target)
			return nil
		},
	}
}

func (r *Root) aliasRemoveCmd(store *alias.Store) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove an alias",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := store.Remove(name); err != nil {
				return err
			}
			if err := store.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed alias %s\n", name)
			return nil
		},
	}
}
