# Gemini Adapter

**ID**: 034
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to push Gemini sessions into wsm so that my Gemini Code Assist and Gemini CLI conversation history is preserved.

## Acceptance Scenarios

1. **Given** a directory containing Gemini session files, **When** I run `wsm sessions push --format=gemini <path>`, **Then** the adapter detects the Gemini session format and imports it into the active workspace.
2. **Given** a valid Gemini session, **When** the adapter parses it, **Then** messages are normalized into wsm entries with role, content, and timestamps preserved.
3. **Given** a Gemini session containing tool calls, **When** the adapter parses it, **Then** tool invocations and their results are mapped to wsm tool-call entries.
4. **Given** a Gemini session containing file changes, **When** the adapter parses it, **Then** file edits are extracted and mapped to wsm file-change entries.
5. **Given** a Gemini session with fields not yet represented in wsm's normalized model, **When** the adapter encounters them, **Then** those fields are preserved in the entry's metadata map for future use.
6. **Given** a malformed or truncated Gemini session file, **When** the adapter attempts to parse it, **Then** it returns a descriptive error without crashing or corrupting existing data.
7. **Given** a Gemini session path that does not exist, **When** I run `wsm sessions push --format=gemini <path>`, **Then** the command exits with an error indicating the path was not found.
8. **Given** the Gemini format auto-detection heuristic, **When** I run `wsm sessions push <path>` without `--format`, **Then** the adapter is selected automatically if the session matches Gemini's structure.

> **Note**: Exact session file paths and format details for Gemini Code Assist / Gemini CLI are to be validated during implementation.

## Tests

### Unit
- `pkg/format/gemini/adapter_test.go` -- `TestDetect`
- `pkg/format/gemini/adapter_test.go` -- `TestParseMessages`
- `pkg/format/gemini/adapter_test.go` -- `TestParseToolCalls`
- `pkg/format/gemini/adapter_test.go` -- `TestParseFileChanges`
- `pkg/format/gemini/adapter_test.go` -- `TestUnknownFieldsPreservedInMetadata`
- `pkg/format/gemini/adapter_test.go` -- `TestMalformedInput`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
