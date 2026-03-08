# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project aims to follow Semantic Versioning.

## [Unreleased]

### Added
- Initial project docs and story pack expansion (`001-026`).
- Decomposed implementation plans (`01a-01h`).
- Repository governance and contributor bootstrap files.
- Explicit `wsm event add <workspace> <type>` command for appending
  `message.added`, `tool_call.*`, `file.changed`, and `device.*` events.
- Explicit `wsm snapshot [workspace]` command for generating/persisting
  snapshots from CLI.
- New `pkg/tui` package with unit-tested navigation/detail/action helpers
  used by charm TUI.
- New `pkg/detector` package — plugin-based detector framework for scanning
  spaces:
  - `Detector` interface + `Registry` for pluggable detection.
  - Built-in detectors: Makefile, package.json, docker-compose, devcontainer,
    .env.example.
  - Knowledge source detectors: package registries, local docs.
  - Dependency health detector: go.mod, package.json, requirements.txt,
    Gemfile.
  - Tool integration detectors: mise (.mise.toml), SecretSpec
    (secretspec.toml), fnox (fnox.toml).
  - Strategy generation from detector results.
  - User overrides per space or workspace-wide.
- New `pkg/access` package — access grants and enforcement:
  - `Grant` / `GrantSet` for declaring subject-resource permissions.
  - `Prerequisite` with enforcement levels (required, recommended, optional).
  - `RunVerification` with `--no-verify` bypass support.
- New `pkg/scope` package — workspace scope and assignment:
  - `ScopeType` (file_patterns, operations, tool_access, secrets, network).
  - `ScopeSet` with glob-based matching for file patterns.
  - Profile/squad assignment to workspaces with scoped permissions.
  - Scope intersection, union, and effective scope query API.
  - Workspace-scoped vs space-scoped session validation.
- `pkg/config`: XDG cache (`$XDG_CACHE_HOME/wsm/`) and state
  (`$XDG_STATE_HOME/wsm/`) directory support with env/flag overrides.
- `pkg/config`: Local config directory discovery (`.wsm/` and `.hop/wsm/`)
  with walk-up search and opt-in loading.
- `pkg/workspace`: YAML workspace export (`ExportWorkspace`) with metadata
  and space definitions.
- `pkg/workspace`: YAML workspace import (`ImportWorkspace`) with conflict
  resolution strategies (skip, rename, overwrite) and dry-run support.
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
