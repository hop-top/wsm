# OWSF Schema Criteria Checklist

Use this checklist to track implementation/compliance against `docs/open-workspace-schema-framework.md`.

Legend:
- `[ ]` = not yet satisfied
- `[x]` = satisfied
- `(Conditional)` = required only when that feature/profile/export mode is used

Assessment scope (current repo):
- Checked items are verified against the existing workspace/event implementation.
- OWSF document/topology/fork/handoff/export fields remain unchecked unless explicitly implemented.

## 1. Document Identity (Core)

- [x] Include `owsf_version`.
- [x] Include globally unique `doc_id`.
- [x] Include `created_at`.
- [x] Include `producer` (name + version).
- [x] If `fork` is present, ensure single parent semantics.
- [x] If `writers` is present, represent multi-writer support consistently.

## 2. Spaces

### 2.1 Space Fields
- [x] Every space defines `id`.
- [x] Every space defines `type`.
- [x] Every space defines `parent` (`null` for root).
- [x] Every space defines `boundary`.
- [x] Every space defines `capabilities` (ceiling).
- [x] Every space defines `trust_level`.

### 2.2 Containment Model
- [x] Enforce strict tree containment.
- [x] Ensure each space has exactly one parent.
- [x] Disallow multi-parent containment (no DAG).
- [x] Keep connectivity-between-spaces representation separate from containment when used.
- [x] Prevent parent-child relationship changes after creation.

### 2.3 Immutability Rules
- [x] Keep `space.id` immutable.
- [x] Keep `space.parent` immutable.
- [x] Keep `space.type` immutable.
- [x] Keep `space.boundary` immutable.
- [x] Keep initial capability ceiling immutable.
- [x] If immutable fields change, mark original space as destroyed.
- [x] If immutable fields change, create a new space via event.

## 3. Event Ledger

### 3.1 Event Invariants
- [x] Every event includes unique `event_id` (within document).
- [x] Every event includes `writer_id`.
- [x] Every event includes `space_id`.
- [x] Every event includes `type`.
- [x] Every event includes ordering metadata.
- [x] Every event includes payload.
- [x] Ledger is append-only.
- [x] Ledger is immutable.
- [x] Ledger is never rewritten.
- [x] Ledger is never merged.

## 4. Multi-Writer Model (Collaboration Profile)

### 4.1 Writers
- [x] Maintain `writers[]` registry.
- [x] Ensure each event carries `writer_id`.
- [x] Ensure each writer uses monotonic `seq`.

### 4.2 Ordering
- [x] Ensure events are deterministically linearizable.
- [x] Canonical ordering honors causal dependencies (if present).
- [x] Canonical ordering sorts by Lamport clock.
- [x] Canonical ordering tie-breaks by `writer_id`.
- [x] Canonical ordering tie-breaks by `seq`.
- [x] Canonical ordering tie-breaks by `event_id`.
- [x] Ensure consumers derive identical ordering from identical input.

## 5. Event Types (Core Minimal Set)

- [x] Support `interaction.message`.
- [x] Support `interaction.tool_call`.
- [x] Support `mutation.file_patch`.
- [x] Support `mutation.artifact`.
- [x] Support `topology.space_create`.
- [x] Support `topology.space_destroy`.
- [x] Support `checkpoint.snapshot`.
- [x] If extending types, do so as profile extensions without breaking core set.

## 6. Fork Model

### 6.1 Fork Rules
- [x] (Conditional) If forked, declare `fork.parent_doc_id`.
- [x] (Conditional) If forked, declare `fork.parent_event_id`.
- [x] Enforce at most one parent per document.
- [x] Disallow multiple parents in declarations.
- [x] Disallow merge semantics.

## 7. Handoff Profile

### 7.1 Handoff Events
- [x] Support `control.handoff.offer`.
- [x] Support `control.handoff.accept`.
- [x] Support `control.handoff.decline`.

### 7.2 Default Behavior
- [x] Treat handoff as non-terminal by default.
- [x] (Conditional) If `intent = terminal`, producer stops appending further events (`SHOULD`).

### 7.3 Fork Linkage
- [x] (Conditional) Receiving tool creates a new document for handoff continuation.
- [x] (Conditional) Set `fork.kind = "handoff"` in receiving document.
- [x] (Conditional) Include reference to the handoff offer event.

## 8. Partial Export (Slice Profile)

### 8.1 Slice Declaration
- [x] Declare `export.kind = "full" | "partial"`.
- [x] (Conditional: partial) Declare slice scope.
- [x] (Conditional: partial) Declare determinism guarantees.
- [x] (Conditional: partial) Declare `max_conformance`.
- [x] (Conditional: partial) Declare missing dependencies.

### 8.2 Closure Rules
- [x] (Conditional: exporting events for a space) Include that space.
- [x] (Conditional: exporting events for a space) Include full ancestor chain.
- [x] Explicitly declare missing artifacts or dependencies.
- [x] Importers do not assume missing context.

## 9. Determinism Guarantees

- [x] Declare `determinism = full | best_effort | none` for partial or full export.
- [x] Declare `max_conformance = 1 | 2 | 3` for partial or full export.
- [x] Importers downgrade conformance when necessary.
- [x] Importers surface degradation in import report.

## 10. Assurance Profiles

### Profile 0 - Integrity Only
- [x] (Conditional: claiming Profile 0+) Use content-addressed blobs.
- [x] (Conditional: claiming Profile 0+) Provide manifest hash.

### Profile 1 - Signed Artifact
- [ ] (Conditional: claiming Profile 1+) Sign manifest.
- [x] (Conditional: claiming Profile 1+) Include producer identity.

### Profile 2 - Provenance-Grade
- [x] (Conditional: claiming Profile 2) Provide verifiable fork lineage.
- [x] (Conditional: claiming Profile 2) Provide attestations.
- [x] (Conditional: claiming Profile 2) Support transparency log integration.

## 11. No-Merge Invariant

- [x] Prohibit multi-parent lineage.
- [x] Prohibit event merging.
- [x] Prohibit CRDT convergence in artifact.
- [x] Prohibit rewriting history.
- [x] If combination is needed, create a new document.
- [x] If combination is needed, reference artifacts as inputs.
- [x] If combination is needed, do not merge histories.

## 12. Relationship to OASF

- [x] (Conditional) If transported via OASF, keep OWSF artifact independently verifiable.

## 13. Framework Interoperability (v0.1 Updates)

### 13.1 Agent Identity (OASF Alignment)
- [x] Link `writer_id` to OASF Agent Record URIs.
- [x] Support `agent_id` resolution via OASF Agent Provider.

### 13.2 Capability Mapping (OASF Alignment)
- [x] Map `space.capabilities` to OASF capability taxonomies.
- [x] Distinguish between requested, granted, and active capabilities.

### 13.3 Event Semantics (OCSF Alignment)
- [x] Align `interaction.message` with OCSF Chat Message event class.
- [x] Align `interaction.tool_call` with OCSF API Call event class.
- [x] Align `mutation.file_patch` with OCSF File System Activity event class.
- [x] Ensure payload extensions follow OCSF attribute dictionary.

### 13.4 Observability (OCSF Alignment)
- [x] Map `space.boundary` to OCSF Observable types (File Path, Process ID, Container ID).
- [x] Support multiple observables per space boundary.

### 13.5 Causal Integrity
- [x] Support explicit `causal_deps` in events.
- [x] Ensure `CanonicalOrder` respects causal happens-before relationships.
