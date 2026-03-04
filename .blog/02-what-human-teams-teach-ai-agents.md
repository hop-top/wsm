# What Human Teams Teach AI Agents

*Part 2 of the Squad Topologies series*

---

Your agents need organizational design more than your humans do.

That sounds backward. Humans have personalities, egos, office politics. Agents just execute code. But that's exactly the problem. Humans have intuition for coordination. Agents have nothing but what you explicitly give them.

This is where Team Topologies comes in—and why it matters more for AI than it ever did for people.

## Team Topologies: A Quick Primer

In 2019, Matthew Skerrett and the team at Red Badger published *Team Topologies*, a book that changed how organizations think about team structure. The core insight: stop drawing org charts based on hierarchy. Design teams based on flow of value.

They identified **four fundamental team types**:

| Type | Purpose |
|------|---------|
| **Stream-aligned** | Deliver value directly to users. Own the full pipeline. |
| **Enabling** | Help other teams gain capabilities. Temporary, with exit conditions. |
| **Complicated-subsystem** | Own specialized domains that need deep expertise. |
| **Platform** | Provide a self-service substrate for other teams. |

And **three interaction modes** between them:

| Mode | Characteristics |
|------|-----------------|
| **Collaboration** | Working together, bidirectional. Time-boxed. For exploring unknowns. |
| **X-as-a-Service** | One consumes what another provides. Minimal coordination. Stable, long-term. |
| **Facilitating** | One helps another learn. One-way, time-boxed. Enabling teams use this. |

The rule: every team-to-team interaction should have a declared mode. No ambiguity. No "we'll figure it out."

## Why Conway's Law Hits AI Harder

Conway's Law: "Any organization that designs a system will produce a design whose structure is a copy of the organization's communication structure."

For humans, this is a tendency. Teams talk to each other informally. Shared understanding emerges from conversation. Two developers who sit near each other will naturally coordinate on overlapping work, even if their official chart says nothing about it.

Agents have no informal channels. No hallway conversations. No "hey, are you working on that file?" They only know what's in their context window and what's explicitly programmed into their behavior.

This makes Conway's Law not a tendency but a hard constraint. If you don't design the communication structure, there is no communication structure. Chaos is guaranteed, not just likely.

## The Inverse Conway Maneuver

Team Topologies recommends an Inverse Conway Maneuver: design the team structure you want, and let the software architecture evolve to match it.

For agents, this is the only viable approach. You cannot wait for communication patterns to emerge organically—they won't. You must declare the topology upfront:

1. What squad types exist
2. Which agents belong to which squad
3. What interaction mode governs each squad pair
4. What interfaces they use to communicate

The topology becomes a contract. Agents don't improvise. They follow the contract.

## The Missing Pieces: Orchestrator and Sentinel

Team Topologies identified four types. Multi-agent systems need six.

### Orchestrator

In human organizations, coordination often happens through hierarchy. A manager decomposes a goal, assigns subtasks, and synthesizes results.

Agents need this too, but it's not a person. It's a specialized squad type:

**Orchestrator squads** receive high-level goals, decompose them into assignable tasks, route those tasks to appropriate squads, and synthesize the results.

They don't build. They coordinate. Their output isn't code—it's task assignments and synthesized deliverables.

### Sentinel

Humans have gut feelings. Something feels off. A metric drifts. A process that used to work starts failing silently.

Agents need explicit drift detection:

**Sentinel squads** observe the system continuously, detect deviations from expected patterns, and raise flags when intervention is needed.

They don't fix. They watch. Their output is signals: compliance violations, performance regressions, interaction pattern anomalies.

## The Six Squad Types

Every agent in your system should belong to exactly one squad. Every squad should have exactly one type. The types are exhaustive—you shouldn't need to invent a seventh.

But before we define them, let's make this concrete with examples from industries that have nothing to do with software.

## The Same Patterns, Different Industries

These organizational patterns aren't unique to tech. They exist wherever work needs to be divided, coordinated, and delivered.

### Real Estate Agency

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Listing Agent | Owns the client relationship from first call to closing. Handles showings, offers, negotiations, paperwork. |
| **Enabling** | New Agent Mentor | Shadows new agents for 90 days, teaches scripts, reviews contracts. Exits when the new agent hits quota. |
| **Complicated-subsystem** | Mortgage Underwriter | Specialized expertise in loan approval. Other agents don't touch this—files get handed off and returned with a decision. |
| **Platform** | MLS + CRM Team | Maintains the listing database, agent tools, compliance templates. Self-service. Agents don't call them; they use what's provided. |
| **Orchestrator** | Broker-in-Charge | Complex commercial deals need coordination across multiple agents, attorneys, inspectors. The broker decomposes the deal, assigns roles, synthesizes results. |
| **Sentinel** | Compliance Auditor | Reviews closed files for regulatory violations. Doesn't sell. Watches. Flags problems before the state board does. |

### Franchise Restaurant Chain

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Shift Team | Takes orders, cooks food, serves customers. Owns the full customer experience during their shift. |
| **Enabling** | Training Squad | New hires spend two weeks here before joining a shift team. Temporary. Exit when trainee is certified. |
| **Complicated-subsystem** | Equipment Repair | The fryer breaks. Shift team doesn't fix it. Specialized technicians handle it. Clean interface: broken → fixed. |
| **Platform** | Corporate Supply Chain | Provides ingredients, uniforms, POS system, marketing materials. Stores consume. Minimal coordination. |
| **Orchestrator** | Area Manager | Three stores need to coordinate a regional promotion. Area manager decomposes the campaign, assigns store-specific tasks, synthesizes results. |
| **Sentinel** | Mystery Shopper / Quality Assurance | Visits unannounced. Scores against standards. Flags drift before it becomes brand damage. |

### Beauty Salon

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Stylist | Consults, cuts, styles, checks out. Owns the client from walk-in to payment. |
| **Enabling** | Senior Stylist (Mentor Role) | Juniors shadow for advanced techniques—balayage, precision cuts. Teaches, then steps back. |
| **Complicated-subsystem** | Color Specialist | Complex color corrections. Other stylists refer these clients. Specialized chemistry knowledge. |
| **Platform** | Front Desk + Inventory | Booking system, product retail, supply ordering. Stylists don't manage inventory; they use what's stocked. |
| **Orchestrator** | Salon Manager | Wedding party of eight needs simultaneous coordination. Manager assigns stylists, sequences timing, ensures cohesive results. |
| **Sentinel** | Health & Safety Inspector (or Internal Auditor) | Checks sanitation logs, licensing, chemical storage. Doesn't cut hair. Ensures compliance. |

### Law Firm

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Client Partner + Associate Team | Owns the client relationship end-to-end. Intake, strategy, filings, court appearances, settlement. |
| **Enabling** | Knowledge Management Team | Teaches associates how to use precedent databases, write briefs, navigate procedures. Exits when the skill is transferred. |
| **Complicated-subsystem** | Tax Specialist / IP Counsel | Deep expertise in narrow domains. M&A team hands off tax questions; gets back a memo. Clean interface. |
| **Platform** | Paralegal Pool + Document Services | Templates, filing systems, court calendaring. Lawyers consume self-service. Don't coordinate; they use. |
| **Orchestrator** | Lead Counsel (Complex Litigation) | Multi-defendant case with parallel tracks. Decomposes strategy, assigns motions to sub-teams, synthesizes for trial. |
| **Sentinel** | Conflicts Committee | Reviews every new engagement for ethical conflicts. Doesn't practice law. Watches. Blocks problems before they become malpractice. |

### Accounting Firm

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Audit Engagement Team | Owns the client audit from planning letter to final opinion. Fieldwork, testing, report. |
| **Enabling** | Technical Accounting Training | New standards roll out (ASC 606, leases). Team teaches engagement teams until adoption is complete. Then exits. |
| **Complicated-subsystem** | Valuation Specialists | Fair value estimates for complex instruments. Engagement teams refer; specialists deliver the number. |
| **Platform** | Audit Software + Methodology Team | Templates, sampling tools, workpaper standards. Engagement teams consume. Minimal back-and-forth. |
| **Orchestrator** | Lead Partner (SEC Filing) | Coordinating audit, tax, and advisory teams on a single client. Decomposes the 10-K workstream, assigns, synthesizes. |
| **Sentinel** | Quality Control Reviewer | Reviews completed workpapers before signing. Flags deficiencies. Doesn't do the audit—watches for errors. |

### Import/Export Business

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Account Manager | Owns the customer relationship. Quote, order, shipment tracking, delivery confirmation. End-to-end. |
| **Enabling** | Trade Compliance Trainer | New regulations (tariff codes, sanctions lists). Trains account managers until compliant. Temporary. |
| **Complicated-subsystem** | Customs Broker | Specialized knowledge of HS codes, duty rates, country-of-origin rules. Handoff: shipment → cleared goods. |
| **Platform** | Warehouse + Logistics Network | Storage, picking, packing, carrier contracts. Account managers book; they don't manage the warehouse. |
| **Orchestrator** | Operations Director (Multi-Country Shipment) | Complex project: factory in Vietnam, transshipment in Singapore, final delivery in Brazil. Decomposes, routes, synthesizes. |
| **Sentinel** | Compliance Auditor | Reviews shipments for denied-party matches, tariff fraud. Doesn't sell. Watches. Flags before customs does. |

### Retail Store

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Department Sales Team | Greets customer, helps selection, processes sale. Owns the transaction from "can I help you" to receipt. |
| **Enabling** | Visual Merchandising Trainer | New season, new displays. Teaches team the standards. Exits when displays are consistent. |
| **Complicated-subsystem** | Loss Prevention | Specialized expertise in theft detection, investigation. Sales team refers; LP handles. Clean handoff. |
| **Platform** | Stockroom + POS Team | Inventory replenishment, register maintenance, receipt systems. Sales team consumes. Self-service. |
| **Orchestrator** | Store Manager (Black Friday) | Coordinating floor coverage, register backups, stockroom flow. Decomposes the day, assigns, adjusts in real-time. |
| **Sentinel** | District Auditor | Inventory counts, cash reconciliation, policy compliance. Doesn't sell. Verifies accuracy. |

### Medical Clinic

| Squad Type | Who | What They Do |
|------------|-----|--------------|
| **Stream-aligned** | Primary Care Provider | Patient intake, diagnosis, treatment, follow-up. Owns the patient journey within their scope. |
| **Enabling** | Clinical Educator | New protocol rollout (e.g., updated hypertension guidelines). Trains providers until adopted. Exits. |
| **Complicated-subsystem** | Radiologist / Pathologist | Specialized interpretation. PCP orders; specialist returns a reading. Doesn't see the patient directly. |
| **Platform** | EHR Team + Lab Services | Scheduling, records, phlebotomy, pharmacy coordination. Providers consume. Don't coordinate each blood draw. |
| **Orchestrator** | Care Coordinator (Complex Case) | Patient with diabetes, kidney disease, and depression. Coordinating PCP, endocrinologist, nephrologist, social worker. Decomposes care plan, routes, synthesizes. |
| **Sentinel** | Quality Assurance / Peer Review | Reviews charts for missed diagnoses, inappropriate prescribing. Doesn't treat. Flags. Improves system. |

### The Pattern Holds

Nine industries. Same six structures.

| Type | Core Behavior |
|------|---------------|
| **Stream-aligned** | Owns end-to-end delivery to the customer |
| **Enabling** | Teaches, then exits. Has a finish line. |
| **Complicated-subsystem** | Deep expertise, narrow scope, clean handoff interface |
| **Platform** | Self-service foundation. Others consume without asking. |
| **Orchestrator** | Decomposes complexity, routes work, synthesizes outputs |
| **Sentinel** | Watches. Flags. Doesn't fix—that's someone else's job. |

If a hair salon needs these patterns, your multi-agent system does too. The difference is that the salon's manager can walk over and clarify confusion. Your agents can't. They need the topology to be explicit.

## From Org Chart to Agent Graph

Here's how to translate this to your agent workflow:

1. **List your agents** — Every distinct agent profile in your system
2. **Group by responsibility** — Which agents naturally work on related things?
3. **Assign squad types** — Is this group stream-aligned? Platform? Enabling?
4. **Declare interactions** — For every pair of squads, pick an interaction mode
5. **Define interfaces** — What does X-as-a-Service look like between these two?
6. **Document evolution rules** — When does collaboration graduate to x-as-a-service?

The output is a topology: a directed graph where nodes are squads (with types) and edges are interaction modes (with contracts).

This graph becomes the source of truth. Not the chat history. Not the task list. The topology.

## What's Next

You now have a mental model: six squad types, four interaction modes, one explicit topology.

Next post, we'll go deep on each squad type—when to use them, what they look like in practice, and how to avoid the anti-patterns that break them.

[Continue to Part 3: The Six Squad Types, Explained →](03-the-six-squad-types-explained.md)

---

*This post is part of the Squad Topologies series. [← Back to Part 1](01-hidden-cost-of-multi-agent-chaos.md)*
