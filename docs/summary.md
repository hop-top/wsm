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
| Record a message | `RecordEvent(wsID, "interaction.message", data)` | Appends message with role/content to log |
| Record a tool call | `RecordEvent(wsID, "interaction.tool_call", data)` | Logs tool name, input, timing |
| Record a file change | `RecordEvent(wsID, "mutation.file_patch", data)` | Logs path, operation, optional diff |
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
| 5 | Forbidden | Capability gate denied the operation |
| 8 | Backend error | Corrupted database |
| 9 | Not configured | IRC not enabled for the workspace |
| 70 | Prerequisite | Missing binary, busy port, unreachable server |

## Configuration

| Layer | Example | Priority |
|-------|---------|----------|
| CLI flags | `--data-dir=/custom` | Highest |
| Env vars | `WSM_BACKEND=sqlite` | |
| Config file | `~/.config/wsm/config.yaml` | |
| Local config | `.wsm/config.yaml` or `.hop/wsm/config.yaml` | |
| Defaults | `backend=sqlite`, `data_dir=~/.local/share/wsm` | Lowest |

### XDG Directories

| Directory | Default | Env Override |
|-----------|---------|-------------|
| Data | `~/.local/share/wsm/` | `WSM_DATA_DIR` |
| Cache | `~/.cache/wsm/` | `WSM_CACHE_DIR` |
| State | `~/.local/state/wsm/` | `WSM_STATE_DIR` |
| Config | `~/.config/wsm/` | `XDG_CONFIG_HOME` |

### Local Config Discovery

Walk-up search for project-local config (opt-in):
- `.wsm/config.yaml` — standalone wsm config
- `.hop/wsm/config.yaml` — when managed via hop

Debug config: `wsm config show`

## Workspace Export / Import

| Action | API | What happens |
|--------|-----|-------------|
| Export workspace | `Manager.ExportWorkspace(ctx, ref)` | YAML with metadata + space defs |
| Import workspace | `Manager.ImportWorkspace(ctx, data, opts)` | Create from YAML with conflict handling |
| Conflict strategies | `skip`, `rename`, `overwrite` | Controls name collision behavior |
| Dry run | `ImportOptions{DryRun: true}` | Preview import without persisting |

## Space Detection

| Action | API | What happens |
|--------|-----|-------------|
| Register detectors | `Registry.Register(detector)` | Add pluggable space scanners |
| Scan a directory | `Registry.RunAll(ctx, dir)` | Run all detectors, collect results |
| Generate strategy | `GenerateStrategy(results)` | Aggregate detections into profile |
| Apply overrides | `ApplyOverrides(&strategy, overrides)` | Disable detectors, custom settings |

Built-in detectors: Makefile, package.json, docker-compose, devcontainer,
.env.example, package registries, local docs, dependency health, mise,
SecretSpec, fnox.

## Access Control

| Action | API | What happens |
|--------|-----|-------------|
| Declare grants | `GrantSet.Add(grant)` | Subject-resource permission binding |
| Check access | `GrantSet.Check(subj, res, perm)` | Boolean permission check |
| Define prerequisites | `Prerequisite{Level: required}` | Checks with enforcement levels |
| Run enforcement | `Enforce(prereqs)` | Error on required failures only |
| Bypass checks | `RunVerification(prereqs, VerifyOptions{NoVerify: true})` | Skip all enforcement |

## Scope & Assignment

| Action | API | What happens |
|--------|-----|-------------|
| Define scopes | `ScopeSet.Add(scope)` | File patterns, operations, secrets, etc. |
| Match scope | `ScopeSet.Match(type, value)` | Glob match for files, exact for others |
| Assign to workspace | `AssignmentSet.Assign(assignment)` | Bind profile/squad with scopes |
| Query effective scope | `EffectiveScope(assignments)` | Merge all assignment scopes |
| Validate session | `ValidateSessionContext(ctx)` | Enforce workspace vs space scoping |

## For Integrators (Go Module)

| Action | API | What happens |
|--------|-----|-------------|
| Create a manager | `workspace.NewManager(backend)` | Ready for workspace CRUD and event recording |
| Implement custom storage | Implement `backend.Backend` interface | Use any storage (Postgres, Git, custom) |
| Validate a backend | Run conformance test suite | Verify contract compliance |
| Detect current device | `device.Current()` | Stable machine ID, OS, arch auto-detected |
| Register a device | `registry.Register(device)` | Track known devices across sessions |
