# Maintainer

## Description

A developer or maintainer of the wsm project who writes and runs automated tests, implements storage backends, and ensures data integrity across changes. This persona cares about test coverage, migration safety, and the stability of the storage backend interface.

## Stories

- [010 - Workspace Lifecycle Tests](../stories/010-workspace-lifecycle-tests.md)
- [011 - Event Integrity Tests](../stories/011-event-integrity-tests.md)
- [012 - Backend Conformance Tests](../stories/012-backend-conformance-tests.md)

## Related Stories

User stories that Maintainer test stories validate:

- [001 - Workspace Creation](../stories/001-workspace-creation.md) (tested by [010](../stories/010-workspace-lifecycle-tests.md))
- [006 - Event Recording](../stories/006-event-recording.md) (tested by [011](../stories/011-event-integrity-tests.md))
