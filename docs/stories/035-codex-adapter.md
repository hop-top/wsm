# Codex Adapter

**ID**: 035
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to push Codex sessions into wsm so that my GitHub/OpenAI Codex conversation history is preserved.

## Acceptance Scenarios

1. **Given** a directory containing Codex session files, **When** I run `wsm sessions push --format=codex <path>`, **Then** the adapter detects the Codex session format and imports it into the active workspace.
2. **Given** a valid Codex session, **When** the adapter parses it, **Then** messages are normalized into wsm entries with role, content, and timestamps preserved.
3. **Given** a Codex session containing tool calls, **When** the adapter parses it, **Then** tool invocations and their results are mapped to wsm tool-call entries.
4. **Given** a Codex session containing file changes, **When** the adapter parses it, **Then** file edits are extracted and mapped to wsm file-change entries.
5. **Given** Codex TUI and GUI variants, **When** both produce session files, **Then** the adapter handles both if they share the same format, or returns a clear error identifying the unsupported variant.
6. **Given** a Codex session with format differences between TUI and GUI variants, **When** the adapter encounters variant-specific fields, **Then** those fields are preserved in the entry's metadata map.
7. **Given** a malformed or truncated Codex session file, **When** the adapter attempts to parse it, **Then** it returns a descriptive error without crashing or corrupting existing data.
8. **Given** a Codex session path that does not exist, **When** I run `wsm sessions push --format=codex <path>`, **Then** the command exits with an error indicating the path was not found.
9. **Given** the Codex format auto-detection heuristic, **When** I run `wsm sessions push <path>` without `--format`, **Then** the adapter is selected automatically if the session matches Codex's structure.

> **Note**: Whether TUI and GUI variants share the same session format is to be validated during implementation.

## Tests

### Unit
- `pkg/format/codex/adapter_test.go` -- `TestDetect`
- `pkg/format/codex/adapter_test.go` -- `TestParseMessages`
- `pkg/format/codex/adapter_test.go` -- `TestParseToolCalls`
- `pkg/format/codex/adapter_test.go` -- `TestParseFileChanges`
- `pkg/format/codex/adapter_test.go` -- `TestFormatVariantHandling`
- `pkg/format/codex/adapter_test.go` -- `TestMalformedInput`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
