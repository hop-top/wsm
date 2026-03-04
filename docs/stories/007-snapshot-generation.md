# Snapshot Generation

**ID**: 007
**Feature**: Event Sourcing
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to generate a snapshot of a workspace so that I can see a summary of its current state without replaying all events.

## Acceptance Scenarios

1. **Given** workspace with 100 events, **When** I run `wsm snapshot generate <workspace>`, **Then** it contains message_count, tool_calls, files_changed, and devices derived from events.
2. **Given** workspace with a previous snapshot at seq 50 and 50 new events, **When** I run `wsm snapshot generate <workspace>` again, **Then** it replays only from seq 51 onward (incremental).
3. **Given** workspace with mutation.file_patch events for paths A, B, A, **When** I generate a snapshot, **Then** files_changed contains [A, B] (deduplicated).
4. **Given** workspace with device.attached and device.detached events, **When** I generate a snapshot, **Then** devices reflects only currently-attached devices.
5. **Given** empty workspace (no events), **When** I generate a snapshot, **Then** all counts are zero.
6. *(Future)* **Given** events with `session_id` and `agent_id`, **When** I generate a snapshot, **Then** the snapshot could include active sessions and agents per device.
7. **Given** workspace with events, **When** I run `wsm snapshot context <workspace>`, **Then** output contains workspace name, summary counts, files changed, and recent events formatted as markdown.

## Tests

### Unit
- `pkg/workspace/snapshot_test.go` -- `TestGenerateSnapshot`
- `pkg/workspace/snapshot_test.go` -- `TestGenerateSnapshot_Incremental`
- `pkg/workspace/snapshot_test.go` -- `TestGenerateSnapshot_Empty`

### E2E
- `tests/e2e/snapshot_test.go` -- `TestSnapshotGeneration`
