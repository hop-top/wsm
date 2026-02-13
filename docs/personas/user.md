# User

## Description

An end-user of the `wsm` CLI who creates workspaces, records events, inspects session history, and manages workspace lifecycle. This persona cares about quick access to workspace state, scriptable output, and reliable storage of session data across AI coding IDEs.

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

## Related Stories

Stories where User functionality is validated by another persona:

- [010 - Workspace Lifecycle Tests](../stories/010-workspace-lifecycle-tests.md) (Maintainer validates [001](../stories/001-workspace-creation.md))
- [011 - Event Integrity Tests](../stories/011-event-integrity-tests.md) (Maintainer validates [006](../stories/006-event-recording.md))
