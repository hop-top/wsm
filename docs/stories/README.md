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
| 028 | [Session Management and Branching](028-session-management.md) | Session Management | P1 |
| 047 | [Single Entry Append](047-single-entry-append.md) | Session Management | P1 |

### Workspace IRC

| ID | Story | Feature | Priority |
|----|-------|---------|----------|
| 048 | [IRC Enable and Lifecycle](048-irc-enable-and-lifecycle.md) | Workspace IRC | P1 |
| 049 | [IRC Member Accounts and Credentials](049-irc-member-accounts-and-credentials.md) | Workspace IRC | P1 |
| 050 | [IRC Channels and Roles via Providers](050-irc-channels-and-roles-via-providers.md) | Workspace IRC | P1 |
| 052 | [IRC Remote Access over WebSocket and TLS](052-irc-remote-access-websocket-tls.md) | Workspace IRC | P2 |
| 054 | [IRC Shared Servers, Guests, and Delegation](054-irc-shared-servers-guests-and-delegation.md) | Workspace IRC | P2 |
| 055 | [IRC Bot Channels: Access Levels, Discovery, and Relay](055-irc-bot-channels-discovery-and-relay.md) | Workspace IRC | P2 |

### IDE Session Import

| ID | Story | Feature | Priority |
|----|-------|---------|----------|
| 029 | [Format Adapter Interface](029-format-adapter-interface.md) | IDE Session Import | P0 |
| 030 | [Claude Code Adapter](030-claude-code-adapter.md) | IDE Session Import | P0 |
| 031 | [Sessions Push Command](031-sessions-push-command.md) | IDE Session Import | P0 |
| 032 | [Push Dry-Run & Preview](032-push-dry-run-and-preview.md) | IDE Session Import | P1 |
| 033 | [Push Idempotency](033-push-idempotency.md) | IDE Session Import | P1 |
| 034 | [Gemini Adapter](034-gemini-adapter.md) | IDE Session Import | P1 |
| 035 | [Codex Adapter](035-codex-adapter.md) | IDE Session Import | P1 |
| 036 | [Vibe Adapter](036-vibe-adapter.md) | IDE Session Import | P1 |
| 037 | [OpenCode Adapter](037-opencode-adapter.md) | IDE Session Import | P1 |
| 038 | [Droid Adapter](038-droid-adapter.md) | IDE Session Import | P1 |
| 039 | [Cursor Adapter](039-cursor-adapter.md) | IDE Session Import | P1 |
| 040 | [VS Code Copilot Adapter](040-vscode-copilot-adapter.md) | IDE Session Import | P1 |
| 041 | [Antigravity Adapter](041-antigravity-adapter.md) | IDE Session Import | P1 |
| 042 | [Aider Adapter](042-aider-adapter.md) | IDE Session Import | P2 |
| 043 | [Goose Adapter](043-goose-adapter.md) | IDE Session Import | P2 |
| 044 | [Sessions Discover](044-sessions-discover.md) | IDE Session Import | P2 |

## Maintainer (Testing/Operations)

| ID | Story | Validates | Priority |
|----|-------|-----------|----------|
| 010 | [Workspace Lifecycle Tests](010-workspace-lifecycle-tests.md) | 001-005 | P0 |
| 011 | [Event Integrity Tests](011-event-integrity-tests.md) | 006 | P0 |
| 012 | [Backend Conformance Tests](012-backend-conformance-tests.md) | Backend interface | P0 |
| 025 | [Repository Foundation and Governance](025-repo-foundation-and-governance.md) | Repo standards | P1 |
| 026 | [Test Harness and Conformance Gates](026-test-harness-and-conformance-gates.md) | 001-024 | P0 |
| 045 | [Format Adapter Conformance Tests](045-format-adapter-conformance-tests.md) | IDE Session Import | P0 |
| 050 | [IRC Channels and Roles via Providers](050-irc-channels-and-roles-via-providers.md) | Workspace IRC | P1 |
| 054 | [IRC Shared Servers, Guests, and Delegation](054-irc-shared-servers-guests-and-delegation.md) | Workspace IRC | P2 |

## External Client (Scripting/CI)

| ID | Story | Feature | Priority |
|----|-------|---------|----------|
| 013 | [JSON Output Mode](013-json-output-mode.md) | Output Modes | P0 |
| 014 | [Pipe-Friendly Output](014-pipe-friendly-output.md) | Output Modes | P1 |
| 015 | [Exit Code Contract](015-exit-code-contract.md) | Output Modes | P0 |
| 023 | [UI Rendering Modes](023-ui-rendering-modes.md) | Output Modes | P1 |
| 051 | [IRC Message Capture and Backfill](051-irc-message-capture-and-backfill.md) | Workspace IRC | P1 |

## Integrator (Go Module)

| ID | Story | Feature | Priority |
|----|-------|---------|----------|
| 016 | [Library API Surface](016-library-api-surface.md) | Go Module | P0 |
| 017 | [Custom Backend Implementation](017-custom-backend-implementation.md) | Go Module | P1 |
| 018 | [Device Auto-Detection](018-device-auto-detection.md) | Go Module | P1 |
| 019 | [Workspace Reference Resolution](019-workspace-reference-resolution.md) | CLI Core / Go Module | P0 |
| 024 | [Vendored UI Optionality](024-vendored-ui-optionality.md) | Go Module | P0 |
| 029 | [Format Adapter Interface](029-format-adapter-interface.md) | IDE Session Import | P0 |
| 046 | [Format Adapter Interface Review](046-format-adapter-interface-review.md) | IDE Session Import | P1 |
| 047 | [Single Entry Append](047-single-entry-append.md) | Session Management | P1 |
| 049 | [IRC Member Accounts and Credentials](049-irc-member-accounts-and-credentials.md) | Workspace IRC | P1 |
| 051 | [IRC Message Capture and Backfill](051-irc-message-capture-and-backfill.md) | Workspace IRC | P1 |
| 053 | [IRC Provider Adapter Contract](053-irc-provider-adapter-contract.md) | Workspace IRC | P1 |
| 055 | [IRC Bot Channels: Access Levels, Discovery, and Relay](055-irc-bot-channels-discovery-and-relay.md) | Workspace IRC | P2 |
