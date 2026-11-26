package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/model"
	"hop.top/wsm/pkg/owsf"
)

// GetSpace retrieves a space by ID.
func (m *Manager) GetSpace(ctx context.Context, id string) (*Space, error) {
	s, err := m.backend.GetSpace(ctx, id)
	if err != nil {
		return nil, backendError("get space", err)
	}
	if s == nil {
		return nil, notFoundError(fmt.Sprintf("space %q not found", id))
	}
	out := Space(*s)
	return &out, nil
}

// CreateSpace creates a new space and records topology.space_create.
func (m *Manager) CreateSpace(ctx context.Context, workspaceRef string, s Space) (*Space, error) {
	if s.ID == "" {
		s.ID = ulid.Make().String()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	if s.WriterID == "" && m.writerID != "" {
		s.WriterID = m.writerID
	}

	// Interop validation
	if err := owsf.ValidateSpaceCapabilities(s.Capabilities); err != nil {
		return nil, usageError(fmt.Sprintf("invalid space capabilities: %v", err))
	}
	modelSpace := model.Space(s)
	owsf.MapBoundaryToObservables(&modelSpace)
	s = Space(modelSpace)

	// Verify parent exists if specified.
	if s.Parent != "" {
		parent, err := m.backend.GetSpace(ctx, s.Parent)
		if err != nil {
			return nil, backendError("check parent space", err)
		}
		if parent == nil {
			return nil, notFoundError(fmt.Sprintf("parent space %q not found", s.Parent))
		}
	}

	if err := m.backend.CreateSpace(ctx, model.Space(s)); err != nil {
		if errors.Is(err, backend.ErrSpaceExists) {
			return nil, conflictError(fmt.Sprintf("space %q already exists", s.ID))
		}
		return nil, mapBackendErr("create space", err)
	}

	_, err := m.RecordEvent(ctx, workspaceRef, EventTopologySpaceCreate, s)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

// UpdateSpace updates a space's properties.
// OWSF s2.3: Changing id, parent, type, boundary, or capabilities requires
// generating topology.space_destroy followed by topology.space_create (C3 fix).
func (m *Manager) UpdateSpace(ctx context.Context, workspaceRef string, s Space) (*Space, error) {
	old, err := m.backend.GetSpace(ctx, s.ID)
	if err != nil {
		return nil, backendError("get existing space for update", err)
	}
	if old == nil {
		return nil, notFoundError(fmt.Sprintf("space %q not found", s.ID))
	}

	if s.WriterID == "" && m.writerID != "" {
		s.WriterID = m.writerID
	}

	// Interop validation
	if err := owsf.ValidateSpaceCapabilities(s.Capabilities); err != nil {
		return nil, usageError(fmt.Sprintf("invalid space capabilities: %v", err))
	}
	modelSpace := model.Space(s)
	owsf.MapBoundaryToObservables(&modelSpace)
	s = Space(modelSpace)

	// Use OWSF tree logic to validate mutation.
	// Note: We don't have the full tree here easily without loading all,
	// but ValidateMutation works on two space structs.
	tree := owsf.NewSpaceTree()
	result := tree.ValidateMutation(*old, model.Space(s))

	if err := m.backend.SaveSpace(ctx, model.Space(s)); err != nil {
		return nil, mapBackendErr("update space", err)
	}

	if result.RequiresDestroyCreate {
		// Record destroy of the OLD state and create of the NEW state.
		// Item C3/I3 fix: ensure these specific OWSF events are in the log.
		_, _ = m.RecordEvent(ctx, workspaceRef, EventTopologySpaceDestroy, map[string]any{
			"id":     old.ID,
			"reason": fmt.Sprintf("immutable field change: %v", result.ChangedFields),
		})
		_, _ = m.RecordEvent(ctx, workspaceRef, EventTopologySpaceCreate, s)
	} else if len(result.ChangedFields) > 0 {
		// Just a regular update (e.g. TrustLevel changed)
		_, _ = m.RecordEvent(ctx, workspaceRef, EventWorkspaceUpdated, map[string]any{
			"space_id": s.ID,
			"fields":   result.ChangedFields,
		})
	}

	return &s, nil
}

// ListSpaces returns spaces for a given parent.
func (m *Manager) ListSpaces(ctx context.Context, parentID string) ([]Space, error) {
	spaces, err := m.backend.ListSpaces(ctx, parentID)
	if err != nil {
		return nil, backendError("list spaces", err)
	}
	out := make([]Space, len(spaces))
	for i, s := range spaces {
		out[i] = Space(s)
	}
	return out, nil
}
