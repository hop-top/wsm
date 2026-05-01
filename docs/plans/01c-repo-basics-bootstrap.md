# Plan 01c: Repo Basics Bootstrap

**Goal**: Establish repository foundations for `hop.top/wsm` with governance and contributor docs.

## Required Files

- `README.md`
- `AGENTS.md`
- `LICENSE` (MIT)
- `.gitignore`
- `CONTRIBUTING.md`
- `SECURITY.md`
- `CODE_OF_CONDUCT.md`
- `CHANGELOG.md`
- `.github/ISSUE_TEMPLATE/bug_report.md`
- `.github/ISSUE_TEMPLATE/feature_request.md`
- `.github/pull_request_template.md`
- `Makefile`
- `go.mod` (`module hop.top/wsm`, `go 1.23`)

## Content Requirements

1. README names the project as **Workspace & Session Manager** and links homepage `https://hop.top/wsm`.
2. AGENTS documents boundaries: core `pkg/*` must remain UI-agnostic; presentation lives only in `cmd/`.
3. Governance docs are coherent and cross-linked.
4. Makefile includes core build/test/verification targets.

## Acceptance Criteria

1. New contributors can build/test from README without tribal knowledge.
2. Integrators can vendor `pkg/*` without pulling UI dependencies.
