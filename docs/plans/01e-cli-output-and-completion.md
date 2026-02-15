# Plan 01e: CLI Output and Completion

**Goal**: Implement `wsm` CLI command surface and robust automation contracts.

## Scope

- Commands: `create`, `list`, `inspect`, `event add`, `events`, `snapshot`, `archive`, `unarchive`, `delete`, `config show`, `completion`, `tui`
- Global flags: `--json`, `--quiet`, `--no-color`, backend/data-dir/backend-url flags
- Output contracts for TTY, non-TTY, JSON mode, and quiet mode
- Exit code taxonomy and completion behavior

## UX Contracts

1. JSON mode emits JSON only on stdout.
2. Non-TTY defaults to pipe-safe plain output unless JSON explicitly required by contract.
3. TTY outputs styled output by default.
4. `--no-color` or `NO_COLOR=1` disables colorization.

## Completion

- Workspace reference completion for `inspect`, `archive`, `unarchive`, `delete`, `events`, `snapshot`, and `event add`.
- Enumerated completion for `--status` and `--sort`.

## Acceptance Criteria

1. Stories `001-005`, `008-009`, `013-015`, `019-020`, `023` pass behavior checks.
2. CLI remains a thin consumer of `pkg/*`.
