# TUI Workspace Operations

**ID**: 022
**Feature**: TUI
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want full workspace lifecycle operations in the TUI so that I can complete common workflows without dropping to raw commands.

## Acceptance Scenarios

1. **Given** TUI is running, **When** I create a workspace, **Then** it appears immediately in the workspace list.
2. **Given** a selected workspace, **When** I inspect details, **Then** metadata and event summary are rendered in the detail view.
3. **Given** a selected active workspace, **When** I archive it, **Then** status changes to archived and confirmation is shown.
4. **Given** a selected archived workspace, **When** I unarchive it, **Then** status changes back to active.
5. **Given** a selected workspace, **When** I delete it, **Then** TUI requires explicit confirmation before removal and reflects the updated state after delete.

## Tests

### Unit
- `pkg/tui/actions_test.go` -- `TestWorkspaceLifecycleActions`

### E2E
None. There is no end-to-end test for TUI workspace operations today; the
behaviour is covered only by the unit test above.
