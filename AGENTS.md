# AGENTS.md

Repository-specific guidance for future coding sessions in `hop.top/wsm`.

## Project Goals

1. Build `wsm` as a standalone CLI + TUI for workspace/session management.
2. Keep `pkg/*` as stable, vendorable Go APIs for integrators.
3. Enforce story-driven delivery via `docs/stories/` and `docs/plans/`.

## Hard Boundaries

1. `pkg/*` must not import UI/presentation libraries.
2. Charmbracelet packages (`bubbletea`, `bubbles`, `lipgloss`) are optional and isolated.
3. Optional UI path is enabled with `-tags charmui` and `-modfile=go.charm.mod`.
4. CLI/TUI should consume manager/backend APIs; no parallel business logic paths.

## Story and Plan Workflow

1. Treat `docs/stories/README.md` as source of truth for requirements.
2. Keep decomposition docs in `docs/plans/01a-*.md` through `01h-*.md` in sync with implementation.
3. When behavior changes, update the matching story acceptance criteria and plan references.

## Testing and Gates

1. Maintain lifecycle, event integrity, backend conformance, and output contract tests.
2. Ensure CI enforces `pkg/*` dependency boundary (`verify-no-ui-leak`).
3. Prefer deterministic tests and explicit exit-code assertions for CLI behavior.
4. Run tests frequently.

   - `make test` (Unit/Integration)
   - `make test-e2e` (Standard E2E)
   - `go test ./tests/e2e -v -run TestInteractiveCompletion` (Interactive Completion)
   - `make test-ubuntu-docker` (Full CI Suite in Linux)

## Coding Expectations

1. Keep APIs typed and explicit; avoid presentation concerns in core types.
2. Preserve stable stdout/stderr contracts for automation use.
3. Favor backward-compatible changes in `pkg/*`; document breaking changes in `CHANGELOG.md`.

## Required Docs to Keep Updated

- `README.md`
- `CHANGELOG.md`
- `docs/stories/README.md`
- `docs/personas/*.md`
- `docs/plans/*.md`
