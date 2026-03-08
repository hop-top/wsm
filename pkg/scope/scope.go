package scope

import (
	"path/filepath"
	"sync"
)

// ScopeType identifies the category of access a scope controls.
type ScopeType string

const (
	ScopeFilePatterns ScopeType = "file_patterns"
	ScopeOperations   ScopeType = "operations"
	ScopeToolAccess   ScopeType = "tool_access"
	ScopeSecrets      ScopeType = "secrets"
	ScopeNetwork      ScopeType = "network"
)

// Scope defines a single access constraint.
type Scope struct {
	ID       string    `json:"id"`
	Type     ScopeType `json:"type"`
	Patterns []string  `json:"patterns"`
	Allow    bool      `json:"allow"` // true = allowlist, false = denylist
}

// Match reports whether value matches any pattern in this scope.
// For file_patterns, filepath.Match is used. For all others, exact match.
func (s Scope) Match(value string) bool {
	for _, p := range s.Patterns {
		if s.Type == ScopeFilePatterns {
			if ok, _ := filepath.Match(p, value); ok {
				return true
			}
		} else {
			if p == value {
				return true
			}
		}
	}
	return false
}

// ScopeSet is a thread-safe collection of scopes.
type ScopeSet struct {
	mu     sync.RWMutex
	scopes map[string]Scope
}

// NewScopeSet creates an empty ScopeSet.
func NewScopeSet() *ScopeSet {
	return &ScopeSet{scopes: make(map[string]Scope)}
}

// Add inserts or replaces a scope by ID.
func (ss *ScopeSet) Add(s Scope) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.scopes[s.ID] = s
}

// Remove deletes a scope by ID.
func (ss *ScopeSet) Remove(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.scopes, id)
}

// Match reports whether value matches any scope of the given type.
// Returns false if no scopes of that type exist.
func (ss *ScopeSet) Match(scopeType ScopeType, value string) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	for _, s := range ss.scopes {
		if s.Type == scopeType && s.Match(value) {
			return true
		}
	}
	return false
}

// List returns all scopes in the set. Order is not guaranteed.
func (ss *ScopeSet) List() []Scope {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	out := make([]Scope, 0, len(ss.scopes))
	for _, s := range ss.scopes {
		out = append(out, s)
	}
	return out
}

// Len returns the number of scopes.
func (ss *ScopeSet) Len() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.scopes)
}

// Get retrieves a scope by ID. Returns the scope and whether it was found.
func (ss *ScopeSet) Get(id string) (Scope, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	s, ok := ss.scopes[id]
	return s, ok
}
