# Plan 01b: Core Contract Gap Closure

**Goal**: Close the contract and consistency gaps identified during review before implementation proceeds.

## Decisions Locked

1. **Reference resolution**
- `id-or-name` is resolved deterministically by `ResolveWorkspaceRef(ctx, ref)`.
- Unique workspace names are enforced.
- ID precedence is explicit where input can be ambiguous.

2. **Delete cascade semantics**
- Workspace deletion is atomic and removes workspace metadata, events, and snapshots in one transaction.
- Backend contract and tests enforce cascade behavior.

3. **Snapshot edge cases**
- Empty workspace snapshots are valid with zero counts.
- Incremental snapshot replay starts from the sequence after latest cached snapshot.

4. **JSON contract and streams**
- `--json` is supported on all user-facing data commands, including `delete` and `delete --dry-run`.
- JSON data goes to stdout only; errors/progress go to stderr only.

5. **Global config flags**
- Root-level persistent flags: `--backend`, `--data-dir`, `--backend-url`, `--no-color`.
- Precedence: flags > env > config file > defaults.

6. **Data format consistency**
- Plan 1 canonical persistence is SQLite rows (not JSONL files).

7. **Device identity contract**
- `device.Current()` yields stable same-machine ID and distinct cross-machine IDs based on deterministic machine identity inputs.

## API/Schema Updates

- `pkg/backend.Backend`: add `GetWorkspaceByName(ctx, name string)`.
- `pkg/workspace.Manager`: add `ResolveWorkspaceRef(ctx, ref string)`.
- SQLite schema: enforce unique workspace names and delete cascade.

## Acceptance Criteria

1. Contracts are explicit enough for independent implementation.
2. Story requirements `019`, `005`, `007`, `013`, `015`, `018` are fully represented.
3. No conflicting statements remain between architecture and storage sections.
