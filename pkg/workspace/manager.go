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
	"hop.top/wsm/pkg/owsf"
)

// Manager provides workspace lifecycle operations.
// Law 1 invariant: every write operation reads the current state first.
type Manager struct {
	backend      backend.Backend
	device       device.Device
	sessionID    string
	agentID      string
	writerID     string
	spaceID      string
	lamportClock uint64
	caps         *Capabilities
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

// WithWriterID sets the OWSF writer ID (OASF Agent URI).
func WithWriterID(id string) ManagerOption {
	return func(m *Manager) {
		m.writerID = id
	}
}

// WithSpaceID sets the OWSF space ID.
func WithSpaceID(id string) ManagerOption {
	return func(m *Manager) {
		m.spaceID = id
	}
}

// WithLamportClock sets the initial Lamport clock value.
func WithLamportClock(c uint64) ManagerOption {
	return func(m *Manager) {
		m.lamportClock = c
	}
}

// WithCapabilities sets the capability policy for this manager instance.
func WithCapabilities(caps *Capabilities) ManagerOption {
	return func(m *Manager) {
		m.caps = caps
	}
}

// NewManager constructs a manager with current device identity.
func NewManager(b backend.Backend, opts ...ManagerOption) (*Manager, error) {
	m := &Manager{backend: b, device: device.Current(), caps: AllCapabilities()}
	for _, opt := range opts {
		opt(m)
	}
	_ = m.backend.SaveDevice(context.Background(), m.device)
	return m, nil
}

// Create creates a new workspace and records workspace.created.
func (m *Manager) Create(ctx context.Context, name string, opts CreateOptions) (*Workspace, error) {
	if !m.caps.Can(CapWorkspaceCreate) {
		return nil, aopRejection(ErrorKindForbidden, "workspace create not permitted", "capability", "law5.capability_check", CapWorkspaceCreate, nil)
	}
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

	if m.writerID != "" {
		if err := owsf.ValidateAgentIdentity(m.writerID); err != nil {
			return nil, usageError(fmt.Sprintf("invalid agent identity: %v", err))
		}
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return nil, usageError("event payload must be JSON-serializable")
	}

	m.lamportClock++

	ev := model.Event{
		WorkspaceID:  ws.ID,
		Type:         model.EventType(eventType),
		DeviceID:     m.device.ID,
		SessionID:    m.sessionID,
		AgentID:      m.agentID,
		WriterID:     m.writerID,
		SpaceID:      m.spaceID,
		LamportClock: m.lamportClock,
		Data:         payload,
	}

	// OCSF Alignment and Defaulting
	if err := owsf.DefaultAndAlign(&ev); err != nil {
		return nil, usageError(fmt.Sprintf("OCSF alignment failure: %v", err))
	}

	appended, err := m.backend.AppendEvent(ctx, ev)
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

// SnapshotOptions controls snapshot generation behavior.
type SnapshotOptions struct {
	SessionID string
	Name      string
	IsSeed    bool
}

// Snapshot generates and stores a point-in-time snapshot.
func (m *Manager) Snapshot(ctx context.Context, workspaceRef string, opts SnapshotOptions) (*Snapshot, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}

	var latest *model.Snapshot
	if opts.SessionID == "" {
		latest, err = m.backend.GetLatestSnapshot(ctx, ws.ID)
		if err != nil {
			return nil, backendError("get latest snapshot", err)
		}
	}

	afterSeq := uint64(0)
	if latest != nil {
		afterSeq = latest.AtSeqNum
	}

	events, err := m.backend.ListEvents(ctx, ws.ID, backend.EventListOptions{AfterSeqNum: afterSeq})
	if err != nil {
		return nil, backendError("list snapshot events", err)
	}

	if opts.SessionID != "" {
		var filtered []model.Event
		for _, ev := range events {
			if ev.SessionID == opts.SessionID {
				filtered = append(filtered, ev)
			}
		}
		events = filtered
		latest = nil
	}

	var snap model.Snapshot
	if latest == nil {
		snap = model.GenerateSnapshot(model.Workspace(*ws), events, 0)
	} else if len(events) == 0 {
		tmp := Snapshot(*latest)
		return &tmp, nil
	} else {
		base := *latest
		base.ID = ""
		base.GeneratedAt = time.Now().UTC()
		snap = applyEventsToSnapshot(base, events)
	}

	snap.Name = opts.Name
	snap.IsSeed = opts.IsSeed

	if err := m.backend.SaveSnapshot(ctx, snap); err != nil {
		return nil, backendError("save snapshot", err)
	}
	_, _ = m.appendLifecycleEvent(ctx, ws.ID, EventSnapshotCreated, model.SnapshotEventData{
		SnapshotID: snap.ID, WorkspaceID: ws.ID, Name: snap.Name,
	})
	out := Snapshot(snap)
	return &out, nil
}

// GetSnapshot retrieves a snapshot by ID or name.
func (m *Manager) GetSnapshot(ctx context.Context, idOrName string) (*Snapshot, error) {
	snap, err := m.backend.GetSnapshot(ctx, idOrName)
	if err != nil {
		return nil, backendError("get snapshot", err)
	}
	if snap == nil {
		return nil, notFoundError(fmt.Sprintf("snapshot %q not found", idOrName))
	}
	out := Snapshot(*snap)
	return &out, nil
}

// UpdateSnapshotOptions controls snapshot update behavior.
type UpdateSnapshotOptions struct {
	Name   *string
	IsSeed *bool
}

// UpdateSnapshot updates a snapshot's metadata.
func (m *Manager) UpdateSnapshot(ctx context.Context, idOrName string, opts UpdateSnapshotOptions) (*Snapshot, error) {
	snap, err := m.backend.GetSnapshot(ctx, idOrName)
	if err != nil {
		return nil, backendError("get snapshot for update", err)
	}
	if snap == nil {
		return nil, notFoundError(fmt.Sprintf("snapshot %q not found", idOrName))
	}
	if opts.Name != nil {
		if strings.TrimSpace(*opts.Name) == "" {
			return nil, aopRejection(ErrorKindUsage, "snapshot name must not be blank", "schema", "law2.validate_schema", "name:non-empty", nil)
		}
		snap.Name = *opts.Name
	}
	if opts.IsSeed != nil {
		snap.IsSeed = *opts.IsSeed
	}
	if err := m.backend.UpdateSnapshot(ctx, *snap); err != nil {
		return nil, backendError("update snapshot", err)
	}
	_, _ = m.appendLifecycleEvent(ctx, snap.WorkspaceID, EventSnapshotUpdated, model.SnapshotEventData{
		SnapshotID: snap.ID, WorkspaceID: snap.WorkspaceID, Name: snap.Name,
	})
	out := Snapshot(*snap)
	return &out, nil
}

// DeleteSnapshot removes a snapshot by ID or name.
func (m *Manager) DeleteSnapshot(ctx context.Context, idOrName string) error {
	// Law 1: read before delete.
	snap, err := m.backend.GetSnapshot(ctx, idOrName)
	if err != nil {
		return backendError("get snapshot for delete", err)
	}
	if snap == nil {
		return notFoundError(fmt.Sprintf("snapshot %q not found", idOrName))
	}
	_, _ = m.appendLifecycleEvent(ctx, snap.WorkspaceID, EventSnapshotDeleted, model.SnapshotEventData{
		SnapshotID: snap.ID, WorkspaceID: snap.WorkspaceID, Name: snap.Name,
	})
	if err := m.backend.DeleteSnapshot(ctx, snap.ID); err != nil {
		return backendError("delete snapshot", err)
	}
	return nil
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
		case model.EventInteractionMessage:
			base.MessageCount++
		case model.EventInteractionToolCall:
			base.ToolCalls++
		case model.EventMutationFilePatch:
			var p model.FileChangeData
			if json.Unmarshal(ev.Data, &p) == nil && p.Path != "" {
				files[p.Path] = struct{}{}
			}
		case model.EventMutationArtifact:
			var p model.ArtifactData
			if json.Unmarshal(ev.Data, &p) == nil && p.Path != "" {
				files[p.Path] = struct{}{}
			}
		case model.EventTopologySpaceCreate, model.EventTopologySpaceDestroy:
			// Topology events affect the snapshot metadata or future state
			// but for now we just track that something changed in the topology.
			// (Maybe add space count to snapshot in the future)
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
	if !m.caps.Can(CapWorkspaceArchive) {
		return aopRejection(ErrorKindForbidden, "workspace archive not permitted", "capability", "law6.lifecycle_gate", CapWorkspaceArchive, nil)
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
	if !m.caps.Can(CapWorkspaceUnarchive) {
		return aopRejection(ErrorKindForbidden, "workspace unarchive not permitted", "capability", "law6.lifecycle_gate", CapWorkspaceUnarchive, nil)
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
	if !m.caps.Can(CapWorkspaceDelete) {
		return aopRejection(ErrorKindForbidden, "workspace delete not permitted", "capability", "law5.capability_check", CapWorkspaceDelete, nil)
	}
	// Law 4: emit deleted event before cascade-delete (ephemeral but observable).
	_, _ = m.appendLifecycleEvent(ctx, ws.ID, EventWorkspaceDeleted, map[string]any{"name": ws.Name})
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

// ListSnapshots returns snapshots with optional filtering.
func (m *Manager) ListSnapshots(ctx context.Context, opts backend.SnapshotListOptions) ([]Snapshot, error) {
	snaps, err := m.backend.ListSnapshots(ctx, opts)
	if err != nil {
		return nil, backendError("list snapshots", err)
	}
	out := make([]Snapshot, len(snaps))
	for i, s := range snaps {
		out[i] = Snapshot(s)
	}
	return out, nil
}

// ListDevices returns all known devices.
func (m *Manager) ListDevices(ctx context.Context) ([]device.Device, error) {
	devs, err := m.backend.ListDevices(ctx)
	if err != nil {
		return nil, backendError("list devices", err)
	}
	return devs, nil
}

// GetDevice returns a device by ID.
func (m *Manager) GetDevice(ctx context.Context, id string) (*device.Device, error) {
	dev, err := m.backend.GetDevice(ctx, id)
	if err != nil {
		return nil, backendError("get device", err)
	}
	if dev == nil {
		return nil, notFoundError(fmt.Sprintf("device %q not found", id))
	}
	return dev, nil
}

// ResolveDeviceRef resolves a device reference (ID or name) to a device.
// Supports full ID, partial ID (min 4 chars), and partial name matching.
// Returns AmbiguousDeviceError if the reference matches multiple devices.
func (m *Manager) ResolveDeviceRef(ctx context.Context, ref string) (*device.Device, error) {
	dev, matches, err := m.backend.ResolveDevice(ctx, ref)
	if err != nil {
		return nil, backendError("resolve device", err)
	}
	if dev != nil {
		return dev, nil
	}
	if len(matches) > 0 {
		return nil, &AmbiguousDeviceError{Ref: ref, Matches: matches}
	}
	return nil, notFoundError(fmt.Sprintf("device %q not found", ref))
}

// DeviceSessions returns session summaries for a device.
func (m *Manager) DeviceSessions(ctx context.Context, deviceID string) ([]backend.DeviceSession, error) {
	sessions, err := m.backend.ListDeviceSessions(ctx, deviceID)
	if err != nil {
		return nil, backendError("list device sessions", err)
	}
	return sessions, nil
}

// AddSpaceToWorkspace creates a space via CreateSpace and associates it with a workspace.
func (m *Manager) AddSpaceToWorkspace(ctx context.Context, workspaceRef string, s Space) (*Space, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}

	created, err := m.CreateSpace(ctx, ws.ID, s)
	if err != nil {
		return nil, err
	}

	if err := m.backend.AssociateSpace(ctx, ws.ID, created.ID); err != nil {
		return nil, mapBackendErr("associate space with workspace", err)
	}

	return created, nil
}

// ListWorkspaceSpaces returns spaces associated with a workspace.
func (m *Manager) ListWorkspaceSpaces(ctx context.Context, workspaceRef string) ([]Space, error) {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}
	spaces, err := m.backend.ListWorkspaceSpaces(ctx, ws.ID)
	if err != nil {
		return nil, backendError("list workspace spaces", err)
	}
	out := make([]Space, len(spaces))
	for i, s := range spaces {
		out[i] = Space(s)
	}
	return out, nil
}

// ListSpaceWorkspaces returns workspaces associated with a space.
func (m *Manager) ListSpaceWorkspaces(ctx context.Context, spaceID string) ([]Workspace, error) {
	workspaces, err := m.backend.ListSpaceWorkspaces(ctx, spaceID)
	if err != nil {
		return nil, backendError("list space workspaces", err)
	}
	out := make([]Workspace, len(workspaces))
	for i, ws := range workspaces {
		out[i] = Workspace(ws)
	}
	return out, nil
}

// RemoveSpace dissociates a space from a workspace and deletes it if no other workspaces reference it.
func (m *Manager) RemoveSpace(ctx context.Context, workspaceRef string, spaceID string, force bool) error {
	ws, err := m.ResolveWorkspaceRef(ctx, workspaceRef)
	if err != nil {
		return err
	}

	s, err := m.backend.GetSpace(ctx, spaceID)
	if err != nil {
		return backendError("get space", err)
	}
	if s == nil {
		return notFoundError(fmt.Sprintf("space %q not found", spaceID))
	}

	if err := m.backend.DissociateSpace(ctx, ws.ID, spaceID); err != nil {
		return mapBackendErr("dissociate space", err)
	}

	remaining, err := m.backend.ListSpaceWorkspaces(ctx, spaceID)
	if err != nil {
		return backendError("check space workspaces", err)
	}
	if len(remaining) == 0 {
		if err := m.backend.DeleteSpace(ctx, spaceID, force); err != nil {
			return mapBackendErr("delete space", err)
		}
	}

	_, _ = m.RecordEvent(ctx, ws.ID, EventTopologySpaceDestroy, map[string]any{
		"space_id": s.ID, "boundary": s.Boundary,
	})

	return nil
}

type AmbiguousDeviceError struct {
	Ref     string
	Matches []device.Device
}

func (e *AmbiguousDeviceError) Error() string {
	return fmt.Sprintf("ambiguous device reference %q matches %d devices", e.Ref, len(e.Matches))
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
