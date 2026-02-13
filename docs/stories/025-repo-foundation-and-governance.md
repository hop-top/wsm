# Repository Foundation and Governance

**ID**: 025
**Feature**: Project Operations
**Persona**: [Maintainer](../personas/maintainer.md)
**Priority**: P1

## Story

As a maintainer, I want standard repository foundation documents so that contributors can onboard quickly and operate safely.

## Acceptance Scenarios

1. **Given** repository root, **When** I inspect it, **Then** `README.md`, `AGENTS.md`, and `LICENSE` (MIT) exist and are coherent.
2. **Given** contributor workflows, **When** I check docs, **Then** `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, and `CHANGELOG.md` exist.
3. **Given** GitHub collaboration, **When** I open templates, **Then** issue and PR templates are present.
4. **Given** daily development tasks, **When** I run `make help`, **Then** basic build/test/verification targets are documented.

## Tests

### E2E
- `tests/e2e/governance_test.go` -- `TestRepositoryFoundation`
- `tests/e2e/governance_test.go` -- `TestMakeHelpTargets`

### CI
- Docs lint / link checks pass for core governance files.
