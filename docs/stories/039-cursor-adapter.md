# Cursor Adapter

**ID**: 039
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to push Cursor sessions into wsm so that my Cursor conversation history is preserved.

## Acceptance Scenarios

1. **Given** a Cursor session database exists at the expected path, **When** I run `wsm sessions push --format=cursor <path>`, **Then** the adapter detects the SQLite database and reads session data.
2. **Given** a valid Cursor SQLite database, **When** the adapter parses it, **Then** messages, tool calls, and file changes are extracted into wsm session entries.
3. **Given** a Cursor session exported as JSON, **When** I run `wsm sessions push --format=cursor <path>`, **Then** the adapter detects the JSON export variant and parses it correctly.
4. **Given** a Cursor SQLite database with an unrecognized schema version, **When** the adapter attempts to parse it, **Then** the adapter returns a clear error identifying the schema mismatch and the expected vs actual versions.
5. **Given** a Cursor session with format-specific fields, **When** the adapter converts entries, **Then** Cursor-specific data is preserved in the entry metadata passthrough.

## Tests

### Unit
- `pkg/format/cursor/adapter_test.go` -- `TestDetectSQLite`
- `pkg/format/cursor/adapter_test.go` -- `TestParseSQLiteEntries`
- `pkg/format/cursor/adapter_test.go` -- `TestParseJSONExport`
- `pkg/format/cursor/adapter_test.go` -- `TestSchemaVersionMismatch`
- `pkg/format/cursor/adapter_test.go` -- `TestMetadataPassthrough`

### E2E
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_Cursor`
