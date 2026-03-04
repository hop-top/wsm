# Squad Topologies — Framework Reference

## Core Premise

Squad Topologies applies the Team Topologies mental model to agent system design.
It answers: **how do you organize autonomous agents to maximize value flow and
minimize context overhead?**

The three foundational shifts from human Team Topologies:

| Dimension | Team Topologies | Squad Topologies |
|---|---|---|
| Primary unit | Human team | Agent squad |
| Primary constraint | Cognitive load | Context load (working memory) |
| Communication default | Ad-hoc | Declared interaction contracts |
| Shape evolution | Slow (org change) | Fast (measured, deliberate) |
| Conway's Law direction | Communication → architecture | Topology → agent comm architecture |

---

## Context Load: The Primary Design Constraint

**Context load** is the total information an agent must hold in working context
to complete a unit of work. It has three components:

- **Intrinsic**: Domain knowledge fundamental to the problem (billing rules,
  product taxonomy, legal corpus). Cannot and should not be removed.
- **Extraneous**: Coordination overhead — what other squads are doing, how to
  interpret their outputs, which mode is active. **Minimize this.**
- **Germane**: Capability-building knowledge — the enabling squad's domain.

**Key rule:** If a squad's context is dominated by extraneous load (coordination
overhead), the topology is wrong. Either the squad is too broad, or the interaction
mode is wrong.

---

## Conway's Law (Agent Version)

> "Agent systems are constrained to produce communication patterns which are
> copies of the squad topology that designs them."

**Inverse Conway Maneuver:** Design the squad topology first, then let the agent
communication architecture follow. Never let implementation convenience define
the topology retroactively.

---

## Foundational Laws

**Brook's Law**: Adding more agents to a squad doesn't immediately increase capacity.
Onboarding cost, context synchronization overhead, and interaction complexity
increase nonlinearly. Prefer splitting squads over growing them.

**Goodhart's Law** (agent-specific): Sentinel squads must watch for squads
optimizing for measured proxies (token count, task completion rate) at the expense
of actual value delivery.

---

## Squad Lifecycle

```
Discover → Establish → Use → Live → Rapid Flow
```

Topology is not fixed. Squad shapes evolve based on measured interaction patterns
and context load signals. The topology should be revisited when:
- A squad's context load has grown significantly
- Two squads are in persistent collaboration (should it be X-as-a-Service?)
- A new value stream has been identified
- A complicated subsystem is being replicated across multiple squads
