package workspace

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ConflictStrategy controls how name collisions are handled during import.
type ConflictStrategy string

const (
	ConflictSkip      ConflictStrategy = "skip"
	ConflictRename    ConflictStrategy = "rename"
	ConflictOverwrite ConflictStrategy = "overwrite"
)

// ImportOptions configures workspace import behaviour.
type ImportOptions struct {
	DryRun     bool             `json:"dry_run"`
	OnConflict ConflictStrategy `json:"on_conflict"`
	Prefix     string           `json:"prefix"`
}

// ImportResult summarises what happened during an import.
type ImportResult struct {
	Created []string `json:"created,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
	Renamed []string `json:"renamed,omitempty"`
	Errors  []error  `json:"errors,omitempty"`
}

// ImportWorkspace creates a workspace (and its spaces) from ExportData.
func (m *Manager) ImportWorkspace(
	ctx context.Context,
	data *ExportData,
	opts ImportOptions,
) (*ImportResult, error) {
	if data == nil {
		return nil, usageError("import data is nil")
	}

	result := &ImportResult{}
	name := data.Workspace.Name
	if opts.Prefix != "" {
		name = opts.Prefix + name
	}

	// Check for existing workspace with same name.
	existing, err := m.backend.GetWorkspaceByName(ctx, name)
	if err != nil {
		return nil, backendError("check existing workspace", err)
	}

	if existing != nil {
		switch opts.OnConflict {
		case ConflictSkip:
			result.Skipped = append(result.Skipped, name)
			return result, nil
		case ConflictRename:
			name = findUniqueName(name)
			result.Renamed = append(result.Renamed, name)
		case ConflictOverwrite:
			if !opts.DryRun {
				if err := m.backend.DeleteWorkspace(ctx, existing.ID); err != nil {
					return nil, backendError("delete existing workspace", err)
				}
			}
		default:
			return nil, conflictError(
				fmt.Sprintf("workspace %q already exists", name),
			)
		}
	}

	if opts.DryRun {
		result.Created = append(result.Created, name)
		for _, sp := range data.Spaces {
			result.Created = append(result.Created,
				fmt.Sprintf("space:%s:%s", sp.Type, sp.Boundary))
		}
		return result, nil
	}

	ws, err := m.Create(ctx, name, CreateOptions{
		Description: data.Workspace.Description,
		Tags:        data.Workspace.Tags,
		Metadata:    data.Workspace.Metadata,
	})
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result, err
	}
	result.Created = append(result.Created, ws.Name)

	for _, sp := range data.Spaces {
		_, err := m.CreateSpace(ctx, ws.ID, Space{
			Type:         SpaceType(sp.Type),
			Boundary:     sp.Boundary,
			Capabilities: sp.Capabilities,
			TrustLevel:   TrustLevel(sp.TrustLevel),
			Parent:       sp.Parent,
		})
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Created = append(result.Created,
			fmt.Sprintf("space:%s:%s", sp.Type, sp.Boundary))
	}

	return result, nil
}

// UnmarshalExportYAML parses YAML bytes into ExportData.
func UnmarshalExportYAML(b []byte) (*ExportData, error) {
	var data ExportData
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("parse export YAML: %w", err)
	}
	return &data, nil
}

func findUniqueName(base string) string {
	return base + "-imported"
}
