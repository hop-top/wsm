# VS Code Copilot Adapter

**ID**: 040
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to push VS Code Copilot sessions into wsm so that my Copilot conversation history is preserved.

## Acceptance Scenarios

1. **Given** a VS Code Copilot session file exists, **When** I run `wsm sessions push --format=vscode <path>`, **Then** the adapter detects the VS Code Copilot session format.
2. **Given** a valid VS Code Copilot session, **When** the adapter parses it, **Then** messages and tool calls are extracted into wsm session entries.
3. **Given** the VS Code Copilot format may overlap with the Codex format (story 035), **When** implementing the adapter, **Then** validate whether the formats share a common parser and reuse logic where applicable.
4. **Given** a VS Code Copilot session with format-specific fields not present in the wsm entry model, **When** the adapter converts entries, **Then** those fields are preserved in the entry metadata passthrough.
5. **Given** a VS Code Copilot session with inline code suggestions and chat interactions, **When** the adapter parses both types, **Then** each type is mapped to the appropriate wsm entry kind.

## Tests

### Unit
- `pkg/format/vscode/adapter_test.go` -- `TestDetectFormat`
- `pkg/format/vscode/adapter_test.go` -- `TestParseMessages`
- `pkg/format/vscode/adapter_test.go` -- `TestParseToolCalls`
- `pkg/format/vscode/adapter_test.go` -- `TestMetadataPassthrough`
- `pkg/format/vscode/adapter_test.go` -- `TestCodexFormatOverlap`

### E2E
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_VSCodeCopilot`
