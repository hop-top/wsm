# Framework Selector Reference
## When to Apply Which Layer

---

## The Stack

Frameworks are additive layers, not alternatives. Apply them bottom-up:

```
┌─────────────────────────────────────────────────┐
│  GOVERNANCE LAYER                               │
│  SAFe guardrails · compliance rails · WSJF      │
│  → Apply when: regulated industry, audit req.   │
├─────────────────────────────────────────────────┤
│  TOPOLOGY LAYER                                 │
│  Squad Topologies                               │
│  → Apply when: 2+ squads with handoffs          │
├─────────────────────────────────────────────────┤
│  BOUNDARY LAYER                                 │
│  Domain-Driven Design (DDD)                     │
│  → Apply when: complex domain, data ownership   │
├─────────────────────────────────────────────────┤
│  EXECUTION LAYER                                │
│  unFIX (dynamic) · Spotify (specialization)     │
│  → Apply when: fluid crews or guild maintenance │
├─────────────────────────────────────────────────┤
│  SIMPLICITY CHECK                               │
│  LeSS — do you actually need all this?          │
│  → Ask before adding any layer                  │
└─────────────────────────────────────────────────┘
```

Below the whole stack: **Tier 0** — single agent, no framework needed.

---

## Framework Decision Rules

### Tier 0: No Framework (Agent Brief only)

**Apply when ALL of these are true:**
- One type of task (not multiple distinct domains)
- Output goes to one place (not fanned out to multiple workflows)
- No handoffs between agents required
- No compliance or audit requirement
- A single well-briefed human could do this job alone

**Output:** Agent Brief document only.

**Profiles:** Assistant (0), sometimes Researcher (1) at small scope.

---

### Squad Topologies (Tier 1 — baseline)

**Apply when ANY of these are true:**
- 2+ distinct value streams that need separate agents
- Clear handoff points between different types of work
- Different agents need different context boundaries
- A platform (shared infra) is emerging

**This is the default Tier 1 framework.** Apply it first before considering
any augmentation.

---

### DDD Augmentation

**Apply when ANY of these are true:**
- Agents share data that means different things in different contexts
  (e.g., "Customer" means different things to billing vs. support)
- There's a risk of "big ball of mud" — agents reaching into each other's
  data directly
- Multiple squads own overlapping domain concepts
- An existing codebase or data model needs to be mapped to agent boundaries

**Key concept to apply:** Bounded Context — each agent/squad has its own
model of the domain. Cross-context communication requires an explicit
interface (Context Map).

**Ubiquitous Language:** Standardize the vocabulary all agents use for
shared concepts. Ambiguous terms cause hallucination and coordination failures.

---

### unFIX Augmentation

**Apply when ANY of these are true:**
- Work is highly variable — squads need to form and dissolve around goals
- A "Crew" model is needed: temporary goal-oriented swarms
- Knowledge management across crews is important (Bases)
- Cross-crew protocol standardization is needed (Forums)

**Key concepts to apply:**
- **Crew** = temporary swarm for a goal-oriented task (maps to Profile 6: Swarm Unit)
- **Base** = persistent knowledge/governance center agents home to
- **Forum** = cross-agent communication protocol governance

---

### Spotify Model Augmentation

**Apply when ANY of these are true:**
- Specialist agents of the same type exist across multiple squads
  (e.g., "coding agents" in 3 different squads)
- Model updates, library patches, or capability improvements need to
  propagate consistently across all agents of a type
- There's a risk of capability drift between same-type agents in different squads

**Key concepts to apply:**
- **Chapter** = all agents of the same type, regardless of squad
  (mechanism for consistent capability updates)
- **Guild** = cross-squad community of practice for a shared capability

---

### SAFe Augmentation

**Apply when ANY of these are true:**
- Regulated industry: finance, healthcare, legal, insurance, government
- Audit trail required for agent decisions
- Hard compliance constraints that cannot be violated
- Multiple agentic systems need synchronized releases
- Resource prioritization logic needs to be explicit and auditable

**Key concepts to apply:**
- **Guardrails** = hard-coded constraints that agents cannot override
  (maps to Profile 4: Validator + Sentinel)
- **ART (Agile Release Train)** = synchronized deployment pipeline for
  multiple agentic swarms
- **WSJF** = Weighted Shortest Job First — algorithmic task prioritization

---

### LeSS (Simplicity Check)

**Ask before adding ANY layer:**
> "Could one generalist agent with full-stack tool access own this entire
> goal without handoffs?"

If yes → use LeSS philosophy. Don't add topology or framework overhead.
Build one capable agent (Profile 1: Researcher or Profile 2: Executor
with broad tooling).

LeSS is not a framework to implement — it's a question to ask. Its agentic
translation is: **eliminate handovers by making agents more capable across
the whole stack**.

**Apply LeSS thinking when:**
- Coordination overhead between squads is high relative to the value produced
- A "distributed monolith" is emerging (squads tightly coupled anyway)
- The problem domain is narrow enough for one agent to own end-to-end

---

## Augmentation Decision Matrix

| Situation | Squad Topologies | + DDD | + unFIX | + Spotify | + SAFe |
|---|---|---|---|---|---|
| Simple multi-agent workflow | ✓ | | | | |
| Complex domain, shared data | ✓ | ✓ | | | |
| Dynamic, goal-driven crews | ✓ | | ✓ | | |
| Many specialist agents of same type | ✓ | | | ✓ | |
| Regulated industry | ✓ | ✓ | | | ✓ |
| Large org, many squads | ✓ | ✓ | ✓ | ✓ | |
| Full enterprise, regulated, at scale | ✓ | ✓ | ✓ | ✓ | ✓ |
