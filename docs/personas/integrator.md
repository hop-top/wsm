# Integrator

## Description

A developer who vendors `wsm` as a Go module (`pkg/`) to build workspace capabilities into another tool. This persona interacts with `Manager`, `Backend`, and core types directly, and requires optional UI dependencies so library embedding remains lean.

## Information Discovery

- GoDoc on `pkg/` public API
- `CHANGELOG.md` (version-to-version diffs)
- `examples/` directory with runnable samples
- Interface contracts in `pkg/types.go`
- `go doc` CLI output
- Migration guide in `docs/upgrading.md`

## Preferred Delivery Format

- Go source with doc comments
- Example test files (`_test.go`)
- Minimal-dependency import paths
- Interface-first type signatures
- Semantic versioning with breaking-change notes

## Stories

- [016 - Library API Surface](../stories/016-library-api-surface.md)
- [017 - Custom Backend Implementation](../stories/017-custom-backend-implementation.md)
- [018 - Device Auto-Detection](../stories/018-device-auto-detection.md)
- [019 - Workspace Reference Resolution](../stories/019-workspace-reference-resolution.md)
- [024 - Vendored UI Optionality](../stories/024-vendored-ui-optionality.md)
- [029 - Format Adapter Interface](../stories/029-format-adapter-interface.md)
- [046 - Format Adapter Interface Review](../stories/046-format-adapter-interface-review.md)
