# User

## Description

An end-user of the `wsm` CLI/TUI who creates workspaces, records events, inspects session history, and manages workspace lifecycle. This persona cares about fast workflows, clear visual feedback, scriptable output, and reliable storage across AI coding IDEs.

## Information Discovery

- User manual (web / hosted docs site)
- FAQ / troubleshooting guide
- `wsm --help` / `wsm <cmd> --help`
- TUI inline tooltips and status bar hints
- `README.md` quick-start section
- Man pages (`wsm.1`)
- Shell completion suggestions

## Preferred Delivery Format

- Illustrated web pages (guides, tutorials, searchable)
- Short plain-text (one-liner summaries)
- Interactive prompts with sensible defaults
- Contextual hints in TUI (hover / focus)
- Color-coded terminal output
- Progressive disclosure (summary → detail on demand)

## Stories

- [001 - Workspace Creation](../stories/001-workspace-creation.md)
- [002 - Workspace Listing](../stories/002-workspace-listing.md)
- [003 - Workspace Inspection](../stories/003-workspace-inspection.md)
- [004 - Workspace Archival](../stories/004-workspace-archival.md)
- [005 - Workspace Deletion](../stories/005-workspace-deletion.md)
- [006 - Event Recording](../stories/006-event-recording.md)
- [007 - Snapshot Generation](../stories/007-snapshot-generation.md)
- [008 - Configuration Management](../stories/008-configuration-management.md)
- [009 - Shell Completion](../stories/009-shell-completion.md)
- [019 - Workspace Reference Resolution](../stories/019-workspace-reference-resolution.md)
- [020 - Event Query Filtering](../stories/020-event-query-filtering.md)
- [021 - TUI Launch and Navigation](../stories/021-tui-launch-and-navigation.md)
- [022 - TUI Workspace Operations](../stories/022-tui-workspace-operations.md)
- [023 - UI Rendering Modes](../stories/023-ui-rendering-modes.md)

## Related Stories

Stories where User functionality is validated by another persona:

- [010 - Workspace Lifecycle Tests](../stories/010-workspace-lifecycle-tests.md) (Maintainer validates [001](../stories/001-workspace-creation.md))
- [011 - Event Integrity Tests](../stories/011-event-integrity-tests.md) (Maintainer validates [006](../stories/006-event-recording.md))
- [026 - Test Harness and Conformance Gates](../stories/026-test-harness-and-conformance-gates.md) (Maintainer enforces story coverage in CI)
