# wsm Cheatsheet — Human

Quick reference for daily use. Scannable in 30 seconds.

---

## Install

```bash
go install hop.top/wsm/cmd/wsm@latest   # from source
make build                               # → bin/wsm (local build)
wsm --version                            # verify
```

---

## Init (project-local config)

```bash
wsm init                  # creates .wsm/config.yaml + .hop/wsm/ symlink
wsm init --hop            # creates .hop/wsm/ as primary
wsm init --force          # overwrite existing
```

Config search order: CLI flags → env vars (`WSM_*`) → `~/.config/wsm/config.yaml`
→ `.wsm/config.yaml` / `.hop/wsm/config.yaml` → defaults

---

## Workspaces

```bash
# Create
wsm workspace add my-project
wsm workspace add my-project --description "CI pipeline" --tag go,ci

# List
wsm workspace list                        # all workspaces
wsm workspace list --status active        # active only
wsm workspace list --status archived
wsm workspace list --sort name            # sort by name (default: created)

# Inspect
wsm workspace show <name-or-id>
wsm workspace show <name-or-id> --events          # + last 20 events
wsm workspace show <name-or-id> --events-all      # + all events
wsm workspace show <name-or-id> --devices         # + device list
wsm workspace show <name-or-id> --json            # full JSON

# Archive / restore
wsm workspace archive <name-or-id>        # freeze; stays queryable
wsm workspace unarchive <name-or-id>

# Remove
wsm workspace remove <name-or-id>         # prompts for confirmation
wsm workspace remove <name-or-id> --dry-run   # preview without deleting
wsm workspace remove <name-or-id> --force     # skip confirmation
```

Aliases: `ws` for `workspace`; `add|create|new|a|+`, `list|ls|all`, `show|inspect|info|get|s`

---

## Spaces

```bash
wsm space add <workspace> /path/to/dir    # auto-detects type + trust
wsm space add <workspace> /path --type filesystem --trust-level local
wsm space add <workspace> /path --capability read,write

wsm space list <workspace>
wsm space show <workspace> <space-id>
wsm space remove <workspace> <space-id>
wsm space remove <workspace> <space-id> --force   # cascade child spaces
```

Space types: `filesystem` | `container` | `vm` | `api` | `database` | `machine` | `remote`
Trust levels: `local` | `verified` | `external` | `untrusted`

Aliases: `sp` for `space`

---

## Events

```bash
# Record
wsm event add <workspace> interaction.message \
  --role user --content "Fixing CI pipeline"

wsm event add <workspace> interaction.tool_call \
  --tool-name Bash --input '{"command":"make test"}'

wsm event add <workspace> mutation.file_patch \
  --path cmd/main.go --operation modify --diff "..."

wsm event add <workspace> device.attached \
  --device-id <id> --device-name my-laptop

# List
wsm event list <workspace>
wsm event list <workspace> --type interaction.message
wsm event list <workspace> --after-seq 10 --before-seq 50
wsm event list <workspace> --limit 20

# Inspect
wsm event show <workspace> <seq-or-id>
```

Appendable event types: `interaction.message` | `interaction.tool_call`
| `interaction.tool_done` | `mutation.file_patch` | `device.attached` | `device.detached`

---

## Snapshots

```bash
# Generate (materialized view from event replay)
wsm snapshot generate <workspace>
wsm snapshot generate <workspace> --name "pre-refactor" --seed

# List
wsm snapshot list
wsm snapshot list --workspace <name>
wsm snapshot list --after 2026-01-01T00:00:00Z --limit 10

# Inspect
wsm snapshot show <workspace>             # latest snapshot
wsm snapshot context <workspace>          # handoff brief (5 recent events)
wsm snapshot context <workspace> --events 10

# Update metadata
wsm snapshot update <snap-id> --name "v2" --seed

# Remove
wsm snapshot remove <snap-id>             # prompts
wsm snapshot remove <snap-id> --force     # skip confirmation
```

---

## Devices

```bash
wsm device list
wsm device show <id-or-name>     # + session history
```

---

## Config

```bash
wsm workspace config show        # resolved config (backend, data_dir, source)
wsm workspace config show --json
```

| Dir     | Default                    | Env override    |
|---------|----------------------------|-----------------|
| Data    | `~/.local/share/wsm/`      | `WSM_DATA_DIR`  |
| Cache   | `~/.cache/wsm/`            | `WSM_CACHE_DIR` |
| State   | `~/.local/state/wsm/`      | `WSM_STATE_DIR` |
| Config  | `~/.config/wsm/`           | `XDG_CONFIG_HOME` |

---

## Output Modes

| Flag / env     | Behavior                            |
|----------------|-------------------------------------|
| (default TTY)  | Styled tables, color, prompts       |
| `--json`       | Structured JSON on stdout           |
| `-q` / `--quiet` | Bare IDs or exit codes only       |
| Non-TTY pipe   | Plain text, auto-JSON on create     |
| `NO_COLOR=1`   | TTY without ANSI codes              |

---

## Global Flags

```bash
--backend sqlite       # storage backend
--data-dir <path>      # override data directory
--json                 # JSON output
-q, --quiet            # minimal output
--no-color             # disable colors
--session-id <id>      # tag events with session ID
--agent-id <id>        # tag events with agent ID (default: "user")
```

---

## Exit Codes

| Code | Meaning        |
|------|----------------|
| 0    | Success        |
| 2    | Usage error    |
| 3    | Not found      |
| 4    | Conflict       |
| 5    | Backend error  |

---

## Common Failures

| Symptom                          | Fix                                          |
|----------------------------------|----------------------------------------------|
| "remove in non-TTY requires..."  | Add `--force` in scripts                     |
| Workspace not found              | Use `wsm workspace list` to verify name/ID   |
| Events not appearing in snapshot | Run `wsm snapshot generate <workspace>`      |
| Wrong data directory             | Check `wsm workspace config show`            |
| Ambiguous device reference       | Use longer ID prefix or `--json` to inspect  |
