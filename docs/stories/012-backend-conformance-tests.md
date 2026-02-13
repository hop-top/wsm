# Backend Conformance Tests

**ID**: 012
**Feature**: Testing
**Persona**: [Maintainer](../personas/maintainer.md)
**Priority**: P0

## Story

As a maintainer, I want a conformance test suite for the Backend interface so that any new backend implementation can be validated against the same contract.

## Acceptance Scenarios

1. **Given** the conformance suite, **When** I run it against the SQLite backend, **Then** all tests pass.
2. **Given** a backend implementation, **When** I create and retrieve a workspace, **Then** the retrieved workspace matches the original.
3. **Given** a backend implementation, **When** I append events and list them, **Then** events are returned in seq_num order.
4. **Given** a backend implementation, **When** I call Migrate twice, **Then** the second call is idempotent (no error).
5. **Given** a backend implementation, **When** I delete a workspace, **Then** its events and snapshots are also deleted (cascade).

## Tests

### Unit
- `pkg/backend/conformance_test.go` -- `TestBackendConformance`
- `pkg/backend/sqlite/sqlite_test.go` -- `TestSQLiteBackend_Conformance`
