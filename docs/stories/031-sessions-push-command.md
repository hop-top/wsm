# Sessions Push Command

**ID**: 031
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want to push IDE session files into a wsm workspace so that my coding history is preserved.

## Details

Command: `wsm sessions push <path>` -- auto-detects the file format, creates a session in the target workspace, appends normalized entries, and records workspace events.

### Flags

- `--workspace` -- target workspace (auto-creates if it does not exist)
- `--format` -- override auto-detection with an explicit format name
- `--json` -- output results as JSON
- `--quiet` -- suppress all output except errors

### Progress Feedback

- Spinner for small files (interactive TTY)
- Progress bar for large files (interactive TTY)
- No decorations when stdout is not a TTY (non-interactive / piped)

## Acceptance Scenarios

1. **Given** a Claude Code transcript file, **When** I run `wsm sessions push transcript.jsonl`, **Then** the format is auto-detected, a session is created in the current workspace, and entries are appended.
2. **Given** a session file, **When** I run `wsm sessions push transcript.jsonl --format claudecode`, **Then** the explicit format is used instead of auto-detection.
3. **Given** an existing workspace `my-project`, **When** I run `wsm sessions push transcript.jsonl --workspace my-project`, **Then** the session is created in the specified workspace.
4. **Given** no workspace named `new-project`, **When** I run `wsm sessions push transcript.jsonl --workspace new-project`, **Then** the workspace is auto-created and the session is pushed into it.
5. **Given** a session file, **When** I run `wsm sessions push transcript.jsonl --json`, **Then** output is JSON with session ID, entry count, workspace ID, and format detected.
6. **Given** stdout is not a TTY, **When** I run `wsm sessions push transcript.jsonl`, **Then** no spinner or progress bar is displayed.
7. **Given** a file with an unknown format, **When** I run `wsm sessions push mystery.dat`, **Then** the command fails with exit code and a helpful message listing supported formats.
8. **Given** an empty file, **When** I run `wsm sessions push empty.jsonl`, **Then** the command reports zero entries pushed and exits cleanly.

## Tests

### Unit
- `cmd/wsm/sessions_push_test.go` -- `TestSessionsPush_AutoDetect`
- `cmd/wsm/sessions_push_test.go` -- `TestSessionsPush_ExplicitFormat`
- `cmd/wsm/sessions_push_test.go` -- `TestSessionsPush_WorkspaceFlag`
- `cmd/wsm/sessions_push_test.go` -- `TestSessionsPush_AutoCreateWorkspace`
- `cmd/wsm/sessions_push_test.go` -- `TestSessionsPush_JSONOutput`
- `cmd/wsm/sessions_push_test.go` -- `TestSessionsPush_UnknownFormat`
- `cmd/wsm/sessions_push_test.go` -- `TestSessionsPush_EmptyFile`
- `pkg/format/push_test.go` -- `TestPushOrchestration_DetectAndParse`
- `pkg/format/push_test.go` -- `TestPushOrchestration_AppendEntries`
- `pkg/format/push_test.go` -- `TestPushOrchestration_RecordWorkspaceEvent`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
