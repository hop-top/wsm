package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"hop.top/wsm/pkg/model"
)

// MemberConfig is the workspace-local member configuration file
// stored at .wsm/members.yaml (or .hop/wsm/members.yaml).
type MemberConfig struct {
	Members []model.Member `json:"members" yaml:"members"`
}

// LoadMemberConfig reads members.yaml from the same directory as
// the local config file. Returns empty config if the file doesn't exist.
func LoadMemberConfig(localConfigPath string) (*MemberConfig, error) {
	dir := filepath.Dir(localConfigPath)
	path := filepath.Join(dir, "members.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &MemberConfig{}, nil
		}
		return nil, fmt.Errorf("read members config: %w", err)
	}

	var mc MemberConfig
	if err := yaml.Unmarshal(data, &mc); err != nil {
		return nil, fmt.Errorf("parse members config %s: %w", path, err)
	}

	// Validate all members.
	for i := range mc.Members {
		if err := mc.Members[i].Validate(); err != nil {
			return nil, fmt.Errorf("members[%d]: %w", i, err)
		}
	}
	return &mc, nil
}

// SaveMemberConfig writes the member configuration to members.yaml.
func SaveMemberConfig(localConfigPath string, mc *MemberConfig) error {
	dir := filepath.Dir(localConfigPath)
	path := filepath.Join(dir, "members.yaml")

	data, err := yaml.Marshal(mc)
	if err != nil {
		return fmt.Errorf("marshal members config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write members config: %w", err)
	}
	return nil
}

// FindMember returns the first member matching profileID, or nil.
func (mc *MemberConfig) FindMember(profileID string) *model.Member {
	for i := range mc.Members {
		if mc.Members[i].ProfileID == profileID {
			return &mc.Members[i]
		}
	}
	return nil
}

// ActiveFacet returns the facet name that should be active for
// the given profile in this workspace. Empty string means the
// base profile identity (no facet override).
func (mc *MemberConfig) ActiveFacet(profileID string) string {
	m := mc.FindMember(profileID)
	if m == nil {
		return ""
	}
	return m.Facet
}

// ActiveCapacity returns the capacity for a profile in this
// workspace. Defaults to principal if not found.
func (mc *MemberConfig) ActiveCapacity(profileID string) model.Capacity {
	m := mc.FindMember(profileID)
	if m == nil {
		return model.CapacityPrincipal
	}
	return m.Capacity
}
