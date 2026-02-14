# Antigravity Adapter

**ID**: 041
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to push Antigravity sessions into wsm so that my Antigravity conversation history is preserved.

## Acceptance Scenarios

1. **Given** an Antigravity session file exists, **When** I run `wsm sessions push --format=antigravity <path>`, **Then** the adapter detects the Antigravity session format.
2. **Given** a valid Antigravity session, **When** the adapter parses it, **Then** messages and tool calls are extracted into wsm session entries.
3. **Given** an Antigravity session with format-specific fields not present in the wsm entry model, **When** the adapter converts entries, **Then** those fields are preserved in the entry metadata passthrough.
4. **Given** an invalid or corrupted Antigravity session file, **When** the adapter attempts to parse it, **Then** a clear error is returned describing the failure.

## Tests

### Unit
- `pkg/format/antigravity/adapter_test.go` -- `TestDetectFormat`
- `pkg/format/antigravity/adapter_test.go` -- `TestParseMessages`
- `pkg/format/antigravity/adapter_test.go` -- `TestParseToolCalls`
- `pkg/format/antigravity/adapter_test.go` -- `TestMetadataPassthrough`

### E2E
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_Antigravity`
