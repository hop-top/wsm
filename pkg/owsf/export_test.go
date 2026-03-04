package owsf

import (
	"testing"

	"hop.top/wsm/pkg/model"
)

func TestExport_Full(t *testing.T) {
	exp := NewExport(ExportFull)
	if exp.Kind != ExportFull {
		t.Fatal("kind must be full")
	}
}

func TestExport_Full_RequiresDeterminism(t *testing.T) {
	// I2 fix: full exports must also declare determinism + max_conformance
	exp := NewExport(ExportFull)
	if err := exp.Validate(); err == nil {
		t.Fatal("full export without determinism should fail")
	}
	exp.Determinism = DeterminismFull
	exp.MaxConformance = 1
	if err := exp.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExport_Partial_RequiresScope(t *testing.T) {
	exp := NewExport(ExportPartial)
	if err := exp.Validate(); err == nil {
		t.Fatal("partial requires scope")
	}
	exp.SliceScope = &SliceScope{
		SpaceIDs: []string{"sp-1"},
	}
	exp.Determinism = DeterminismFull
	exp.MaxConformance = 1
	if err := exp.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExport_Closure_AncestorChain(t *testing.T) {
	tree := NewSpaceTree()
	root := model.Space{
		ID: "root", Type: model.SpaceFilesystem,
		Parent: "", Boundary: "/",
		Capabilities: []string{},
		TrustLevel:   model.TrustLocal,
	}
	child := model.Space{
		ID: "child", Type: model.SpaceContainer,
		Parent: "root", Boundary: "docker://x",
		Capabilities: []string{},
		TrustLevel:   model.TrustVerified,
	}
	_ = tree.Add(root)
	_ = tree.Add(child)

	needed := tree.AncestorChain("child")
	if len(needed) != 2 {
		t.Fatalf("want 2, got %d", len(needed))
	}
	if needed[0] != "root" || needed[1] != "child" {
		t.Fatal("chain: root -> child")
	}
}
