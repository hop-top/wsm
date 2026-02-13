package workspace

import "hop.top/wsm/pkg/model"

// Re-export public model types from the workspace package for integrators.
type (
	Status         = model.Status
	Workspace      = model.Workspace
	EventType      = model.EventType
	Event          = model.Event
	MessageData    = model.MessageData
	ToolCallData   = model.ToolCallData
	FileChangeData = model.FileChangeData
	DeviceData     = model.DeviceData
	Snapshot       = model.Snapshot
)

const (
	StatusActive   = model.StatusActive
	StatusArchived = model.StatusArchived

	EventWorkspaceCreated    = model.EventWorkspaceCreated
	EventWorkspaceUpdated    = model.EventWorkspaceUpdated
	EventWorkspaceArchived   = model.EventWorkspaceArchived
	EventWorkspaceUnarchived = model.EventWorkspaceUnarchived
	EventMessageAdded        = model.EventMessageAdded
	EventToolCallStarted     = model.EventToolCallStarted
	EventToolCallCompleted   = model.EventToolCallCompleted
	EventFileChanged         = model.EventFileChanged
	EventDeviceAttached      = model.EventDeviceAttached
	EventDeviceDetached      = model.EventDeviceDetached
	EventSnapshotTaken       = model.EventSnapshotTaken
)
