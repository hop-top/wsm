# Plan 01f: Charmbracelet TUI Implementation

**Goal**: Deliver first-class TUI functionality in v1 using Charmbracelet packages.

## Stack

- `bubbletea` for state/update loop
- `bubbles` for components (lists, help, viewport)
- `lipgloss` for styling

## Scope

- `wsm tui` command
- Main views: workspace list, detail pane, action footer/help
- Actions: create, inspect, archive, unarchive, delete with confirmations
- Empty/error/loading states and keyboard help (`?`)

## Interaction Model

- Up/down and `j/k` navigation
- `Enter` to inspect/open details
- Contextual action keys for archive/unarchive/delete/create
- `q`/`Ctrl+C` to quit

## Acceptance Criteria

1. Stories `021` and `022` are fully supported.
2. TUI uses core manager APIs; no parallel data layer.
3. Failure states are actionable and non-destructive by default.
4. List navigation updates a dedicated detail pane (not status-line-only inspection).
