package scope

import (
	"testing"
	"time"
)

func TestAssignmentSet_AssignAndUnassign(t *testing.T) {
	as := NewAssignmentSet()
	a := Assignment{
		ID:          "a1",
		ProfileID:   "p1",
		WorkspaceID: "ws1",
		AssignedAt:  time.Now(),
	}

	as.Assign(a)
	if as.Len() != 1 {
		t.Fatalf("expected 1, got %d", as.Len())
	}

	as.Unassign("a1")
	if as.Len() != 0 {
		t.Fatalf("expected 0 after unassign, got %d", as.Len())
	}
}

func TestAssignmentSet_ForWorkspace(t *testing.T) {
	as := NewAssignmentSet()
	now := time.Now()
	as.Assign(Assignment{ID: "a1", ProfileID: "p1", WorkspaceID: "ws1", AssignedAt: now})
	as.Assign(Assignment{ID: "a2", ProfileID: "p2", WorkspaceID: "ws1", AssignedAt: now})
	as.Assign(Assignment{ID: "a3", ProfileID: "p3", WorkspaceID: "ws2", AssignedAt: now})

	ws1 := as.ForWorkspace("ws1")
	if len(ws1) != 2 {
		t.Errorf("expected 2 for ws1, got %d", len(ws1))
	}

	ws2 := as.ForWorkspace("ws2")
	if len(ws2) != 1 {
		t.Errorf("expected 1 for ws2, got %d", len(ws2))
	}

	ws3 := as.ForWorkspace("ws3")
	if len(ws3) != 0 {
		t.Errorf("expected 0 for ws3, got %d", len(ws3))
	}
}

func TestAssignmentSet_ForProfile(t *testing.T) {
	as := NewAssignmentSet()
	now := time.Now()
	as.Assign(Assignment{ID: "a1", ProfileID: "p1", WorkspaceID: "ws1", AssignedAt: now})
	as.Assign(Assignment{ID: "a2", ProfileID: "p1", WorkspaceID: "ws2", AssignedAt: now})
	as.Assign(Assignment{ID: "a3", ProfileID: "p2", WorkspaceID: "ws1", AssignedAt: now})

	p1 := as.ForProfile("p1")
	if len(p1) != 2 {
		t.Errorf("expected 2 for p1, got %d", len(p1))
	}

	p2 := as.ForProfile("p2")
	if len(p2) != 1 {
		t.Errorf("expected 1 for p2, got %d", len(p2))
	}
}

func TestAssignmentSet_WithScopes(t *testing.T) {
	as := NewAssignmentSet()
	scopes := []Scope{
		{ID: "s1", Type: ScopeFilePatterns, Patterns: []string{"*.go"}, Allow: true},
		{ID: "s2", Type: ScopeOperations, Patterns: []string{"build"}, Allow: true},
	}
	a := Assignment{
		ID:          "a1",
		ProfileID:   "p1",
		SquadID:     "sq1",
		WorkspaceID: "ws1",
		SpaceID:     "sp1",
		Scopes:      scopes,
		AssignedAt:  time.Now(),
	}
	as.Assign(a)

	list := as.List()
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if len(list[0].Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(list[0].Scopes))
	}
}

func TestAssignmentSet_OverwriteByID(t *testing.T) {
	as := NewAssignmentSet()
	now := time.Now()
	as.Assign(Assignment{ID: "a1", ProfileID: "p1", WorkspaceID: "ws1", AssignedAt: now})
	as.Assign(Assignment{ID: "a1", ProfileID: "p2", WorkspaceID: "ws1", AssignedAt: now})

	if as.Len() != 1 {
		t.Fatalf("expected 1 after overwrite, got %d", as.Len())
	}
	list := as.List()
	if list[0].ProfileID != "p2" {
		t.Errorf("expected overwritten profile p2, got %s", list[0].ProfileID)
	}
}
