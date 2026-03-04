# Squad Topologies Architect — Quick Start

## What This Skill Does

This skill facilitates a structured design session to help you identify, define,
and design the agent squads for your business or product.

At the end of a session, you get:
- **Squad Registry** — one card per squad with type, purpose, inputs, outputs, and context boundary
- **Interaction Contract Map** — how every squad pair communicates
- **BlockDiag diagram** — the full topology, renderable at blockdiag.com
- **Design Decisions Log** — what was decided and why
- **Next Steps** — what to build first

## How to Start a Session

Just describe your business or what you're building. The skill will take it from there.

Examples:
- "I run a recruitment agency and want to automate candidate sourcing and screening."
- "We're building a SaaS product that helps e-commerce companies manage returns."
- "Here's our current agent setup: [description]. Can you review it?"

## What to Expect

The session runs in 7 phases:
1. **Orientation** — the skill calibrates to your level (technical or non-technical)
2. **Business Discovery** — understanding your business and its workflows
3. **Value Stream Mapping** — identifying the main flows that agents will serve
4. **Squad Identification** — proposing and validating the squad structure
5. **Context Boundary Definition** — making sure each squad has a clear, bounded scope
6. **Interaction Mode Design** — defining how squads communicate
7. **Output Generation** — producing the full documented topology

The skill will ask you questions one or two at a time — it should feel like a
good consulting conversation, not a form to fill out.

## Rendering the BlockDiag Output

The skill generates a `.diag` file. To render it:

**Online:** blockdiag.com/en/blockdiag/demo.html (paste the file contents)

**Local:**
```bash
pip install blockdiag
blockdiag -Tsvg squad-topology.diag -o squad-topology.svg
```

**VS Code:** Install the `blockdiag` extension for inline rendering.

## Framework Reference

This skill implements the Squad Topologies framework — an adaptation of
Team Topologies (Skelton & Pais) for autonomous agent systems.

The 6 squad types:
- **Stream-Aligned** — owns a domain pipeline end-to-end
- **Enabling** — temporary capability uplift (has an exit condition)
- **Complicated Subsystem** — hard problem behind a clean interface
- **Platform** — shared substrate consumed by all squads
- **Orchestrator** — goal decomposition and routing (no domain ownership)
- **Sentinel** — continuous observation and drift detection

The 4 interaction modes:
- **X-as-a-Service** — stable, versioned, no coordination (default target)
- **Collaboration** — bidirectional, time-boxed, for unknowns
- **Facilitating** — enabling squad teaches, then exits
- **Supervised Delegation** — orchestrator delegates but retains oversight
