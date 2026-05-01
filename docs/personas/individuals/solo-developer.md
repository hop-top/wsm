# Persona: Solo Developer (wsm Extension)

**Primary Role:** Independent dev juggling many parallel
worktrees, side projects, and concurrent AI CLI sessions.
Uses wsm as the IDE-agnostic memory layer: one workspace
per project context, events recorded across whichever AI
CLI happens to be active, snapshots for hand-off when
context-switching.

---

## Base Goals

- Get up and running in under 30 min; no Kubernetes, no
  distributed systems.
- Own the infrastructure — no vendor lock-in.
- Single-binary deployment.

## Base Pain Points

- Managed-service pricing not worth it for low-volume work.
- Complex docs assume large teams.
- Debugging requires digging through third-party dashboards.

---

## wsm Interaction Patterns

### Bootstrap

```pseudocode
// install single binary; SQLite backend by default; no daemon
$ wsm create my-project --tags="side,go"
// → ULID printed; workspace ready
```

### Daily flow across worktrees

```pseudocode
// list active work across all parallel worktrees
$ wsm list --status=active

// jump into one; inspect what was happening
$ wsm inspect <ws-id> --events
// → last 20 events: messages, tool calls, file patches

// archive when project ships
$ wsm archive <ws-id>
```

### Cross-IDE continuity

```pseudocode
// morning: claude session on workspace foo
// afternoon: codex session on same workspace
// both adapters push into the same wsm workspace via
// hop.top/wsm/pkg/workspace.Manager — solo dev sees ONE
// timeline, not per-IDE silos
$ wsm inspect <ws-id> --events --json
  | jq '[.[] | .source] | unique'
// → ["claude", "codex"]
```

### Context switch / handoff

```pseudocode
// pause project; capture state for later
$ wsm inspect <ws-id> > /tmp/project-foo-pause.txt

// weeks later: resume
$ wsm inspect <ws-id> --events
// → full history intact; no recovery archaeology
```

### Project-local config

```pseudocode
// optional .wsm/config.yaml in project root
$ cat .wsm/config.yaml
backend: sqlite
data_dir: .wsm/data
// per-project DB; no global pollution
```

---

## User Stories

- As a solo dev with 5 parallel worktrees, I run
  `wsm list --status=active` to see what's in flight
  across all of them in one table, instead of `tmux ls` +
  `git worktree list` + memory.
- As a solo dev switching between claude and codex on the
  same project, both IDE adapters push into the same wsm
  workspace so my timeline shows one continuous session,
  not two disjoint ones.
- As a solo dev coming back to a paused side project after
  weeks, I run `wsm inspect <id> --events` and immediately
  see what I was doing and why, instead of re-reading my
  own commit history.
- As a solo dev, I install wsm as a single Go binary with
  SQLite backend and zero infrastructure setup so I'm
  productive in < 5 min on a fresh machine.
- As a solo dev, I drop a `.wsm/config.yaml` in a project
  root so workspace state lives WITH the project (committed
  or git-ignored as I choose), not in a global directory.
- As a solo dev, I archive a finished side project so my
  active workspace list stays focused, but the history
  remains queryable for blog posts / retros.

---

## Success Metrics

- Time-to-first-workspace-create on fresh install: < 30s.
- `wsm list` across 20 active workspaces returns in < 200ms.
- Workspace recall (`wsm inspect <id>`): < 2s for a
  10000-event workspace.
- Zero data loss on crash: SQLite WAL guarantees event
  durability without manual checkpointing.

---

## Collaboration with Other Personas

- **AI Agent (wsm):** agents inside the solo dev's IDEs
  emit the events that populate the workspace timeline.
- **System Integrator (wsm):** the IDE adapters the solo
  dev relies on are built by integrators against the
  `pkg/workspace` API.
- **Operations Manager (wsm):** solo dev "graduates" to
  this persona when working on team projects with multiple
  contributors / squad-level audit needs.
