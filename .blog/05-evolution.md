# Evolution: When and How to Reshape Your Topology

*Part 5 of the Squad Topologies series*

---

Your topology is alive. Or it's dead.

Alive means it changes. Squads get added. Interactions shift. Collaboration graduates to services. Capabilities transfer. The shape adapts to reality.

Dead means it's frozen. The `squads.yaml` file was written once and never touched. New work gets shoehorned into old structures. Friction grows. No one remembers why the topology was designed this way.

Evolution isn't optional. It's the difference between a topology that serves you and one that constrains you.

---

## Why Topologies Drift

The topology you designed was right for the problems you had then. But:

- **New capabilities emerge** — Suddenly you need a squad for something that didn't exist
- **Workloads shift** — One stream-aligned squad is drowning; another is idle
- **Collaboration overstays** — Two squads that were "just collaborating temporarily" have been at it for six months
- **Expertise concentrates** — A complicated-subsystem becomes a bottleneck because one person understands it
- **Platform decays** — Self-service isn't self anymore; it requires tickets and meetings

The topology that was correct becomes incorrect. Not because you were wrong. Because the world changed.

---

## Signals That Evolution Is Needed

### Signal 1: Collaboration Never Graduated

Two squads have been collaborating for more than one sprint cycle (or equivalent). The interface isn't stable. They keep having "just one more meeting."

**What it means:** Either the problem is harder than expected (keep collaborating with a deadline) or the squads are avoiding the work of defining a stable interface (force graduation).

**Action:** Set a hard deadline. "In two weeks, we switch to X-as-a-Service. The interface will be whatever we have documented by then."

### Signal 2: The Bottleneck Squad

A complicated-subsystem or platform squad has a queue. Requests take days or weeks. Other squads work around them.

**What it means:** The squad is mis-typed or under-resourced.

**Actions:**
- If the work is repetitive and high-volume → convert to platform (self-service)
- If the work requires genuine expertise → add capacity or split into multiple specialized squads
- If the queue is due to poor interface → fix the interface, not the squad

### Signal 3: The Shadow Platform

A stream-aligned squad builds their own tools. They don't use the official platform. They say "it's faster to build our own."

**What it means:** The platform isn't meeting their needs. Either the interface is wrong, the service is too slow, or the capability is missing.

**Action:** Don't ban shadow platforms. Understand why they exist. Fix the platform or admit it's not a platform and let the stream-aligned squad own the capability.

### Signal 4: The Permanent Enabling Squad

An enabling squad has existed for over six months. They're still "teaching." The target squads haven't internalized the capability.

**What it means:** The exit condition is wrong or not enforced. Or the capability isn't teachable—it needs to stay as a complicated-subsystem.

**Action:** Audit the exit condition. If it's being gamed, enforce it. If it's impossible, convert the enabling squad to a complicated-subsystem and stop pretending knowledge will transfer.

### Signal 5: Orchestrator Does Everything

The orchestrator squad is involved in every decision. Nothing moves without their approval. They're the only ones who "understand the big picture."

**What it means:** The orchestrator has become a bottleneck. Supervised-delegation has mutated into micromanagement.

**Action:** Audit the reclaim conditions. Are they too sensitive? Is the orchestrator intervening unnecessarily? Push decision-making back to stream-aligned squads. The orchestrator should coordinate, not control.

### Signal 6: Sentinel Alerts Are Ignored

The sentinel squad raises flags. No one responds. Alerts pile up in a queue that no one checks.

**What it means:** Either the signals are wrong (too many false positives) or the organization doesn't value compliance.

**Action:** Tune the signals. Reduce noise. But if the signals are accurate and still ignored, the problem isn't the topology—it's culture. That's beyond this post.

---

## Evolution Triggers

Some changes are reactive (respond to signals). Others are proactive (planned transitions).

### Reactive Triggers

| Trigger | Typical Evolution |
|---------|-------------------|
| Collaboration > 1 cycle | Graduate to X-as-a-Service |
| Queue length > threshold | Convert to platform or add capacity |
| Shadow platforms emerge | Fix or replace official platform |
| Enabling squad > 6 months | Enforce exit or convert to complicated-subsystem |
| Orchestrator bottleneck | Decentralize, reduce reclaim sensitivity |

### Proactive Triggers

| Trigger | Typical Evolution |
|---------|-------------------|
| New business capability | Add stream-aligned squad |
| New specialized domain | Add complicated-subsystem |
| Capability needs teaching | Add enabling squad with exit condition |
| Multiple squads need same tool | Create platform squad |
| Complex multi-squad projects | Add orchestrator (temporary or permanent) |
| Compliance requirements | Add sentinel squad |

---

## The Evolution Process

Evolution isn't "rewrite the YAML and hope." It's a structured process.

### Step 1: Measure

You can't evolve what you don't measure. Track:

| Metric | What It Tells You |
|--------|-------------------|
| Interaction frequency | How often squads communicate |
| Collaboration duration | How long temporary modes persist |
| Queue depth | Bottleneck detection |
| Reclaim frequency | Orchestrator intervention rate |
| Alert response time | Sentinel effectiveness |

### Step 2: Identify Drift

Compare measurements to expectations:

- Collaboration was supposed to last 2 weeks. It's been 8.
- Queue should be < 5 items. It's at 23.
- Reclaims should be rare. There were 12 last week.

### Step 3: Diagnose Root Cause

Drift is a symptom. Find the cause:

- Is the interface fundamentally broken?
- Is the squad mis-typed?
- Is the exit condition impossible?
- Is capacity insufficient?

### Step 4: Design the Evolution

What changes?

- Add a squad?
- Remove a squad?
- Change a squad type?
- Change an interaction mode?
- Add/modify an exit condition?
- Add/modify a reclaim condition?

Document the change before implementing it.

### Step 5: Implement and Monitor

Update `squads.yaml`. Communicate the change. Monitor the metrics.

Did the drift decrease? Did new drift emerge?

### Step 6: Iterate

Evolution isn't one-and-done. It's continuous. Repeat the cycle.

---

## Graduation Rules

The most common evolution is mode graduation. Here are the rules:

### Collaboration → X-as-a-Service

**When:** The interface is stable. Both squads know what to expect.

**What changes:**
- Remove scheduled syncs
- Document the interface as self-service
- Provider treats requests as anonymous

**Metric:** Collaboration meetings drop to zero. Provider can't name individual consumers.

### Facilitating → Independence

**When:** The learner squad can operate without the facilitator.

**What changes:**
- Enabling squad exits
- Learner squad owns the capability
- No more scheduled teaching sessions

**Metric:** Learner completes N tasks without facilitator involvement.

### Supervised-Delegation → X-as-a-Service

**When:** The executor has demonstrated reliability. Reclaims are rare.

**What changes:**
- Remove checkpoints
- Remove reclaim conditions
- Executor operates autonomously

**Metric:** Zero reclaims in N tasks. Checkpoints pass automatically.

---

## Anti-Patterns in Evolution

### Evolution by Crisis

Only changing the topology when everything is on fire. Reactive, rushed, poorly designed.

**Fix:** Regular topology reviews. Monthly or quarterly. Measure drift before it becomes crisis.

### Evolution by Hippo

The Highest Paid Person's Opinion decides the topology. No measurement. No signals. Just hierarchy.

**Fix:** Data-driven evolution. Present metrics. Show drift. Propose changes based on evidence.

### Evolution Never

The `squads.yaml` is sacred. Changing it requires approval from three committees. By the time approval arrives, the problem has changed.

**Fix:** Topology is configuration, not legislation. Make it easy to change. Require documentation, not permission.

### Evolution Too Often

Every week brings a new topology. Squads can't form stable working patterns. Constant churn.

**Fix:** Batch changes. Review monthly. Implement quarterly (except for critical fixes).

---

## The Evolution Audit Checklist

Every quarter, run this audit:

| Question | Red Flag | Action |
|----------|----------|--------|
| Are any collaborations > 1 month old? | Yes | Force graduation or extend deadline |
| Are any queues > 10 deep? | Yes | Add capacity or convert to platform |
| Are any enabling squads > 6 months old? | Yes | Audit exit condition |
| Are reclaim rates > 10%? | Yes | Reduce oversight or fix root cause |
| Are sentinel alerts ignored? | Yes | Tune signals or address culture |
| Have any shadow platforms emerged? | Yes | Fix official platform or legitimize shadow |

---

## What's Next

You have types. You have modes. You have evolution.

Now you need to make it real. The topology is a design. Execution is the implementation.

Next post: From Design to Execution—how WSM turns `squads.yaml` into running, auditable, evolvable agent workflows.

[Continue to Part 6: From Design to Execution →](06-from-design-to-execution.md)

---

*This post is part of the Squad Topologies series. [← Back to Part 4](04-interaction-modes.md)*
