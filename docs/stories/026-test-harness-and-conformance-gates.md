# Test Harness and Conformance Gates

**ID**: 026
**Feature**: Testing
**Persona**: [Maintainer](../personas/maintainer.md)
**Priority**: P0

## Story

As a maintainer, I want mandatory CI gates mapped to stories so that regressions are blocked before merge.

## Acceptance Scenarios

1. **Given** a pull request, **When** CI runs, **Then** lifecycle, event integrity, backend conformance, and output-contract suites execute.
2. **Given** story coverage requirements, **When** test plans are reviewed, **Then** stories `001`-`024` are mapped to tests and enforced.
3. **Given** library dependency boundaries, **When** CI checks run, **Then** `pkg/*` remains UI-free.
4. **Given** migration safety, **When** conformance tests run, **Then** backend migration idempotency is validated.

## Tests

### Unit
- `pkg/backend/conformance_test.go` -- `TestBackendConformance`
- `pkg/workspace/manager_test.go` -- lifecycle and event integrity suites

### E2E
- `tests/e2e/*` -- command behavior and output contract suites
- `tests/e2e/gates_test.go` -- `TestStoryCoverageMatrix_001To026`
- `tests/e2e/gates_test.go` -- `TestCIIncludesConformanceGates`
