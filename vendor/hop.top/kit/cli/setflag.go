package cli

import (
	"slices"
	"strings"
)

// SetFlag is a pflag.Value that supports +append, -remove, =replace
// semantics for list-valued flags.
//
//	--tag feat       append (default)
//	--tag +feat      append (explicit)
//	--tag -bug       remove
//	--tag =feat,docs replace all
//	--tag =          clear all
//
// Comma-separated values are split automatically. Duplicates are suppressed.
type SetFlag struct {
	items []string
}

// Set implements pflag.Value. Parses the +/-/= prefix and mutates items.
func (sf *SetFlag) Set(val string) error {
	if val == "" {
		return nil
	}

	switch val[0] {
	case '=':
		raw := val[1:]
		if raw == "" {
			sf.items = nil
			return nil
		}
		sf.items = splitAndTrim(raw)
		return nil
	case '-':
		target := val[1:]
		sf.items = slices.DeleteFunc(sf.items, func(s string) bool {
			return s == target
		})
		return nil
	case '+':
		val = val[1:]
	}

	for _, v := range splitAndTrim(val) {
		if !slices.Contains(sf.items, v) {
			sf.items = append(sf.items, v)
		}
	}
	return nil
}

// String implements pflag.Value.
func (sf *SetFlag) String() string {
	return strings.Join(sf.items, ",")
}

// Type implements pflag.Value.
func (sf *SetFlag) Type() string {
	return "set"
}

// Add appends val literally (no prefix interpretation).
func (sf *SetFlag) Add(val string) {
	if !slices.Contains(sf.items, val) {
		sf.items = append(sf.items, val)
	}
}

// Remove removes val literally (no prefix interpretation).
func (sf *SetFlag) Remove(val string) {
	sf.items = slices.DeleteFunc(sf.items, func(s string) bool {
		return s == val
	})
}

// Clear removes all items.
func (sf *SetFlag) Clear() {
	sf.items = nil
}

// Values returns a copy of the current items.
func (sf *SetFlag) Values() []string {
	if sf.items == nil {
		return nil
	}
	return slices.Clone(sf.items)
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
