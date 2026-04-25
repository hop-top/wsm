package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ResetFlags resets all flag values on cmd and its children to their
// defaults. Call before re-executing a Root in tests, or between
// sequential command invocations in the same process.
//
// This walks the entire command tree recursively, resetting both
// local and persistent flags on every command. Viper bindings
// are preserved (they re-read from flags on next access).
func ResetFlags(cmd *cobra.Command) {
	resetFlagSet(cmd.Flags())
	resetFlagSet(cmd.PersistentFlags())
	for _, child := range cmd.Commands() {
		ResetFlags(child)
	}
}

func resetFlagSet(fs *pflag.FlagSet) {
	fs.VisitAll(func(f *pflag.Flag) {
		if err := f.Value.Set(f.DefValue); err != nil {
			panic("cli.ResetFlags: cannot reset flag " +
				f.Name + " to default " + f.DefValue +
				": " + err.Error())
		}
		f.Changed = false
	})
}

// Reset restores all flags to defaults and clears args.
// After Reset, subsequent Execute() calls run with an explicit
// empty arg list rather than falling back to os.Args.
func (r *Root) Reset() {
	ResetFlags(r.Cmd)
	r.Cmd.SetArgs([]string{})
}
