# Parallel Implementation Plan: 3 Teams with Worktrees

**Goal**: Execute plans `01a-01h` in parallel with minimal merge conflicts.

## Branch + Worktree Setup

Create three long-lived implementation branches from `main`:

```bash
git hop add team-a-foundation
git hop add team-b-core-cli
git hop add team-c-tui-optional
```

## Team Ownership (Hard File Boundaries)

### Team A — Foundation, Contracts, and CI Gates

**Primary scope**: `01a`, `01b`, `01c`, `01h` (contract/docs/CI side)

**Owns**:
- `docs/stories/**`
- `docs/personas/**`
- `docs/plans/01a-*.md`, `docs/plans/01b-*.md`, `docs/plans/01c-*.md`, `docs/plans/01h-*.md`
- `README.md`, `AGENTS.md`, `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md`, `LICENSE`
- `.github/**`
- `Makefile`
- `go.mod` (core only; no charm deps)

**Must not edit**:
- `cmd/wsm/**` (except global flag docs)
- `internal/ui/charm/**`
- `pkg/**` implementation files

### Team B — Core Library + CLI (Non-TUI)

**Primary scope**: `01d`, `01e` (core and output/completion)

**Owns**:
- `pkg/workspace/**`
- `pkg/backend/**`
- `pkg/backend/sqlite/**`
- `pkg/config/**`
- `pkg/device/**`
- `cmd/wsm/main.go`
- `cmd/wsm/commands_*.go` (core commands)
- `tests/e2e/workspace_test.go`, `tests/e2e/event_test.go`, `tests/e2e/output_test.go`, `tests/e2e/exit_code_test.go`, `tests/e2e/config_test.go`, `tests/e2e/completion_test.go`

**Must not edit**:
- `internal/ui/charm/**`
- `go.charm.mod`
- `.github/workflows/**` (except via Team A approved changes)

### Team C — TUI + Optional Dependency Path

**Primary scope**: `01f`, `01g`

**Owns**:
- `internal/ui/charm/**`
- `cmd/wsm/tui*.go`
- `go.charm.mod`
- `pkg/tui/**` (if created)
- `tests/e2e/tui_test.go`

**May edit with Team B approval**:
- `cmd/wsm/main.go` only for wiring `tui` subcommand

**Must not edit**:
- `pkg/workspace/**`, `pkg/backend/**`, `pkg/config/**`, `pkg/device/**`
- `go.mod` (core dependency set)

## Integration Contracts (Frozen Early)

These interfaces are frozen before feature completion:

1. `backend.Backend` includes explicit name lookup for workspace references.
2. `workspace.Manager` includes `ResolveWorkspaceRef` and typed delete preview.
3. Global CLI flags include `--no-color` for disabling styled output.
4. `pkg/*` has no Charmbracelet imports.

If a contract change is required after freeze, open a small RFC PR tagged `contract-change` and merge it first.

## Merge Choreography (Low-Conflict)

1. Team A opens and merges **Contract Freeze PR** first (docs + interfaces + CI baseline).
2. Teams B and C branch from the freeze commit and work in parallel.
3. Team B merges first for core APIs and CLI plumbing.
4. Team C rebases onto latest `main`, resolves only `cmd/wsm` wiring, then merges.
5. Team A finalizes `01h` gate enforcement and story traceability updates.

## PR Slicing Rules

Each team uses small PRs (1-2 day slices):

- Team A: docs/contracts -> repo basics -> CI gates
- Team B: backend/schema -> manager APIs -> CLI output/completion -> tests
- Team C: charm model/navigation -> workspace actions -> optional build path -> TUI tests

## Required Checks per PR

1. `make test`
2. `make verify-no-ui-leak`
3. Story IDs covered listed in PR template
4. Any changed behavior reflected in matching story/plan docs

## Conflict Prevention Checklist

1. Never edit another team's owned path unless explicitly coordinated.
2. Keep `cmd/wsm/main.go` changes additive and isolated; use separate files for command wiring.
3. Prefer new files over modifying shared files where possible.
4. Rebase daily to reduce drift.

## Worktree Daily Routine

In each worktree:

```bash
git fetch origin
git rebase origin/main
make test
make verify-no-ui-leak
```

## Definition of Done

1. Stories `001-026` are implemented and mapped to tests.
2. TUI is available in v1 via charm build path.
3. Integrators can consume `pkg/*` without charm dependencies.
4. CI enforces lifecycle, integrity, conformance, output, and dependency-boundary gates.
