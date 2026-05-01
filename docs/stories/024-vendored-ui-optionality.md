# Vendored UI Optionality

> **Deprecated 2026-04-30**: charmui retired. The dual-build path
> (`-tags charmui` + `go.charm.mod`) was removed in T-0086 / T-0094 /
> T-0095 once bubbletea v1 -> v2 migration landed and kit became the
> single CLI substrate. Acceptance scenario 1 (no `charmbracelet`
> deps in `pkg/*`) is still enforced by `make verify-no-ui-leak`.
> Scenarios 2-3 no longer apply: there is one build path now.

**ID**: 024
**Feature**: Go Module
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P0 (historical)

## Story

As an integrator, I want optional UI dependencies so that vendoring `pkg/*` does not pull in Charmbracelet unless explicitly requested.

## Acceptance Scenarios

1. **Given** the core module, **When** I run `go list -deps ./pkg/...`, **Then** no `charmbracelet` packages appear.
2. ~~**Given** charm UI support, **When** I build with `-tags charmui` and `-modfile=go.charm.mod`, **Then** TUI binaries compile with Charmbracelet.~~ (removed; single build path)
3. ~~**Given** default builds without `charmui`, **When** I build CLI/library, **Then** no charm dependencies are required.~~ (subsumed by #1)
4. **Given** the public API for `pkg/workspace` and `pkg/backend`, **When** I inspect types, **Then** no presentation/UI concerns leak into library contracts.

## Tests

### Unit
- `pkg/backend/backend_test.go` -- `TestNoUIPresentationLeak`

### E2E
- `tests/e2e/module_test.go` -- `TestVendoredUIOptionality`

### CI
- `go list -deps ./pkg/...` does not include `charmbracelet`
