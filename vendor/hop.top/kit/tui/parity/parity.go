// Package parity provides cross-language TUI constants loaded from parity.json.
// This is the single source of truth for all TUI parity values (symbols,
// spinner frames, animation runes, timing, etc.) across Go, TypeScript, and Python.
//
// Other packages (tui, tui/styles) import from here; parity imports nothing
// from the kit tree to avoid circular dependencies.
package parity

import (
	_ "embed"
	"encoding/json"
)

//go:embed parity.json
var raw []byte

// SectionConfig holds display metadata for a single help section.
type SectionConfig struct {
	Title string `json:"title"`
}

// Data is the parsed content of parity.json.
type Data struct {
	Status struct {
		Symbols map[string]string `json:"symbols"`
	} `json:"status"`
	Spinner struct {
		Frames     []string `json:"frames"`
		IntervalMs int      `json:"interval_ms"`
	} `json:"spinner"`
	Anim struct {
		Runes        string `json:"runes"`
		IntervalMs   int    `json:"interval_ms"`
		DefaultWidth int    `json:"default_width"`
	} `json:"anim"`
	Help struct {
		SectionOrder []string                 `json:"section_order"`
		Sections     map[string]SectionConfig `json:"sections"`
	} `json:"help"`
}

// Values is the parsed parity data, available at init time.
var Values Data

func init() {
	if err := json.Unmarshal(raw, &Values); err != nil {
		panic("tui/parity: failed to parse parity.json: " + err.Error())
	}
}
