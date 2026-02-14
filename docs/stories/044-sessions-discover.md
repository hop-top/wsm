# Sessions Discover

**ID**: 044
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P2

## Story

As a user, I want to discover IDE sessions on my machine so that I know what I can push into wsm without memorizing file paths.

## Design

The `wsm sessions discover` command scans known IDE session paths on the local machine and reports what it finds. It covers all supported formats: Claude Code, Gemini, Codex, Vibe, OpenCode, Droid, Cursor, VS Code, Antigravity, Aider, and Goose.

Output is a table showing IDE name, path, session count, and last modified timestamp. When stdout is not a TTY or `--json` is passed, output switches to JSON for scripting.

## Acceptance Scenarios

1. **Given** Claude Code transcripts exist at their known path, **When** I run `wsm sessions discover`, **Then** the output table includes a row for Claude Code with the correct path, session count, and last modified date.
2. **Given** multiple IDEs have sessions on disk, **When** I run `wsm sessions discover`, **Then** each IDE appears as a separate row sorted by IDE name.
3. **Given** I pass `--json`, **When** I run `wsm sessions discover --json`, **Then** output is valid JSON containing an array of objects with ide, path, session_count, and last_modified fields.
4. **Given** no IDE sessions exist on the machine, **When** I run `wsm sessions discover`, **Then** a helpful message explains that no sessions were found and suggests checking supported IDE paths.
5. **Given** stdout is not a TTY, **When** I run `wsm sessions discover` piped to another command, **Then** output defaults to JSON instead of the table format.

## Tests

### Unit
- `pkg/format/discover_test.go` -- `TestDiscover_FindsClaudeCode`
- `pkg/format/discover_test.go` -- `TestDiscover_FindsMultipleIDEs`
- `pkg/format/discover_test.go` -- `TestDiscover_JSONOutput`
- `pkg/format/discover_test.go` -- `TestDiscover_NoSessionsFound`
- `pkg/format/discover_test.go` -- `TestDiscover_TTYDetection`

### E2E
- `tests/e2e/sessions_discover_test.go` -- `TestSessionsDiscover_FindsSessions`
- `tests/e2e/sessions_discover_test.go` -- `TestSessionsDiscover_JSONOutput`
- `tests/e2e/sessions_discover_test.go` -- `TestSessionsDiscover_EmptyResult`
