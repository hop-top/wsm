# Format Adapter Conformance Tests

**ID**: 045
**Feature**: IDE Session Import
**Persona**: [Maintainer](../personas/maintainer.md)
**Priority**: P0

## Story

As a maintainer, I want conformance tests for all format adapters so that every adapter meets the same contract and quality bar.

## Design

A shared test suite that every registered adapter must pass. The suite validates: detection accuracy, parse output structure, error handling for malformed input, metadata correctness, and idempotent re-parse.

Real fixture files from each IDE live in `tests/fixtures/formats/`. Each adapter directory contains at least one happy-path fixture and one malformed fixture. Adding a new adapter to the registry automatically includes it in the conformance run -- no manual test registration required.

A CI gate in the Makefile ensures conformance failures block merge.

## Acceptance Scenarios

1. **Given** all format adapters are registered, **When** the conformance suite runs, **Then** every adapter passes detection accuracy, parse structure, error handling, metadata, and idempotency checks.
2. **Given** a new adapter is registered via `format.Register()`, **When** the conformance suite runs, **Then** the new adapter is automatically included without manual test registration.
3. **Given** fixture files in `tests/fixtures/formats/`, **When** the conformance suite runs, **Then** each adapter's fixtures cover at least one happy-path and one malformed-input case.
4. **Given** the conformance suite is wired into CI via `make test-conformance`, **When** any adapter fails a conformance check, **Then** the CI pipeline blocks the merge.

## Tests

### Unit
- `pkg/format/conformance_test.go` -- `TestConformance_DetectionAccuracy`
- `pkg/format/conformance_test.go` -- `TestConformance_ParseOutputStructure`
- `pkg/format/conformance_test.go` -- `TestConformance_ErrorHandlingMalformed`
- `pkg/format/conformance_test.go` -- `TestConformance_MetadataCorrectness`
- `pkg/format/conformance_test.go` -- `TestConformance_IdempotentReparse`
- `pkg/format/conformance_test.go` -- `TestConformance_AllRegisteredAdapters`

### CI
- `Makefile` -- `test-conformance` target
