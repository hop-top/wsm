# Workspace Deletion

**ID**: 005
**Feature**: CLI Core
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to delete a workspace so that I can permanently remove unwanted session data.

## Acceptance Scenarios

1. **Given** workspace `old-work` exists in a TTY, **When** I run `wsm delete old-work` and confirm, **Then** workspace and all events are permanently removed.
2. **Given** workspace `old-work` exists, **When** I run `wsm delete old-work --force`, **Then** workspace is deleted without confirmation prompt.
3. **Given** workspace `old-work` exists, **When** I run `wsm delete old-work --dry-run`, **Then** I see what would be deleted (event count, snapshot count) but nothing is removed.
4. **Given** non-TTY environment without `--confirm=yes` (or the legacy `--force`), **When** I run `wsm delete old-work`, **Then** it fails with exit code 5 (kit's UNAUTHORIZED confirm gate; it cannot prompt off a TTY).
5. **Given** no workspace `xyz`, **When** I run `wsm delete xyz`, **Then** it fails with exit code 3 (not found).

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_Delete`
- `pkg/workspace/manager_test.go` -- `TestManager_Delete_NotFound`

### E2E
- `tests/e2e/workspace_test.go` -- `TestWorkspaceDelete`
- `tests/e2e/workspace_test.go` -- `TestWorkspaceDelete_Force`
- `tests/e2e/workspace_test.go` -- `TestWorkspaceDelete_DryRun`
