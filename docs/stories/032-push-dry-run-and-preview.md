# Push Dry-Run and Preview

**ID**: 032
**Feature**: IDE Session Import
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want to preview what a push would do before committing so that I can verify the data before it enters my workspace.

## Details

Command: `wsm sessions push --dry-run <path>`

Displays a summary without writing anything to the backend:

- Detected format (e.g., `claudecode`)
- Total entry count
- Date range (earliest to latest entry)
- Breakdown: message count, tool call count, file change count
- Warnings (e.g., malformed lines skipped)

## Acceptance Scenarios

1. **Given** a valid session file, **When** I run `wsm sessions push --dry-run transcript.jsonl`, **Then** a summary is displayed showing format, entry count, date range, and breakdown.
2. **Given** a valid session file, **When** I run `wsm sessions push --dry-run transcript.jsonl`, **Then** no data is written to the backend.
3. **Given** a valid session file, **When** I run `wsm sessions push --dry-run --json transcript.jsonl`, **Then** the summary is output as JSON with format, counts, date range, and breakdown fields.
4. **Given** a session file with malformed lines, **When** I run `wsm sessions push --dry-run transcript.jsonl`, **Then** warnings are displayed for each malformed line alongside the valid entry summary.
5. **Given** an empty session file, **When** I run `wsm sessions push --dry-run empty.jsonl`, **Then** the summary shows zero entries and no date range.

## Tests

### Unit
- `pkg/format/dryrun_test.go` -- `TestDryRun_Summary`
- `pkg/format/dryrun_test.go` -- `TestDryRun_WritesNothing`
- `pkg/format/dryrun_test.go` -- `TestDryRun_JSONOutput`
- `pkg/format/dryrun_test.go` -- `TestDryRun_MalformedWarnings`
- `pkg/format/dryrun_test.go` -- `TestDryRun_EmptyFile`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
