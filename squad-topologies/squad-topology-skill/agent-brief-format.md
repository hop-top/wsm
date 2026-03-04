# Agent Brief Format
## Output format for Tier 0 single-agent cases

When the triage determines a single agent is sufficient (no topology needed),
produce an Agent Brief — not a Squad Registry or BlockDiag.

---

## Agent Brief Template

```markdown
# Agent Brief: [Agent Name]
Generated: [date]

---

## Identity

**Name:** [descriptive name, e.g., "Email Reply Drafter"]
**Profile:** [Assistant / Researcher / Executor / Sentinel — from agent-strategies.md]
**Owner:** [who is responsible for this agent]

---

## Purpose

[One paragraph. What does this agent do? What problem does it solve?
What would break if it didn't exist?]

---

## Reasoning & Execution Strategy

**Reasoning:** [CoT / ToT / Reflection / etc. — and why]
**Execution:** [Direct / ReAct / Plan-and-Execute / Event-Driven — and why]
**Memory:** [In-Context / Scratchpad / RAG / Episodic — and why]

---

## Context

What this agent needs to know to do its job:

- [Domain knowledge, e.g., "Jad's communication style and tone preferences"]
- [Relevant background, e.g., "Current open deals and their status"]
- [Standing instructions, e.g., "Always reply within 24 hours of receipt"]

What this agent does NOT need to know:
- [Explicitly out of scope]

---

## Tools

| Tool | Purpose | Permission level |
|---|---|---|
| [e.g., Gmail read] | Read inbound emails | Read-only |
| [e.g., Gmail send] | Send drafted replies | Requires approval before send |
| [e.g., Calendar] | Check availability for scheduling | Read-only |

---

## Inputs

**Trigger:** [What starts this agent? Inbound email / scheduled time / manual / event]
**Input format:** [What does it receive? Raw email / structured brief / file / etc.]

---

## Outputs

**Output:** [What does it produce? Draft reply / summary / structured data / action]
**Output format:** [Where does the output go? Drafts folder / Notion / Slack / etc.]
**Recipient:** [Who or what receives the output?]

---

## Tone & Authority

**Tone:** [e.g., Professional and warm. Match the tone of the incoming email.]
**Voice:** [e.g., First person as Jad. Never refer to itself as an AI.]
**Authority level:** [e.g., Can draft but NOT send. All replies require human approval.]

---

## Escalation Conditions

Escalate to human when:
- [e.g., Email contains legal, financial, or contractual language]
- [e.g., Sender is a tier-1 client or investor]
- [e.g., Reply requires a commitment the agent has no context for]
- [e.g., Emotional tenor is negative or confrontational]

Escalation path: [e.g., Flag in Slack #agent-escalations with a 1-line summary]

---

## Constraints

This agent must NEVER:
- [e.g., Send an email without human approval]
- [e.g., Refer to internal pricing or strategy]
- [e.g., Make commitments on behalf of the business]
- [e.g., Respond to emails marked confidential]

---

## Success Criteria

This agent is working well when:
- [e.g., 80%+ of drafted replies require only minor edits before sending]
- [e.g., Escalation rate is under 20% of inbound emails]
- [e.g., Average draft time is under 60 seconds]

---

## Evolution Path

When this agent might need to grow into a squad:
- [e.g., If email volume exceeds X/day and draft quality starts dropping]
- [e.g., If the scope expands to include outbound prospecting as well as replies]

At that point: revisit with Squad Topologies Architect skill.
```

---

## Notes on Agent Brief Quality

**The brief should be operational, not aspirational.** Every section should
contain specific, actionable information — not vague goals.

**Constraints are as important as capabilities.** An agent without explicit
constraints will eventually do something unexpected. Every "must NEVER" is
a failure mode you've prevented.

**Escalation conditions prevent the worst outcomes.** The cases where a human
must be in the loop should be named explicitly, not left to the agent's judgment.

**Evolution path closes the loop.** If the agent grows beyond its brief,
the owner should know when to re-engage the triage process.
