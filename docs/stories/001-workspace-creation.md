# Workspace Creation

**ID**: 001
**Feature**: CLI Core
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to create a new workspace so that I can start recording session events.

## Acceptance Scenarios

1. **Given** no existing workspaces, **When** I run `wsm create my-project`, **Then** a workspace is created and confirmation shown with ID.
2. **Given** no existing workspaces, **When** I run `wsm create my-project -d "Auth refactor" -t backend -t auth`, **Then** workspace is created with description and tags.
3. **Given** an existing workspace named `my-project`, **When** I run `wsm create my-project`, **Then** it fails with exit code 4 (conflict).
4. **Given** no existing workspaces, **When** I run `wsm create my-project --json`, **Then** JSON output with id and created_at is returned.
5. **Given** no existing workspaces, **When** I run `wsm create my-project -q`, **Then** only the workspace ID is printed.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_Create`
- `pkg/workspace/manager_test.go` -- `TestManager_Create_Duplicate`

### E2E
- `tests/e2e/workspace_test.go` -- `TestWorkspaceCreate`
- `tests/e2e/workspace_test.go` -- `TestWorkspaceCreate_Duplicate`
