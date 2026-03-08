package scope

import "testing"

func TestScopeMatch_FilePatterns(t *testing.T) {
	s := Scope{
		ID:       "s1",
		Type:     ScopeFilePatterns,
		Patterns: []string{"*.go", "docs/*.md"},
		Allow:    true,
	}

	tests := []struct {
		value string
		want  bool
	}{
		{"main.go", true},
		{"foo.go", true},
		{"docs/readme.md", true},
		{"readme.md", false},
		{"main.py", false},
	}

	for _, tt := range tests {
		if got := s.Match(tt.value); got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestScopeMatch_ExactMatch(t *testing.T) {
	s := Scope{
		ID:       "s2",
		Type:     ScopeOperations,
		Patterns: []string{"read", "write"},
		Allow:    true,
	}

	if !s.Match("read") {
		t.Error("expected match for 'read'")
	}
	if s.Match("delete") {
		t.Error("unexpected match for 'delete'")
	}
}

func TestScopeMatch_EmptyPatterns(t *testing.T) {
	s := Scope{ID: "s3", Type: ScopeNetwork, Patterns: nil}
	if s.Match("anything") {
		t.Error("empty patterns should not match")
	}
}

func TestScopeSet_AddRemoveList(t *testing.T) {
	ss := NewScopeSet()
	s1 := Scope{ID: "a", Type: ScopeToolAccess, Patterns: []string{"git"}}
	s2 := Scope{ID: "b", Type: ScopeSecrets, Patterns: []string{"AWS_KEY"}}

	ss.Add(s1)
	ss.Add(s2)
	if ss.Len() != 2 {
		t.Fatalf("expected 2 scopes, got %d", ss.Len())
	}

	ss.Remove("a")
	if ss.Len() != 1 {
		t.Fatalf("expected 1 scope after remove, got %d", ss.Len())
	}

	list := ss.List()
	if len(list) != 1 || list[0].ID != "b" {
		t.Errorf("unexpected list: %+v", list)
	}
}

func TestScopeSet_Match(t *testing.T) {
	ss := NewScopeSet()
	ss.Add(Scope{
		ID:       "fp",
		Type:     ScopeFilePatterns,
		Patterns: []string{"src/*.go"},
		Allow:    true,
	})
	ss.Add(Scope{
		ID:       "ops",
		Type:     ScopeOperations,
		Patterns: []string{"build", "test"},
		Allow:    true,
	})

	if !ss.Match(ScopeFilePatterns, "src/main.go") {
		t.Error("expected file pattern match")
	}
	if ss.Match(ScopeFilePatterns, "vendor/lib.go") {
		t.Error("unexpected file pattern match")
	}
	if !ss.Match(ScopeOperations, "build") {
		t.Error("expected operations match")
	}
	if ss.Match(ScopeOperations, "deploy") {
		t.Error("unexpected operations match")
	}
	if ss.Match(ScopeNetwork, "localhost") {
		t.Error("no network scopes, should not match")
	}
}

func TestScopeSet_Get(t *testing.T) {
	ss := NewScopeSet()
	ss.Add(Scope{ID: "x", Type: ScopeSecrets, Patterns: []string{"TOKEN"}})

	s, ok := ss.Get("x")
	if !ok || s.ID != "x" {
		t.Error("expected to find scope 'x'")
	}

	_, ok = ss.Get("missing")
	if ok {
		t.Error("expected not found for 'missing'")
	}
}

func TestScopeSet_AddOverwrite(t *testing.T) {
	ss := NewScopeSet()
	ss.Add(Scope{ID: "a", Type: ScopeNetwork, Patterns: []string{"old"}})
	ss.Add(Scope{ID: "a", Type: ScopeNetwork, Patterns: []string{"new"}})

	if ss.Len() != 1 {
		t.Fatalf("expected 1 after overwrite, got %d", ss.Len())
	}
	s, _ := ss.Get("a")
	if len(s.Patterns) != 1 || s.Patterns[0] != "new" {
		t.Errorf("expected overwritten patterns, got %v", s.Patterns)
	}
}
