# Workspace Reference Resolution

**ID**: 019
**Feature**: CLI Core / Go Module
**Persona**: [User](../personas/user.md)
**Related Personas**: [Integrator](../personas/integrator.md)
**Priority**: P0

## Story

As a user and integrator, I want deterministic workspace reference resolution so that `id-or-name` commands behave consistently and safely.

## Acceptance Scenarios

1. **Given** a workspace ID and workspace name both exist, **When** I pass an ID-like token, **Then** resolution behavior is deterministic and documented (ID precedence over name).
2. **Given** an existing workspace name `my-project`, **When** I run `wsm create my-project`, **Then** it fails with exit code 4 (conflict) because names are unique.
3. **Given** `inspect`, `archive`, `unarchive`, and `delete` commands, **When** I pass the same reference token, **Then** all commands resolve the same workspace.
4. **Given** no workspace matches a reference, **When** I run a ref-based command, **Then** it fails with exit code 3 and suggests `wsm list`.
5. **Given** library consumers, **When** they call `ResolveWorkspaceRef(ctx, ref)`, **Then** they receive a typed workspace result without shelling out.

## Tests

### Unit
- `pkg/workspace/manager_test.go` -- `TestManager_ResolveRef`
- `pkg/workspace/manager_test.go` -- `TestManager_Create_UniqueNameConflict`

### E2E
- `tests/e2e/workspace_test.go` -- `TestRefResolution_ByNameAndID`
