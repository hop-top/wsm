# Interaction Modes: The Glue That Makes Autonomy Work

*Part 4 of the Squad Topologies series*

---

Two squads. Three ways to talk. One that's a trap.

Interaction modes are the contracts between squads. They define how work flows, what coordination looks like, and when conversations happen. Get them right, and squads operate independently. Get them wrong, and you're back to chaos—except now it's organized chaos.

Let's break down the four modes.

---

## The Four Interaction Modes

| Mode | Direction | Duration | Coordination |
|------|-----------|----------|--------------|
| **X-as-a-Service** | One-way | Indefinite | Minimal |
| **Collaboration** | Bidirectional | Time-boxed | High |
| **Facilitating** | One-way | Time-boxed | Moderate |
| **Supervised-Delegation** | Orchestrator → Executor | Task-scoped | Moderate |

Every pair of squads that might interact should have a declared mode. No "we'll figure it out when the time comes." The time is now.

---

## 1. X-as-a-Service

### The Contract

One squad provides. The other consumes. The provider doesn't know or care who's consuming. The consumer doesn't need to coordinate—they just use.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Direction** | One-way (provider → consumer) |
| **Duration** | Indefinite. This is the steady state. |
| **Coordination** | Minimal. Self-service. |
| **Change** | Provider evolves the service. Consumers adapt. |

### When to Use

- The provider has a stable, well-defined interface
- Consumers don't need customization
- The service is mature enough to be treated as a utility

### Example

A platform squad provides a CI/CD pipeline. Stream-aligned squads push code. The pipeline runs. No one calls the platform team to ask permission. No coordination meetings. The service is just there.

### The Anti-Pattern: Collaboration Disguised as Service

The platform says "we provide a service." But every time a consumer wants to use it, they need a meeting. The interface is unclear. The documentation is wrong. The service requires hand-holding.

That's not X-as-a-Service. That's collaboration with extra steps.

### The Test

Can a new squad start consuming this service without talking to the provider? If yes, it's X-as-a-Service. If no, it's something else—probably collaboration.

---

## 2. Collaboration

### The Contract

Two squads work together. Bidirectional. High coordination. Time-boxed. They explore unknowns together. Then they graduate to a cleaner mode.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Direction** | Bidirectional |
| **Duration** | Time-boxed. Has an end date. |
| **Coordination** | High. Frequent communication. |
| **Change** | Both squads adapt. Mutual discovery. |

### When to Use

- The problem space is unclear
- Neither squad knows the right approach yet
- Both need to learn from the other

### Example

A stream-aligned squad is building a new feature that requires a complicated-subsystem's expertise. Neither knows the right integration pattern. They collaborate for two weeks: daily syncs, shared documents, joint problem-solving. The interface emerges. Then they switch to X-as-a-Service.

### The Trap: Permanent Collaboration

Collaboration feels good. You're solving problems together! But permanent collaboration is coordination debt. It never scales. Every new squad that needs to collaborate with every other squad creates an N² communication graph.

Collaboration must have an exit condition:

- "Interface defined and documented"
- "Both squads can proceed independently"
- "Handoff process established"

If collaboration has no end date, you don't have squads. You have one big team with artificial boundaries.

### The Graduation Rule

Collaboration should graduate to X-as-a-Service. The outcome of collaboration is a stable interface. Once that interface exists, the squads no longer need to collaborate. They can consume each other as services.

---

## 3. Facilitating

### The Contract

One squad teaches. The other learns. One-way knowledge transfer. Time-boxed. The facilitator exits when the capability is transferred.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Direction** | One-way (facilitator → learner) |
| **Duration** | Time-boxed. Has an exit condition. |
| **Coordination** | Moderate. Scheduled sessions. |
| **Change** | Learner gains capability. Facilitator moves on. |

### When to Use

- An enabling squad is transferring capability
- A stream-aligned squad needs to level up
- The knowledge is teachable, not just executable

### Example

An enabling squad teaches a stream-aligned squad how to use a new testing framework. Weekly sessions. Pair programming. Code reviews with feedback. After six weeks, the stream-aligned squad tests independently. The enabling squad moves to the next target.

### The Anti-Pattern: Teaching Without Exit

The enabling squad stays forever. They become a permanent crutch. The stream-aligned squad never owns the capability. Dependency crystallizes.

Every facilitating relationship needs a clear exit:

- "Target squad completes three projects without assistance"
- "All team members pass certification"
- "Documentation is sufficient for self-service"

### The Test

Can the learner operate independently after the facilitator leaves? If no, the exit condition wasn't met. Keep facilitating. But set a new deadline.

---

## 4. Supervised-Delegation

### The Contract

An orchestrator delegates work to an executor squad. But the orchestrator retains oversight. The executor has autonomy within bounds. The orchestrator can reclaim the work if conditions aren't met.

### Characteristics

| Attribute | Behavior |
|-----------|----------|
| **Direction** | Orchestrator → Executor (with feedback loop) |
| **Duration** | Task-scoped. Ends when task completes. |
| **Coordination** | Moderate. Checkpoints, approval gates. |
| **Change** | Orchestrator can intervene. Executor can escalate. |

### When to Use

- The work is high-stakes (security, compliance, production)
- The executor needs autonomy but not full ownership
- There's a clear reclaim condition

### Example

An orchestrator delegates a security-critical migration to a stream-aligned squad. The squad has autonomy to execute. But the orchestrator reviews at three checkpoints: design approval, test results, pre-deploy verification. If the squad misses a checkpoint, the orchestrator reclaims.

### The Reclaim Condition

Supervised-delegation needs an explicit trigger for intervention:

```yaml
reclaim_condition:
  - checkpoint_failed
  - compliance_violation
  - deadline_missed
  - scope_creep_detected
```

Without this, supervision becomes micromanagement. The orchestrator hovers. The executor can't move.

### The Anti-Pattern: Delegation Without Supervision

That's just X-as-a-Service. If there's no oversight, no checkpoints, no reclaim condition, you called it supervised-delegation but it's actually unsupervised. Either add supervision or admit it's a service.

---

## Mode Compatibility Matrix

Not all squad pairs can use all modes. The squad types constrain the interaction modes.

| From \ To | Stream-aligned | Enabling | Complicated-subsystem | Platform | Orchestrator | Sentinel |
|-----------|---------------|----------|----------------------|----------|--------------|----------|
| **Stream-aligned** | Collaboration | — | X-as-a-Service | X-as-a-Service | Supervised-Delegation | — |
| **Enabling** | Facilitating | — | — | — | — | — |
| **Complicated-subsystem** | X-as-a-Service | — | — | — | — | — |
| **Platform** | X-as-a-Service | — | X-as-a-Service | — | — | — |
| **Orchestrator** | Supervised-Delegation | — | Supervised-Delegation | X-as-a-Service | — | — |
| **Sentinel** | — | — | — | — | — | — |

Sentinel squads observe. They don't interact in the normal sense—they raise flags, and other squads respond. That's a different kind of contract.

---

## Declaring Interaction Contracts

In `squads.yaml`, every squad declares its interactions:

```yaml
squads:
  - name: feature-delivery
    type: stream-aligned
    contracts:
      - interacts_with: database-migrations
        mode: x-as-a-service
        interface: migration_request → migration_result
        
      - interacts_with: developer-tools
        mode: x-as-a-service
        interface: cli commands, templates
        
      - interacts_with: project-coordinator
        mode: supervised-delegation
        direction: receiver
        checkpoints:
          - design_review
          - test_verification
          - pre_deploy_approval
        reclaim_conditions:
          - checkpoint_failed
          - scope_creep

  - name: testing-excellence
    type: enabling
    contracts:
      - interacts_with: feature-delivery
        mode: facilitating
        exit_condition: "3 projects completed without assistance"
        
  - name: project-coordinator
    type: orchestrator
    contracts:
      - interacts_with: feature-delivery
        mode: supervised-delegation
        direction: sender
      - interacts_with: database-migrations
        mode: supervised-delegation
        direction: sender
```

This is machine-readable. It's also human-readable. It's the contract.

---

## Common Mistakes

| Mistake | Symptom | Fix |
|---------|---------|-----|
| **Everything is collaboration** | Coordination debt. N² meetings. | Force graduation to X-as-a-Service. |
| **X-as-a-Service requires meetings** | "Service" needs hand-holding. | Fix the interface. Make it self-service. |
| **Facilitating never exits** | Enabling squad becomes permanent. | Set measurable exit conditions. |
| **Supervised-delegation without reclaim** | Micromanagement or abandonment. | Define explicit reclaim triggers. |
| **Missing contracts** | Squads "figure it out" ad-hoc. | Every pair must have a declared mode. |

---

## The Golden Rule

Every interaction mode except X-as-a-Service is temporary.

- Collaboration graduates to X-as-a-Service
- Facilitating exits when capability transfers
- Supervised-delegation ends when the task completes

X-as-a-Service is the steady state. Everything else is a transition. If you find yourself in permanent collaboration or permanent facilitating, something is wrong.

---

## What's Next

You have squad types. You have interaction modes. You have contracts.

But topologies aren't static. They evolve. Squads get added. Interactions change. Collaboration graduates.

Next post: Evolution—when and how to reshape your topology.

[Continue to Part 5: Evolution →](05-evolution.md)

---

*This post is part of the Squad Topologies series. [← Back to Part 3](03-the-six-squad-types-explained.md)*
