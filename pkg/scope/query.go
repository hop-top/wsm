package scope

// IntersectScopes returns a new ScopeSet containing only scopes
// whose IDs exist in both a and b. When both contain the same ID,
// the scope from a is used.
func IntersectScopes(a, b *ScopeSet) *ScopeSet {
	result := NewScopeSet()
	a.mu.RLock()
	defer a.mu.RUnlock()
	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, s := range a.scopes {
		if _, ok := b.scopes[id]; ok {
			result.scopes[id] = s
		}
	}
	return result
}

// UnionScopes returns a new ScopeSet containing all scopes from
// both a and b. When both contain the same ID, the scope from b wins.
func UnionScopes(a, b *ScopeSet) *ScopeSet {
	result := NewScopeSet()
	a.mu.RLock()
	defer a.mu.RUnlock()
	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, s := range a.scopes {
		result.scopes[id] = s
	}
	for id, s := range b.scopes {
		result.scopes[id] = s
	}
	return result
}

// EffectiveScope merges all scopes from the given assignments
// into a single ScopeSet. Later assignments override earlier ones
// when scope IDs collide.
func EffectiveScope(assignments []Assignment) *ScopeSet {
	result := NewScopeSet()
	for _, a := range assignments {
		for _, s := range a.Scopes {
			result.scopes[s.ID] = s
		}
	}
	return result
}
