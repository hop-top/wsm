// Package cli — theme.go defines the Theme struct and builder for kit CLIs.
//
// Two named palettes ship with kit:
//
//   - Neon  — vivid: grass green (#7ED957), neon pink (#FF00FF), white titles
//   - Dark  — softer: lime (#C1FF72), pink (#FF66C4), white titles
//
// CharmTone is used only for semantic/utility colors (error, muted, warn).
package cli

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
)

// Palette holds the two brand colors used across a theme.
type Palette struct {
	Command color.Color // commands / primary accent
	Flag    color.Color // flags / secondary accent
}

// Built-in palettes.
var (
	Neon = Palette{
		Command: lipgloss.Color("#7ED957"), // grass green
		Flag:    lipgloss.Color("#FF00FF"), // vivid neon pink
	}
	Dark = Palette{
		Command: lipgloss.Color("#C1FF72"), // lime
		Flag:    lipgloss.Color("#FF66C4"), // pink
	}
)

// Theme holds semantic colors and pre-built lipgloss styles for CLI output.
type Theme struct {
	// Brand colors.
	Palette Palette

	// Semantic colors.
	Accent    color.Color
	Secondary color.Color
	Muted     color.Color
	Error     color.Color
	Success   color.Color

	// Pre-built styles.
	Title  lipgloss.Style
	Subtle lipgloss.Style
	Bold   lipgloss.Style
}

// buildTheme constructs a Theme. When accent is non-empty it is used as the
// command color; otherwise the Neon palette is used.
func buildTheme(accent string) Theme {
	p := Neon
	if accent != "" {
		p.Command = lipgloss.Color(accent)
	}
	return themeFromPalette(p)
}

// themeFromPalette builds a full Theme from a Palette.
func themeFromPalette(p Palette) Theme {
	muted := color.Color(charmtone.Squid)
	white := color.Color(lipgloss.Color("#FFFFFF"))

	return Theme{
		Palette:   p,
		Accent:    p.Command,
		Secondary: p.Flag,
		Muted:     muted,
		Error:     color.Color(charmtone.Cherry),
		Success:   color.Color(charmtone.Guac),

		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(white),
		Subtle: lipgloss.NewStyle().
			Foreground(muted),
		Bold: lipgloss.NewStyle().
			Bold(true),
	}
}
