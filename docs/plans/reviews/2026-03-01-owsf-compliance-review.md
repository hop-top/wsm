# Review: OWSF Core Compliance Implementation Plan

**Plan:** `docs/plans/2026-03-01-owsf-compliance.md`
**Spec:** `docs/open-workspace-schema-framework.md`
**Checklist:** `docs/open-workspace-schema-criteria-checklist.md`
**Reviewed:** 2026-03-01

---

## Critical

### C1: `WriterRegistry.NextSeq` — nil pointer panic
**Ref:** Task 2 step 3; **Confidence:** 100

`w := r.writers[id]; w.Seq++` — nil deref on unregistered ID.
Fix: guard with existence check.

### C2: SQLite migration breaks existing databases
**Ref:** Task 3 step 4; Task 7 step 4; **Confidence:** 100

Plan modifies `CREATE TABLE IF NOT EXISTS events` DDL.
Existing DBs skip execution; new columns never added.
Fix: use `ALTER TABLE events ADD COLUMN IF NOT EXISTS`.

### C3: Immutability enforcement wiring missing
**Checklist:** #46-47; **Ref:** Task 6; **Confidence:** 95

`ValidateMutation` detects but never emits
`topology.space_destroy` + `topology.space_create` events.
Items 46-47 remain unchecked after Task 19.

### C4: Causal dependencies field absent
**Checklist:** #72; **Ref:** Task 3 + Task 8; **Confidence:** 90

No `CausalDeps` field in `model.Event`; no toposort in
`CanonicalOrder`. Ordering rule 1 untestable.

---

## Important

### I1: `manager.go` snapshot switch not updated
**Existing:** `pkg/workspace/manager.go#342-369`
**Ref:** Task 9 step 4; **Confidence:** 95

Parallel switch in manager not mentioned; new event types
won't drive incremental snapshot updates.

### I2: `Export.Validate()` exempts full exports from s9
**Checklist:** #132-133; **Ref:** Task 12 step 3; **Confidence:** 90

Spec says BOTH full+partial must declare determinism +
max_conformance. Plan gates checks on `ExportPartial` only.

### I3: `Space.Capabilities` not checked in ValidateMutation
**Checklist:** #45; **Ref:** Task 6 step 3; **Confidence:** 88

OWSF s2.3: "initial capability ceiling" immutable.
Not compared in `ValidateMutation`.

### I4: `mutation.artifact` — no payload struct
**Checklist:** #83; **Ref:** Task 9 step 3; **Confidence:** 85

Constant added, no `ArtifactData` struct. All other event
types have typed payloads.

### I5: Profile 1 signing deferred; checklist falsely [x]
**Checklist:** #144; **Ref:** Task 15; **Confidence:** 85

Actual signing deferred; "Sign manifest" != "declare type."
Either implement ed25519 stub or leave items unchecked.

### I6: DB type-string mismatch — old events invisible
**Ref:** Task 9; **Confidence:** 82

Existing rows: `"message.added"`, `"tool_call.started"`.
New constants: `"interaction.message"`, etc.
Queries with new constants return 0 for old data.

---

## Minor

### G1: `AncestorChain` O(n^2) prepend
Forward-append + reverse preferred.

### G2: `SpaceTree` no cycle detection
Two-node cycle causes infinite loop in `AncestorChain`.

### G3: s11 items 158-160 — no runtime assertion
`CombineAsNewDocument` correct by construction; no test
verifies combined ledger is empty.

---

## Summary

| Severity | Count |
|---|---|
| Critical | 4 |
| Important | 6 |
| Minor | 3 |

Priority: C2 > C1 > C3 > I1 > I6 > rest.
