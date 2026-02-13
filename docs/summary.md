# wsm - What Users Can Do

## Workspace Lifecycle

| Action | Command | What happens |
|--------|---------|-------------|
| Create a workspace | `wsm create <name>` | New workspace with ULID, optional description and tags |
| List workspaces | `wsm list` | Table of all workspaces with name, status, event count, last activity |
| Filter workspaces | `wsm list --status=active` | Show only active or archived workspaces |
| Inspect a workspace | `wsm inspect <id>` | Summary: metadata, event count, device count |
| View recent events | `wsm inspect <id> --events` | Last 20 events in table format |
| Export full history | `wsm inspect <id> --events-all --json` | All events as JSON for piping |
| Archive a workspace | `wsm archive <id>` | Freezes workspace (no new events), remains queryable |
| Restore a workspace | `wsm unarchive <id>` | Returns archived workspace to active status |
| Delete a workspace | `wsm delete <id>` | Permanent removal with confirmation prompt |
| Preview deletion | `wsm delete <id> --dry-run` | Shows what would be deleted without acting |

## Event Recording

| Action | How | What happens |
|--------|-----|-------------|
| Record a message | `RecordEvent(wsID, "message.added", data)` | Appends message with role/content to log |
| Record a tool call | `RecordEvent(wsID, "tool_call.started", data)` | Logs tool name, input, timing |
| Record a file change | `RecordEvent(wsID, "file.changed", data)` | Logs path, operation, optional diff |
| Attach a device | `RecordEvent(wsID, "device.attached", data)` | Associates a machine with the workspace |
| Generate a snapshot | `wsm inspect` / `Manager.Snapshot()` | Materialized view from event replay |

## Output Modes

| Mode | Trigger | Behavior |
|------|---------|----------|
| TTY (default) | Terminal | Styled tables, color, confirmation prompts |
| JSON | `--json` | Structured JSON on stdout |
| Quiet | `-q` / `--quiet` | Bare IDs or exit codes only |
| Pipe | Non-TTY detected | Plain text, no color, auto-JSON for create |
| No color | `NO_COLOR=1` | TTY output without ANSI codes |

## Exit Codes

| Code | Meaning | Example |
|------|---------|---------|
| 0 | Success | `wsm create foo` succeeds |
| 2 | Usage error | Missing required argument |
| 3 | Not found | `wsm inspect nonexistent` |
| 4 | Conflict | `wsm create duplicate` or `wsm archive already-archived` |
| 5 | Backend error | Corrupted database |

## Configuration

| Layer | Example | Priority |
|-------|---------|----------|
| CLI flags | `--data-dir=/custom` | Highest |
| Env vars | `WSM_BACKEND=sqlite` | |
| Config file | `~/.config/wsm/config.yaml` | |
| Defaults | `backend=sqlite`, `data_dir=~/.local/share/wsm` | Lowest |

Debug config: `wsm config show`

## For Integrators (Go Module)

| Action | API | What happens |
|--------|-----|-------------|
| Create a manager | `workspace.NewManager(backend)` | Ready for workspace CRUD and event recording |
| Implement custom storage | Implement `backend.Backend` interface | Use any storage (Postgres, Git, custom) |
| Validate a backend | Run conformance test suite | Verify contract compliance |
| Detect current device | `device.Current()` | Stable machine ID, OS, arch auto-detected |
| Register a device | `registry.Register(device)` | Track known devices across sessions |
