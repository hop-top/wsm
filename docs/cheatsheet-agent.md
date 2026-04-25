# wsm Cheatsheet — Agent

Quick reference for autonomous agents, scripts, and LLMs consuming the CLI.
Scannable in 30 seconds.

---

## Prerequisites

```bash
wsm --version                            # verify binary on PATH
wsm workspace list --json                # sanity-check backend
wsm workspace config show --json         # inspect active config

# Global flags (all commands)
wsm --json ...                           # structured output
wsm -q ...                               # bare IDs only (no headers)
wsm --data-dir /custom ...               # override data dir
wsm --session-id <sid> ...               # tag events with session ID
wsm --agent-id <aid> ...                 # tag events with agent ID
```

---

## Agent Loop Contract

```
1. Workspace   →  wsm workspace add <name> --json         (once; idempotent on conflict)
2. Record      →  wsm event add <ws> <type> [flags]       (per action)
3. Snapshot    →  wsm snapshot generate <ws> --json       (gate on seq advance)
4. Handoff     →  wsm snapshot context <ws>               (inject into next agent context)
5. Inspect     →  wsm workspace show <ws> --json          (verify state)
```

**DO:** generate snapshot after recording a meaningful batch of events.
**DON'T:** pass context without first generating a snapshot.
**DO:** use `--json` + `--quiet` for scripted pipelines.
**DON'T:** rely on human-readable table output for parsing.

---

## Workspace CRUD

```bash
# Create
wsm workspace add my-project --json
wsm workspace add my-project --description "..." --tag go,ci --json

# List
wsm workspace list --json
wsm workspace list --status active --json

# Show (full structured view)
wsm workspace show <name-or-id> --json
wsm workspace show <name-or-id> --events --json        # + last 20 events
wsm workspace show <name-or-id> --events-all --json    # + all events

# Archive / restore
wsm workspace archive <name-or-id>
wsm workspace unarchive <name-or-id>

# Remove (non-interactive)
wsm workspace remove <name-or-id> --force --json
wsm workspace remove <name-or-id> --dry-run --json    # preview
```

---

## Event Recording

```bash
# Interaction: message
wsm event add <ws> interaction.message \
  --role user|assistant|system --content "..." --session-id <sid>

# Interaction: tool call
wsm event add <ws> interaction.tool_call \
  --tool-name <name> --input '<json>' --session-id <sid>

# Interaction: tool done
wsm event add <ws> interaction.tool_done \
  --tool-name <name> --output '<json>' --status ok|error \
  --duration-ms 250 --session-id <sid>

# Mutation: file patch
wsm event add <ws> mutation.file_patch \
  --path cmd/main.go --operation create|modify|delete --diff "..."

# Device lifecycle
wsm event add <ws> device.attached --device-id <id> --device-name <name>
wsm event add <ws> device.detached --device-id <id>

# Raw payload
wsm event add <ws> interaction.message --data '{"role":"user","content":"..."}'
```

### Event list + inspect

```bash
wsm event list <ws> --json
wsm event list <ws> --type interaction.message --limit 50 --json
wsm event list <ws> --after-seq 10 --before-seq 100 --json
wsm event show <ws> <seq-or-id> --json
```

---

## Snapshots

```bash
# Generate (replay events → materialized view)
wsm snapshot generate <ws> --json
wsm snapshot generate <ws> --name "pre-refactor" --seed --json

# List
wsm snapshot list --workspace <name> --json
wsm snapshot list --after 2026-01-01T00:00:00Z --json

# Read
wsm snapshot show <ws> --json          # latest snapshot
wsm snapshot context <ws>              # handoff brief (plain text; inject as context)
wsm snapshot context <ws> --events 10  # more events in brief

# Update
wsm snapshot update <snap-id> --name "v2" --seed --json

# Remove (non-interactive)
wsm snapshot remove <snap-id> --force
```

---

## Spaces

```bash
wsm space add <ws> /path/to/dir --json
wsm space add <ws> /path --type filesystem --trust-level local --json
wsm space list <ws> --json
wsm space show <ws> <space-id> --json
wsm space remove <ws> <space-id> --force
```

---

## Devices

```bash
wsm device list --json
wsm device show <id-or-name> --json    # + session history
```

---

## Config Inspection

```bash
wsm workspace config show --json
# → {"backend":"sqlite","data_dir":"...","config_file":"...","source":"..."}
```

| Env Var         | Purpose                        |
|-----------------|--------------------------------|
| `WSM_DATA_DIR`  | Override data directory        |
| `WSM_CACHE_DIR` | Override cache directory       |
| `WSM_STATE_DIR` | Override state directory       |
| `NO_COLOR`      | Disable ANSI in TTY output     |

---

## Output Contracts

| Flag      | stdout contract                        |
|-----------|----------------------------------------|
| `--json`  | Valid JSON object or array             |
| `-q`      | One ID or seq per line, no headers     |
| (default) | Human table — do NOT parse in scripts  |

`snapshot context` always emits plain text regardless of `--json` — designed
for direct injection into agent context.

---

## Exit Codes

| Code | Meaning       | Recovery                              |
|------|---------------|---------------------------------------|
| 0    | Success       | —                                     |
| 2    | Usage error   | Fix flags/args; check `--help`         |
| 3    | Not found     | Verify workspace name with `list`     |
| 4    | Conflict      | Already exists or already archived    |
| 5    | Backend error | Check data dir; verify binary version |

---

## Workspace Data Model (Key Fields)

| Field        | Type     | Notes                                    |
|--------------|----------|------------------------------------------|
| `id`         | string   | stable ULID                              |
| `name`       | string   | human-readable identifier                |
| `status`     | string   | `active` \| `archived`                  |
| `description`| string   | optional                                 |
| `tags`       | []string | optional labels                          |
| `created_at` | RFC3339  | creation timestamp                       |
| `updated_at` | RFC3339  | last modification                        |

## Snapshot Data Model (Key Fields)

| Field           | Type    | Notes                                   |
|-----------------|---------|-----------------------------------------|
| `id`            | string  | stable ULID                             |
| `workspace_id`  | string  | parent workspace                        |
| `at_seq_num`    | int     | event sequence high-water mark          |
| `message_count` | int     | interaction messages replayed           |
| `tool_calls`    | int     | tool calls replayed                     |
| `files_changed` | []path  | distinct file paths mutated             |
| `devices`       | []id    | devices seen in session                 |
| `sessions`      | []id    | session IDs seen                        |
| `is_seed`       | bool    | pinned reference snapshot               |
| `name`          | string  | optional alias                          |
| `generated_at`  | RFC3339 | snapshot creation time                  |

---

## Error Handling

| Condition                             | Handling                                       |
|---------------------------------------|------------------------------------------------|
| Workspace not found (exit 3)          | `wsm workspace list --json` to verify names    |
| Conflict on create (exit 4)           | Use existing or rename; check `list` output    |
| Non-TTY remove without `--force`      | Always pass `--force` in scripts               |
| Ambiguous device reference            | Use longer ID prefix; `device list --json`     |
| Backend error (exit 5)               | Check `wsm workspace config show --json`       |
| Snapshot stale after events recorded  | `wsm snapshot generate <ws>` before handoff    |
