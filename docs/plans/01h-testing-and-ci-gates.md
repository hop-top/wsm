# Plan 01h: Testing and CI Gates

**Goal**: Enforce story-mapped quality gates in CI for lifecycle, integrity, conformance, output modes, and dependency boundaries.

## Required Suites

1. Unit tests for manager, backend contract, snapshot logic, device detection.
2. E2E CLI tests for lifecycle, output modes, exit codes, and completion.
3. E2E TUI tests for launch and workspace operations.
4. Backend conformance suite against SQLite implementation.

## CI Gates

1. `go test ./...` baseline.
2. Story-critical suites required on every pull request.
3. Dependency boundary check: `go list -deps ./pkg/...` must not contain `charmbracelet`.
4. Docs checks for story index and governance docs.

## Coverage Contract

- Stories `001-024` and `027` must map to automated checks.
- Story `026` is complete when CI enforces this mapping as merge gate.

## Acceptance Criteria

1. A failing lifecycle/integrity/conformance/output/dependency test blocks merge.
2. CI output clearly identifies which gate failed and why.
