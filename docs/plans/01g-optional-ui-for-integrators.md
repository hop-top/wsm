# Plan 01g: Optional UI for Integrators

**Goal**: Keep Charmbracelet optional and prevent UI dependencies from leaking into vendored core packages.

## Dependency Strategy

1. Single module: `hop.top/wsm`.
2. Charm path behind build tag: `charmui`.
3. Alternate modfile for charm builds: `go.charm.mod`.
4. Core modfile (`go.mod`) remains lean for integrator vendoring.

## Build Contracts

- Core build: `go build ./...`
- Charm build: `go build -tags charmui -modfile=go.charm.mod ./cmd/wsm`

## Boundary Rules

1. `pkg/*` must never import Charmbracelet packages.
2. Charm imports are restricted to CLI/TUI packages (for example `cmd/wsm`, `internal/ui/charm`).
3. CI enforces no charm dependencies in `pkg/*` via dependency check.

## Acceptance Criteria

1. Story `024` passes with explicit CI verification.
2. Integrator API surface remains UI-free.
