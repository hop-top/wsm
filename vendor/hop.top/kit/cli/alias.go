package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/kit/alias"
)

// Alias registers name as a root-level alias for target. If target is a
// direct child of Root.Cmd, cobra's Aliases field is used. For deeper
// commands a shim command is added to Root.Cmd that delegates to target.
//
// Returns an error on collision with an existing command or alias, or if
// name is empty / contains whitespace.
func (r *Root) Alias(name string, target *cobra.Command) error {
	if name == "" || strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("alias name %q must be non-empty without whitespace", name)
	}
	for _, c := range r.Cmd.Commands() {
		if c.Name() == name {
			return fmt.Errorf("alias %q collides with command %q", name, c.Name())
		}
	}
	if _, ok := r.aliases[name]; ok {
		return fmt.Errorf("alias %q already registered for %q", name, r.aliases[name])
	}

	path := commandPath(r.Cmd, target)

	if target.Parent() == r.Cmd {
		target.Aliases = append(target.Aliases, name)
	} else {
		shim := &cobra.Command{
			Use:    name,
			Short:  fmt.Sprintf("Alias for %s", path),
			Hidden: true,
			RunE:   target.RunE,
			Run:    target.Run,
		}
		r.Cmd.AddCommand(shim)
	}

	r.aliases[name] = path
	r.registerAliasCompletions()
	return nil
}

// Aliases returns a copy of the registered alias-to-target mapping.
func (r *Root) Aliases() map[string]string {
	out := make(map[string]string, len(r.aliases))
	for k, v := range r.aliases {
		out[k] = v
	}
	return out
}

// LoadAliases reads the "aliases" map from Viper config and registers each
// entry. Unknown target paths produce an error. Iteration order is sorted
// by alias name for deterministic behavior.
func (r *Root) LoadAliases() error {
	raw := r.Viper.GetStringMapString("aliases")
	if len(raw) == 0 {
		return nil
	}

	names := make([]string, 0, len(raw))
	for n := range raw {
		names = append(names, n)
	}
	sort.Strings(names)

	var errs []string
	for _, name := range names {
		path := raw[name]
		cmd, err := r.resolveCommand(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s -> %s: %v", name, path, err))
			continue
		}
		if err := r.Alias(name, cmd); err != nil {
			errs = append(errs, fmt.Sprintf("%s -> %s: %v", name, path, err))
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("alias errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// LoadAliasStore loads aliases from an alias.Store and registers each as
// a cobra command. Unknown targets are collected and returned as a single
// error.
func (r *Root) LoadAliasStore(store *alias.Store) error {
	all := store.All()
	if len(all) == 0 {
		return nil
	}

	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)

	var errs []string
	for _, name := range names {
		path := all[name]
		cmd, err := r.resolveCommand(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s -> %s: %v", name, path, err))
			continue
		}
		if err := r.Alias(name, cmd); err != nil {
			errs = append(errs, fmt.Sprintf("%s -> %s: %v", name, path, err))
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("alias errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// resolveCommand walks r.Cmd's subcommand tree using a space-separated path.
func (r *Root) resolveCommand(path string) (*cobra.Command, error) {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command path")
	}
	cur := r.Cmd
	for _, p := range parts {
		found := false
		for _, c := range cur.Commands() {
			if c.Name() == p {
				cur = c
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("command %q not found in %q", p, cur.Name())
		}
	}
	return cur, nil
}

// registerAliasCompletions sets ValidArgsFunction on Root.Cmd to include
// alias names alongside any previously registered completions.
func (r *Root) registerAliasCompletions() {
	aliases := r.aliases // capture current map ref
	original := r.Cmd.ValidArgsFunction

	// avoid re-wrapping: if we already wrapped, the closure below
	// captures r.aliases by reference so new entries are visible.
	if r.aliasCompletionSet {
		return
	}
	r.aliasCompletionSet = true

	r.Cmd.ValidArgsFunction = func(
		cmd *cobra.Command, args []string, toComplete string,
	) ([]string, cobra.ShellCompDirective) {
		var results []string
		directive := cobra.ShellCompDirectiveNoFileComp
		if original != nil {
			orig, origDir := original(cmd, args, toComplete)
			results = append(results, orig...)
			directive |= origDir
		}
		for name, target := range aliases {
			if strings.HasPrefix(name, toComplete) {
				results = append(results, name+"\t"+"alias for "+target)
			}
		}
		return results, directive
	}
}

// commandPath returns the target's path relative to root, e.g. "router start".
func commandPath(root, target *cobra.Command) string {
	full := target.CommandPath()
	prefix := root.Name() + " "
	return strings.TrimPrefix(full, prefix)
}
