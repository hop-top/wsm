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
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      Status            `json:"status"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ArchivedAt  *time.Time        `json:"archived_at,omitempty"`
}

// EventType identifies a durable event.
type EventType string

const (
	EventWorkspaceCreated    EventType = "workspace.created"
	EventWorkspaceUpdated    EventType = "workspace.updated"
	EventWorkspaceArchived   EventType = "workspace.archived"
	EventWorkspaceUnarchived EventType = "workspace.unarchived"

	EventMessageAdded      EventType = "message.added"
	EventToolCallStarted   EventType = "tool_call.started"
	EventToolCallCompleted EventType = "tool_call.completed"
	EventFileChanged       EventType = "file.changed"

	EventDeviceAttached EventType = "device.attached"
	EventDeviceDetached EventType = "device.detached"

	EventSnapshotTaken EventType = "snapshot.taken"
)

// Event is an immutable event-log record.
type Event struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Type        EventType       `json:"type"`
	DeviceID    string          `json:"device_id,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	AgentID     string          `json:"agent_id,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
	Data        json.RawMessage `json:"data"`
	SeqNum      uint64          `json:"seq_num"`
}

// MessageData payload for EventMessageAdded.
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

// FileChangeData payload for EventFileChanged.
type FileChangeData struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Diff      string `json:"diff,omitempty"`
}

// DeviceData payload for device attach/detach.
type DeviceData struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}

// Snapshot is a point-in-time materialized workspace view.
type Snapshot struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	WorkspaceID  string    `json:"workspace_id"`
	AtSeqNum     uint64    `json:"at_seq_num"`
	AtTimestamp  time.Time `json:"at_timestamp"`
	Workspace    Workspace `json:"workspace"`
	MessageCount int       `json:"message_count"`
	ToolCalls    int       `json:"tool_calls"`
	FilesChanged []string  `json:"files_changed"`
	Devices      []string  `json:"devices"`
	Sessions     []string  `json:"sessions"`
	IsSeed       bool      `json:"is_seed"`
	GeneratedAt  time.Time `json:"generated_at"`
}
