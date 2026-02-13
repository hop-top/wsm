package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"hop.top/wsm/pkg/backend"
	"hop.top/wsm/pkg/device"
	"hop.top/wsm/pkg/model"
)

// Manager provides workspace lifecycle operations.
type Manager struct {
	backend   backend.Backend
	device    device.Device
	sessionID string
	agentID   string
}

// CreateOptions configures workspace creation.
type CreateOptions struct {
	Description string
	Tags        []string
	Metadata    map[string]string
}

// DeletePreview summarizes destructive delete impact.
type DeletePreview struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	EventCount    uint64 `json:"event_count"`
	SnapshotCount uint64 `json:"snapshot_count"`
}

// ManagerOption configures Manager.
type ManagerOption func(*Manager)

// WithSessionID sets the session ID.
func WithSessionID(id string) ManagerOption {
	return func(m *Manager) {
		m.sessionID = id
	}
}

// WithAgentID sets the agent ID.
func WithAgentID(id string) ManagerOption {
	return func(m *Manager) {
		m.agentID = id
	}
}

// NewManager constructs a manager with current device identity.
func NewManager(b backend.Backend, opts ...ManagerOption) (*Manager, error) {
	m := &Manager{backend: b, device: device.Current()}
	for _, opt := range opts {
		opt(m)
	}
	_ = m.backend.SaveDevice(context.Background(), m.device)
	return m, nil
}

// Create creates a new workspace and records workspace.created.
func (m *Manager) Create(ctx context.Context, name string, opts CreateOptions) (*Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, usageError("workspace name is required")
	}
	existing, err := m.backend.GetWorkspaceByName(ctx, name)
	if err != nil {
		return nil, backendError("check existing workspace", err)
	}
	if existing != nil {
		return nil, conflictError(fmt.Sprintf("workspace %q already exists", name))
	}
	now := time.Now().UTC()
	ws := model.Workspace{
		ID:          ulid.Make().String(),
		Name:        name,
		Status:      model.StatusActive,
		Description: opts.Description,
		Tags:        opts.Tags,
		Metadata:    opts.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.backend.CreateWorkspace(ctx, ws); err != nil {
		return nil, mapBackendErr("create workspace", err)
	}
	_, _ = m.RecordEvent(ctx, ws.ID, EventWorkspaceCreated, map[string]any{"name": ws.Name})
	out := Workspace(ws)
	return &out, nil
}

// Get retrieves a workspace by ID.
func (m *Manager) Get(ctx context.Context, id string) (*Workspace, error) {
	ws, err := m.backend.GetWorkspace(ctx, id)
	if err != nil {
		return nil, backendError("get workspace", err)
	}
	if ws == nil {
		return nil, notFoundError(fmt.Sprintf("workspace %q not found", id))
	}
	out := Workspace(*ws)
	return &out, nil
}

// ResolveWorkspaceRef resolves an ID first, then name.
func (m *Manager) ResolveWorkspaceRef(ctx context.Context, ref string) (*Workspace, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, usageError("workspace reference is required")
	}
	if ws, err := m.backend.GetWorkspace(ctx, ref); err == nil && ws != nil {
		out := Workspace(*ws)
		return &out, nil
	} else if err != nil {
		return nil, backendError("resolve workspace by id", err)
	}
	ws, err := m.backend.GetWorkspaceByName(ctx, ref)
	if err != nil {
		return nil, backendError("resolve workspace by name", err)
	}
	if ws == nil {
		return nil, notFoundError(fmt.Sprintf("workspace %q not found", ref))
	}
	out := Workspace(*ws)
	return &out, nil
}

// List lists workspaces with filters.
func (m *Manager) List(ctx context.Context, opts backend.ListOptions) ([]Workspace, error) {
	workspaces, err := m.backend.ListWorkspaces(ctx, opts)
	if err != nil {
		return nil, backendError("list workspaces", err)
	}
	out := make([]Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		out = append(out, Workspace(ws))
	}
	return out, nil
}

// RecordEvent appends an event to workspace log.
func (m *Manager) RecordEvent(ctx context.Context, workspaceRef string, eventType EventType, data interface{}) (*Event, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}
	if ws.Status == StatusArchived {
		return nil, conflictError("archived workspaces reject new events")
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, usageError("event payload must be JSON-serializable")
	}
	appended, err := m.backend.AppendEvent(ctx, model.Event{
		WorkspaceID: ws.ID,
		Type:        model.EventType(eventType),
		DeviceID:    m.device.ID,
		SessionID:   m.sessionID,
		AgentID:     m.agentID,
		Data:        payload,
	})
	if err != nil {
		return nil, mapBackendErr("append event", err)
	}
	out := Event(appended)
	return &out, nil
}

// GetEvent retrieves a single event by seq_num or ULID.
func (m *Manager) GetEvent(ctx context.Context, workspaceRef string, eventRef string) (*Event, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}
	ev, err := m.backend.GetEvent(ctx, ws.ID, eventRef)
	if err != nil {
		return nil, backendError("get event", err)
	}
	if ev == nil {
		return nil, notFoundError(fmt.Sprintf("event %q not found", eventRef))
	}
	out := Event(*ev)
	return &out, nil
}

// Events returns ordered event log for workspace.
func (m *Manager) Events(ctx context.Context, workspaceRef string, opts backend.EventListOptions) ([]Event, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}
	events, err := m.backend.ListEvents(ctx, ws.ID, opts)
	if err != nil {
		return nil, backendError("list events", err)
	}
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		out = append(out, Event(ev))
	}
	return out, nil
}

// Snapshot generates and stores a point-in-time snapshot.
func (m *Manager) Snapshot(ctx context.Context, workspaceRef string) (*Snapshot, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}

	latest, err := m.backend.GetLatestSnapshot(ctx, ws.ID)
	if err != nil {
		return nil, backendError("get latest snapshot", err)
	}

	afterSeq := uint64(0)
	if latest != nil {
		afterSeq = latest.AtSeqNum
	}

	events, err := m.backend.ListEvents(ctx, ws.ID, backend.EventListOptions{AfterSeqNum: afterSeq})
	if err != nil {
		return nil, backendError("list snapshot events", err)
	}

	var snap model.Snapshot
	if latest == nil {
		snap = model.GenerateSnapshot(model.Workspace(*ws), events, 0)
	} else if len(events) == 0 {
		tmp := Snapshot(*latest)
		return &tmp, nil
	} else {
		// Incremental path: synthesize full state by replaying from latest sequence only.
		base := *latest
		base.GeneratedAt = time.Now().UTC()
		snap = applyEventsToSnapshot(base, events)
	}

	if err := m.backend.SaveSnapshot(ctx, snap); err != nil {
		return nil, backendError("save snapshot", err)
	}
	out := Snapshot(snap)
	return &out, nil
}

func applyEventsToSnapshot(base model.Snapshot, events []model.Event) model.Snapshot {
	files := make(map[string]struct{}, len(base.FilesChanged))
	for _, p := range base.FilesChanged {
		files[p] = struct{}{}
	}
	attached := make(map[string]struct{}, len(base.Devices))
	for _, d := range base.Devices {
		attached[d] = struct{}{}
	}

	for _, ev := range events {
		base.AtSeqNum = ev.SeqNum
		base.AtTimestamp = ev.Timestamp
		switch ev.Type {
		case model.EventMessageAdded:
			base.MessageCount++
		case model.EventToolCallStarted:
			base.ToolCalls++
		case model.EventFileChanged:
			var p model.FileChangeData
			if json.Unmarshal(ev.Data, &p) == nil && p.Path != "" {
				files[p.Path] = struct{}{}
			}
		case model.EventDeviceAttached:
			var p model.DeviceData
			if json.Unmarshal(ev.Data, &p) == nil {
				attached[p.DeviceID] = struct{}{}
			}
		case model.EventDeviceDetached:
			var p model.DeviceData
			if json.Unmarshal(ev.Data, &p) == nil {
				delete(attached, p.DeviceID)
			}
		case model.EventWorkspaceArchived:
			t := ev.Timestamp
			base.Workspace.Status = model.StatusArchived
			base.Workspace.ArchivedAt = &t
		case model.EventWorkspaceUnarchived:
			base.Workspace.Status = model.StatusActive
			base.Workspace.ArchivedAt = nil
		}
	}
	base.FilesChanged = base.FilesChanged[:0]
	for p := range files {
		base.FilesChanged = append(base.FilesChanged, p)
	}
	sort.Strings(base.FilesChanged)
	base.Devices = base.Devices[:0]
	for d := range attached {
		base.Devices = append(base.Devices, d)
	}
	sort.Strings(base.Devices)
	base.GeneratedAt = time.Now().UTC()
	return base
}

// Archive marks workspace archived and records event.
func (m *Manager) Archive(ctx context.Context, ref string) error {
	ws, err := m.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return err
	}
	if ws.Status == StatusArchived {
		return conflictError("workspace already archived")
	}
	now := time.Now().UTC()
	ws.Status = StatusArchived
	ws.ArchivedAt = &now
	ws.UpdatedAt = now
	if err := m.backend.UpdateWorkspace(ctx, model.Workspace(*ws)); err != nil {
		return backendError("archive workspace", err)
	}
	if _, err := m.appendLifecycleEvent(ctx, ws.ID, EventWorkspaceArchived, map[string]any{"archived_at": now}); err != nil {
		return err
	}
	return nil
}

// Unarchive marks workspace active and records event.
func (m *Manager) Unarchive(ctx context.Context, ref string) error {
	ws, err := m.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return err
	}
	if ws.Status != StatusArchived {
		return conflictError("workspace is not archived")
	}
	now := time.Now().UTC()
	ws.Status = StatusActive
	ws.ArchivedAt = nil
	ws.UpdatedAt = now
	if err := m.backend.UpdateWorkspace(ctx, model.Workspace(*ws)); err != nil {
		return backendError("unarchive workspace", err)
	}
	if _, err := m.appendLifecycleEvent(ctx, ws.ID, EventWorkspaceUnarchived, map[string]any{"updated_at": now}); err != nil {
		return err
	}
	return nil
}

// DeletePreview returns delete impact counts.
func (m *Manager) DeletePreview(ctx context.Context, ref string) (*DeletePreview, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	events, err := m.backend.EventCount(ctx, ws.ID)
	if err != nil {
		return nil, backendError("count workspace events", err)
	}
	snaps, err := m.backend.SnapshotCount(ctx, ws.ID)
	if err != nil {
		return nil, backendError("count workspace snapshots", err)
	}
	return &DeletePreview{WorkspaceID: ws.ID, WorkspaceName: ws.Name, EventCount: events, SnapshotCount: snaps}, nil
}

// Delete permanently removes workspace and dependent records.
func (m *Manager) Delete(ctx context.Context, ref string) error {
	ws, err := m.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return err
	}
	if err := m.backend.DeleteWorkspace(ctx, ws.ID); err != nil {
		return backendError("delete workspace", err)
	}
	return nil
}

// EventCount returns total events for a workspace reference.
func (m *Manager) EventCount(ctx context.Context, ref string) (uint64, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return 0, err
	}
	n, err := m.backend.EventCount(ctx, ws.ID)
	if err != nil {
		return 0, backendError("count events", err)
	}
	return n, nil
}

// SnapshotCount returns total snapshots for a workspace reference.
func (m *Manager) SnapshotCount(ctx context.Context, ref string) (uint64, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, ref)
	if err != nil {
		return 0, err
	}
	n, err := m.backend.SnapshotCount(ctx, ws.ID)
	if err != nil {
		return 0, backendError("count snapshots", err)
	}
	return n, nil
}

func mapBackendErr(msg string, err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "not_found") || strings.Contains(lower, "no rows"):
		return notFoundError(msg)
	case strings.Contains(lower, "conflict") || strings.Contains(lower, "unique"):
		return conflictError(msg)
	default:
		return backendError(msg, err)
	}
}

func (m *Manager) appendLifecycleEvent(ctx context.Context, workspaceID string, eventType EventType, data interface{}) (*Event, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, usageError("event payload must be JSON-serializable")
	}
	appended, err := m.backend.AppendEvent(ctx, model.Event{
		WorkspaceID: workspaceID,
		Type:        model.EventType(eventType),
		DeviceID:    m.device.ID,
		SessionID:   m.sessionID,
		AgentID:     m.agentID,
		Data:        payload,
	})
	if err != nil {
		return nil, mapBackendErr("append lifecycle event", err)
	}
	out := Event(appended)
	return &out, nil
}
