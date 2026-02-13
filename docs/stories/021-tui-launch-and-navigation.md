# TUI Launch and Navigation

**ID**: 021
**Feature**: TUI
**Persona**: [User](../personas/user.md)
**Priority**: P0

## Story

As a user, I want a first-class terminal UI so that I can manage workspaces quickly with keyboard-driven navigation.

## Acceptance Scenarios

1. **Given** a build with charm UI enabled, **When** I run `wsm tui`, **Then** the TUI launches successfully.
2. **Given** workspaces exist, **When** I navigate with arrow keys or `j/k`, **Then** the selection and detail pane update correctly.
3. **Given** no workspaces exist, **When** TUI opens, **Then** an empty state with clear next steps is shown.
4. **Given** data loading or backend failure, **When** errors occur, **Then** visible, actionable error messaging is shown in-TUI.
5. **Given** I need help, **When** I press `?`, **Then** keybindings help is shown.

## Tests

### Unit
- `pkg/tui/model_test.go` -- `TestRootNavigation`

### E2E
- `tests/e2e/tui_test.go` -- `TestTUILaunch`
