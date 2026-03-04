package owsf

import "hop.top/wsm/pkg/model"

type SpaceTree struct {
	spaces   map[string]model.Space
	children map[string][]string
}

func NewSpaceTree() *SpaceTree {
	return &SpaceTree{
		spaces:   make(map[string]model.Space),
		children: make(map[string][]string),
	}
}

func (t *SpaceTree) Add(s model.Space) error {
	if _, ok := t.spaces[s.ID]; ok {
		return ErrSpaceExists
	}
	if s.Parent != "" {
		if _, ok := t.spaces[s.Parent]; !ok {
			return ErrParentNotFound
		}
	}
	t.spaces[s.ID] = s
	t.children[s.Parent] = append(t.children[s.Parent], s.ID)
	return nil
}

func (t *SpaceTree) Get(id string) (model.Space, bool) {
	s, ok := t.spaces[id]
	return s, ok
}

func (t *SpaceTree) Reparent(_, _ string) error {
	return ErrParentImmutable
}

// WouldCycle returns true if making childID a parent of nodeID
// would create a cycle (i.e. nodeID is an ancestor of childID).
func (t *SpaceTree) WouldCycle(nodeID, proposedParentID string) bool {
	cur := proposedParentID
	for cur != "" {
		if cur == nodeID {
			return true
		}
		s, ok := t.spaces[cur]
		if !ok {
			break
		}
		cur = s.Parent
	}
	return false
}

// MutationResult describes what happens when a space is mutated.
type MutationResult struct {
	RequiresDestroyCreate bool
	ChangedFields         []string
	Events                []string
}

// ValidateMutation checks which fields changed between old and new space.
// Immutable fields (id, parent, type, boundary, capabilities) trigger
// destroy+create with corresponding topology events (C3/I3 fixes).
func (t *SpaceTree) ValidateMutation(old, new model.Space) MutationResult {
	var changed []string
	if old.ID != new.ID {
		changed = append(changed, "id")
	}
	if old.Parent != new.Parent {
		changed = append(changed, "parent")
	}
	if old.Type != new.Type {
		changed = append(changed, "type")
	}
	if old.Boundary != new.Boundary {
		changed = append(changed, "boundary")
	}
	if !stringSliceEqual(old.Capabilities, new.Capabilities) {
		changed = append(changed, "capabilities")
	}

	result := MutationResult{
		RequiresDestroyCreate: len(changed) > 0,
		ChangedFields:         changed,
	}
	if result.RequiresDestroyCreate {
		result.Events = []string{
			"topology.space_destroy",
			"topology.space_create",
		}
	}
	return result
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// AncestorChain returns the path from root to id (inclusive).
// G1 fix: forward-append + reverse instead of O(n^2) prepend.
func (t *SpaceTree) AncestorChain(id string) []string {
	var chain []string
	cur := id
	for cur != "" {
		chain = append(chain, cur)
		s, ok := t.spaces[cur]
		if !ok {
			break
		}
		cur = s.Parent
	}
	// Reverse so root is first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
