// Package alias provides framework-agnostic command alias management
// backed by a YAML file.
package alias

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Store manages command aliases backed by a YAML file.
type Store struct {
	path    string
	aliases map[string]string // name → target command path
}

// NewStore creates a store that reads/writes aliases at path.
// The file need not exist yet.
func NewStore(path string) *Store {
	return &Store{
		path:    path,
		aliases: make(map[string]string),
	}
}

// Load reads aliases from the YAML file. A missing file is not an error
// (the store stays empty).
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("alias: read %s: %w", s.path, err)
	}
	m := make(map[string]string)
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("alias: parse %s: %w", s.path, err)
	}
	s.aliases = m
	return nil
}

// Save writes the current aliases to the YAML file, creating parent
// directories as needed.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("alias: mkdir %s: %w", filepath.Dir(s.path), err)
	}
	data, err := yaml.Marshal(s.aliases)
	if err != nil {
		return fmt.Errorf("alias: marshal: %w", err)
	}
	return os.WriteFile(s.path, data, 0644)
}

// Set adds or updates an alias. Name must be non-empty without whitespace;
// target must be non-empty.
func (s *Store) Set(name, target string) error {
	if name == "" || strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf(
			"alias: name %q must be non-empty without whitespace", name,
		)
	}
	if target == "" {
		return fmt.Errorf("alias: target must be non-empty")
	}
	s.aliases[name] = target
	return nil
}

// Remove deletes an alias. Returns an error if the name does not exist.
func (s *Store) Remove(name string) error {
	if _, ok := s.aliases[name]; !ok {
		return fmt.Errorf("alias: %q not found", name)
	}
	delete(s.aliases, name)
	return nil
}

// Get returns the target for an alias and whether it exists.
func (s *Store) Get(name string) (string, bool) {
	v, ok := s.aliases[name]
	return v, ok
}

// All returns a copy of the alias map.
func (s *Store) All() map[string]string {
	out := make(map[string]string, len(s.aliases))
	for k, v := range s.aliases {
		out[k] = v
	}
	return out
}

// Expand replaces args[0] with its target (split on spaces) if it matches
// an alias, appending remaining args. Returns args unchanged if no match.
func (s *Store) Expand(args []string) []string {
	if len(args) == 0 {
		return args
	}
	target, ok := s.aliases[args[0]]
	if !ok {
		return args
	}
	expanded := strings.Fields(target)
	return append(expanded, args[1:]...)
}
