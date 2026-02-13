# Event Integrity Tests

**ID**: 011
**Feature**: Testing
**Persona**: [Maintainer](../personas/maintainer.md)
**Related Personas**: [User](../personas/user.md) (validates [006](006-event-recording.md))
**Priority**: P0

## Story

As a maintainer, I want automated tests verifying event integrity so that the append-only log never loses, reorders, or corrupts events.

## Acceptance Scenarios

1. **Given** 1000 events recorded sequentially, **When** I list all events, **Then** seq_nums are 1..1000 with no gaps.
2. **Given** events recorded with different types, **When** I filter by type, **Then** only matching events are returned and seq order is preserved.
3. **Given** a crash (simulated by rolling back a transaction), **When** I list events, **Then** no partial event was persisted.
4. **Given** events with various payload types (MessageData, ToolCallData, FileChangeData), **When** I read them back, **Then** payloads deserialize correctly.
5. **Given** events recorded with `session_id` and `agent_id`, **When** I read them back, **Then** `session_id` and `agent_id` are preserved exactly.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_EventIntegrity_Sequential`
- `pkg/workspace/manager_test.go` -- `TestManager_EventIntegrity_Filter`
- `pkg/workspace/event_test.go` -- `TestEvent_PayloadRoundTrip`
- `pkg/workspace/event_test.go` -- `TestEvent_SessionAgentRoundTrip`

### E2E
- `tests/e2e/event_test.go` -- `TestEventIntegrity_LargeVolume`
