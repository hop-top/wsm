package model

import "testing"

func TestNewSpace_RequiredFields(t *testing.T) {
	s := Space{
		ID:           "sp-1",
		Type:         SpaceFilesystem,
		Parent:       "",
		Boundary:     "/home/user/project",
		Capabilities: []string{"read", "write"},
		TrustLevel:   TrustLocal,
	}
	if s.ID == "" {
		t.Fatal("id required")
	}
	if s.Type == "" {
		t.Fatal("type required")
	}
	if s.Boundary == "" {
		t.Fatal("boundary required")
	}
	if s.TrustLevel == "" {
		t.Fatal("trust_level required")
	}
}

func TestSpaceType_Constants(t *testing.T) {
	types := []SpaceType{
		SpaceFilesystem, SpaceContainer, SpaceVM,
		SpaceAPI, SpaceDatabase, SpaceMachine, SpaceRemote,
	}
	for _, st := range types {
		if st == "" {
			t.Fatal("space type constant must not be empty")
		}
	}
}

func TestTrustLevel_Constants(t *testing.T) {
	levels := []TrustLevel{
		TrustLocal, TrustVerified, TrustExternal, TrustUntrusted,
	}
	for _, tl := range levels {
		if tl == "" {
			t.Fatal("trust level constant must not be empty")
		}
	}
}
