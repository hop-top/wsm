# Workspace Inspection

**ID**: 003
**Feature**: CLI Core
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to inspect a workspace so that I can see its metadata, event count, and recent activity.

## Acceptance Scenarios

1. **Given** workspace `my-project` exists with events, **When** I run `wsm inspect my-project`, **Then** I see summary with name, ID, status, created/updated dates, event count, device count.
2. **Given** workspace `my-project` exists, **When** I run `wsm inspect my-project --events`, **Then** I see the summary plus the last 20 events in a table.
3. **Given** workspace `my-project` exists, **When** I run `wsm inspect my-project --events-all --json`, **Then** I get full JSON including all events.
4. **Given** workspace `my-project` exists, **When** I run `wsm inspect my-project --devices`, **Then** I see attached devices.
5. **Given** no workspace `xyz` exists, **When** I run `wsm inspect xyz`, **Then** I get an error with exit code 3 and a hint to run `wsm list`.
6. *(Future)* **Given** events with `session_id` and `agent_id`, **When** I inspect a workspace with `--events`, **Then** session and agent columns are visible.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_Get`
- `pkg/workspace/manager_test.go` -- `TestManager_Get_NotFound`

### E2E
- `tests/e2e/workspace_test.go` -- `TestWorkspaceInspect`
- `tests/e2e/workspace_test.go` -- `TestWorkspaceInspect_NotFound`
