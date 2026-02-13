# Stories

User stories for the wsm (Workspace Session Manager) project, organized by persona.

## User (CLI/TUI)

| ID | Story | Feature | Priority |
|----|-------|---------|----------|
| 001 | [Workspace Creation](001-workspace-creation.md) | CLI Core | P0 |
| 002 | [Workspace Listing](002-workspace-listing.md) | CLI Core | P0 |
| 003 | [Workspace Inspection](003-workspace-inspection.md) | CLI Core | P0 |
| 004 | [Workspace Archival](004-workspace-archival.md) | CLI Core | P0 |
| 005 | [Workspace Deletion](005-workspace-deletion.md) | CLI Core | P0 |
| 006 | [Event Recording](006-event-recording.md) | Event Sourcing | P0 |
| 007 | [Snapshot Generation](007-snapshot-generation.md) | Event Sourcing | P0 |
| 008 | [Configuration Management](008-configuration-management.md) | CLI Core | P1 |
| 009 | [Shell Completion](009-shell-completion.md) | CLI Core | P1 |
| 019 | [Workspace Reference Resolution](019-workspace-reference-resolution.md) | CLI Core / Go Module | P0 |
| 020 | [Event Query Filtering](020-event-query-filtering.md) | Event Sourcing | P1 |
| 021 | [TUI Launch and Navigation](021-tui-launch-and-navigation.md) | TUI | P0 |
| 022 | [TUI Workspace Operations](022-tui-workspace-operations.md) | TUI | P0 |
| 023 | [UI Rendering Modes](023-ui-rendering-modes.md) | Output Modes | P1 |
| 027 | [Cross-IDE Session Continuity](027-cross-ide-session-continuity.md) | CLI Core / Event Sourcing | P0 |

## Maintainer (Testing/Operations)

| ID | Story | Validates | Priority |
|----|-------|-----------|----------|
| 010 | [Workspace Lifecycle Tests](010-workspace-lifecycle-tests.md) | 001-005 | P0 |
| 011 | [Event Integrity Tests](011-event-integrity-tests.md) | 006 | P0 |
| 012 | [Backend Conformance Tests](012-backend-conformance-tests.md) | Backend interface | P0 |
| 025 | [Repository Foundation and Governance](025-repo-foundation-and-governance.md) | Repo standards | P1 |
| 026 | [Test Harness and Conformance Gates](026-test-harness-and-conformance-gates.md) | 001-024 | P0 |

## External Client (Scripting/CI)

| ID | Story | Feature | Priority |
|----|-------|---------|----------|
| 013 | [JSON Output Mode](013-json-output-mode.md) | Output Modes | P0 |
| 014 | [Pipe-Friendly Output](014-pipe-friendly-output.md) | Output Modes | P1 |
| 015 | [Exit Code Contract](015-exit-code-contract.md) | Output Modes | P0 |
| 023 | [UI Rendering Modes](023-ui-rendering-modes.md) | Output Modes | P1 |

## Integrator (Go Module)

| ID | Story | Feature | Priority |
|----|-------|---------|----------|
| 016 | [Library API Surface](016-library-api-surface.md) | Go Module | P0 |
| 017 | [Custom Backend Implementation](017-custom-backend-implementation.md) | Go Module | P1 |
| 018 | [Device Auto-Detection](018-device-auto-detection.md) | Go Module | P1 |
| 019 | [Workspace Reference Resolution](019-workspace-reference-resolution.md) | CLI Core / Go Module | P0 |
| 024 | [Vendored UI Optionality](024-vendored-ui-optionality.md) | Go Module | P0 |
