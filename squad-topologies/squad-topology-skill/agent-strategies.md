# Agent Strategies & Profiles
## The Foundational Vocabulary for Agentic System Design

---

## Overview

Every agent operates across four strategy layers. The combination of choices
across these layers defines how the agent thinks, acts, remembers, and relates
to other agents. This vocabulary is prerequisite to any topology or framework
decision.

```
Layer 1: REASONING  — how the agent thinks through a problem
Layer 2: EXECUTION  — how the agent acts in the world
Layer 3: MEMORY     — how the agent retains and retrieves context
Layer 4: COORDINATION — how the agent relates to other agents
```

---

## Layer 1: Reasoning Strategies

### Chain-of-Thought (CoT)
**What:** Explicit step-by-step reasoning before producing an answer. The agent
"thinks out loud" through the problem before committing to a response.

**Characteristics:**
- Linear, sequential reasoning
- Transparent: reasoning steps are visible
- Single pass (no revision unless combined with reflection)

**Best for:** Complex single-shot problems, math, logical deduction, structured
analysis where the path matters as much as the answer.

**Plain language:** "Let me think through this step by step before I answer."

**Limitations:** Doesn't backtrack. If the first reasoning path is wrong, CoT
follows it to a wrong conclusion.

---

### Self-Consistency
**What:** Runs CoT multiple times with different random seeds, then takes the
majority answer. Trades compute for reliability.

**Characteristics:**
- Multiple independent reasoning passes
- Aggregation by majority vote or synthesis
- Higher latency and cost than single CoT

**Best for:** High-stakes decisions where variance in outputs is unacceptable.
Financial decisions, medical triage, legal analysis.

**Plain language:** "Ask three people independently, go with the consensus."

**Limitations:** Expensive. Doesn't help when all reasoning paths share the
same systematic bias.

---

### Tree of Thought (ToT)
**What:** Explores multiple reasoning branches simultaneously, evaluates each,
backtracks dead ends, and continues promising paths. Like a chess engine thinking
ahead.

**Characteristics:**
- Branching, non-linear reasoning
- Explicit backtracking
- Evaluates intermediate states

**Best for:** Open-ended creative problems, planning with many valid paths,
situations where the right answer requires trying multiple approaches.

**Plain language:** "Explore several directions, abandon dead ends, deepen the
best path."

**Limitations:** Computationally expensive. Overkill for well-defined problems
with clear solution paths.

---

### Reflection / Self-Critique
**What:** Agent produces an initial output, then critiques it against a rubric
or its own standards, then revises. Can iterate multiple rounds.

**Characteristics:**
- Two-phase: generate → evaluate → revise
- Can be combined with any other reasoning strategy
- Improves quality at the cost of additional passes

**Best for:** Writing, code generation, any quality-sensitive output where
"good enough on first pass" is insufficient.

**Plain language:** "Write a draft, then review it critically, then improve it."

**Limitations:** Can get stuck in revision loops. Needs a clear stopping
condition (quality threshold or max iterations).

---

### Least-to-Most (LtM)
**What:** Decomposes a hard problem into a sequence of progressively easier
subproblems. Solves the easiest first, uses each solution as context for the next.

**Characteristics:**
- Problem decomposition before solving
- Sequential dependency: each step builds on the last
- More structured than CoT

**Best for:** Hierarchical tasks with dependencies. "To do X, I first need Y,
and to do Y I first need Z."

**Plain language:** "Break the big problem into smaller ones, solve from the
bottom up."

**Limitations:** Requires correct decomposition upfront. Wrong decomposition
propagates errors through all subsequent steps.

---

### Step-Back
**What:** Before solving the specific problem, abstracts to a higher-level
principle or category, reasons from that abstraction, then applies back to
the specific case.

**Characteristics:**
- Abstraction-first
- Good for novel problems that look superficially different from training examples
- Two-phase: abstract → apply

**Best for:** Novel problems, first-principles reasoning, situations where
surface features are misleading.

**Plain language:** "Step back and ask: what kind of problem is this, really?
Then solve that."

**Limitations:** Abstraction can overshoot — treating a simple specific problem
as a general case when a direct answer would be faster.

---

## Layer 2: Execution Strategies

### ReAct (Reason + Act)
**What:** Interleaves reasoning and action in a loop: Reason → Act → Observe →
Reason → Act → Observe... until the task is complete. The agent uses tools,
observes results, and reasons about what to do next.

**Characteristics:**
- Iterative, adaptive
- Tool-native (requires access to external tools)
- Observable: each step is logged

**Best for:** Any agent that needs to use tools, call APIs, search the web,
read files, or interact with external systems.

**Plain language:** "Think about what to do, do it, see what happened, think
about what to do next."

**Limitations:** Can get stuck in loops. Needs a clear termination condition.
Error recovery requires explicit handling.

---

### Plan-and-Execute
**What:** Generates a complete plan upfront (all steps), then executes each
step in sequence. Planning and execution are separate phases.

**Characteristics:**
- Front-loaded planning
- Sequential execution
- Plan is visible and auditable before execution begins

**Best for:** Well-defined tasks with known structure where the full plan can
be specified before any action is taken. Good for tasks requiring human approval
of the plan before execution.

**Plain language:** "Write the whole recipe before you start cooking."

**Limitations:** Brittle if the environment changes during execution (the plan
was made on stale assumptions). Requires replanning if steps fail.

---

### Plan-and-Solve
**What:** Improved variant of Plan-and-Execute. Generates the plan and executes
in a single, more integrated pass. Less separation between planning and doing.

**Characteristics:**
- Faster than Plan-and-Execute
- Less overhead for simpler tasks
- Plan is still generated, but execution begins immediately

**Best for:** Moderately complex tasks where the full Plan-and-Execute overhead
isn't warranted but pure ReAct would be too reactive.

**Plain language:** "Sketch the plan as you go, but keep moving."

---

### RAISE (ReAct + Memory)
**What:** ReAct with an explicit working memory (scratchpad) and long-term
memory store. The agent can write to and read from its scratchpad between
action steps.

**Characteristics:**
- All ReAct characteristics, plus
- Persistent scratchpad across steps
- Can retrieve from long-term store mid-task

**Best for:** Long-horizon tasks where context would otherwise overflow the
window. Complex multi-step workflows needing to track state.

**Plain language:** "ReAct, but with a notepad to keep track of what you've
figured out so far."

---

### Event-Driven (Reactive)
**What:** Agent sits idle until triggered by an event (message received, file
changed, threshold crossed, schedule fired). Responds to the event, then returns
to idle.

**Characteristics:**
- Passive until triggered
- Low resource use in idle state
- Response is bounded to the triggering event

**Best for:** Monitoring agents, notification agents, inbox managers, any agent
that reacts to the world rather than proactively pursuing a goal.

**Plain language:** "Wait and watch. When something happens, respond."

---

## Layer 3: Memory Strategies

### In-Context Memory
**What:** All information lives in the prompt window. No external storage.
Agent "forgets" between sessions.

**Use when:** Task is self-contained in a single session. Short tasks. Simple
agents. Cost-sensitive deployments.

**Ceiling:** Whatever fits in the context window.

---

### Scratchpad
**What:** Agent maintains a working memory document it reads from and writes to
during execution. Not persistent between sessions unless explicitly saved.

**Use when:** Multi-step tasks where the agent needs to track state, intermediate
results, or reasoning across many steps within a session.

---

### Retrieval-Augmented Generation (RAG)
**What:** Agent retrieves relevant information from an external knowledge base
on demand. Knowledge base is separate from the agent and can be updated
independently.

**Use when:** Domain knowledge is too large for the context window. Knowledge
needs to stay current (can update the KB without retraining). Agent needs
access to proprietary or specialized information.

---

### Episodic Memory
**What:** Agent stores and retrieves records of past interactions, decisions,
and outcomes. Can learn from experience across sessions.

**Use when:** Agent needs to remember what worked and what didn't. Personalization
over time. Agents that handle recurring tasks with the same users or systems.

---

### Shared Memory
**What:** Multiple agents read from and write to a common memory store.
Enables coordination without direct communication.

**Use when:** Multiple agents need access to the same state. Blackboard
coordination patterns. Platform-level memory shared across a squad.

---

## Layer 4: Coordination Strategies

### None (Solo)
**What:** Single agent, no coordination. Operates entirely independently.

**Use when:** Task is fully self-contained. Tier 0 / single-agent cases.

---

### Orchestrator-Subagent
**What:** One agent decomposes the goal and delegates tasks to specialist
subagents. Subagents report back. Orchestrator synthesizes results.

**Use when:** Complex goals that require multiple specialist capabilities.
Most multi-agent systems.

---

### Peer-to-Peer
**What:** Agents communicate directly with each other without a central
coordinator. Each agent decides when and what to share.

**Use when:** Collaborative problem-solving. Agents with overlapping but
distinct perspectives that need to integrate their views.

---

### Blackboard
**What:** Agents post to and read from a shared workspace (the "blackboard").
No direct agent-to-agent communication — all coordination happens through
the shared state.

**Use when:** Loosely coupled pipelines. Agents that produce outputs consumed
by downstream agents without knowing who they are. High-throughput systems.

---

### Voting / Ensemble
**What:** Multiple agents independently produce outputs. A judge (human or
agent) selects the best or synthesizes a combined answer.

**Use when:** High-stakes decisions needing diverse perspectives. Reducing
single-agent bias. QA and validation workflows.

---

### Debate
**What:** Two or more agents argue opposing positions or approaches. A judge
evaluates the arguments and decides.

**Use when:** Adversarial validation. Stress-testing proposals. Reducing
hallucination through opposition. Compliance review.

---

### Swarm
**What:** Many lightweight agents work in parallel toward a shared goal, each
handling a subset of the work. Results are aggregated.

**Use when:** High-volume, parallelizable tasks. Web crawling, batch processing,
large-scale research synthesis.

---

## The 7 Agent Profiles

These profiles are combinations of the four layers that map to recognizable
archetypes in real systems. Use them as starting points, not rigid constraints.

---

### Profile 0: The Assistant
*"Draft my email replies. Summarize this document. Schedule this meeting."*

| Layer | Strategy |
|---|---|
| Reasoning | CoT (or direct, no explicit reasoning) |
| Execution | Direct / Event-Driven |
| Memory | In-Context |
| Coordination | None (Solo) |

**Characteristics:** Single session, single task type, no tools or minimal tools,
no handoffs. The EA archetype.

**Tier:** 0 — single agent brief, no topology needed.

**Design output:** Agent Brief (role, context, tools, constraints, tone, escalation).

**Examples:** Email drafter, meeting summarizer, inbox monitor, document Q&A,
first-draft writer, scheduling assistant.

---

### Profile 1: The Researcher
*"Find everything about X, synthesize it, tell me what matters."*

| Layer | Strategy |
|---|---|
| Reasoning | ToT + Reflection |
| Execution | ReAct |
| Memory | RAG + Scratchpad |
| Coordination | Peer-to-Peer (optional: debate with a critic agent) |

**Characteristics:** Long-horizon, exploratory, tool-heavy (web search, document
retrieval), output is a synthesized artifact. May spawn sub-researchers.

**Tier:** 0–1 depending on scope. Single deep-research agent (Tier 0) or a
research squad with parallel subagents (Tier 1).

**Examples:** Competitive intelligence, due diligence, literature review,
market research, lead enrichment.

---

### Profile 2: The Executor
*"Run this workflow. Process this batch. Complete this defined task."*

| Layer | Strategy |
|---|---|
| Reasoning | Plan-and-Execute |
| Execution | ReAct |
| Memory | Scratchpad |
| Coordination | Subagent (receives delegation) |

**Characteristics:** Well-defined task with known structure. Follows a plan.
Reliable and auditable. Typically a subagent receiving work from an Orchestrator.

**Tier:** 1 — part of a Squad Topology, typically a stream-aligned squad member.

**Examples:** Lead scoring, content publishing, CRM sync, report generation,
data transformation.

---

### Profile 3: The Orchestrator
*"Take this goal. Figure out what needs to happen. Make it happen."*

| Layer | Strategy |
|---|---|
| Reasoning | Least-to-Most |
| Execution | Plan-and-Solve |
| Memory | Shared Memory |
| Coordination | Orchestrator-Subagent |

**Characteristics:** Decomposes complex goals into subtasks. Routes to the right
agents. Synthesizes results. Tracks overall progress. No domain work of its own.

**Tier:** 1 — the Orchestrator squad type in Squad Topologies.

**Examples:** Campaign manager, sales sequence coordinator, product release
orchestrator, multi-step research director.

---

### Profile 4: The Validator
*"Check this. Is it correct? Is it safe? Does it meet the standard?"*

| Layer | Strategy |
|---|---|
| Reasoning | Self-Consistency + Debate |
| Execution | Reflection |
| Memory | External (rulebook / compliance corpus) |
| Coordination | Voting / Ensemble |

**Characteristics:** Adversarial by design. Runs multiple passes. Has access
to ground truth or compliance rules. Output is a verdict, not a product.

**Tier:** 1 + SAFe augmentation in regulated industries.

**Examples:** QA agent, compliance reviewer, hallucination detector, legal
check, code security scanner.

---

### Profile 5: The Sentinel
*"Watch this. Tell me when something is wrong."*

| Layer | Strategy |
|---|---|
| Reasoning | Step-Back |
| Execution | Event-Driven (Reactive) |
| Memory | Episodic |
| Coordination | Blackboard |

**Characteristics:** Passive until triggered. Maintains a model of "normal"
and alerts on deviation. Long-running, low-resource in idle state. Needs a
reclaim path — detection without correction is just logging.

**Tier:** 1 — the Sentinel squad type in Squad Topologies.

**Examples:** Cost overrun monitor, drift detector, safety boundary watcher,
reply classifier, uptime monitor.

---

### Profile 6: The Swarm Unit
*"Process this at scale. In parallel. Fast."*

| Layer | Strategy |
|---|---|
| Reasoning | CoT (lightweight) |
| Execution | Direct |
| Memory | Shared (blackboard) |
| Coordination | Swarm + Blackboard |

**Characteristics:** One of many identical (or near-identical) lightweight agents
working in parallel. Each handles a slice of a large workload. Results aggregated
centrally.

**Tier:** 1 + unFIX augmentation for dynamic crew scaling.

**Examples:** Bulk lead enrichment, large-scale web crawling, parallel document
processing, batch content generation.

---

## Profile → Tier → Framework Mapping

| Profile | Tier | Primary Framework | Common Augmentation |
|---|---|---|---|
| 0: Assistant | 0 | Agent Brief only | — |
| 1: Researcher | 0–1 | Agent Brief or Squad Topologies | DDD (knowledge boundaries) |
| 2: Executor | 1 | Squad Topologies | DDD (bounded context) |
| 3: Orchestrator | 1 | Squad Topologies | unFIX (if dynamic crews needed) |
| 4: Validator | 1 | Squad Topologies | SAFe (regulated contexts) |
| 5: Sentinel | 1 | Squad Topologies | SAFe (reclaim authority) |
| 6: Swarm Unit | 1 | Squad Topologies | unFIX (dynamic scaling) |
