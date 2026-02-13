# UI Rendering Modes

**ID**: 023
**Feature**: Output Modes
**Persona**: [External Client](../personas/external-client.md)
**Related Personas**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user and external client, I want explicit UI mode control so that output is predictable across TTY, CI, and optional Charm builds.

## Acceptance Scenarios

1. **Given** default mode `--ui=auto`, **When** stdout is non-TTY, **Then** plain rendering is selected.
2. **Given** `NO_COLOR=1`, **When** I run in TTY, **Then** output contains no ANSI color codes.
3. **Given** `--ui=plain`, **When** I run commands in TTY, **Then** output stays plain and script-safe.
4. **Given** `--ui=charm` with a non-charm build, **When** I run the command, **Then** it fails with usage exit code 2 and explains how to enable charm mode.
5. **Given** `--ui=charm` in a charm-enabled build, **When** I run `wsm tui`, **Then** Charmbracelet rendering is used.

## Tests

### E2E
- `tests/e2e/output_test.go` -- `TestUIModeSelection`
- `tests/e2e/output_test.go` -- `TestNoColorTTY`
