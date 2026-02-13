# Workspace Lifecycle Tests

**ID**: 010
**Feature**: Testing
**Persona**: [Maintainer](../personas/maintainer.md)
**Related Personas**: [User](../personas/user.md) (validates [001](001-workspace-creation.md), [002](002-workspace-listing.md), [003](003-workspace-inspection.md), [004](004-workspace-archival.md), [005](005-workspace-deletion.md))
**Priority**: P0

## Story

As a maintainer, I want automated tests covering the full workspace lifecycle so that regressions in create/list/inspect/archive/delete are caught.

## Acceptance Scenarios

1. **Given** a clean test database, **When** I create, list, inspect, archive, unarchive, and delete a workspace in sequence, **Then** each operation succeeds and produces correct state transitions.
2. **Given** a workspace with events, **When** I delete it, **Then** all associated events and snapshots are also removed.
3. **Given** concurrent create operations with the same name, **When** both execute, **Then** exactly one succeeds and one fails with conflict.
4. **Given** a workspace created on one Manager instance, **When** a second Manager (same backend) lists workspaces, **Then** it sees the workspace.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_FullLifecycle`

### E2E
- `tests/e2e/workspace_test.go` -- `TestWorkspaceFullLifecycle`
