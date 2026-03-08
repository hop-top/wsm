package backend

import (
	"context"
	"time"

	"hop.top/wsm/pkg/device"
	"hop.top/wsm/pkg/model"
)

// Backend is the storage contract for workspace persistence.
type Backend interface {
	CreateWorkspace(ctx context.Context, ws model.Workspace) error
	GetWorkspace(ctx context.Context, id string) (*model.Workspace, error)
	GetWorkspaceByName(ctx context.Context, name string) (*model.Workspace, error)
	ListWorkspaces(ctx context.Context, opts ListOptions) ([]model.Workspace, error)
	UpdateWorkspace(ctx context.Context, ws model.Workspace) error
	DeleteWorkspace(ctx context.Context, id string) error

	AppendEvent(ctx context.Context, event model.Event) (model.Event, error)
	GetEvent(ctx context.Context, workspaceID string, ref string) (*model.Event, error)
	ListEvents(ctx context.Context, workspaceID string, opts EventListOptions) ([]model.Event, error)
	EventCount(ctx context.Context, workspaceID string) (uint64, error)

	SaveSnapshot(ctx context.Context, snap model.Snapshot) error
	GetSnapshot(ctx context.Context, id string) (*model.Snapshot, error)
	GetLatestSnapshot(ctx context.Context, workspaceID string) (*model.Snapshot, error)
	ListSnapshots(ctx context.Context, opts SnapshotListOptions) ([]model.Snapshot, error)
	UpdateSnapshot(ctx context.Context, snap model.Snapshot) error
	DeleteSnapshot(ctx context.Context, id string) error
	SnapshotCount(ctx context.Context, workspaceID string) (uint64, error)

	SaveDevice(ctx context.Context, dev device.Device) error
	GetDevice(ctx context.Context, id string) (*device.Device, error)
	ListDevices(ctx context.Context) ([]device.Device, error)
	ResolveDevice(ctx context.Context, ref string) (*device.Device, []device.Device, error)
	ListDeviceSessions(ctx context.Context, deviceID string) ([]DeviceSession, error)

	SaveSpace(ctx context.Context, s model.Space) error
	GetSpace(ctx context.Context, id string) (*model.Space, error)
	ListSpaces(ctx context.Context, parentID string) ([]model.Space, error)
	DeleteSpace(ctx context.Context, id string, force bool) error

	AssociateSpace(ctx context.Context, workspaceID, spaceID string) error
	DissociateSpace(ctx context.Context, workspaceID, spaceID string) error
	ListWorkspaceSpaces(ctx context.Context, workspaceID string) ([]model.Space, error)
	ListSpaceWorkspaces(ctx context.Context, spaceID string) ([]model.Workspace, error)

	Migrate(ctx context.Context) error
	Close() error
}

// ListOptions controls workspace listing behavior.
type ListOptions struct {
	Status  *model.Status
	SortBy  string // created_at, updated_at, name
	SortDir string // asc, desc
	Limit   int
	Offset  int
}

// DeviceSession summarizes a session on a device.
type DeviceSession struct {
	DeviceID      string    `json:"device_id"`
	SessionID     string    `json:"session_id"`
	AgentID       string    `json:"agent_id"`
	WorkspaceID   string    `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	EventCount    int       `json:"event_count"`
	LastEventAt   time.Time `json:"last_event_at"`
}

// EventListOptions controls event filtering.
type EventListOptions struct {
	AfterSeqNum  uint64
	BeforeSeqNum uint64
	Types        []model.EventType
	Limit        int
}

// SnapshotListOptions controls snapshot listing behavior.
type SnapshotListOptions struct {
	WorkspaceIDs []string
	Sessions     []string
	After        time.Time
	Before       time.Time
	SortDir      string // asc, desc
	Limit        int
}
