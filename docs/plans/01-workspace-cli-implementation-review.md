# UX Review: Plan 1 — workspace-cli (`wsm`)

**Reviewed**: 2026-02-12
**Scope**: CLI/TUI UX gaps, improvements, and decisions
**Skills Applied**: cli-design, tui-design
**Reference**: Existing APS CLI patterns (Cobra + Lipgloss + tabwriter)

---

## 1. GAPS — Missing UX Concerns

### 1.1 No Output Mode Strategy

The plan lists five commands (`create`, `list`, `inspect`, `archive`, `delete`) but specifies nothing about output formatting. The existing APS CLI has an established pattern: `--json` flag on `list` and `inspect` commands, `tabwriter` for human-readable tables, `dimStyle` for empty states, `headerStyle` for section titles, and `lipgloss` for semantic color.

**Why it matters:** `wsm` is described as both a standalone CLI and a Go module. A standalone CLI serving developers and CI/CD pipelines *must* define its output contract from day one. Machine consumers will break if output formatting is treated as an afterthought.

**Recommendation:** Specify the output mode matrix explicitly:

| Command   | Default (TTY) | `--json` | `--quiet` | Pipe (non-TTY) |
|-----------|---------------|----------|-----------|-----------------|
| `create`  | Confirmation message | `{"id":"...","created_at":"..."}` | Exit code only | Same as `--json` |
| `list`    | Styled table | JSON array | IDs only, one per line | Plain table, no color |
| `inspect` | Key-value detail view | Full JSON | N/A | Plain text, no color |
| `archive` | Confirmation | `{"id":"...","archived_at":"..."}` | Exit code only | Same as `--json` |
| `delete`  | Confirmation prompt | N/A (requires TTY for prompt) | Error (needs `--force`) | Error (needs `--force`) |

Auto-detect TTY and suppress ANSI when piped, respecting `NO_COLOR`. This matches 12-factor output philosophy and the existing APS pattern.

---

### 1.2 No Error UX Specification

The plan describes a storage backend interface with SQLite, Postgres, and Git backends but says nothing about error taxonomy, error messages, or exit codes.

**Why it matters:** A workspace tool with multiple storage backends will produce a wide variety of failure modes: connection refused (Postgres), locked database (SQLite), auth failure (Git), migration version mismatch, corrupt event log, workspace not found, workspace already archived. Each needs a distinct, actionable error message.

**Recommendation:** Define an error taxonomy with exit codes:

```
Exit 0  - Success
Exit 1  - General error
Exit 2  - Usage error (bad arguments, missing required flags)
Exit 3  - Not found (workspace, device, backend)
Exit 4  - Conflict (workspace already exists, already archived)
Exit 5  - Backend error (connection, auth, corruption)
Exit 64 - Partial success (e.g., multi-workspace operations)
```

Every error message should follow the pattern:

```
Error: workspace 'xyz' not found

  Available workspaces:
    abc    created 2h ago
    def    created 1d ago

  To create a new workspace:
    wsm create xyz
```

---

### 1.3 No First-Run / Empty State UX

The plan has no mention of what happens when a user runs `wsm` for the first time, or runs `wsm list` with no workspaces.

**Why it matters:** The first interaction with a CLI tool determines whether a user keeps using it or abandons it. The existing APS pattern is good here — `cap list` shows `"No capabilities found."` followed by a hint.

**Recommendation:** Define empty states for every command:

```
$ wsm list
No workspaces yet.

  Create your first workspace:
    wsm create my-project

  Import from an IDE session:
    wsm import --from claude-code ~/.claude/sessions/

$ wsm inspect abc-123
Error: workspace 'abc-123' not found

  Run 'wsm list' to see available workspaces.
```

Additionally, `wsm` with no arguments should print a brief welcome + usage, not just raw help text:

```
$ wsm
Workspace Session Manager - IDE-agnostic session storage

Quick start:
  wsm create my-project    Create a new workspace
  wsm list                 List all workspaces
  wsm inspect <id>         View workspace details

Run 'wsm --help' for full command reference.
```

---

### 1.4 No `--dry-run` for Destructive Operations

The plan includes `archive` and `delete` commands but mentions no `--dry-run` flag and no confirmation prompt design.

**Why it matters:** `delete` destroys workspace data including the entire event log. `archive` changes workspace state irreversibly (or does it? the plan does not say).

**Recommendation:** For `delete`:
- Default: interactive confirmation prompt (detect TTY; error if non-TTY without `--force`)
- `--force` / `-f`: skip prompt
- `--dry-run`: show what would be deleted without doing it

For `archive`:
- Clarify whether archive is reversible (can you `wsm unarchive`?)
- If irreversible, same pattern as delete
- If reversible, a simple confirmation message is sufficient

---

### 1.5 No Shell Completion Strategy

The plan's directory structure shows `cmd/wsm/main.go` and cobra commands but does not mention shell completion.

**Why it matters:** The existing APS CLI has a fully implemented `completion` subcommand supporting bash, zsh, fish, and PowerShell. Shell completion is table-stakes for CLI tools that take workspace IDs as arguments.

**Recommendation:** Include a `wsm completion` command following the APS pattern. Register `ValidArgsFunction` on `inspect`, `archive`, and `delete` to auto-complete workspace IDs (and device IDs where applicable).

---

### 1.6 No Signal Handling or Graceful Shutdown

The plan describes an event-sourcing persistence layer but says nothing about what happens when a long-running operation is interrupted by SIGINT/SIGTERM.

**Why it matters:** A partial write to an event log or a half-completed migration can corrupt the backend. The SQLite backend with JSONL is especially vulnerable — an interrupted append can leave malformed JSON at the end of the file.

**Recommendation:** Specify that:
- All backend writes use transactions (SQLite has this built in; JSONL needs explicit fsync)
- SIGINT during `create`/`archive`/`delete` triggers graceful cleanup
- Progress feedback (spinner) for operations taking longer than 200ms, with clear cancellation messaging

---

### 1.7 No Config File / Env Var Hierarchy

The plan mentions three backends but does not specify how the user selects one, or where backend configuration lives.

**Why it matters:** The 12-factor principle is: CLI flags > env vars > config file > defaults.

**Recommendation:** Define the config hierarchy:

```
1. CLI flags:     --backend=postgres --backend-url=postgres://...
2. Env vars:      WSM_BACKEND=postgres  WSM_BACKEND_URL=postgres://...
3. Config file:   ~/.config/wsm/config.yaml (or $XDG_CONFIG_HOME/wsm/config.yaml)
4. Defaults:      backend=sqlite, data-dir=~/.local/share/wsm/
```

Include `wsm config show` to print the resolved configuration for debugging.

---

### 1.8 No Pipe / Composition Design

The plan does not discuss how `wsm` composes with other Unix tools.

**Recommendation:** Ensure:
- `wsm list -q` outputs bare workspace IDs, one per line, suitable for piping to `xargs`
- `wsm create` outputs the created workspace ID to stdout when non-TTY: `WS_ID=$(wsm create my-project)`
- `wsm inspect --json` outputs valid JSON to stdout with no decorative text mixed in
- stderr is used for progress/error messages, stdout for data only (when piped)

---

## 2. IMPROVEMENTS — Partially Covered, Needs More Thought

### 2.1 Command Naming: `create` vs `new`

The plan uses `create` for workspaces. The existing APS CLI uses `new` for profiles. This inconsistency will confuse users who interact with both tools.

**Recommendation:** Pick one convention and apply it everywhere. At minimum, add aliases so both work: `wsm new` as alias for `wsm create`, and `wsm show` as alias for `wsm inspect`.

---

### 2.2 `inspect` Needs Depth Levels

A workspace with event-sourcing contains potentially thousands of events. The plan does not address how `inspect` handles data volume.

**Recommendation:** Design `inspect` with progressive disclosure:

```
# Default: summary view (metadata + stats)
$ wsm inspect abc-123
Workspace: abc-123
Name:      my-project
Status:    active
Backend:   sqlite
Created:   2026-02-12 14:30:00
Events:    1,247
Devices:   2
Snapshots: 3
Last Activity: 12 minutes ago

# Detailed: include latest events
$ wsm inspect abc-123 --events

# Full event log (pipe-friendly)
$ wsm inspect abc-123 --events --all --json | jq ...

# Specific section
$ wsm inspect abc-123 --devices
$ wsm inspect abc-123 --snapshots
```

---

### 2.3 `archive` Needs Clearer Semantics

The plan lists `archive` as a command but does not explain what archiving means operationally.

**Recommendation:** Define archive clearly and reflect it in the CLI UX:

```
$ wsm archive abc-123
Archiving workspace 'abc-123'...

  This will:
    - Mark the workspace as archived (no new events can be recorded)
    - Compress the event log (1,247 events -> ~2.1 MB archive)
    - The workspace will still be visible with 'wsm list --archived'

  To restore later: wsm unarchive abc-123

Proceed? [y/N]:
```

---

### 2.4 Missing Status Badges in `list` Output

The plan mentions workspace lifecycle but does not define workspace states or how they are displayed.

**Recommendation:** Define workspace states and their visual treatment:

```
$ wsm list
Workspaces

NAME           ID          STATUS    EVENTS  LAST ACTIVITY
my-project     abc-123     active    1,247   12 min ago
old-work       def-456     archived    892   3 months ago
broken-thing   ghi-789     errored       3   1 day ago

3 workspaces (2 active, 1 archived)
```

Use `StatusDot` or colored badges for the STATUS column, consistent with the APS palette.

---

### 2.5 Device Registry UX is Undefined

The plan includes `pkg/device/` in the directory structure but there are no CLI commands for device management.

**Recommendation:** Either explicitly state "device registry is internal-only in Plan 1, CLI surface deferred to Plan 5/7" or add `wsm device list` and `wsm device show <id>` as read-only commands.

---

## 3. QUESTIONS / DECISIONS — Below 85% Confidence

### 3.1 Should `wsm` with no arguments launch a TUI?

| Option | Description | Tradeoff |
|--------|-------------|----------|
| A | TUI on bare invocation (like APS) | Consistent but adds bubbletea dependency to vendored library |
| B | Help text on bare invocation | Lighter, more conventional for standalone tools |
| **C (lean)** | **Defer TUI to Plan 4 (APS integration)** | **Keep `wsm` standalone and lightweight. Let `aps workspace` provide TUI.** |

---

### 3.2 Verb Naming: `create`/`inspect` vs `new`/`show`?

| Option | Description | Tradeoff |
|--------|-------------|----------|
| A | Follow APS convention (`new`, `show`) | Ecosystem consistency |
| B | Follow Docker/kubectl convention (`create`, `inspect`) | Broader CLI ecosystem alignment |
| **C (lean)** | **Both with aliases** | **`create`/`inspect` primary in standalone, APS verbs primary when integrated** |

---

### 3.3 How should `inspect` render event-sourced data?

| Option | Description | Tradeoff |
|--------|-------------|----------|
| A | Flat table for events, key-value for metadata | Simple but degrades with volume |
| B | Timeline/tree view with lipgloss | Richer but higher implementation cost |
| **C (lean)** | **Summary by default, `--events` for raw log** | **Default shows aggregate stats; flags for depth** |

---

### 3.4 Should `list` support sorting and filtering from day one?

| Option | Description | Tradeoff |
|--------|-------------|----------|
| A | Ship minimal (no filters) | Fastest to implement |
| **B (lean)** | **Ship with `--status` and `--sort`** | **`--status=active\|archived` is essential once `archive` exists** |
| C | Ship with filters, sorting, and pagination | YAGNI for P0 |

---

## Highest-Priority Items

If only three things from this review make it into the plan revision:

1. **Output mode matrix** (Gap 1.1) — without this, every command's output is undefined
2. **Error taxonomy with exit codes** (Gap 1.2) — without this, scripts cannot reliably use `wsm`
3. **Empty state / first-run UX** (Gap 1.3) — without this, the first user experience is bare Cobra help text
