package workspace

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ExportData is the top-level portable workspace representation.
type ExportData struct {
	Version   string          `json:"version" yaml:"version"`
	Workspace WorkspaceExport `json:"workspace" yaml:"workspace"`
	Spaces    []SpaceExport   `json:"spaces" yaml:"spaces"`
}

// WorkspaceExport holds workspace-level fields for export.
type WorkspaceExport struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// SpaceExport holds space-level fields for export.
type SpaceExport struct {
	Type         string   `json:"type" yaml:"type"`
	Boundary     string   `json:"boundary" yaml:"boundary"`
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	TrustLevel   string   `json:"trust_level" yaml:"trust_level"`
	Parent       string   `json:"parent,omitempty" yaml:"parent,omitempty"`
}

// ExportWorkspace produces a portable ExportData for the given workspace ref.
func (m *Manager) ExportWorkspace(
	ctx context.Context, workspaceRef string,
) (*ExportData, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}

	spaces, err := m.collectAllSpaces(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("collect spaces: %w", err)
	}

	data := &ExportData{
		Version: "1",
		Workspace: WorkspaceExport{
			Name:        ws.Name,
			Description: ws.Description,
			Tags:        ws.Tags,
			Metadata:    ws.Metadata,
		},
		Spaces: spaces,
	}
	return data, nil
}

// MarshalExportYAML serialises ExportData to YAML bytes.
func MarshalExportYAML(data *ExportData) ([]byte, error) {
	return yaml.Marshal(data)
}

// collectAllSpaces recursively gathers spaces starting from parentID.
func (m *Manager) collectAllSpaces(
	ctx context.Context, parentID string,
) ([]SpaceExport, error) {
	list, err := m.ListSpaces(ctx, parentID)
	if err != nil {
		return nil, err
	}
	var out []SpaceExport
	for _, s := range list {
		out = append(out, SpaceExport{
			Type:         string(s.Type),
			Boundary:     s.Boundary,
			Capabilities: s.Capabilities,
			TrustLevel:   string(s.TrustLevel),
			Parent:       s.Parent,
		})
		children, err := m.collectAllSpaces(ctx, s.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, children...)
	}
	return out, nil
}
