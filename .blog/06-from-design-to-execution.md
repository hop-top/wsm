# From Design to Execution: Making Squad Topologies Real

*Part 6 of the Squad Topologies series*

---

A beautiful topology means nothing without execution.

You've designed your squad types. You've declared your interaction modes. You've planned for evolution. Now what?

The design is just a document until agents are actually running, communicating through the contracts you defined, and leaving an audit trail that proves the topology works.

This is where WSM comes in.

---

## The Execution Gap

Most organizational designs fail in the gap between intention and reality:

| What You Designed | What Actually Happens |
|-------------------|----------------------|
| Clear squad boundaries | Agents don't know which squad they're in |
| Declared interaction modes | Ad-hoc communication bypasses the contracts |
| Evolution rules | No one measures drift; topology fossilizes |
| Audit requirements | No trail of who did what when |

The problem isn't the design. The problem is that the design exists in a document, not in the system that actually runs.

WSM closes this gap by making the topology executable.

---

## The Machine-Readable Contract: squads.yaml

Your topology design lives in `squads.yaml`. This isn't documentation—it's configuration. The system reads it. The system enforces it.

```yaml
version: "1.0"

squads:
  - name: feature-delivery
    type: stream-aligned
    agents:
      - profile: researcher
        capabilities:
          - codebase-exploration: true
          - requirement-analysis: true
      - profile: builder
        capabilities:
          - code-generation: true
          - test-writing: true
      - profile: reviewer
        capabilities:
          - code-review: true
          - security-audit: true
    contracts:
      - interacts_with: database-migrations
        mode: x-as-a-service
        interface:
          request: migration_request
          response: migration_result
      - interacts_with: testing-excellence
        mode: collaboration
        duration: 2 weeks
        exit_condition: "Interface documented and stable"
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

  - name: database-migrations
    type: complicated-subsystem
    agents:
      - profile: schema-analyst
        capabilities:
          - schema-design: true
          - migration-planning: true
      - profile: migration-executor
        capabilities:
          - migration-execution: true
          - rollback-handling: true
    contracts:
      - interacts_with: feature-delivery
        mode: x-as-a-service
        interface:
          request: migration_request
          response: migration_result

  - name: testing-excellence
    type: enabling
    agents:
      - profile: testing-coach
        capabilities:
          - framework-training: true
          - test-pattern-guidance: true
    contracts:
      - interacts_with: feature-delivery
        mode: facilitating
        exit_condition: "3 projects completed without assistance"
        max_duration: 6 weeks

  - name: project-coordinator
    type: orchestrator
    agents:
      - profile: task-decomposer
        capabilities:
          - goal-decomposition: true
          - dependency-mapping: true
      - profile: result-synthesizer
        capabilities:
          - output-integration: true
          - delivery-verification: true
    contracts:
      - interacts_with: feature-delivery
        mode: supervised-delegation
        direction: sender
      - interacts_with: database-migrations
        mode: supervised-delegation
        direction: sender

  - name: compliance-watch
    type: sentinel
    agents:
      - profile: policy-watcher
        capabilities:
          - drift-detection: true
          - compliance-checking: true
      - profile: alert-generator
        capabilities:
          - alert-routing: true
          - audit-logging: true
    signals:
      - policy_violation
      - unauthorized_access
      - configuration_drift
```

This file is the source of truth. Not the whiteboard. Not the wiki. This YAML.

---

## Design Artifacts → Configuration

The `squads.yaml` doesn't appear from thin air. It's generated from design artifacts created during the squad design process:

| Artifact | What It Contains |
|----------|------------------|
| `squad-registry.md` | One card per squad: type, purpose, inputs, outputs |
| `interaction-contracts.md` | How every squad pair communicates |
| `squad-topology.diag` | Visual representation of the topology |
| `design-decisions.md` | What was decided and why |
| `next-steps.md` | What to build first |

These artifacts are human-readable. `squads.yaml` is machine-readable. The translation is intentional—design in the open, configure for execution.

---

## The Handoff to Workspace Operations

Squads don't execute in a vacuum. They operate within a workspace, which has tasks that need doing.

The handoff works like this:

```
Squad Topology (squads.yaml)
         │
         ▼
   Task Backlog
         │
         ▼
   TLC (Task/Workflow System)
         │
         ├─→ Assigns tasks to squads based on capability
         │
         ├─→ Respects interaction contracts
         │
         └─→ Records all activity to event store
```

**TLC** (the task/workflow component) doesn't invent work. It takes work from the backlog and routes it according to the topology:

1. **Decomposition** — Complex tasks go to orchestrator squads
2. **Routing** — Tasks are assigned to squads with matching capabilities
3. **Coordination** — Interactions follow declared contracts
4. **Supervision** — Checkpoints and reclaim conditions are enforced

The topology is the routing logic. TLC is the execution engine.

---

## Event Sourcing: The Audit Trail

Every action in the system is an event. Events are immutable, append-only, and replayable.

```json
{
  "id": "01JX2K3M4N5P6Q7R8S9T",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "type": "squad.task_assigned",
  "squad_id": "feature-delivery",
  "agent_id": "builder-01",
  "task_id": "implement-auth-flow",
  "timestamp": "2025-02-20T14:32:00Z",
  "data": {
    "assigned_by": "project-coordinator",
    "checkpoint": "design_review",
    "contract_mode": "supervised-delegation"
  }
}
```

```json
{
  "id": "01JX2K3M4N5P6Q7R8S9V",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "type": "squad.checkpoint_passed",
  "squad_id": "feature-delivery",
  "checkpoint": "design_review",
  "timestamp": "2025-02-20T16:45:00Z",
  "data": {
    "reviewed_by": "project-coordinator",
    "result": "approved",
    "artifacts": ["design-doc.md", "api-spec.yaml"]
  }
}
```

```json
{
  "id": "01JX2K3M4N5P6Q7R8S9W",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "type": "squad.interaction",
  "squad_id": "feature-delivery",
  "timestamp": "2025-02-20T17:00:00Z",
  "data": {
    "target_squad": "database-migrations",
    "mode": "x-as-a-service",
    "request": "migration_request",
    "response": "migration_result",
    "duration_ms": 3400
  }
}
```

This isn't logging. This is the source of truth. If you want to know:

- Which squad handled which task
- Whether interaction contracts were followed
- How long collaboration actually took
- Whether checkpoints were passed or reclaimed

...you replay the events.

---

## Snapshots: Fast State Access

Replaying all events from the beginning is slow. Snapshots solve this.

A **snapshot** is a materialized view at a point in time:

```json
{
  "id": "01JX2K3M4N5P6Q7R8SA0",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "at_seq_num": 15234,
  "at_timestamp": "2025-02-20T18:00:00Z",
  "squad_state": {
    "feature-delivery": {
      "status": "active",
      "current_tasks": ["implement-auth-flow"],
      "pending_checkpoints": ["test_verification", "pre_deploy_approval"],
      "interactions_today": {
        "database-migrations": { "count": 3, "mode": "x-as-a-service" },
        "testing-excellence": { "count": 1, "mode": "collaboration", "duration_days": 12 }
      }
    },
    "testing-excellence": {
      "status": "enabling",
      "target_squad": "feature-delivery",
      "weeks_active": 2,
      "exit_progress": "1/3 projects completed"
    }
  },
  "topology_metrics": {
    "collaboration_duration_avg_days": 14,
    "x_as_a_service_requests": 47,
    "reclaim_rate_percent": 3,
    "sentinel_alerts": 0
  }
}
```

Snapshots enable fast queries:

- "What's the current state of squad X?"
- "How long has this collaboration been running?"
- "What's the reclaim rate for supervised-delegation?"

They're generated by replaying events since the last snapshot, then persisted for future queries.

---

## Validation: Enforcing the Topology

The topology isn't just suggestions. It's enforced.

### At Configuration Load

When `squads.yaml` is loaded, the system validates:

| Check | Error If Fails |
|-------|----------------|
| Every squad has a type | "Squad 'foo' missing required field 'type'" |
| Type is valid | "Unknown squad type 'super-squad'" |
| Every agent has a profile | "Agent in 'feature-delivery' missing profile reference" |
| Every contract has a mode | "Contract between 'foo' and 'bar' missing interaction mode" |
| Mode is valid for squad pair | "Squad type 'enabling' cannot use mode 'x-as-a-service' as sender" |
| Enabling squads have exit conditions | "Enabling squad 'testing-excellence' missing exit_condition" |
| Supervised-delegation has reclaim conditions | "Contract missing reclaim_conditions for supervised-delegation" |

### At Runtime

During execution, the system enforces:

| Check | Behavior |
|-------|----------|
| Agent belongs to declared squad | Rejected if agent tries to act for wrong squad |
| Interaction follows declared mode | Warned if communication bypasses contract |
| Checkpoints respected | Task cannot proceed past checkpoint until approved |
| Reclaim conditions triggered | Orchestrator notified; can intervene |
| Exit conditions monitored | Enabling squad notified when exit is met |

---

## Evolution in Practice

Remember Post 5? Evolution is measured drift followed by topology change.

WSM makes this concrete:

### Drift Detection

```bash
wsm topology drift-report --workspace my-project
```

Output:

```
DRIFT DETECTED:

Squad: testing-excellence
  Mode: facilitating (target: feature-delivery)
  Expected duration: 6 weeks
  Actual duration: 9 weeks
  Exit progress: 1/3 projects
  Status: OVERDUE

Squad: database-migrations
  Mode: x-as-a-service
  Avg response time: 2.3 days (threshold: 1 day)
  Queue depth: 14 (threshold: 10)
  Status: BOTTLENECK

RECOMMENDED ACTIONS:
  1. Audit exit condition for testing-excellence
  2. Consider converting database-migrations to platform or adding capacity
```

### Topology Update

```bash
wsm topology evolve --workspace my-project --apply
```

This reads the current `squads.yaml`, applies recommended changes (with human approval), and updates the configuration. All changes are recorded as events.

---

## The Full Picture

```
┌─────────────────────────────────────────────────────────────────┐
│                      DESIGN TIME                                │
│                                                                 │
│   squad-registry.md    interaction-contracts.md                │
│          │                      │                              │
│          └──────────┬───────────┘                              │
│                     ▼                                          │
│               squads.yaml                                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                     CONFIGURATION                               │
│                                                                 │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│   │   Squads    │    │  Contracts  │    │   Agents    │        │
│   │   (types)   │───▶│   (modes)   │◀───│ (profiles)  │        │
│   └─────────────┘    └─────────────┘    └─────────────┘        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                      EXECUTION                                  │
│                                                                 │
│   Task Backlog ──▶ TLC ──▶ Squad Assignment ──▶ Agent Work     │
│                          │                                      │
│                          ▼                                      │
│                    Validation                                   │
│                    (contracts enforced)                         │
│                          │                                      │
│                          ▼                                      │
│                    Event Store                                  │
│                    (immutable audit trail)                      │
│                          │                                      │
│                          ▼                                      │
│                    Snapshots                                    │
│                    (fast state access)                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                      EVOLUTION                                  │
│                                                                 │
│   Drift Detection ──▶ Metrics ──▶ Recommendations              │
│          │                                      │               │
│          └──────────────────────────────────────┘               │
│                          │                                      │
│                          ▼                                      │
│                    squads.yaml updated                          │
│                          │                                      │
│                          └──────────▶ back to EXECUTION        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## What You Get

With this system in place:

| Capability | How WSM Delivers |
|------------|------------------|
| **Clear boundaries** | Squads are explicit in config; agents belong to exactly one |
| **Declared interactions** | Contracts are machine-readable; violations are flagged |
| **Enforced evolution** | Drift is measured; recommendations are generated |
| **Full audit trail** | Every action is an immutable event |
| **Debuggable topology** | Replay events to understand what happened and why |
| **Measurable improvement** | Metrics on collaboration duration, reclaim rates, queue depth |

---

## The Series Recap

We covered:

1. **Post 1** — The hidden cost of multi-agent chaos: context load, coordination debt
2. **Post 2** — What human teams teach AI: Team Topologies adapted, six squad types
3. **Post 3** — The six squad types explained: when to use each, anti-patterns
4. **Post 4** — Interaction modes: four modes, graduation rules, compatibility matrix
5. **Post 5** — Evolution: signals, triggers, process, anti-patterns
6. **Post 6** — From design to execution: squads.yaml, event sourcing, validation

The framework is complete. The topology is opinionated. The execution is auditable.

Now go organize your agents.

---

*Squad Topologies is a framework for organizing multi-agent AI workflows with explicit structure, declared interactions, and measurable evolution. Built into WSM (Workspace Session Manager).*

*This post is part of the Squad Topologies series. [← Back to Part 5](05-evolution.md) | [Back to Part 1](01-hidden-cost-of-multi-agent-chaos.md)*
