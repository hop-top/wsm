# Shell Completion

**ID**: 009
**Feature**: CLI Core
**Persona**: [User](../personas/user.md)
**Priority**: P1

## Story

As a user, I want shell completion for wsm commands so that I can tab-complete workspace IDs and flag values.

## Acceptance Scenarios

1. **Given** bash shell, **When** I run `wsm completion bash`, **Then** a bash completion script is printed to stdout.
2. **Given** zsh shell, **When** I run `wsm completion zsh`, **Then** a zsh completion script is printed to stdout.
3. **Given** workspaces `alpha` and `beta` exist, **When** I tab-complete `wsm inspect `, **Then** both workspace names are suggested.
4. **Given** I type `wsm list --status=`, **When** I trigger completion, **Then** `active` and `archived` are suggested.

## Tests

### Unit
- `pkg/cli/completion_test.go` -- `TestCompletion_WorkspaceNames`

### E2E
- `tests/e2e/completion_test.go` -- `TestCompletionBash`
- `tests/e2e/completion_test.go` -- `TestCompletionZsh`
