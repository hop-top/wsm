# External Client

## Description

A script, CI/CD pipeline, or external tool that consumes `wsm` output programmatically. This persona relies on stable JSON, predictable exit codes, and rendering-mode controls that keep automation output clean.

## Information Discovery

- API / output reference docs (`docs/api/`)
- JSON schema files for output contracts
- Exit code table in `docs/exit-codes.md`
- CI integration examples (GitHub Actions, scripts)
- Changelog / release notes
- `--help` flag with machine-oriented examples

## Preferred Delivery Format

- JSON (structured, stable keys)
- Predictable exit codes (numeric, documented)
- Versioned output schemas
- Pipe-safe plain text (no ANSI, no TTY tricks)
- Inline `jq` / shell examples in docs

## Stories

- [013 - JSON Output Mode](../stories/013-json-output-mode.md)
- [014 - Pipe-Friendly Output](../stories/014-pipe-friendly-output.md)
- [015 - Exit Code Contract](../stories/015-exit-code-contract.md)
- [023 - UI Rendering Modes](../stories/023-ui-rendering-modes.md)
