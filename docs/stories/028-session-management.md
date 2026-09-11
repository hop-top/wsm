# Session Management and Branching

**ID**: 028
**Feature**: Session Management
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to manage IDE sessions with branching and forking so that I can explore alternative conversation paths without creating new workspaces.

## Acceptance Scenarios

1. **Given** workspace `my-project` with active session, **When** I run `wsm sessions create my-project --name "Feature A"`, **Then** a new session is created with JSONL storage and session ID.
2. **Given** session `s1` with 10 entries, **When** I run `wsm sessions fork s1 --at-entry 5 --name "Branch B"`, **Then** a new session is forked from entry 5 with parent pointer and branch summary.
3. **Given** session with multiple branches, **When** I run `wsm sessions tree s1`, **Then** a tree view shows all branches with current path highlighted.
4. **Given** session tree displayed, **When** I select a branch entry and navigate to it, **Then** session context is rebuilt up to that entry.
5. **Given** session `s1`, **When** I run `wsm sessions stats s1`, **Then** statistics show message count, file edits, duration, and branch count.
6. **Given** session `s1`, **When** I run `wsm sessions export s1 --output report.html`, **Then** a self-contained HTML file is generated with full history and tree structure.
7. **Given** Manager created with SessionManager, **When** I record an event to a session, **Then** event is automatically converted to an entry and appended to JSONL.
8. **Given** session `s1` with parent session `s0`, **When** I query lineage, **Then** I can trace back to `s0` and any ancestors.

## Tests

### Unit
- `pkg/session/manager_test.go` -- `TestSessionManager_NewSession`
- `pkg/session/manager_test.go` -- `TestSessionManager_AppendEntry`
- `pkg/session/storage_test.go` -- `TestJSONL_ReadWrite`
- `pkg/session/fork_test.go` -- `TestForkSession`
- `pkg/session/tree_test.go` -- `TestPrepareTreeNavigation`
- `pkg/session/navigate_test.go` -- `TestNavigateToEntry`
- `pkg/session/stats_test.go` -- `TestSessionStats`
- `pkg/session/export_test.go` -- `TestExportHTML`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.

## Out of Scope

- LLM-based branch summaries (deferred enhancement)
- Git-based session sharing (deferred)
- Multi-user collaborative editing (out of scope)
- Session merge conflict resolution (deferred)
