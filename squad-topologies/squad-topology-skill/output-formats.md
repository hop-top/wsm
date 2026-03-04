# Output Formats Reference

When the design session is complete, generate these files.

---

## 1. Squad Registry (`squad-registry.md`)

One card per squad. Use this template:

```markdown
# Squad Registry
Generated: [date]
Business: [business name]

---

## [Squad Name]

**Type:** [Stream-Aligned | Enabling | Complicated Subsystem | Platform | Orchestrator | Sentinel]
**Domain:** [one-line domain description]
**Purpose:** [one sentence — what does this squad produce?]

### Inputs
- [what triggers this squad / what it receives]

### Outputs
- [what it produces / delivers]

### Context Boundary
What this squad knows:
- [domain knowledge it holds]

What this squad does NOT own:
- [explicitly out of scope]

### Interaction Contracts
| Interacts with | Mode | Direction | Notes |
|---|---|---|---|
| [Squad Name] | [X-as-a-Service / Collaboration / Facilitating / Supervised Delegation] | [→ / ← / ↔] | [brief note] |

### Warning Signs to Watch
- [specific degradation signal for this squad]
- [specific drift pattern to sentinel-monitor]

---
```

---

## 2. Interaction Contract Map (`interaction-contracts.md`)

```markdown
# Interaction Contract Map
Generated: [date]

## All Squad Pairs

| From | To | Mode | Notes |
|---|---|---|---|
| [Squad A] | [Squad B] | X-as-a-Service | [Squad A delivers X to Squad B] |
| [Orchestrator] | [Squad C] | Supervised Delegation | [task scope, reclaim condition] |
| [Enabling] | [Squad D] | Facilitating | [capability: X, exit condition: Y] |

## Collaboration Engagements (time-boxed)
| Squad A | Squad B | Purpose | End Condition | Target Mode |
|---|---|---|---|---|
| ... | ... | ... | ... | X-as-a-Service |

## Notes
- [any cross-cutting observations about the interaction map]
```

---

## 3. BlockDiag Diagram (`squad-topology.diag`)

Generate valid BlockDiag syntax. Use the color conventions:

```
stream:    color="#e8fff3", textcolor="#005e33", linecolor="#00c46a"
enabling:  color="#eef0ff", textcolor="#2a35a0", linecolor="#5b6eff"
subsystem: color="#fff0f4", textcolor="#8a0030", linecolor="#e8366b"
platform:  color="#fff5e8", textcolor="#7a3d00", linecolor="#e07b00"
orch:      color="#e8f8ff", textcolor="#004d66", linecolor="#00b8d9"
sentinel:  color="#fffbe8", textcolor="#5a4500", linecolor="#c9a800"
```

Edge styles by mode:
```
X-as-a-Service:        style=solid,   color="#e07b00"
Collaboration:         style=dashed,  color="#5b6eff"  (bidirectional: <->)
Facilitating:          style=dotted,  color="#00c46a"
Supervised Delegation: style=solid,   color="#00b8d9"
```

Template:
```
blockdiag {
  orientation = portrait;
  default_shape = roundedbox;
  default_fontsize = 11;

  group orchestrators {
    label = "ORCHESTRATOR";
    // orchestrator squads
  }

  group stream {
    label = "STREAM-ALIGNED — value flow";
    // stream squads
  }

  group support {
    label = "SUPPORT LAYER";
    // enabling + subsystem squads
  }

  group platform {
    label = "PLATFORM — substrate";
    // platform squad
  }

  group sentinel {
    label = "SENTINEL — continuous observation";
    // sentinel squad
  }

  // Edges with mode labels
  SA1 -> SA2 [ label="X-as-a-Service\n[what is delivered]", style=solid, color="#e07b00" ];
  EN1 -> SA1 [ label="Facilitating\n[capability: X]", style=dotted, color="#00c46a" ];
}
```

---

## 4. Design Decisions Log (`design-decisions.md`)

```markdown
# Design Decisions Log
Generated: [date]

## Decision 1: [Squad name or design choice]
**Decision:** [What was decided]
**Rationale:** [Why this choice was made]
**Alternatives considered:** [What else was on the table]
**Confirmed by:** User (session date: [date])

---
```

---

## 5. Next Steps (`next-steps.md`)

```markdown
# Next Steps
Generated: [date]

## Build Priority

### Phase 1 — Foundation (build first)
1. [Platform squad] — everything else depends on this
2. [Most critical stream squad] — [why first]

### Phase 2 — Core Value Flows
3. [Next stream squad]
4. [Complicated subsystem if needed]

### Phase 3 — Coordination & Safety
5. [Orchestrator squad if needed]
6. [Sentinel squad]

## Enabling Engagements to Schedule
| Stream Squad | Enabling Squad | Capability | Exit Condition | Suggested Duration |
|---|---|---|---|---|

## Collaboration Engagements to Time-Box
| Squad A | Squad B | Purpose | Suggested Duration | Graduation Target |
|---|---|---|---|---|

## Warning Signs to Monitor
- [specific signal] → [specific action]

## Open Questions
- [anything unresolved from the design session]
```
