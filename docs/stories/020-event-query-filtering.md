# Event Query Filtering

**ID**: 020
**Feature**: Event Sourcing
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want event query filtering so that I can inspect large workspaces by type and sequence ranges.

## Acceptance Scenarios

1. **Given** events of mixed types, **When** I query with `--type=interaction.message`, **Then** only matching events are returned in `seq_num` order.
2. **Given** a workspace with events 1..100, **When** I query with `--after-seq=20 --before-seq=40`, **Then** only events within the range are returned.
3. **Given** a large event stream, **When** I query with `--limit=25`, **Then** at most 25 events are returned.
4. **Given** filtered event queries, **When** I use `--json`, **Then** output is valid JSON and preserves sequence ordering.
5. **Given** invalid filter values, **When** I run the command, **Then** it fails with usage exit code 2 and actionable guidance.
6. *(Future)* **Given** events with `session_id` and `agent_id`, **When** I query with `--session-id=...` or `--agent-id=...`, **Then** only matching events are returned.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_Events_Filtering`

### E2E
- `tests/e2e/event_test.go` -- `TestEventFilteringCLI`
