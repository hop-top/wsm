# Maintainer

## Description

A developer or maintainer of the wsm project who writes and runs automated tests, manages repository standards, implements storage backends, and ensures data integrity across changes.

## Information Discovery

- Inline code comments and GoDoc
- ADRs (architecture decision records)
- `CONTRIBUTING.md` and `docs/` directory
- Test fixtures and golden files
- CI pipeline logs and conformance reports
- Git history and PR descriptions

## Preferred Delivery Format

- Structured markdown (tables, checklists)
- `go test -v` verbose output and coverage reports
- Diff / patch format for change review
- YAML/JSON schema definitions
- Automated lint and conformance gate summaries

## Stories

- [010 - Workspace Lifecycle Tests](../stories/010-workspace-lifecycle-tests.md)
- [011 - Event Integrity Tests](../stories/011-event-integrity-tests.md)
- [012 - Backend Conformance Tests](../stories/012-backend-conformance-tests.md)
- [025 - Repository Foundation and Governance](../stories/025-repo-foundation-and-governance.md)
- [026 - Test Harness and Conformance Gates](../stories/026-test-harness-and-conformance-gates.md)

## Related Stories

User stories that Maintainer test stories validate:

- [001 - Workspace Creation](../stories/001-workspace-creation.md) (tested by [010](../stories/010-workspace-lifecycle-tests.md))
- [006 - Event Recording](../stories/006-event-recording.md) (tested by [011](../stories/011-event-integrity-tests.md))
- [001-024](../stories/README.md) (gated by [026](../stories/026-test-harness-and-conformance-gates.md))
