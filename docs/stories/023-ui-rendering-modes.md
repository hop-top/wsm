# UI Rendering Modes

**ID**: 023
**Feature**: Output Modes
**Persona**: [External Client](../personas/external-client.md)
**Related Personas**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user and external client, I want styled output by default in TTY with the ability to disable it so that output is predictable across TTY, CI, and scripting contexts.

## Acceptance Scenarios

1. **Given** stdout is TTY, **When** I run commands, **Then** styled output with ANSI codes is displayed.
2. **Given** stdout is non-TTY (pipe/redirect), **When** I run commands, **Then** plain output is rendered.
3. **Given** `--no-color` flag, **When** I run in TTY, **Then** output contains no ANSI color codes.
4. **Given** `NO_COLOR=1` environment variable, **When** I run in TTY, **Then** output contains no ANSI color codes.

## Tests

### E2E
- `tests/e2e/output_test.go` -- `TestPipe_NoANSI`
- `tests/e2e/output_test.go` -- `TestNoColorFlag`
- `tests/e2e/output_test.go` -- `TestNoColorEnv`
