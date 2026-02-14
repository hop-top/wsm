# Vibe Adapter

**ID**: 036
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to push Vibe sessions into wsm so that my Mistral Vibe conversation history is preserved.

## Acceptance Scenarios

1. **Given** a directory containing Vibe session files, **When** I run `wsm sessions push --format=vibe <path>`, **Then** the adapter detects the Vibe session format and imports it into the active workspace.
2. **Given** a valid Vibe session, **When** the adapter parses it, **Then** messages are normalized into wsm entries with role, content, and timestamps preserved.
3. **Given** a Vibe session containing tool calls, **When** the adapter parses it, **Then** tool invocations and their results are mapped to wsm tool-call entries.
4. **Given** a Vibe session containing file changes, **When** the adapter parses it, **Then** file edits are extracted and mapped to wsm file-change entries.
5. **Given** a Vibe session with format-specific fields not yet represented in wsm's normalized model, **When** the adapter encounters them, **Then** those fields are preserved in the entry's metadata map for future use.
6. **Given** a malformed or truncated Vibe session file, **When** the adapter attempts to parse it, **Then** it returns a descriptive error without crashing or corrupting existing data.
7. **Given** a Vibe session path that does not exist, **When** I run `wsm sessions push --format=vibe <path>`, **Then** the command exits with an error indicating the path was not found.
8. **Given** the Vibe format auto-detection heuristic, **When** I run `wsm sessions push <path>` without `--format`, **Then** the adapter is selected automatically if the session matches Vibe's structure.

## Tests

### Unit
- `pkg/format/vibe/adapter_test.go` -- `TestDetect`
- `pkg/format/vibe/adapter_test.go` -- `TestParseMessages`
- `pkg/format/vibe/adapter_test.go` -- `TestParseToolCalls`
- `pkg/format/vibe/adapter_test.go` -- `TestParseFileChanges`
- `pkg/format/vibe/adapter_test.go` -- `TestFormatSpecificFieldsPreservedInMetadata`
- `pkg/format/vibe/adapter_test.go` -- `TestMalformedInput`

### E2E
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_Vibe`
