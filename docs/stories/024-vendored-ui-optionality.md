# Vendored UI Optionality

**ID**: 024
**Feature**: Go Module
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P0

## Story

As an integrator, I want optional UI dependencies so that vendoring `pkg/*` does not pull in Charmbracelet unless explicitly requested.

## Acceptance Scenarios

1. **Given** the core module, **When** I run `go list -deps ./pkg/...`, **Then** no `charmbracelet` packages appear.
2. **Given** charm UI support, **When** I build with `-tags charmui` and `-modfile=go.charm.mod`, **Then** TUI binaries compile with Charmbracelet.
3. **Given** default builds without `charmui`, **When** I build CLI/library, **Then** no charm dependencies are required.
4. **Given** the public API for `pkg/workspace` and `pkg/backend`, **When** I inspect types, **Then** no presentation/UI concerns leak into library contracts.

## Tests

### Unit
- `pkg/backend/backend_test.go` -- `TestNoUIPresentationLeak`

### E2E
- `tests/e2e/module_test.go` -- `TestVendoredUIOptionality`

### CI
- `go list -deps ./pkg/...` does not include `charmbracelet`
