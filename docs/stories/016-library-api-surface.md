# Library API Surface

**ID**: 016
**Feature**: Go Module
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P0

## Story

As an integrator, I want a clean Go API for workspace management so that I can embed wsm in my own tool without using the CLI.

## Acceptance Scenarios

1. **Given** I import `pkg/workspace`, **When** I call `NewManager(backend)`, **Then** I get a Manager ready for workspace operations.
2. **Given** a Manager, **When** I call `Create`, `Get`, `List`, `RecordEvent`, `Snapshot`, `Archive`, `Delete`, **Then** each returns typed results (not raw JSON or CLI output).
3. **Given** a Manager, **When** I call `RecordEvent` with a typed payload (MessageData, ToolCallData), **Then** the event is serialized and stored correctly.
4. **Given** a Manager, **When** I call `Events` with filter options, **Then** I get filtered, ordered events without shelling out.
5. **Given** `pkg/workspace` and `pkg/backend`, **When** I check their public API, **Then** there are no CLI or presentation concerns leaked into the library types.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_APIContract`
- `pkg/workspace/event_test.go` -- `TestEventPayload_TypeSafety`

### E2E
- None (library-level; covered by unit tests)
