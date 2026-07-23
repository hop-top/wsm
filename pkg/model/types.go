package model

import (
	"encoding/json"
	"time"
)

// Status represents lifecycle state of a workspace.
type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// Workspace is a session container.
type Workspace struct {
	ID          string            `json:"id" table:"ID,priority=9"`
	Name        string            `json:"name" table:"NAME,priority=9"`
	Status      Status            `json:"status" table:"STATUS,priority=7"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Layout      *Layout           `json:"layout,omitempty"` // nil = no layout attached
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" table:"UPDATED,priority=4"`
	ArchivedAt  *time.Time        `json:"archived_at,omitempty"`
}

// EventType identifies a durable event.
type EventType string

const (
	EventWorkspaceCreated    EventType = "workspace.created"
	EventWorkspaceUpdated    EventType = "workspace.updated"
	EventWorkspaceArchived   EventType = "workspace.archived"
	EventWorkspaceUnarchived EventType = "workspace.unarchived"

	EventInteractionMessage   EventType = "interaction.message"
	EventInteractionToolCall  EventType = "interaction.tool_call"
	EventInteractionToolDone  EventType = "interaction.tool_done"
	EventMutationFilePatch    EventType = "mutation.file_patch"
	EventMutationArtifact     EventType = "mutation.artifact"
	EventTopologySpaceCreate  EventType = "topology.space_create"
	EventTopologySpaceDestroy EventType = "topology.space_destroy"
	EventCheckpointSnapshot   EventType = "checkpoint.snapshot"

	EventDeviceAttached EventType = "device.attached"
	EventDeviceDetached EventType = "device.detached"

	EventWorkspaceDeleted EventType = "workspace.deleted"
	EventSnapshotCreated  EventType = "checkpoint.snapshot_created"
	EventSnapshotUpdated  EventType = "checkpoint.snapshot_updated"
	EventSnapshotDeleted  EventType = "checkpoint.snapshot_deleted"
)

// Event is an immutable event-log record.
type Event struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id"`
	Type         EventType       `json:"type" table:"TYPE,priority=8"`
	DeviceID     string          `json:"device_id,omitempty" table:"DEVICE,priority=4"`
	SessionID    string          `json:"session_id,omitempty" table:"SESSION,priority=3"`
	AgentID      string          `json:"agent_id,omitempty"`
	WriterID     string          `json:"writer_id,omitempty"`
	WriterSeq    uint64          `json:"writer_seq,omitempty"`
	LamportClock uint64          `json:"lamport_clock,omitempty"`
	SpaceID      string          `json:"space_id,omitempty"`
	CausalDeps   []string        `json:"causal_deps,omitempty"`
	Timestamp    time.Time       `json:"timestamp" table:"TIMESTAMP,priority=6"`
	Data         json.RawMessage `json:"data"`
	SeqNum       uint64          `json:"seq_num" table:"SEQ,priority=9"`
}

// MessageData payload for EventInteractionMessage.
type MessageData struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Format  string `json:"format,omitempty"`
}

// ToolCallData payload for tool-call events.
type ToolCallData struct {
	ToolName string          `json:"tool_name"`
	Input    json.RawMessage `json:"input,omitempty"`
	Output   json.RawMessage `json:"output,omitempty"`
	Status   string          `json:"status,omitempty"`
	Duration int64           `json:"duration_ms,omitempty"`
}

// FileChangeData payload for EventMutationFilePatch.
type FileChangeData struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Diff      string `json:"diff,omitempty"`
}

// ArtifactData payload for EventMutationArtifact.
type ArtifactData struct {
	ArtifactID string `json:"artifact_id"`
	Type       string `json:"type"`
	Path       string `json:"path,omitempty"`
	Hash       string `json:"hash,omitempty"`
	Size       int64  `json:"size,omitempty"`
}

// DeviceData payload for device attach/detach.
type DeviceData struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}

// SnapshotEventData payload for snapshot lifecycle events.
type SnapshotEventData struct {
	SnapshotID  string `json:"snapshot_id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name,omitempty"`
}

// Snapshot is a point-in-time materialized workspace view.
type Snapshot struct {
	ID           string    `json:"id" table:"ID,priority=9"`
	Name         string    `json:"name" table:"NAME,priority=7"`
	WorkspaceID  string    `json:"workspace_id" table:"WORKSPACE,priority=8"`
	AtSeqNum     uint64    `json:"at_seq_num" table:"SEQ,priority=6"`
	AtTimestamp  time.Time `json:"at_timestamp"`
	Workspace    Workspace `json:"workspace"`
	MessageCount int       `json:"message_count"`
	ToolCalls    int       `json:"tool_calls"`
	FilesChanged []string  `json:"files_changed"`
	Devices      []string  `json:"devices"`
	Sessions     []string  `json:"sessions"`
	IsSeed       bool      `json:"is_seed"`
	GeneratedAt  time.Time `json:"generated_at" table:"GENERATED,priority=4"`
}

// GetID satisfies kit/runtime/domain.Entity so Workspace and Snapshot
// can flow through domain.Service[T] CRUD pipelines (and therefore the
// kit/runtime/policy seams). Defined as value receivers so both value
// and pointer instantiations satisfy the interface.
func (w Workspace) GetID() string { return w.ID }

// GetID satisfies kit/runtime/domain.Entity for Snapshot.
func (s Snapshot) GetID() string { return s.ID }
