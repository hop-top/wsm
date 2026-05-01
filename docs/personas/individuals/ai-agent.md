# Persona: AI Agent (wsm Extension)

**Primary Role:** Autonomous agent running inside an AI CLI
(claude, codex, copilot, gemini, opencode, cursor, ...) that
records its own activity into wsm and reads workspace state
back to maintain continuity across sessions, machines, and
parent/child agent dispatches.

---

## Base Goals

- Discover available capabilities at runtime via structured
  metadata.
- Execute multi-step workflows with predictable,
  machine-readable responses at every boundary.
- Maintain session continuity across interruptions without
  losing accumulated context or partial results.
- Minimise token/context-window spend per task while
  preserving decision-relevant information.

## Base Pain Points

- Unstructured output forces brittle parsing.
- Breaking schema changes destroy in-flight workflows.
- No standard way to resume a partially completed workflow
  after a crash.

---

## wsm Interaction Patterns

### Record activity from inside the agent loop

```pseudocode
// every message
wsm event record <ws-id> interaction.message --json '{
  "role": "assistant", "content": "..."
}'

// every tool call
wsm event record <ws-id> interaction.tool_call --json '{
  "tool": "Bash", "input": "...", "duration_ms": 123
}'

// every file mutation
wsm event record <ws-id> mutation.file_patch --json '{
  "path": "src/foo.go", "op": "edit"
}'
```

### Resume after crash via event replay

```pseudocode
// agent crashed mid-task; on restart:
wsm inspect <ws-id> --events-all --json
  | jq '[.[] | select(.timestamp > $checkpoint)]'
// replay tail to reconstruct state
```

### Cross-machine handoff via snapshot

```pseudocode
// finish work on laptop
wsm inspect <ws-id> --json > /tmp/handoff.json

// pick up on desktop after sync
wsm import < /tmp/handoff.json
wsm inspect <new-ws-id> --events-all --json
// agent reads recent events, resumes from checkpoint
```

### Subagent dispatch

```pseudocode
// parent agent records dispatch
wsm event record <ws-id> agent.subagent_dispatched --json '{
  "parent_session_id": "$current",
  "child_task": "audit dep tree"
}'
// child agent records its work in same workspace, tagged with
// parent_session_id so the lineage is queryable later
```

---

## User Stories

- As an AI agent, I record every message, tool call, and
  file patch into wsm via `RecordEvent` so my session
  history is durable and queryable across IDE restarts.
- As an AI agent, I resume after a crash by reading
  `wsm inspect <id> --events-all --json` and replaying
  events newer than my last checkpoint, instead of losing
  partial progress.
- As an AI agent, I read `WSM_BACKEND` and `WSM_DATA_DIR`
  env vars to discover where to read/write workspace state
  without per-machine configuration drift.
- As an AI agent dispatching a subagent, I record the
  parent → child link as a typed event so the audit log
  reconstructs the full dispatch tree.
- As an AI agent, every `--json` output validates against a
  versioned schema so I never parse human-readable tables
  to drive my next decision.
- As an AI agent crossing machine boundaries, I use
  `ExportWorkspace` / `ImportWorkspace` with stable IDs so
  my context survives the laptop-to-desktop hop.

---

## Success Metrics

- 100% of agent-emitted events validate against documented
  type schemas (`interaction.*`, `mutation.*`, `device.*`,
  `agent.*`).
- Resume-after-crash round-trip (read events → reconstruct
  state) completes in < 2s for a 1000-event workspace.
- Cross-machine snapshot delta size < 100KB for a typical
  day's worth of agent activity (event payloads kept lean).
- Zero breaking changes to event-payload schemas per minor
  release; deprecations announced one release ahead.

---

## Collaboration with Other Personas

- **Operations Manager (wsm):** ops manager audits exactly
  the events agents emit; agents must keep schemas stable
  for the audit trail to remain parseable.
- **System Integrator (wsm):** integrator builds the IDE
  adapters that translate CLI-specific events into
  canonical wsm event types agents can rely on.
- **Solo Developer (wsm):** the user behind the agent;
  triggers tasks, inspects workspaces, archives finished
  work.
