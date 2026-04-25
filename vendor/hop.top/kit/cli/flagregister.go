package cli

import "github.com/spf13/cobra"

// FlagDisplay controls which flag forms appear in help output.
// All forms always work for parsing regardless of display setting.
type FlagDisplay int

const (
	// FlagDisplayPrefix shows only --name with +/-/= prefix notation.
	FlagDisplayPrefix FlagDisplay = iota
	// FlagDisplayVerbose shows --add-name, --remove-name, --clear-name.
	FlagDisplayVerbose
	// FlagDisplayBoth shows all forms.
	FlagDisplayBoth
)

// RegisterSetFlag registers a set-valued flag on cmd with the given display
// style. Returns the shared SetFlag that accumulates values from all forms.
func RegisterSetFlag(cmd *cobra.Command, name, usage string, display FlagDisplay) *SetFlag {
	sf := &SetFlag{}

	showPrefix := display == FlagDisplayPrefix || display == FlagDisplayBoth
	showVerbose := display == FlagDisplayVerbose || display == FlagDisplayBoth

	// Always register prefix form so it parses.
	cmd.Flags().VarP(sf, name, "", usage+" (+add, -remove, =replace)")
	if !showPrefix {
		cmd.Flags().MarkHidden(name) //nolint:errcheck // best-effort hide
	}

	if showVerbose {
		cmd.Flags().Var(&setFlagAddAdapter{sf: sf}, "add-"+name, "Add to "+usage)
		cmd.Flags().Var(&setFlagRemoveAdapter{sf: sf}, "remove-"+name, "Remove from "+usage)
		cmd.Flags().BoolFunc("clear-"+name, "Clear all "+usage, func(_ string) error {
			sf.Clear()
			return nil
		})
	}

	return sf
}

// RegisterTextFlag registers a text-valued flag on cmd with the given display
// style. Returns the shared TextFlag that accumulates mutations.
func RegisterTextFlag(cmd *cobra.Command, name, usage string, display FlagDisplay) *TextFlag {
	tf := &TextFlag{}

	// Base --name always registered (replace semantics).
	cmd.Flags().VarP(tf, name, "", usage)

	showVerbose := display == FlagDisplayVerbose || display == FlagDisplayBoth

	if showVerbose {
		cmd.Flags().Var(&textFlagAppendAdapter{tf: tf}, name+"-append", "Append to "+usage+" (new line)")
		cmd.Flags().Var(&textFlagAppendInlineAdapter{tf: tf}, name+"-append-inline", "Append to "+usage+" (inline)")
		cmd.Flags().Var(&textFlagPrependAdapter{tf: tf}, name+"-prepend", "Prepend to "+usage+" (new line)")
		cmd.Flags().Var(&textFlagPrependInlineAdapter{tf: tf}, name+"-prepend-inline", "Prepend to "+usage+" (inline)")
	}

	return tf
}

// ── SetFlag verbose adapters ─────────────────────────────────────────────────

type setFlagAddAdapter struct {
	sf *SetFlag
}

func (a *setFlagAddAdapter) Set(val string) error { a.sf.Add(val); return nil }
func (a *setFlagAddAdapter) String() string       { return "" }
func (a *setFlagAddAdapter) Type() string         { return "string" }

type setFlagRemoveAdapter struct {
	sf *SetFlag
}

func (a *setFlagRemoveAdapter) Set(val string) error { a.sf.Remove(val); return nil }
func (a *setFlagRemoveAdapter) String() string       { return "" }
func (a *setFlagRemoveAdapter) Type() string         { return "string" }

// ── TextFlag verbose adapters ────────────────────────────────────────────────

type textFlagAppendAdapter struct{ tf *TextFlag }

func (a *textFlagAppendAdapter) Set(val string) error { a.tf.Append(val); return nil }
func (a *textFlagAppendAdapter) String() string       { return "" }
func (a *textFlagAppendAdapter) Type() string         { return "string" }

type textFlagAppendInlineAdapter struct{ tf *TextFlag }

func (a *textFlagAppendInlineAdapter) Set(val string) error { a.tf.AppendInline(val); return nil }
func (a *textFlagAppendInlineAdapter) String() string       { return "" }
func (a *textFlagAppendInlineAdapter) Type() string         { return "string" }

type textFlagPrependAdapter struct{ tf *TextFlag }

func (a *textFlagPrependAdapter) Set(val string) error { a.tf.Prepend(val); return nil }
func (a *textFlagPrependAdapter) String() string       { return "" }
func (a *textFlagPrependAdapter) Type() string         { return "string" }

type textFlagPrependInlineAdapter struct{ tf *TextFlag }

func (a *textFlagPrependInlineAdapter) Set(val string) error { a.tf.PrependInline(val); return nil }
func (a *textFlagPrependInlineAdapter) String() string       { return "" }
func (a *textFlagPrependInlineAdapter) Type() string         { return "string" }
