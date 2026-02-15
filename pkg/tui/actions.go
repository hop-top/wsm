package tui

import (
	"context"

	"hop.top/wsm/pkg/workspace"
)

// ActionManager captures the manager APIs used by TUI actions.
type ActionManager interface {
	Create(ctx context.Context, name string, opts workspace.CreateOptions) (*workspace.Workspace, error)
	Archive(ctx context.Context, ref string) error
	Unarchive(ctx context.Context, ref string) error
	Delete(ctx context.Context, ref string) error
	EventCount(ctx context.Context, ref string) (uint64, error)
	Snapshot(ctx context.Context, ref string, opts workspace.SnapshotOptions) (*workspace.Snapshot, error)
}

// CreateWorkspace executes the create action.
func CreateWorkspace(ctx context.Context, manager ActionManager, name string) (workspace.Workspace, error) {
	ws, err := manager.Create(ctx, name, workspace.CreateOptions{})
	if err != nil {
		return workspace.Workspace{}, err
	}
	return *ws, nil
}

// ToggleArchiveWorkspace archives active workspaces and unarchives archived workspaces.
func ToggleArchiveWorkspace(ctx context.Context, manager ActionManager, ws workspace.Workspace) (workspace.Status, error) {
	if ws.Status == workspace.StatusArchived {
		if err := manager.Unarchive(ctx, ws.ID); err != nil {
			return ws.Status, err
		}
		return workspace.StatusActive, nil
	}
	if err := manager.Archive(ctx, ws.ID); err != nil {
		return ws.Status, err
	}
	return workspace.StatusArchived, nil
}

// DeleteWorkspace executes the delete action.
func DeleteWorkspace(ctx context.Context, manager ActionManager, ws workspace.Workspace) error {
	return manager.Delete(ctx, ws.ID)
}

// LoadWorkspaceDetail returns event and snapshot context for a selected workspace.
func LoadWorkspaceDetail(ctx context.Context, manager ActionManager, ws workspace.Workspace) (WorkspaceDetail, error) {
	count, err := manager.EventCount(ctx, ws.ID)
	if err != nil {
		return WorkspaceDetail{}, err
	}
	snap, err := manager.Snapshot(ctx, ws.ID, workspace.SnapshotOptions{})
	if err != nil {
		return WorkspaceDetail{}, err
	}
	return WorkspaceDetail{
		Workspace:  ws,
		EventCount: count,
		Snapshot:   snap,
	}, nil
}
