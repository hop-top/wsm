# Plan 01d: Library and Storage Implementation

**Goal**: Implement core Go module APIs and storage backends that satisfy stories `016`, `017`, and foundations for CLI/TUI.

## Scope

- `pkg/workspace` (types, manager, events, snapshots)
- `pkg/backend` (contract, options)
- `pkg/backend/sqlite` (default backend)
- `pkg/config` (resolved config)
- `pkg/device` (auto-detection + registry)

## Interfaces

- `workspace.NewManager(backend.Backend)`
- `Manager.Create/Get/List/RecordEvent/Snapshot/Archive/Unarchive/Delete/Events/ResolveWorkspaceRef`
- `backend.Backend` includes workspace CRUD, event operations, snapshot operations, device operations, migrate/close lifecycle.

## Data Guarantees

1. Event `seq_num` monotonicity per workspace.
2. Atomic writes and rollback safety.
3. Unique workspace names.
4. Typed payload round-trip for event data.

## Acceptance Criteria

1. Core package APIs are usable without CLI dependencies.
2. SQLite backend passes conformance test suite.
3. No UI/presentation concerns leak into `pkg/*` contracts.
