# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project aims to follow Semantic Versioning.

## [Unreleased]

### Added
- Initial project docs and story pack expansion (`001-026`).
- Decomposed implementation plans (`01a-01h`).
- Repository governance and contributor bootstrap files.
- Explicit `wsm event add <workspace> <type>` command for appending `message.added`, `tool_call.*`, `file.changed`, and `device.*` events.
- Explicit `wsm snapshot [workspace]` command for generating/persisting snapshots from CLI.
- New `pkg/tui` package with unit-tested navigation/detail/action helpers used by charm TUI.
- Story-aligned unit suites:
  - `pkg/workspace/event_test.go`
  - `pkg/workspace/snapshot_test.go`
  - expanded `pkg/workspace/manager_test.go`
  - `pkg/tui/model_test.go`
  - `pkg/tui/actions_test.go`
  - `pkg/cli/completion_test.go`

### Changed
- Plan 1 updated to include TUI in v1 with optional Charmbracelet path.
- Charm TUI now renders a master/detail layout with a dedicated detail pane that updates with selection.
