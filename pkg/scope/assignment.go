package scope

import (
	"sync"
	"time"
)

// Assignment links a profile or squad to a workspace/space with scopes.
type Assignment struct {
	ID          string    `json:"id"`
	ProfileID   string    `json:"profile_id,omitempty"`
	SquadID     string    `json:"squad_id,omitempty"`
	WorkspaceID string    `json:"workspace_id"`
	SpaceID     string    `json:"space_id,omitempty"`
	Scopes      []Scope   `json:"scopes,omitempty"`
	AssignedAt  time.Time `json:"assigned_at"`
}

// AssignmentSet is a thread-safe collection of assignments.
type AssignmentSet struct {
	mu          sync.RWMutex
	assignments map[string]Assignment
}

// NewAssignmentSet creates an empty AssignmentSet.
func NewAssignmentSet() *AssignmentSet {
	return &AssignmentSet{assignments: make(map[string]Assignment)}
}

// Assign adds or replaces an assignment by ID.
func (as *AssignmentSet) Assign(a Assignment) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.assignments[a.ID] = a
}

// Unassign removes an assignment by ID.
func (as *AssignmentSet) Unassign(id string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	delete(as.assignments, id)
}

// ForWorkspace returns all assignments for the given workspace ID.
func (as *AssignmentSet) ForWorkspace(wsID string) []Assignment {
	as.mu.RLock()
	defer as.mu.RUnlock()
	var out []Assignment
	for _, a := range as.assignments {
		if a.WorkspaceID == wsID {
			out = append(out, a)
		}
	}
	return out
}

// ForProfile returns all assignments for the given profile ID.
func (as *AssignmentSet) ForProfile(profileID string) []Assignment {
	as.mu.RLock()
	defer as.mu.RUnlock()
	var out []Assignment
	for _, a := range as.assignments {
		if a.ProfileID == profileID {
			out = append(out, a)
		}
	}
	return out
}

// List returns all assignments. Order is not guaranteed.
func (as *AssignmentSet) List() []Assignment {
	as.mu.RLock()
	defer as.mu.RUnlock()
	out := make([]Assignment, 0, len(as.assignments))
	for _, a := range as.assignments {
		out = append(out, a)
	}
	return out
}

// Len returns the number of assignments.
func (as *AssignmentSet) Len() int {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return len(as.assignments)
}
