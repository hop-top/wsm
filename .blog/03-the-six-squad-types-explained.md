# The Six Squad Types, Explained

*Part 3 of the Squad Topologies series*

---

Not all agents should talk to each other.

That's the core insight behind squad types. When you give every agent access to every other agent, you don't get collaboration. You get noise. Context pollution. Coordination overhead that compounds until nothing gets done.

Squad types solve this by assigning clear purpose and boundaries. Each type has a job. Each type has constraints. Each type knows what it should and shouldn't do.

Let's go deep on all six.

---

## 1. Stream-Aligned

### The Job

Own a value stream end-to-end. Take requests from users (or other squads), deliver complete outcomes, handle the full pipeline without handoffs.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Scope** | Broad. Knows the whole domain. |
| **Handoffs** | Minimal. Keeps work inside the squad when possible. |
| **Dependencies** | Consumes platform services. Collaborates temporarily with others. |
| **Lifespan** | Permanent. Exists as long as the value stream exists. |

### When to Use

- A distinct user journey or business capability
- Work that benefits from continuity (same agents, same context)
- Deliverables that span multiple steps without natural break points

### Agent Profile Example

A feature delivery squad might include:

- **Researcher** — Gathers requirements, explores codebase
- **Builder** — Implements the solution
- **Reviewer** — Catches issues before merge

They pass work among themselves. They don't hand off to other squads unless the scope changes fundamentally.

### Anti-Patterns

| Anti-Pattern | Why It Breaks |
|--------------|---------------|
| Too many agents | Context load explodes. Everyone needs to know everything. |
| Too narrow scope | Squad becomes a handoff point, not a value stream. |
| Undifferentiated membership | Every agent does everything. No specialization. |

### The Rule of Thumb

If this squad disappeared, would a user notice a missing capability? If yes, it's probably stream-aligned. If no, it might be something else.

---

## 2. Enabling

### The Job

Transfer capability to other squads. Help them level up. Then exit.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Scope** | Narrow. Deep expertise in one capability. |
| **Handoffs** | Frequent outbound. Teaching is the work. |
| **Dependencies** | Collaborates with target squad. Time-boxed. |
| **Lifespan** | Temporary. Has an exit condition. |

### When to Use

- A new technology, process, or standard needs adoption
- Multiple squads need the same skill
- The capability should eventually be owned by stream-aligned squads

### Agent Profile Example

A testing excellence squad might include:

- **Trainer** — Delivers curriculum, runs workshops
- **Coach** — Shadows target squad, provides feedback
- **Toolsmith** — Builds templates, utilities, documentation

They work with a feature squad for six weeks. The feature squad learns. The enabling squad moves to the next target.

### Anti-Patterns

| Anti-Pattern | Why It Breaks |
|--------------|---------------|
| No exit condition | Squad becomes permanent crutch. Dependency never ends. |
| Doing the work instead of teaching | Target squad doesn't learn. Knowledge stays concentrated. |
| Too many targets at once | Context diluted. Teaching quality drops. |

### The Exit Condition

Every enabling squad needs a measurable finish line:

- "Target squad passes 80% of tests independently"
- "Three team members certified in the new framework"
- "All projects migrated to the new CI pipeline"

If you can't define the exit, you don't have an enabling squad. You have a permanent dependency.

---

## 3. Complicated-Subsystem

### The Job

Own a specialized domain that requires deep expertise. Provide a clean interface. Other squads consume without needing to understand the internals.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Scope** | Narrow and deep. One subsystem. |
| **Handoffs** | Clean. Request in, response out. |
| **Dependencies** | Minimal. Provides service to others. |
| **Lifespan** | Permanent. The subsystem persists. |

### When to Use

- The domain requires specialized knowledge most agents don't have
- Getting it wrong is expensive (security, compliance, performance)
- The interface is stable enough to treat as a service

### Agent Profile Example

A database migrations squad might include:

- **Schema Analyst** — Designs migration paths
- **Executor** — Runs migrations, handles rollbacks
- **Validator** — Verifies data integrity post-migration

Other squads submit a migration request. They get back a success/failure. They don't need to know how the sausage is made.

### Anti-Patterns

| Anti-Pattern | Why It Breaks |
|--------------|---------------|
| Leaky abstractions | Consumers need to know internals. Interface isn't clean. |
| Bottleneck | Queue grows. Response time slows. Should be a platform instead. |
| Scope creep | Squad starts owning adjacent concerns. Loses focus. |

### The Interface Test

Can a consumer squad use this service without reading the documentation? If they need to understand the internals to get it right, the interface isn't clean enough.

---

## 4. Platform

### The Job

Provide a self-service substrate. Other squads consume without coordination. The platform team builds the road; others drive on it.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Scope** | Broad but shallow. Many capabilities, well-abstracted. |
| **Handoffs** | None. Self-service consumption. |
| **Dependencies** | Minimal. Others depend on platform. |
| **Lifespan** | Permanent. The substrate persists. |

### When to Use

- Multiple squads need the same capability
- The capability can be productized as a self-service tool
- Coordination overhead would otherwise be repeated

### Agent Profile Example

A developer tools squad might include:

- **CLI Builder** — Maintains the command-line interface
- **Template Curator** — Manages project scaffolds
- **Doc Writer** — Keeps documentation accurate

Other squads run `wsm workspace create`. They don't call the platform team. They consume.

### Anti-Patterns

| Anti-Pattern | Why It Breaks |
|--------------|---------------|
| Ticket-based requests | Becomes a help desk, not a platform. Self-service fails. |
| One-size-fits-all | Abstraction too rigid. Squads work around it. |
| Neglected maintenance | Platform decays. Consumers lose trust. Build their own. |

### The Self-Service Test

Can a new squad start using this platform in under an hour without talking to anyone? If they need a meeting first, it's not a platform. It's a service desk.

---

## 5. Orchestrator

### The Job

Decompose complex goals into assignable tasks. Route those tasks to appropriate squads. Synthesize results into coherent outcomes.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Scope** | Meta. Operates across squads, not within them. |
| **Handoffs** | Bidirectional. Receives goals, distributes tasks, collects results. |
| **Dependencies** | Depends on all squads. The hub. |
| **Lifespan** | Permanent or project-scoped. |

### When to Use

- A goal spans multiple squads and requires coordination
- The decomposition is non-trivial (not just "all of you do X")
- Synthesis is needed (combining partial results into whole)

### Agent Profile Example

A project delivery squad might include:

- **Decomposer** — Breaks goals into tasks, identifies dependencies
- **Router** — Assigns tasks to appropriate squads based on capability
- **Synthesizer** — Combines partial deliverables into final output

A request comes in: "Migrate the billing system to a new provider." The orchestrator decomposes: data migration (stream-aligned), compliance review (complicated-subsystem), documentation (platform). Routes. Collects. Delivers.

### Anti-Patterns

| Anti-Pattern | Why It Breaks |
|--------------|---------------|
| Micromanagement | Tells squads *how* to do their work. Overreach. |
| Single point of failure | Orchestrator becomes bottleneck. Everything waits. |
| Permanent for temporary problems | Simple coordination dressed up as orchestration. |

### The Complexity Test

Does this goal require decomposition, routing, AND synthesis? If it's just one of those, you might not need an orchestrator. If it's all three, you do.

---

## 6. Sentinel

### The Job

Observe continuously. Detect drift from expected patterns. Raise flags. Do not fix—that's someone else's job.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Scope** | Observational. Watches signals, not outcomes. |
| **Handoffs** | Outbound only. Raises alerts. Doesn't receive work. |
| **Dependencies** | Minimal. Observes everything, coordinates nothing. |
| **Lifespan** | Permanent. Vigilance never ends. |

### When to Use

- Compliance requirements (security, regulatory, contractual)
- Quality gates that shouldn't be self-policed
- Early warning systems for emerging problems

### Agent Profile Example

A compliance squad might include:

- **Watcher** — Monitors event streams for policy violations
- **Reporter** — Generates alerts, dashboards, audit logs
- **Auditor** — Periodically reviews flagged items for patterns

They don't fix the security vulnerability. They detect it, flag it, and ensure someone else owns the remediation.

### Anti-Patterns

| Anti-Pattern | Why It Breaks |
|--------------|---------------|
| Fixing instead of flagging | Becomes a reactive repair squad. Loses independent oversight. |
| Alert fatigue | Too many flags. Real signals drown in noise. |
| Missing the important signals | Watching the wrong metrics. Drift goes undetected. |

### The Independence Test

If this squad starts fixing problems, who's watching the watcher? Sentinel squads must stay independent. Their job is to observe, not intervene.

---

## Choosing Squad Types: A Decision Guide

| Question | If Yes | If No |
|----------|--------|-------|
| Does this squad deliver direct value to users? | Stream-aligned | Consider other types |
| Is this work temporary with a clear exit? | Enabling | Consider other types |
| Does this require deep expertise most don't have? | Complicated-subsystem | Consider other types |
| Can this be self-service with no coordination? | Platform | Consider other types |
| Does this require decomposition + routing + synthesis? | Orchestrator | Consider other types |
| Is this about watching, not doing? | Sentinel | Consider other types |

## Common Mistakes

1. **Everything is stream-aligned** — No. You end up with 20 squads all doing everything. Chaos.
2. **No enabling squads** — Knowledge never transfers. Dependencies on specialists persist forever.
3. **Complicated-subsystem becomes a bottleneck** — Should be a platform. Was mis-typed.
4. **Orchestrator for simple coordination** — Overhead without value. Let squads self-coordinate.
5. **Sentinel that fixes** — Conflict of interest. Can't audit your own repairs.

## What's Next

You have the six types. Now you need to understand how they talk to each other.

Next post: Interaction Modes—the contracts that make autonomy work.

[Continue to Part 4: Interaction Modes →](04-interaction-modes.md)

---

*This post is part of the Squad Topologies series. [← Back to Part 2](02-what-human-teams-teach-ai-agents.md)*
