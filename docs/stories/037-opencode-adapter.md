# OpenCode Adapter

**ID**: 037
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to push OpenCode sessions into wsm so that my OpenCode TUI conversation history is preserved.

## Acceptance Scenarios

1. **Given** a directory containing OpenCode session files, **When** I run `wsm sessions push --format=opencode <path>`, **Then** the adapter detects the OpenCode session format and imports it into the active workspace.
2. **Given** a valid OpenCode session, **When** the adapter parses it, **Then** messages are normalized into wsm entries with role, content, and timestamps preserved.
3. **Given** an OpenCode session containing tool calls, **When** the adapter parses it, **Then** tool invocations and their results are mapped to wsm tool-call entries.
4. **Given** an OpenCode session containing file changes, **When** the adapter parses it, **Then** file edits are extracted and mapped to wsm file-change entries.
5. **Given** an OpenCode session with format-specific fields not yet represented in wsm's normalized model, **When** the adapter encounters them, **Then** those fields are preserved in the entry's metadata map for future use.
6. **Given** a malformed or truncated OpenCode session file, **When** the adapter attempts to parse it, **Then** it returns a descriptive error without crashing or corrupting existing data.
7. **Given** an OpenCode session path that does not exist, **When** I run `wsm sessions push --format=opencode <path>`, **Then** the command exits with an error indicating the path was not found.
8. **Given** the OpenCode format auto-detection heuristic, **When** I run `wsm sessions push <path>` without `--format`, **Then** the adapter is selected automatically if the session matches OpenCode's structure.

## Tests

### Unit
- `pkg/format/opencode/adapter_test.go` -- `TestDetect`
- `pkg/format/opencode/adapter_test.go` -- `TestParseMessages`
- `pkg/format/opencode/adapter_test.go` -- `TestParseToolCalls`
- `pkg/format/opencode/adapter_test.go` -- `TestParseFileChanges`
- `pkg/format/opencode/adapter_test.go` -- `TestFormatSpecificFieldsPreservedInMetadata`
- `pkg/format/opencode/adapter_test.go` -- `TestMalformedInput`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
