package scope

import (
	"testing"
	"time"
)

func TestIntersectScopes(t *testing.T) {
	a := NewScopeSet()
	a.Add(Scope{ID: "s1", Type: ScopeFilePatterns, Patterns: []string{"*.go"}})
	a.Add(Scope{ID: "s2", Type: ScopeOperations, Patterns: []string{"build"}})
	a.Add(Scope{ID: "s3", Type: ScopeNetwork, Patterns: []string{"localhost"}})

	b := NewScopeSet()
	b.Add(Scope{ID: "s2", Type: ScopeOperations, Patterns: []string{"test"}})
	b.Add(Scope{ID: "s4", Type: ScopeSecrets, Patterns: []string{"TOKEN"}})

	result := IntersectScopes(a, b)
	if result.Len() != 1 {
		t.Fatalf("expected 1 in intersection, got %d", result.Len())
	}
	s, ok := result.Get("s2")
	if !ok {
		t.Fatal("expected s2 in intersection")
	}
	// Intersection keeps scope from a
	if s.Patterns[0] != "build" {
		t.Errorf("expected pattern from a ('build'), got %s", s.Patterns[0])
	}
}

func TestIntersectScopes_Empty(t *testing.T) {
	a := NewScopeSet()
	a.Add(Scope{ID: "s1", Type: ScopeToolAccess, Patterns: []string{"git"}})

	b := NewScopeSet()

	result := IntersectScopes(a, b)
	if result.Len() != 0 {
		t.Fatalf("expected 0 in intersection with empty, got %d", result.Len())
	}
}

func TestUnionScopes(t *testing.T) {
	a := NewScopeSet()
	a.Add(Scope{ID: "s1", Type: ScopeFilePatterns, Patterns: []string{"*.go"}})
	a.Add(Scope{ID: "s2", Type: ScopeOperations, Patterns: []string{"build"}})

	b := NewScopeSet()
	b.Add(Scope{ID: "s2", Type: ScopeOperations, Patterns: []string{"test"}})
	b.Add(Scope{ID: "s3", Type: ScopeSecrets, Patterns: []string{"TOKEN"}})

	result := UnionScopes(a, b)
	if result.Len() != 3 {
		t.Fatalf("expected 3 in union, got %d", result.Len())
	}

	// s2 should come from b (b wins on collision)
	s, _ := result.Get("s2")
	if s.Patterns[0] != "test" {
		t.Errorf("expected pattern from b ('test'), got %s", s.Patterns[0])
	}
}

func TestUnionScopes_BothEmpty(t *testing.T) {
	result := UnionScopes(NewScopeSet(), NewScopeSet())
	if result.Len() != 0 {
		t.Fatalf("expected 0, got %d", result.Len())
	}
}

func TestEffectiveScope(t *testing.T) {
	now := time.Now()
	assignments := []Assignment{
		{
			ID:          "a1",
			ProfileID:   "p1",
			WorkspaceID: "ws1",
			AssignedAt:  now,
			Scopes: []Scope{
				{ID: "s1", Type: ScopeFilePatterns, Patterns: []string{"*.go"}, Allow: true},
				{ID: "s2", Type: ScopeOperations, Patterns: []string{"build"}, Allow: true},
			},
		},
		{
			ID:          "a2",
			ProfileID:   "p2",
			WorkspaceID: "ws1",
			AssignedAt:  now,
			Scopes: []Scope{
				{ID: "s2", Type: ScopeOperations, Patterns: []string{"test"}, Allow: true},
				{ID: "s3", Type: ScopeNetwork, Patterns: []string{"*.internal"}, Allow: false},
			},
		},
	}

	result := EffectiveScope(assignments)
	if result.Len() != 3 {
		t.Fatalf("expected 3 effective scopes, got %d", result.Len())
	}

	// s2 should be overridden by second assignment
	s, _ := result.Get("s2")
	if s.Patterns[0] != "test" {
		t.Errorf("expected overridden pattern 'test', got %s", s.Patterns[0])
	}
}

func TestEffectiveScope_NoAssignments(t *testing.T) {
	result := EffectiveScope(nil)
	if result.Len() != 0 {
		t.Fatalf("expected 0 for nil assignments, got %d", result.Len())
	}
}

func TestEffectiveScope_NoScopes(t *testing.T) {
	assignments := []Assignment{
		{ID: "a1", ProfileID: "p1", WorkspaceID: "ws1", AssignedAt: time.Now()},
	}
	result := EffectiveScope(assignments)
	if result.Len() != 0 {
		t.Fatalf("expected 0 for assignments without scopes, got %d", result.Len())
	}
}
