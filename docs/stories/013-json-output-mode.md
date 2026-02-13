# JSON Output Mode

**ID**: 013
**Feature**: Output Modes
**Persona**: [External Client](../personas/external-client.md)
**Priority**: P0

## Story

As an external client, I want all commands to support `--json` output so that I can parse wsm output programmatically.

## Acceptance Scenarios

1. **Given** workspace created with `--json`, **When** I parse stdout, **Then** it is valid JSON with `id` and `created_at` fields.
2. **Given** workspaces exist, **When** I run `wsm list --json`, **Then** stdout is a valid JSON array of workspace objects.
3. **Given** workspace exists, **When** I run `wsm inspect my-project --json`, **Then** stdout is a valid JSON object with workspace metadata and event count.
4. **Given** workspace archived with `--json`, **When** I parse stdout, **Then** it is valid JSON with `id` and `archived_at` fields.
5. **Given** `--json` flag on any command, **When** errors occur, **Then** stderr contains the error message (not mixed into stdout JSON).

## Tests

### Unit

### E2E
- `tests/e2e/output_test.go` — `TestJSON_Create`
- `tests/e2e/output_test.go` — `TestJSON_List`
- `tests/e2e/output_test.go` — `TestJSON_Inspect`
- `tests/e2e/output_test.go` — `TestJSON_Archive`
