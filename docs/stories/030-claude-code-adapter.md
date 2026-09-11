# Claude Code Adapter

**ID**: 030
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to push Claude Code sessions into wsm so that my Claude Code conversation history is preserved in my workspace.

## Details

Claude Code stores JSONL transcripts at `~/.claude/projects/<hash>/transcript.jsonl`. Each line is a JSON object representing a conversation turn with role, content, tool calls, and tool results. The adapter parses this format and maps it to wsm's normalized entry model.

## Acceptance Scenarios

1. **Given** a valid Claude Code `transcript.jsonl` file, **When** the adapter's `Detect` is called with the file path, **Then** it returns `true` with high confidence.
2. **Given** a non-Claude-Code JSONL file, **When** the adapter's `Detect` is called, **Then** it returns `false` or low confidence.
3. **Given** a valid Claude Code transcript, **When** the adapter's `Parse` is called, **Then** messages are extracted with role, content, and timestamps mapped to wsm's normalized entries.
4. **Given** a Claude Code transcript with tool calls, **When** the adapter's `Parse` is called, **Then** tool call entries are extracted with tool name, input, and output.
5. **Given** a Claude Code transcript with file changes, **When** the adapter's `Parse` is called, **Then** file change entries are extracted with path and operation type.
6. **Given** a Claude Code transcript with malformed lines, **When** the adapter's `Parse` is called, **Then** malformed lines are skipped with warnings and valid lines are still parsed.
7. **Given** a Claude Code transcript, **When** roles are mapped, **Then** Claude Code roles (human, assistant, tool) are mapped to wsm roles correctly.
8. **Given** the adapter, **When** `Metadata()` is called, **Then** it returns name `"claudecode"`, file patterns `["transcript.jsonl"]`, and a description.

## Tests

### Unit
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_Detect_Valid`
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_Detect_NonClaudeCode`
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_Parse_Messages`
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_Parse_ToolCalls`
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_Parse_FileChanges`
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_Parse_MalformedLines`
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_RoleMapping`
- `pkg/format/claudecode/adapter_test.go` -- `TestClaudeCode_Metadata`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
