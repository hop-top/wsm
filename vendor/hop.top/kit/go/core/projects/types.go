package projects

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the on-disk schema version this build understands.
// Reads of files with schema > SchemaVersion fail with ErrSchemaUnsupported
// (forward-incompatibility guard).
const SchemaVersion = 1

// Source identifies who registered a project entry. wsm-written entries
// are rewritten on workspace re-registration; manual entries (added via
// a future `rux project add`) are preserved across rewrites.
type Source string

const (
	// SourceWSM marks entries written by `wsm space add` / `wsm space sync`.
	SourceWSM Source = "wsm"
	// SourceManual marks entries added by hand or via `rux project add`.
	SourceManual Source = "manual"
)

// String implements fmt.Stringer.
func (s Source) String() string { return string(s) }

// UnmarshalYAML accepts the empty scalar as SourceWSM so older or hand-
// edited files without a `source:` field still load cleanly.
func (s *Source) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("projects: decode source: %w", err)
	}
	if raw == "" {
		*s = SourceWSM
		return nil
	}
	*s = Source(raw)
	return nil
}

// Entry is a single project registration.
type Entry struct {
	Path       string `yaml:"path"`
	StartupCmd string `yaml:"startup_cmd,omitempty"`
	Source     Source `yaml:"source,omitempty"`
}

// File is the on-disk shape of projects.yaml.
type File struct {
	Schema   int              `yaml:"schema"`
	Projects map[string]Entry `yaml:"projects"`
}
