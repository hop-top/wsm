# Custom Backend Implementation

**ID**: 017
**Feature**: Go Module
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P1

## Story

As an integrator, I want to implement the Backend interface for my own storage so that I can use wsm with my existing infrastructure.

## Acceptance Scenarios

1. **Given** the `Backend` interface in `pkg/backend`, **When** I implement all methods, **Then** my backend works with Manager without modification.
2. **Given** a custom backend, **When** I run the conformance test suite against it, **Then** I can verify correctness.
3. **Given** the Backend interface, **When** I review it, **Then** it has no SQLite-specific types or behaviors leaked into the contract.
4. **Given** `ListOptions` and `EventListOptions`, **When** I implement filtering and sorting, **Then** the option types are self-documenting with no hidden contracts.

## Tests

### Unit
- `pkg/backend/conformance_test.go` -- `TestBackendConformance`
- `pkg/backend/backend_test.go` -- `TestBackendInterface_NoLeakedTypes`

### E2E
- `tests/e2e/module_test.go` -- `TestBackendConformanceGate`
