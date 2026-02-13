# Event Recording

**ID**: 006
**Feature**: Event Sourcing
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to record events to a workspace so that session history is preserved as an append-only log.

## Acceptance Scenarios

1. **Given** active workspace `my-project`, **When** I record a `message.added` event, **Then** the event is appended with a ULID, monotonic seq_num, and timestamp.
2. **Given** active workspace `my-project` with 5 events, **When** I record another event, **Then** its seq_num is 6.
3. **Given** active workspace with events, **When** I list events, **Then** they are returned in seq_num order.
4. **Given** active workspace with events of different types, **When** I list events with type filter `message.added`, **Then** only message events are returned.
5. **Given** archived workspace, **When** I try to record an event, **Then** it fails because archived workspaces are immutable.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_RecordEvent`
- `pkg/workspace/manager_test.go` -- `TestManager_RecordEvent_SeqNum`
- `pkg/workspace/manager_test.go` -- `TestManager_RecordEvent_Archived`
- `pkg/workspace/event_test.go` -- `TestEvent_Serialization`

### E2E
- `tests/e2e/event_test.go` -- `TestEventRecording`
- `tests/e2e/event_test.go` -- `TestEventListing`
