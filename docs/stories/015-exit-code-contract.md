# Exit Code Contract

**ID**: 015
**Feature**: Output Modes
**Persona**: [External Client](../personas/external-client.md)
**Priority**: P0

## Story

As an external client, I want predictable exit codes so that my scripts can handle wsm errors programmatically.

## Acceptance Scenarios

1. **Given** successful `wsm create`, **When** I check the exit code, **Then** it is 0.
2. **Given** `wsm create` with missing name argument, **When** I check the exit code, **Then** it is 2 (usage error).
3. **Given** `wsm inspect nonexistent`, **When** I check the exit code, **Then** it is 3 (not found).
4. **Given** `wsm create duplicate-name` when it already exists, **When** I check the exit code, **Then** it is 4 (conflict).
5. **Given** a corrupted SQLite database, **When** I run any command, **Then** the exit code is 8 (backend error).
6. **Given** `wsm archive already-archived`, **When** I check the exit code, **Then** it is 4 (conflict).
7. **Given** an operation blocked by a capability gate, **When** I check the exit code, **Then** it is 5 (forbidden).
8. **Given** an operation whose environment is not ready (missing binary, busy port, unreachable server), **When** I check the exit code, **Then** it is 70 (prerequisite) and the message states the fix.
9. **Given** an operation refused because wsm has not been configured for it (IRC not enabled, no port triple recorded, an empty `irc.server.host`), **When** I check the exit code, **Then** it is 9 (not configured) and the message names the command to run or the key to set.

## Alignment with kit

Codes 0-5 and 70 are `hop.top/kit`'s standard table, so an agent that
already understands kit understands wsm without a second lookup table:

| Code | kit name | wsm kind |
|---|---|---|
| 0 | `OK` | success |
| 1 | `GENERIC` | unclassified |
| 2 | `USAGE` | usage |
| 3 | `NOT_FOUND` | not_found |
| 4 | `CONFLICT` | conflict (including policy denial) |
| 5 | `UNAUTHORIZED` | forbidden; also kit's `--confirm` refusal |
| 70 | `PREREQUISITE` | prerequisite |

8 (backend) and 9 (not_configured) are wsm-local, for the two classes
kit defines no code for. They sit below kit's reserved 64-70 band —
which is contiguous and already full (64 `RATE_LIMITED`, 65
`PROVENANCE_MISSING`, 66 `LEAK_DETECTED`, 67 `CONFIG`, 68 `GRADE_FAIL`,
69 `GRADE_UNGRADABLE`, 70 `PREREQUISITE`) — rather than above it,
because kit grows that band upward and would collide.

Scenarios 5, 7 and 8 previously used 5, 6 and 7. Each collided with a
kit code meaning something else, and the 7 was the sharpest: kit's 7 is
`CONSENT_REFUSED`, which kit classifies **transient**, while a wsm
prerequisite is frequently permanent. An agent branching on 7 per kit's
taxonomy would have retried a missing binary no retry can fix.

Exit code alone never answers whether a retry can help. Under
`--format json` each error carries a `transience` field (`transient` |
`permanent` | `unknown`); branch on that.

## Tests

### Unit
- `cmd/wsm/commands/exit_code_test.go` — `TestExitCode_Forbidden`
- `cmd/wsm/commands/exit_code_test.go` — `TestExitCode_Prerequisite`
- `cmd/wsm/commands/exit_code_test.go` — `TestExitCode_BackendMapping`
- `cmd/wsm/commands/exit_code_test.go` — `TestExitCode_NoKitCollision`
- `cmd/wsm/commands/exit_code_test.go` — `TestTransience_ByKind`
- `cmd/wsm/commands/exit_code_test.go` — `TestNotConfigured_IsDistinctFromPrerequisite`
- `cmd/wsm/exit_code_test.go` — `TestExitCodeForError_Story015`

### E2E
- `tests/e2e/exit_code_test.go` — `TestExitCode_Success`
- `tests/e2e/exit_code_test.go` — `TestExitCode_Usage`
- `tests/e2e/exit_code_test.go` — `TestExitCode_NotFound`
- `tests/e2e/exit_code_test.go` — `TestExitCode_Conflict`
- `tests/e2e/exit_code_test.go` — `TestExitCode_BackendError`
- `tests/e2e/irc_cli_contract_test.go` — `TestIRC_PurgeRefusedIsDistinctFromBackendCode`
- `tests/e2e/irc_cli_contract_test.go` — `TestIRC_PrerequisiteAndForbiddenOnTheSameVerb`
