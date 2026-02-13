# Workspace Archival

**ID**: 004
**Feature**: CLI Core
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to archive a workspace so that it stops accepting events but remains queryable.

## Acceptance Scenarios

1. **Given** active workspace `my-project`, **When** I run `wsm archive my-project`, **Then** workspace status becomes archived and confirmation is shown.
2. **Given** active workspace `my-project`, **When** I run `wsm archive my-project --json`, **Then** JSON with id and archived_at is returned.
3. **Given** already-archived workspace `old-work`, **When** I run `wsm archive old-work`, **Then** it fails with exit code 4 (conflict, already archived).
4. **Given** archived workspace `old-work`, **When** I run `wsm unarchive old-work`, **Then** workspace status returns to active.
5. **Given** archived workspace `old-work`, **When** I try to record an event, **Then** it fails because archived workspaces reject new events.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_Archive`
- `pkg/workspace/manager_test.go` -- `TestManager_Archive_AlreadyArchived`
- `pkg/workspace/manager_test.go` -- `TestManager_Unarchive`

### E2E
- `tests/e2e/workspace_test.go` -- `TestWorkspaceArchive`
- `tests/e2e/workspace_test.go` -- `TestWorkspaceUnarchive`
