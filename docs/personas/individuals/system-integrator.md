# Persona: System Integrator (wsm Extension)

**Primary Role:** Engineer wiring wsm into existing dev
infrastructure (IDE adapters, CI pipelines, tlc tracks,
agent runtimes). Uses `pkg/workspace.Manager` as the embed
seam; implements `backend.Backend` for custom storage; uses
the conformance suite to validate.

---

## Base Goals

- Connect automation layer to existing systems with minimal
  disruption to production workloads.
- Guarantee data consistency across system boundaries.
- Build resilient integrations that degrade gracefully on
  upstream failures.

## Base Pain Points

- Every system has a different data model; mapping is
  tedious and error-prone.
- Partial failures leave data in inconsistent state.
- Vendor APIs change without notice; integrations break
  silently.

---

## wsm Interaction Patterns

### Embed wsm as a Go module

```pseudocode
import "hop.top/wsm/pkg/workspace"
import "hop.top/wsm/pkg/backend/sqlite"

mgr := workspace.NewManager(sqlite.New(dataDir))
ws, _ := mgr.Create(ctx, "build-pipeline", workspace.Opts{
    Tags: []string{"ci", "release"},
})
mgr.RecordEvent(ctx, ws.ID, "interaction.tool_call",
    map[string]any{"tool": "go-build", "exit": 0})
```

### Implement a custom backend

```pseudocode
// implement backend.Backend interface (Postgres, S3, Git, ...)
type MyBackend struct { /* ... */ }

func (b *MyBackend) Append(ctx, wsID, event) error { /* ... */ }
func (b *MyBackend) Replay(ctx, wsID) ([]Event, error) { /* ... */ }
// ... rest of interface

// run conformance suite to validate
go test ./pkg/backend/conformance -backend=mybackend
```

### Wire IDE / CLI adapter

```pseudocode
// detect session from IDE event stream, push to wsm
adapter := claudecode.NewAdapter(mgr)
adapter.OnMessage(func(msg) {
    mgr.RecordEvent(ctx, wsID, "interaction.message", msg)
})
adapter.OnToolCall(func(tc) {
    mgr.RecordEvent(ctx, wsID, "interaction.tool_call", tc)
})
```

### Export / import for handoff

```pseudocode
// Export workspace YAML for cross-machine handoff
data, _ := mgr.ExportWorkspace(ctx, wsRef)
// Import on target machine; opts handle name conflicts
mgr.ImportWorkspace(ctx, data, ImportOptions{
    Strategy: "rename",
    DryRun:   false,
})
```

---

## User Stories

- As a system integrator, I `import hop.top/wsm/pkg/workspace`
  and embed `Manager` in my CI tool so workspace events flow
  from build pipelines into wsm without me building event
  storage from scratch.
- As a system integrator, I implement `backend.Backend` for
  Postgres and validate it via the conformance test suite
  so I know my custom backend honours the contract before
  shipping.
- As a system integrator, I write IDE adapters
  (claude/codex/cursor/...) that push session events into
  wsm via `RecordEvent` so users see one timeline across
  all their AI CLIs.
- As a system integrator, I use `ExportWorkspace` /
  `ImportWorkspace` with `Strategy: rename` so users can
  hand off workspace state across machines without name
  collisions.
- As a system integrator, the `pkg/*` API never imports
  TUI/presentation libs (enforced by `verify-no-ui-leak`)
  so embedding wsm in a headless service keeps my binary
  small and dependency-clean.

---

## Success Metrics

- Conformance suite catches 100% of backend contract
  violations; no production backend ships without passing.
- IDE adapter integration round-trip (IDE event → wsm
  RecordEvent → queryable via `wsm inspect`): < 100ms.
- `pkg/*` import surface stays UI-free across releases
  (CI gate `verify-no-ui-leak` green).
- Zero breaking changes per minor version in `pkg/workspace`
  public API; semver discipline.

---

## Collaboration with Other Personas

- **Operations Manager (wsm):** consumes events that
  integrator pipes in; integrator owns the schemas, ops
  manager owns the queries.
- **AI Agent (wsm):** agents are first-class event
  emitters; integrator builds the adapters that translate
  agent CLI events into canonical wsm event types.
- **Solo Developer (wsm):** reuses the same `pkg/workspace`
  API but via `wsm` CLI rather than embedded Go.
