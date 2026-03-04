# Social Media Variants: The Six Squad Types, Explained

## X (Twitter)

### Variant 1
Not all agents should talk to each other.

When you give every agent access to every other agent, you don't get collaboration. You get noise.

Squad types solve this. Each type has a job, constraints, and knows what it should and shouldn't do.

Here are all 6 ↓

### Variant 2
The 6 squad types:

1. Stream-aligned → Own pipeline end-to-end
2. Enabling → Transfer capability, then exit
3. Complicated-subsystem → Deep expertise, clean interface
4. Platform → Self-service substrate
5. Orchestrator → Decompose, route, synthesize
6. Sentinel → Watch, detect, flag

### Variant 3
Rule of thumb for stream-aligned squads:

If this squad disappeared, would a user notice a missing capability?

Yes = probably stream-aligned.
No = something else.

---

## Facebook Groups

### Variant 1
Quick breakdown of squad types for multi-agent systems:

Stream-aligned: Owns a value stream end-to-end. Feature delivery.

Enabling: Teaches, then exits. Has a finish line. "3 projects completed independently."

Complicated-subsystem: Deep expertise. Database migrations. Security. Clean handoff interface.

Platform: Self-service. CI/CD, tooling. Agents consume without asking.

Orchestrator: Decomposes complex goals, routes work, synthesizes results.

Sentinel: Watches for drift. Compliance. Observability. Doesn't fix—flags.

Which types are you using?

### Variant 2
The enabling squad anti-pattern that kills multi-agent systems:

No exit condition.

If your enabling squad has been "teaching" for 6 months and the target squad still can't do it independently, you don't have an enabling squad. You have a permanent dependency.

Every enabling squad needs a measurable finish line.

### Variant 3
How to choose a squad type:

Q: Does this deliver direct value to users?
A: Stream-aligned

Q: Is this work temporary with a clear exit?
A: Enabling

Q: Does this require deep expertise most don't have?
A: Complicated-subsystem

Q: Can this be self-service with no coordination?
A: Platform

---

## Threads

### Variant 1
Six squad types. One per agent group.

Stream-aligned: End-to-end delivery
Enabling: Teach and exit
Complicated-subsystem: Expert interface
Platform: Self-service foundation
Orchestrator: Coordination hub
Sentinel: Watch and warn

Mix wrong = chaos. Mix right = flow.

### Variant 2
The platform test:

Can a new squad start using this in under an hour without talking to anyone?

If they need a meeting first, it's not a platform. It's a service desk.

### Variant 3
Sentinel squads must stay independent.

Their job is to observe, not intervene.

If your sentinel starts fixing problems, who's watching the watcher?

---

## LinkedIn Article

**Title: The Six Squad Types Every Multi-Agent System Needs**

Not all agents should talk to each other.

That's the core insight behind squad types. When you give every agent access to every other agent, you don't get collaboration. You get noise. Context pollution. Coordination overhead that compounds until nothing gets done.

Squad types solve this by assigning clear purpose and boundaries. Each type has a job. Each type has constraints. Each type knows what it should and shouldn't do.

Here's the complete breakdown:

**Stream-aligned**: Own a value stream end-to-end. Take requests, deliver complete outcomes, handle the full pipeline without handoffs. If this squad disappeared, users would notice.

**Enabling**: Transfer capability to other squads, then exit. Has an exit condition. "Target squad passes 80% of tests independently."

**Complicated-subsystem**: Own a specialized domain. Provide a clean interface. Other squads consume without understanding internals.

**Platform**: Provide a self-service substrate. Other squads consume without coordination. "Can a new squad start using this in under an hour without talking to anyone?"

**Orchestrator**: Decompose complex goals, route to appropriate squads, synthesize results. They don't build—they coordinate.

**Sentinel**: Observe continuously, detect drift, raise flags. Don't fix—that's someone else's job.

[Continue reading the full series for anti-patterns and decision guides...]

---

## LinkedIn Post

### Variant 1
The self-service test for platform squads:

Can a new agent start using this capability in under an hour without any coordination?

If they need a meeting, a ticket, or a conversation → it's not a platform.

Real platforms don't require permission.

### Variant 2
Six squad types. One choice per agent group.

- Stream-aligned: Deliver value
- Enabling: Teach and leave
- Complicated-subsystem: Expert interface
- Platform: Self-service
- Orchestrator: Coordinate
- Sentinel: Observe

Get the types wrong, and you get bottlenecks. Get them right, and agents flow.

### Variant 3
The most common multi-agent mistake:

Everything is stream-aligned.

No. You end up with 20 squads all doing everything. No specialization. No boundaries. Chaos.

Force yourself to choose. Not every agent group delivers direct user value.
