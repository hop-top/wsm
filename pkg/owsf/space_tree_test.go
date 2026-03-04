package owsf

import (
	"testing"

	"hop.top/wsm/pkg/model"
)

func TestSpaceTree_StrictTree(t *testing.T) {
	tree := NewSpaceTree()
	root := model.Space{
		ID: "root", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/",
		Capabilities: []string{"all"},
		TrustLevel:   model.TrustLocal,
	}
	child := model.Space{
		ID: "child", Type: model.SpaceContainer,
		Parent: "root", Boundary: "docker://app",
		Capabilities: []string{"read"},
		TrustLevel:   model.TrustVerified,
	}
	if err := tree.Add(root); err != nil {
		t.Fatal(err)
	}
	if err := tree.Add(child); err != nil {
		t.Fatal(err)
	}
}

func TestSpaceTree_RejectDAG(t *testing.T) {
	tree := NewSpaceTree()
	s := model.Space{
		ID: "orphan", Type: model.SpaceAPI,
		Parent: "nonexistent", Boundary: "/api",
		Capabilities: []string{}, TrustLevel: model.TrustExternal,
	}
	if err := tree.Add(s); err == nil {
		t.Fatal("expected error for missing parent")
	}
}

func TestSpaceTree_RejectDuplicateID(t *testing.T) {
	tree := NewSpaceTree()
	root := model.Space{
		ID: "s1", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/",
		Capabilities: []string{}, TrustLevel: model.TrustLocal,
	}
	_ = tree.Add(root)
	if err := tree.Add(root); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestSpaceTree_RejectParentChange(t *testing.T) {
	tree := NewSpaceTree()
	root := model.Space{
		ID: "root", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/",
		Capabilities: []string{}, TrustLevel: model.TrustLocal,
	}
	_ = tree.Add(root)
	err := tree.Reparent("root", "other")
	if err == nil {
		t.Fatal("expected error: parent immutable")
	}
}

func TestSpaceTree_RejectCycle(t *testing.T) {
	tree := NewSpaceTree()
	// Build root -> a -> b, then try to reparent root under b
	root := model.Space{
		ID: "root", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/",
		Capabilities: []string{}, TrustLevel: model.TrustLocal,
	}
	a := model.Space{
		ID: "a", Type: model.SpaceContainer,
		Parent: "root", Boundary: "docker://a",
		Capabilities: []string{}, TrustLevel: model.TrustVerified,
	}
	b := model.Space{
		ID: "b", Type: model.SpaceContainer,
		Parent: "a", Boundary: "docker://b",
		Capabilities: []string{}, TrustLevel: model.TrustVerified,
	}
	_ = tree.Add(root)
	_ = tree.Add(a)
	_ = tree.Add(b)

	// Try adding a space whose parent is its own descendant (cycle)
	// This is caught because Add only allows existing parents,
	// and Reparent is always rejected. The cycle scenario happens
	// if someone modifies the tree directly.
	// We test via WouldCycle helper.
	if !tree.WouldCycle("root", "b") {
		t.Fatal("expected cycle detection: root -> a -> b -> root")
	}
	if tree.WouldCycle("b", "root") {
		t.Fatal("b under root is not a cycle")
	}
}

func TestSpaceTree_Get(t *testing.T) {
	tree := NewSpaceTree()
	root := model.Space{
		ID: "root", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/",
		Capabilities: []string{}, TrustLevel: model.TrustLocal,
	}
	_ = tree.Add(root)
	got, ok := tree.Get("root")
	if !ok {
		t.Fatal("expected to find root")
	}
	if got.Boundary != "/" {
		t.Fatal("boundary mismatch")
	}
	_, ok = tree.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestSpaceTree_ImmutableFields(t *testing.T) {
	tree := NewSpaceTree()
	orig := model.Space{
		ID: "s1", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/data",
		Capabilities: []string{"read"},
		TrustLevel:   model.TrustLocal,
	}
	_ = tree.Add(orig)

	// Changing type requires destroy+create
	mutated := orig
	mutated.Type = model.SpaceContainer
	result := tree.ValidateMutation(orig, mutated)
	if !result.RequiresDestroyCreate {
		t.Fatal("type change requires destroy+create")
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected 2 events (destroy+create), got %d", len(result.Events))
	}
	if result.Events[0] != "topology.space_destroy" {
		t.Fatalf("first event should be destroy, got %s", result.Events[0])
	}
	if result.Events[1] != "topology.space_create" {
		t.Fatalf("second event should be create, got %s", result.Events[1])
	}
}

func TestSpaceTree_ImmutableCapabilities(t *testing.T) {
	tree := NewSpaceTree()
	orig := model.Space{
		ID: "s1", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/data",
		Capabilities: []string{"read", "write"},
		TrustLevel:   model.TrustLocal,
	}
	_ = tree.Add(orig)

	// I3 fix: Capabilities change requires destroy+create
	mutated := orig
	mutated.Capabilities = []string{"read"}
	result := tree.ValidateMutation(orig, mutated)
	if !result.RequiresDestroyCreate {
		t.Fatal("capabilities change requires destroy+create")
	}
	if !contains(result.ChangedFields, "capabilities") {
		t.Fatal("expected capabilities in changed fields")
	}
}

func TestSpaceTree_MutableTrustLevel(t *testing.T) {
	tree := NewSpaceTree()
	orig := model.Space{
		ID: "s1", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/data",
		Capabilities: []string{"read"},
		TrustLevel:   model.TrustLocal,
	}
	_ = tree.Add(orig)

	// TrustLevel is mutable — no destroy+create
	mutated := orig
	mutated.TrustLevel = model.TrustVerified
	result := tree.ValidateMutation(orig, mutated)
	if result.RequiresDestroyCreate {
		t.Fatal("trust_level change should not require destroy+create")
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
