// Package output — hint.go provides a registry for contextual next-step
// hints that CLI commands can register and the output pipeline renders
// after primary output.
//
// Hints guide users (and agents) toward the logical next action without
// burying them in a wall of text. They are suppressed when output is
// machine-formatted (JSON/YAML), piped (non-TTY), or explicitly disabled
// via config/env/flag.
package output

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Hint is a single next-step suggestion attached to a command.
type Hint struct {
	// Message is the human-readable hint text (e.g. "Run `hop version`
	// to verify").
	Message string
	// Condition returns true when this hint is relevant. A nil Condition
	// means the hint always applies.
	Condition func() bool
}

// HintSet is a concurrency-safe registry mapping command names to hints.
type HintSet struct {
	mu sync.RWMutex
	m  map[string][]Hint
}

// NewHintSet returns an empty hint registry.
func NewHintSet() *HintSet {
	return &HintSet{m: make(map[string][]Hint)}
}

// Register adds one or more hints for the given command name.
func (s *HintSet) Register(cmd string, hints ...Hint) {
	s.mu.Lock()
	s.m[cmd] = append(s.m[cmd], hints...)
	s.mu.Unlock()
}

// Lookup returns a copy of the hints registered for cmd. Returns nil
// when none are registered.
func (s *HintSet) Lookup(cmd string) []Hint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hints := s.m[cmd]
	if hints == nil {
		return nil
	}
	out := make([]Hint, len(hints))
	copy(out, hints)
	return out
}

// Active returns only the hints whose Condition is nil or returns true.
func Active(hints []Hint) []Hint {
	var out []Hint
	for _, h := range hints {
		if h.Condition == nil || h.Condition() {
			out = append(out, h)
		}
	}
	return out
}

// RegisterHintFlags adds the --no-hints persistent flag to cmd and binds
// it to the "no-hints" key in v.
func RegisterHintFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.PersistentFlags().Bool("no-hints", false,
		"Suppress next-step hints after command output")
	_ = v.BindPFlag("no-hints", cmd.PersistentFlags().Lookup("no-hints"))
}

// HintsEnabled reports whether hints should be rendered given the current
// viper config. Hints are disabled when any of:
//   - "no-hints" flag is true
//   - "hints.enabled" config key is false
//   - HOP_QUIET_HINTS env var is set to a truthy value ("1", "true", "yes")
//   - "quiet" flag is true
func HintsEnabled(v *viper.Viper) bool {
	if v.GetBool("no-hints") {
		return false
	}
	if v.IsSet("hints.enabled") && !v.GetBool("hints.enabled") {
		return false
	}
	if v.GetBool("quiet") {
		return false
	}
	switch os.Getenv("HOP_QUIET_HINTS") {
	case "1", "true", "yes":
		return false
	}
	return true
}

// RenderHints writes active hints to w with dimmed styling. It is a
// no-op when format is not Table, w is not a TTY, or hints are disabled
// via HintsEnabled.
//
// The muted color is used for the "→" prefix and hint text.
func RenderHints(w io.Writer, hints []Hint, format Format, v *viper.Viper, muted color.Color) {
	if format != Table {
		return
	}
	if !HintsEnabled(v) {
		return
	}
	if f, ok := w.(*os.File); !ok || !isatty.IsTerminal(f.Fd()) {
		return
	}

	active := Active(hints)
	if len(active) == 0 {
		return
	}

	noColor := v.GetBool("no-color")

	fmt.Fprintln(w)
	for _, h := range active {
		text := "→ " + h.Message
		if noColor {
			fmt.Fprintln(w, text)
		} else {
			fmt.Fprintln(w, lipgloss.NewStyle().
				Foreground(muted).Render(text))
		}
	}
}

// RegisterUpgradeHints adds a standard next-step hint for a binary's
// upgrade command: "Run `<binary> version` to verify."
//
// The hint is only active when *upgraded is true, allowing the caller to
// set the flag after a successful upgrade.
func RegisterUpgradeHints(hints *HintSet, binary string, upgraded *bool) {
	hints.Register("upgrade", Hint{
		Message:   "Run `" + binary + " version` to verify.",
		Condition: func() bool { return *upgraded },
	})
}

// RegisterVersionHints adds a standard next-step hint for a binary's
// version command: "Run `<binary> upgrade` to get latest."
//
// The hint is only active when *updateAvail is true, allowing the caller
// to set the flag after checking for updates.
func RegisterVersionHints(hints *HintSet, binary string, updateAvail *bool) {
	hints.Register("version", Hint{
		Message:   "Run `" + binary + " upgrade` to get latest.",
		Condition: func() bool { return *updateAvail },
	})
}
