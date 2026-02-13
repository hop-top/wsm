# Cross-IDE Session Continuity

**ID**: 027
**Feature**: CLI Core / Event Sourcing
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to switch between IDEs while using the same wsm workspace so that one continuous session history stays synced.

## Acceptance Scenarios

1. **Given** IDE A records events to workspace `my-project`, **When** IDE B connects to the same backend and inspects `my-project`, **Then** IDE B sees the events from IDE A without export/import.
2. **Given** IDE A and IDE B both record events to the same workspace, **When** I list events, **Then** all events appear in one monotonic `seq_num` stream.
3. **Given** IDE A creates workspace `my-project`, **When** IDE B resolves the same workspace by name or ID, **Then** both references resolve to the same workspace record.
4. **Given** IDE A archives a workspace, **When** IDE B tries to record a new event to that workspace, **Then** the write fails with the archived-workspace conflict behavior.
5. **Given** multiple IDEs use the same workspace over time, **When** I inspect workspace details, **Then** event count and recent activity reflect the shared session history.
6. **Given** IDE A records with `session_id="s-a"` and IDE B records with `session_id="s-b"` to the same workspace, **When** I list events, **Then** each event carries its originating `session_id`, allowing attribution within the unified stream.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_FullLifecycle`
- `pkg/workspace/manager_test.go` -- `TestManager_RecordEvent_SeqNum`
- `pkg/workspace/manager_test.go` -- `TestManager_ResolveRef`
- `pkg/workspace/manager_test.go` -- `TestManager_CrossSessionAttribution`

### E2E
- `tests/e2e/workspace_test.go` -- `TestWorkspaceFullLifecycle`
- `tests/e2e/workspace_test.go` -- `TestRefResolution_ByNameAndID`
- `tests/e2e/event_test.go` -- `TestEventListing`
