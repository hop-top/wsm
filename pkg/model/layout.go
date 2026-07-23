package model

import "fmt"

// Layout is an optional, opaque organizational layout attached to a
// workspace. wsm never parses content at Path — the pair is metadata
// interpreted by external tooling.
type Layout struct {
	// Kind is an opaque free-form discriminator, e.g. "agentcompanies/v1"
	// or "none". Required when a layout is attached.
	Kind string `json:"kind" yaml:"kind"`

	// Path is a filesystem path, relative to a space boundary or
	// absolute. May be empty.
	Path string `json:"path" yaml:"path"`
}

// Validate checks required fields. Path may be empty.
func (l *Layout) Validate() error {
	if l.Kind == "" {
		return fmt.Errorf("layout: kind is required")
	}
	return nil
}
