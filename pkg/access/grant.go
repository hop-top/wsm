package access

import (
	"sync"
	"time"
)

// Permission represents an access permission level.
type Permission string

const (
	PermRead    Permission = "read"
	PermWrite   Permission = "write"
	PermAdmin   Permission = "admin"
	PermExecute Permission = "execute"
)

// Grant represents an access grant for a subject on a resource.
type Grant struct {
	ID           string       `json:"id"`
	SubjectType  string       `json:"subject_type"`
	SubjectID    string       `json:"subject_id"`
	ResourceType string       `json:"resource_type"`
	ResourceID   string       `json:"resource_id"`
	Permissions  []Permission `json:"permissions"`
	GrantedAt    time.Time    `json:"granted_at"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
}

// HasPermission reports whether the grant includes the given permission.
func (g Grant) HasPermission(perm Permission) bool {
	for _, p := range g.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// IsExpired reports whether the grant has expired.
func (g Grant) IsExpired() bool {
	if g.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*g.ExpiresAt)
}

// GrantSet is a thread-safe collection of grants.
type GrantSet struct {
	mu     sync.RWMutex
	grants map[string]Grant
}

// NewGrantSet returns an empty GrantSet.
func NewGrantSet() *GrantSet {
	return &GrantSet{grants: make(map[string]Grant)}
}

// Add inserts or replaces a grant by ID.
func (gs *GrantSet) Add(g Grant) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.grants[g.ID] = g
}

// Remove deletes a grant by ID.
func (gs *GrantSet) Remove(id string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	delete(gs.grants, id)
}

// Check reports whether subjectID has perm on resourceID via any
// non-expired grant in the set.
func (gs *GrantSet) Check(subjectID, resourceID string, perm Permission) bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	for _, g := range gs.grants {
		if g.SubjectID == subjectID && g.ResourceID == resourceID &&
			!g.IsExpired() && g.HasPermission(perm) {
			return true
		}
	}
	return false
}

// List returns all grants in insertion-indeterminate order.
func (gs *GrantSet) List() []Grant {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	out := make([]Grant, 0, len(gs.grants))
	for _, g := range gs.grants {
		out = append(out, g)
	}
	return out
}
