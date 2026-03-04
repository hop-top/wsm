# Interaction Modes Reference

## Overview

Every squad pair that communicates must have a declared interaction mode.
The mode defines: direction, duration, who owns the output, and what the
failure mode looks like.

The target steady state for all squad pairs is **X-as-a-Service**.
Collaboration and Facilitating are transitional modes that should graduate
to X-as-a-Service over time.

---

## X-as-a-Service

**Direction:** One-way (provider → consumer)
**Duration:** Indefinite (stable relationship)
**Coordination required:** None
**Output owner:** Provider

**When to use:**
- One squad reliably produces something the other consumes
- The interface is stable and well-understood
- The consumer doesn't need to understand how the provider works

**Contract requirements:**
- Input/output schema is versioned and documented
- SLA is defined (latency, availability)
- Breaking changes require a deprecation window
- Consumer never needs to understand the provider's implementation

**Plain language:** "Squad A sends Squad B a report every time it finishes.
Squad B doesn't need to know how Squad A made the report."

**Failure mode:** Provider keeps changing the interface. Consumer works around
the interface. Either signals the contract is wrong, not the mode.

**Graduation path:** This IS the target state. No graduation needed.

---

## Collaboration

**Direction:** Bidirectional
**Duration:** Time-boxed (must have a defined end date/condition)
**Coordination required:** High
**Output owner:** Must be assigned to one squad at the end

**When to use:**
- Two squads are working on an unknown or novel problem together
- Neither squad can complete the work without the other's active involvement
- Short-lived integration work or design discovery

**Contract requirements:**
- Time-boxed before it begins ("2 weeks to validate the integration boundary")
- Exit criteria defined: what does success look like?
- One squad owns the final artifact — no permanent co-ownership

**Plain language:** "Two departments are sitting together in the same room for
two weeks to figure something out. After that, they go back to their own spaces."

**Failure mode:** Collaboration persists indefinitely. Both squads become
context-entangled. Neither can operate independently. This is captured dependency.

**Graduation path:** → X-as-a-Service (the most common target)
or → Merge the two squads if the collaboration reveals they should be one.

---

## Facilitating

**Direction:** One-way (enabling squad → stream squad)
**Duration:** Time-boxed (defined exit condition required)
**Coordination required:** Medium (regular check-ins until exit)
**Output owner:** Stream squad (the capability stays with them after exit)

**When to use:**
- An enabling squad is building a capability into a stream squad
- The stream squad has a gap that prevents it from operating independently
- There's a clear definition of "self-sufficient"

**Contract requirements:**
- Capability definition: what exactly is being embedded?
- Exit condition: what does "self-sufficient" look like?
- Handoff artifact: documentation, tooling, or embedded prompt that persists

**Plain language:** "A trainer is working with a department to teach them
a new skill. Once they've got it, the trainer moves on."

**Failure mode:** Stream squad remains dependent on enabling squad after the
engagement. Either the capability wasn't truly embedded, or the exit condition
was never enforced.

**Graduation path:** → Enabling squad exits. Stream squad now self-sufficient.
Interaction mode between them: none (or X-as-a-Service for occasional support).

---

## Supervised Delegation

**Direction:** One-way (orchestrator → squad), with oversight return channel
**Duration:** Per-task (orchestrator stays in loop until task complete)
**Coordination required:** Low (handoff + status + result)
**Output owner:** Orchestrator synthesizes; squad produces the work

**When to use:**
- An orchestrator squad is assigning a task to a stream squad
- The orchestrator needs to retain the ability to interrupt, redirect, or reclaim
- Multi-squad workflows where the orchestrator assembles the final result

**Contract requirements:**
- Task definition: clear scope, inputs, expected outputs
- Status protocol: how does the squad report progress back?
- Reclaim condition: under what circumstances does the orchestrator take the task back?

**Plain language:** "The project manager assigns work to a team, but stays
close enough to redirect if something goes sideways. The team doesn't just
disappear with the task."

**Failure mode:** Blind delegation — orchestrator hands off and loses visibility.
Or: micro-management — orchestrator is so involved it creates a bottleneck.

**Graduation path:** For well-understood, stable tasks → X-as-a-Service
(orchestrator becomes a consumer of the squad's output, not an active delegator).

---

## Choosing the Right Mode

Use this decision tree:

```
Is this a new, uncertain, exploratory situation?
  YES → Collaboration (time-box it)
  NO ↓

Is one squad teaching another squad a capability?
  YES → Facilitating (define exit condition)
  NO ↓

Is an orchestrator assigning work and staying in the loop?
  YES → Supervised Delegation
  NO ↓

Is one squad reliably producing something the other consumes?
  YES → X-as-a-Service ← TARGET STATE
```
