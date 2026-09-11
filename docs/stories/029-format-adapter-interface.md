# Format Adapter Interface

**ID**: 029
**Feature**: IDE Session Import
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P0

## Story

As an integrator, I want a format adapter interface so that I can implement adapters for any IDE session format.

## Design

The interface lives in `pkg/format/` and is designed from wsm's existing event model (messages, tool calls, file changes). It will evolve as adapters are built -- this is the initial design based on wsm's own model.

### Adapter Interface

- `Detect(path string) (supported bool, confidence float64, err error)` -- returns whether the adapter can handle the file, with a confidence score (0.0-1.0) for disambiguation when multiple adapters match.
- `Parse(path string) ([]NormalizedEntry, error)` -- reads the file and returns normalized entries mapped to wsm's event model.
- `Metadata() AdapterInfo` -- returns adapter info including name, file patterns (e.g., `*.jsonl`), and human-readable description.

### Registry

Adapters register themselves via `format.Register(adapter)`. The registry handles detection by querying all registered adapters and selecting the highest-confidence match.

## Acceptance Scenarios

1. **Given** a new adapter implementing the `Adapter` interface, **When** I call `format.Register(adapter)`, **Then** the adapter is available for detection and parsing.
2. **Given** a file matching one registered adapter, **When** I call `format.Detect(path)`, **Then** the matching adapter is returned with its confidence score.
3. **Given** a file matching multiple adapters, **When** I call `format.Detect(path)`, **Then** the adapter with the highest confidence is returned.
4. **Given** a valid adapter, **When** I call `adapter.Parse(path)` on a supported file, **Then** normalized entries are returned with messages, tool calls, and file changes mapped to wsm's event model.
5. **Given** a valid adapter, **When** I call `adapter.Metadata()`, **Then** the adapter info includes name, file patterns, and description.
6. **Given** a malformed input file, **When** I call `adapter.Parse(path)`, **Then** a structured error is returned describing the parse failure.
7. **Given** no registered adapters match a file, **When** I call `format.Detect(path)`, **Then** an `ErrUnsupportedFormat` error is returned.

## Tests

### Unit
- `pkg/format/adapter_test.go` -- `TestAdapterInterface_Detect`
- `pkg/format/adapter_test.go` -- `TestAdapterInterface_Parse`
- `pkg/format/adapter_test.go` -- `TestAdapterInterface_Metadata`
- `pkg/format/adapter_test.go` -- `TestAdapterInterface_ParseMalformed`
- `pkg/format/registry_test.go` -- `TestRegistry_Register`
- `pkg/format/registry_test.go` -- `TestRegistry_Detect_SingleMatch`
- `pkg/format/registry_test.go` -- `TestRegistry_Detect_MultipleMatch`
- `pkg/format/registry_test.go` -- `TestRegistry_Detect_NoMatch`

### E2E
None. This story is not implemented yet -- neither the unit tests listed
above nor any end-to-end test exist. Coverage must be added alongside the
implementation.
