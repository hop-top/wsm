# Aider Adapter

**ID**: 042
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P2

## Story

As a user, I want to push Aider sessions into wsm so that my Aider conversation history is preserved.

## Acceptance Scenarios

1. **Given** an Aider chat history file exists at `.aider.chat.history.md`, **When** I run `wsm sessions push --format=aider <path>`, **Then** the adapter detects the Aider markdown format.
2. **Given** a valid Aider markdown chat history, **When** the adapter parses it, **Then** the markdown conversation is split into individual messages as wsm session entries.
3. **Given** an Aider chat history referencing file edits, **When** the adapter parses it, **Then** file references and edits are extracted and attached to the corresponding entries.
4. **Given** an incomplete or truncated Aider chat history file, **When** the adapter attempts to parse it, **Then** the adapter processes all complete entries and returns a warning for the truncated portion.
5. **Given** the Aider markdown format does not include timestamps, **When** the adapter converts entries, **Then** sequential timestamps are assigned based on entry order within the file.

## Tests

### Unit
- `pkg/format/aider/adapter_test.go` -- `TestDetectMarkdownFormat`
- `pkg/format/aider/adapter_test.go` -- `TestParseMessages`
- `pkg/format/aider/adapter_test.go` -- `TestExtractFileReferences`
- `pkg/format/aider/adapter_test.go` -- `TestTruncatedFileHandling`
- `pkg/format/aider/adapter_test.go` -- `TestSequentialTimestamps`

### E2E
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_Aider`
