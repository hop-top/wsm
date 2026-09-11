# Goose Adapter

**ID**: 043
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P2

## Story

As a user, I want to push Goose sessions into wsm so that my Goose conversation history is preserved.

## Acceptance Scenarios

1. **Given** a Goose session stored in SQLite format, **When** I run `wsm sessions push --format=goose <path>`, **Then** the adapter detects the modern SQLite format and parses it.
2. **Given** a Goose session stored in legacy JSONL format, **When** I run `wsm sessions push --format=goose <path>`, **Then** the adapter detects the legacy JSONL format and parses it.
3. **Given** either Goose format variant, **When** the adapter parses it, **Then** messages and tool calls are extracted into wsm session entries.
4. **Given** a Goose session file, **When** the adapter opens it, **Then** the format version (SQLite vs JSONL) is detected automatically without requiring user specification.
5. **Given** a Goose session with format-specific fields, **When** the adapter converts entries, **Then** Goose-specific data is preserved in the entry metadata passthrough.

## Tests

### Unit
- `pkg/format/goose/adapter_test.go` -- `TestDetectSQLiteFormat`
- `pkg/format/goose/adapter_test.go` -- `TestDetectJSONLFormat`
- `pkg/format/goose/adapter_test.go` -- `TestParseSQLiteEntries`
- `pkg/format/goose/adapter_test.go` -- `TestParseJSONLEntries`
- `pkg/format/goose/adapter_test.go` -- `TestAutoFormatDetection`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
