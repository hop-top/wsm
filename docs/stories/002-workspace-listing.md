# Workspace Listing

**ID**: 002
**Feature**: CLI Core
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to list all workspaces so that I can find and manage my sessions.

## Acceptance Scenarios

1. **Given** no workspaces exist, **When** I run `wsm list`, **Then** I see "No workspaces yet." with a hint to create one.
2. **Given** workspaces exist, **When** I run `wsm list`, **Then** I see a table with NAME, ID, STATUS, EVENTS, LAST ACTIVITY columns.
3. **Given** active and archived workspaces exist, **When** I run `wsm list --status=active`, **Then** only active workspaces are shown.
4. **Given** workspaces exist, **When** I run `wsm list --sort=name`, **Then** workspaces are sorted by name.
5. **Given** workspaces exist, **When** I run `wsm list --json`, **Then** a JSON array of workspaces is returned.
6. **Given** workspaces exist, **When** I run `wsm list -q`, **Then** workspace IDs are printed one per line.
7. **Given** the data directory and database do not exist, **When** I run `wsm workspace list --format json` (or any other read-only query), **Then** `[]` is printed, exit code is 0, and no data, config, state or cache directory and no database file is created. Commands that write still create the store on first use.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_List`
- `pkg/workspace/manager_test.go` -- `TestManager_List_FilterStatus`

### E2E
- `tests/e2e/workspace_test.go` -- `TestWorkspaceList`
- `tests/e2e/workspace_test.go` -- `TestWorkspaceList_Empty`
- `tests/e2e/workspace_test.go` -- `TestWorkspaceList_Filter`
- `tests/e2e/readonly_no_store_test.go` -- `TestReadOnlyQuery_NoStore_CreatesNothing`
- `tests/e2e/readonly_no_store_test.go` -- `TestWriteCommand_StillCreatesStore`
- `tests/e2e/readonly_no_store_test.go` -- `TestReadOnlyQuery_LegacyStoreStillMigrated`
