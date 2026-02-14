# Format Adapter Interface Review

**ID**: 046
**Feature**: IDE Session Import
**Persona**: [Integrator](../personas/integrator.md)
**Priority**: P1

## Story

As an integrator, I want the format adapter interface to be reviewed after all adapters are built so that the final API is coherent and complete.

## Design

This is a review story -- triggered after all adapter stories (030-043) are complete. The review evaluates whether the interface that emerged from building all adapters is minimal, consistent, and covers every adapter's needs without workarounds.

Any interface changes produced by this review are validated by the existing conformance tests (045).

## Acceptance Scenarios

1. **Given** all adapter stories (030-043) are complete, **When** the interface is reviewed, **Then** all adapter-specific fields are represented in the interface or in structured metadata -- no adapter resorts to untyped escape hatches.
2. **Given** the review is performed, **When** each adapter's implementation is inspected, **Then** no adapter uses workarounds (type assertions, raw maps, interface casts) for data that should be a first-class field.
3. **Given** the review is performed, **When** the interface methods are evaluated, **Then** the method set is minimal (no redundant methods) and consistent (uniform naming, parameter order, return types).
4. **Given** the review produces interface changes, **When** the changes are applied, **Then** GoDoc and inline documentation reflect the final design.

## Tests

No new test files -- this story produces interface changes tested by the existing conformance suite ([045](045-format-adapter-conformance-tests.md)).
