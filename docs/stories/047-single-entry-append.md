# Single Entry Append

**ID**: 047
**Feature**: Session Management
**Persona**: [User](../personas/user.md), [Integrator](../personas/integrator.md)
**Priority**: P1

## Story

As a user or integrator, I want to create a session and append individual entries to it so that I can stream events from an external system (IDE hooks, CI pipelines, plugins) without writing a file first.

## Design

Two operations, available as both CLI commands and Go API:

### CLI

```bash
# Create a session, get back the session ID
wsm sessions create <workspace> --name "live-capture"

# Append a single entry to an existing session
wsm sessions append <session-id> --type message --data '{"role":"user","content":"fix the auth bug"}'
```

`sessions append` accepts one entry per invocation. The entry type maps to wsm's event model (`message`, `tool_call`, `file_change`). The `--data` flag takes a JSON payload matching the corresponding data struct.

### Go API

```go
sess, err := manager.NewSession(workspaceID, session.Options{Name: "live-capture"})
err = manager.AppendEntry(sess.ID, entry)
```

`NewSession` returns a session with a ULID. `AppendEntry` appends one typed entry and records the corresponding workspace event.

### Flags (CLI)

- `--type` -- entry type: `message`, `tool_call`, `file_change`
- `--data` -- JSON payload for the entry (or read from stdin with `--data -`)
- `--json` -- output result as JSON
- `--quiet` -- suppress output except errors

## Acceptance Scenarios

1. **Given** workspace `my-project`, **When** I run `wsm sessions create my-project --name "live"`, **Then** a session ID is returned and the session exists in the workspace.
2. **Given** session `s1`, **When** I run `wsm sessions append s1 --type message --data '{"role":"assistant","content":"done"}'`, **Then** the entry is appended to the session's JSONL and a `interaction.message` workspace event is recorded.
3. **Given** session `s1`, **When** I pipe JSON to `echo '{"role":"user","content":"hello"}' | wsm sessions append s1 --type message --data -`, **Then** the entry is read from stdin and appended.
4. **Given** session `s1`, **When** I append a `tool_call` entry, **Then** a `interaction.tool_call` workspace event is recorded with the tool name and input.
5. **Given** session `s1`, **When** I append a `file_change` entry, **Then** a `mutation.file_patch` workspace event is recorded with the file path and operation.
6. **Given** an invalid session ID, **When** I run `wsm sessions append bad-id ...`, **Then** the command fails with a clear error and non-zero exit code.
7. **Given** `--json` flag, **When** I run `wsm sessions append s1 --type message --data '...' --json`, **Then** the output is JSON with session ID, entry ID, and seq num.
8. **Given** the Go API, **When** I call `manager.NewSession` followed by `manager.AppendEntry`, **Then** the entry is persisted in JSONL and the workspace event is recorded.

## Tests

### Unit
- `pkg/session/manager_test.go` -- `TestSessionManager_NewSession_ReturnsID`
- `pkg/session/manager_test.go` -- `TestSessionManager_AppendEntry_Message`
- `pkg/session/manager_test.go` -- `TestSessionManager_AppendEntry_ToolCall`
- `pkg/session/manager_test.go` -- `TestSessionManager_AppendEntry_FileChange`
- `pkg/session/manager_test.go` -- `TestSessionManager_AppendEntry_InvalidSession`
- `pkg/session/manager_test.go` -- `TestSessionManager_AppendEntry_RecordsWorkspaceEvent`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
