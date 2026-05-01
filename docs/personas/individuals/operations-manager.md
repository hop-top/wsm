# Persona: Operations Manager (wsm Extension)

**Primary Role:** Team lead / squad operator who treats wsm
workspaces as the system-of-record for what happened in a
project. Uses event logs as audit trail, snapshots as
period-over-period checkpoints, and `wsm list --status=...`
as the daily standup view.

---

## Base Goals

- Maintain real-time visibility into automation health and
  throughput.
- Keep cost-per-automation within budget; demonstrate ROI.
- Ensure SLOs are met across all agent-driven workflows.

## Base Pain Points

- Metrics scattered across systems; no unified operational view.
- Hard to attribute outcomes to specific automations.
- Alert fatigue from noisy, uncorrelated notifications.

---

## wsm Interaction Patterns

### Daily standup view

```pseudocode
// Active workspaces across the squad
wsm list --status=active --json
  | jq '.[] | {name, last_event_at, event_count}'

// Drill into one
wsm inspect <ws-id> --events --json
  | jq '[.[] | select(.type == "interaction.tool_call")] | length'
```

### Period-over-period audit

```pseudocode
// Snapshot at end of sprint
wsm inspect <ws-id> > snapshots/sprint-42.txt

// Diff vs previous sprint
diff snapshots/sprint-41.txt snapshots/sprint-42.txt
```

### Archive completed work

```pseudocode
// Freeze workspace once project ships; remains queryable
wsm archive <ws-id>

// Surface only live work in standup
wsm list --status=active
```

---

## User Stories

- As an ops manager, I run `wsm list --status=active --json`
  every morning so I see all in-flight workspaces in one
  scriptable view, no per-IDE plumbing.
- As an ops manager, I export full event history with
  `wsm inspect <id> --events-all --json` so I have an audit
  trail when leadership asks "what changed last sprint?".
- As an ops manager, I `wsm archive <id>` finished projects
  so the active list stays focused on current work, without
  losing historical queryability.
- As an ops manager, I count tool-call events per workspace
  via `jq` over `--events-all --json` so I track which
  automations are actually being used vs. shelfware.
- As an ops manager, I rely on stable exit codes (0/2/3/4/5)
  so my standup script reliably distinguishes "workspace
  not found" from "backend error" without parsing stderr.

---

## Success Metrics

- Daily standup script runs in < 2s end-to-end across a
  typical squad's active workspace set.
- Event audit query (`--events-all --json`) returns full
  history for a 6-month workspace in < 5s.
- Zero workspace state ambiguity per sprint: every workspace
  is either `active` or `archived`, never lost.
- Stable contract: 100% of `--json` outputs parse against
  documented schema across releases.

---

## Collaboration with Other Personas

- **System Integrator (wsm):** ops manager consumes the event
  streams that integrators wire from external systems
  (CI, tlc, IDE adapters) into wsm.
- **AI Agent (wsm):** agents emit the events ops manager
  audits; consistent event schemas keep the audit trail
  parseable.
