# Pipe-Friendly Output

**ID**: 014
**Feature**: Output Modes
**Persona**: [External Client](../personas/external-client.md)
**Priority**: P1

## Story

As an external client, I want wsm to auto-detect non-TTY and suppress color/styling so that piped output is clean.

## Acceptance Scenarios

1. **Given** non-TTY stdout (piped), **When** I run `wsm list`, **Then** output is a plain table with no ANSI escape codes.
2. **Given** non-TTY stdout, **When** I run `wsm create my-project`, **Then** output is JSON (same as --json).
3. **Given** `wsm list -q` piped to xargs, **When** I pipe the output, **Then** each line is a bare workspace ID suitable for scripting.
4. **Given** NO_COLOR=1 env var set, **When** I run `wsm list` in a TTY, **Then** output has no color codes.
5. **Given** non-TTY and `wsm delete` without `--confirm=yes` (or the legacy `--force`), **When** I run it, **Then** it fails with exit code 5 (kit's UNAUTHORIZED confirm gate; it cannot prompt in non-TTY).

## Tests

### Unit

### E2E
- `tests/e2e/output_test.go` — `TestPipe_NoANSI`
- `tests/e2e/output_test.go` — `TestPipe_QuietIds`
- `tests/e2e/output_test.go` — `TestPipe_DeleteRequiresForce`
