# Push Idempotency

**ID**: 033
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to re-push a growing IDE session file without duplicating entries so that I can keep my workspace in sync with ongoing sessions.

## Details

### Default Behavior (Entry-Level Idempotency)

Each entry is hashed on parse. On push, existing hashes are checked against the target session. Duplicate entries are skipped; only new entries are appended. The command reports "N new, M skipped."

### Flags

- `--session-level` -- skip the entire push if the source path has already been pushed to this workspace. Faster than entry-level checking for large files that have not changed.
- `--force` -- re-push everything, overwriting existing entries. Useful for correcting a previous bad import.

## Acceptance Scenarios

1. **Given** a session file previously pushed with 100 entries, **When** the file grows to 120 entries and I run `wsm sessions push transcript.jsonl`, **Then** 20 new entries are appended and 100 are skipped, with output "20 new, 100 skipped."
2. **Given** a session file previously pushed, **When** I re-push the identical file, **Then** all entries are skipped with output "0 new, 100 skipped."
3. **Given** a session file previously pushed, **When** I run `wsm sessions push --session-level transcript.jsonl`, **Then** the entire push is skipped because the source path was already pushed, with output indicating session-level skip.
4. **Given** a session file previously pushed, **When** I run `wsm sessions push --force transcript.jsonl`, **Then** all entries are re-pushed regardless of duplicates.
5. **Given** two different files producing an entry with the same hash, **When** both are pushed, **Then** the duplicate entry is detected and skipped on the second push.

## Tests

### Unit
- `pkg/format/idempotency_test.go` -- `TestIdempotency_EntryHash`
- `pkg/format/idempotency_test.go` -- `TestIdempotency_AppendNewOnly`
- `pkg/format/idempotency_test.go` -- `TestIdempotency_SkipAllDuplicates`
- `pkg/format/idempotency_test.go` -- `TestIdempotency_SessionLevel`
- `pkg/format/idempotency_test.go` -- `TestIdempotency_Force`
- `pkg/format/idempotency_test.go` -- `TestIdempotency_HashCollision`

### E2E
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_Idempotent_GrowingFile`
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_Idempotent_IdenticalFile`
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_SessionLevel`
- `tests/e2e/sessions_push_test.go` -- `TestSessionsPush_Force`
