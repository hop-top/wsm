# Exit Code Contract

**ID**: 015
**Feature**: Output Modes
**Persona**: [External Client](../personas/external-client.md)
**Priority**: P0

## Story

As an external client, I want predictable exit codes so that my scripts can handle wsm errors programmatically.

## Acceptance Scenarios

1. **Given** successful `wsm create`, **When** I check the exit code, **Then** it is 0.
2. **Given** `wsm create` with missing name argument, **When** I check the exit code, **Then** it is 2 (usage error).
3. **Given** `wsm inspect nonexistent`, **When** I check the exit code, **Then** it is 3 (not found).
4. **Given** `wsm create duplicate-name` when it already exists, **When** I check the exit code, **Then** it is 4 (conflict).
5. **Given** a corrupted SQLite database, **When** I run any command, **Then** the exit code is 5 (backend error).
6. **Given** `wsm archive already-archived`, **When** I check the exit code, **Then** it is 4 (conflict).

## Tests

### Unit

### E2E
- `tests/e2e/exit_code_test.go` — `TestExitCode_Success`
- `tests/e2e/exit_code_test.go` — `TestExitCode_Usage`
- `tests/e2e/exit_code_test.go` — `TestExitCode_NotFound`
- `tests/e2e/exit_code_test.go` — `TestExitCode_Conflict`
- `tests/e2e/exit_code_test.go` — `TestExitCode_BackendError`
