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
	Space          = model.Space
	SpaceType      = model.SpaceType
	TrustLevel     = model.TrustLevel
)

const (
	StatusActive   = model.StatusActive
	StatusArchived = model.StatusArchived

	EventWorkspaceCreated     = model.EventWorkspaceCreated
	EventWorkspaceUpdated     = model.EventWorkspaceUpdated
	EventWorkspaceArchived    = model.EventWorkspaceArchived
	EventWorkspaceUnarchived  = model.EventWorkspaceUnarchived
	EventInteractionMessage    = model.EventInteractionMessage
	EventInteractionToolCall   = model.EventInteractionToolCall
	EventInteractionToolDone   = model.EventInteractionToolDone
	EventMutationFilePatch     = model.EventMutationFilePatch
	EventMutationArtifact      = model.EventMutationArtifact
	EventTopologySpaceCreate   = model.EventTopologySpaceCreate
	EventTopologySpaceDestroy  = model.EventTopologySpaceDestroy
	EventCheckpointSnapshot    = model.EventCheckpointSnapshot
	EventDeviceAttached        = model.EventDeviceAttached
	EventDeviceDetached        = model.EventDeviceDetached

	SpaceFilesystem = model.SpaceFilesystem
	SpaceContainer  = model.SpaceContainer
	SpaceVM         = model.SpaceVM
	SpaceAPI        = model.SpaceAPI
	SpaceDatabase   = model.SpaceDatabase
	SpaceMachine    = model.SpaceMachine
	SpaceRemote     = model.SpaceRemote

	TrustLocal     = model.TrustLocal
	TrustVerified  = model.TrustVerified
	TrustExternal  = model.TrustExternal
	TrustUntrusted = model.TrustUntrusted
)
