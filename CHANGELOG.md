# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project aims to follow Semantic Versioning.

## [Unreleased]

### Added
- `--note|-n` flag on `wsm workspace archive`, `wsm workspace unarchive`,
  `wsm workspace delete`, `wsm snapshot delete`, and `wsm space delete`.
  Records a transition note plumbed into ctx via
  `policy.ContextAttrsKey` for the runtime policy engine to read
  (T-1287).
- `kit/runtime/policy` adoption: declarative policy YAML at
  `$XDG_CONFIG_HOME/wsm/policies.yaml`, evaluated against state changes
  via kit's policy engine. First boot seeds the file from a bundled
  default; existing user files are never overwritten. Override via
  `WSM_POLICY_FILE` env. Default policies: `archive-requires-note`,
  `delete-requires-note`. See [`docs/wsm/policies.md`](docs/wsm/policies.md).
  (T-1288)

### Changed
- `pkg/workspace.Manager` (workspace) and snapshot manager paths now
  backed by `kit/runtime/domain.Service[Workspace]` and
  `domain.Service[Snapshot]`. Internals-only refactor; public Manager
  API and CLI behavior unchanged for callers (T-1286).
- New topic aliases: `wsm.runtime.snapshot.updated` and
  `wsm.runtime.snapshot.deleted` now publish on snapshot Update/Delete
  operations (closes the alias gap noted in kit ADR-0008 §6).

### Fixed
- `wsm space add` no longer overwrites an existing space with the same ID.
  Space IDs are global; a colliding add, in any workspace including the
  same one, fails with exit code 4 (conflict) and leaves the existing space
  and its associations unchanged.

### Note for operators
- `wsm workspace archive <id>` and `wsm workspace delete <id>` without
  `--note` now exit 4 with a `policy "<name>" denied: ...` error.
  Update scripts to pass `--note "<reason>"`. To restore pre-T-1288
  behavior temporarily, edit `$XDG_CONFIG_HOME/wsm/policies.yaml` and
  remove the offending rule. See [`docs/wsm/policies.md`](docs/wsm/policies.md)
  for migration guidance.

### Breaking
- Exit codes realigned with story 015. Capability denials now exit `6`
  (was `5`); backend / persistence failures now exit `5` (was `64`). A
  new exit `7` marks an unsatisfied prerequisite (missing binary, busy
  port, unreachable server) with the fix in the message. Scripts
  branching on `5` or `64` must be updated.
- Removed `--json` flag. Use `--format json` instead.
- Renamed `event add --format` (payload format) to `--content-format` to free
  the `--format` name for the global output selector provided by kit/cli.
- Renamed `wsm workspace remove` → `wsm workspace delete` (aliases: `rm`, `del`).
- Renamed `wsm space remove` → `wsm space delete`.
- Renamed `wsm snapshot remove` → `wsm snapshot delete` (aliases: `rm`, `del`).
- Swapped `hop.top/upgrade` for `hop.top/kit/go/core/upgrade`. Behavior parity
  for end users; agent-facing skill preamble surface preserved via
  `wsm upgrade preamble`.
- Migrated TUI to Charmbracelet bubbletea v2 (`charm.land/bubbletea/v2`,
  `charm.land/lipgloss/v2`). `wsm tui` flow unchanged for users.

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
- `pkg/config`: `XDG_DATA_HOME` support for `defaultDataDir()`, consistent
  with cache/state dir resolution.
- `pkg/config`: Legacy migration helper (`MigrateFromLegacy`) moves
  `~/.wsm/wsm.db` to XDG data dir on first run.
- `cmd/wsm`: Auto-create XDG directories (data, cache, state) on startup.
- `pkg/device`: Persistent machine fingerprint using platform-specific
  sources (macOS IOPlatformUUID, Linux machine-id) with fallback UUID
  generation and `$XDG_DATA_HOME/wsm/machine-id` persistence.
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
- Adopted `hop.top/kit` across the CLI: `kit/console/cli` (`cli.New`,
  fang-styled help, `--help-all`, `--format`, `-V`, `-C`/`--chdir`),
  `kit/console/output` (`output.Render`, table/JSON/YAML/CSV/text), `kit/console/log`
  (slog handler), `kit/core/config.Loader`, `kit/core/xdg`, `kit/core/upgrade`,
  and `kit/tui` helpers.
- Help is now grouped: `ORGANIZE` (`workspace`, `space`, `device`),
  `CAPTURE` (`event`, `snapshot`), `INTERACT` (`init`, `tui`), and
  `MANAGEMENT` (`config`, `upgrade`, `version`, `completion`).
  `MANAGEMENT` is hidden by default; surface it with `--help-all` or
  `--help-management`.
- Bare `wsm` (no args) prints the quick-start hint to stderr (was stdout) so
  `wsm | jq` keeps stdout empty.
- Documentation: added `docs/wsm/api-cli.md` (full subcommand reference) and
  `docs/wsm/configuration.md` (config keys, env vars, layered loading).
