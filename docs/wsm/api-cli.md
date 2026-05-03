# wsm — CLI Reference

Last updated: 2026-05-02
Author: $USER

Generated reference for every wsm subcommand. See also:

- [configuration.md](configuration.md) — config keys, env vars, layered loading
- [../architecture.md](../architecture.md) — system overview

## Synopsis

```
wsm [command] [--flags]
```

Bare `wsm` (no args) prints a quick-start hint to stderr (stdout stays empty so
`wsm | jq` keeps working).

## Help groups

`wsm --help` shows the four primary groups; management commands are hidden by
default. Use `--help-all` to see everything, or one of the group-scoped flags.

| Group         | Commands                                                   |
|---------------|------------------------------------------------------------|
| ORGANIZE      | `workspace`, `space`, `device`                             |
| CAPTURE       | `event`, `snapshot`                                        |
| INTERACT      | `init`, `tui`                                              |
| MANAGEMENT    | `config`, `alias`, `upgrade`, `version`, `completion`      |

Group filters: `--help-organize`, `--help-capture`, `--help-interact`,
`--help-management`, `--help-all`.

### Subcommand groups (KNOWLEDGE / CURATE)

Inside `wsm workspace`, `wsm snapshot`, and `wsm event`, verbs are bucketed
into KNOWLEDGE (read) and CURATE (mutate) help groups per CLI conv §4:

| Parent      | KNOWLEDGE                  | CURATE                                              | CAPTURE |
|-------------|----------------------------|-----------------------------------------------------|---------|
| `workspace` | `list`, `show`             | `add`, `archive`, `unarchive`, `delete`, `config`   | —       |
| `snapshot`  | `list`, `show`, `context`  | `generate`, `update`, `delete`                      | —       |
| `event`     | `list`, `show`             | —                                                   | `add`   |

## Global flags

Provided by `kit/cli` plus wsm-specific globals registered on the root command.

| Flag                  | Type   | Default | Description                                                       |
|-----------------------|--------|---------|-------------------------------------------------------------------|
| `-h`, `--help`        | bool   | false   | Help for current command                                          |
| `--help-all`          | bool   | false   | Show all commands including management                            |
| `--help-organize`     | bool   | false   | Show only ORGANIZE commands                                       |
| `--help-capture`      | bool   | false   | Show only CAPTURE commands                                        |
| `--help-interact`     | bool   | false   | Show only INTERACT commands                                       |
| `--help-management`   | bool   | false   | Show only MANAGEMENT commands                                     |
| `-v`, `--version`     | bool   | false   | Print wsm version and exit                                        |
| `-V`, `--verbose`     | count  | 0       | Increase log verbosity (`-V`=debug, `-VV`=trace)                  |
| `-C`, `--chdir`       | string | ""      | Change directory before running                                   |
| `--quiet`             | bool   | false   | Suppress non-essential output                                     |
| `--no-color`          | bool   | false   | Disable ANSI color                                                |
| `--no-hints`          | bool   | false   | Suppress next-step hints after command output                     |
| `--format`            | string | table   | Output format: `csv`, `json`, `table`, `text`, `yaml`             |
| `--format-help`       | bool   | false   | Show available formats and their options                          |
| `--format-opt`        | k=v    | -       | Per-format option (repeatable; bool keys may omit `=value`)       |
| `--cols`, `--columns` | string | -       | Restrict columns to comma-separated list (repeatable)             |
| `--template`          | string | ""      | Go text/template applied to results (mutually exclusive `--cols`) |
| `-o`, `--output`      | string | ""      | Write output to path (use `-` or empty for stdout)                |
| `--config`            | path   | XDG     | Path to config file (overrides XDG location)                      |
| `--data-dir`          | path   | XDG     | Data directory                                                    |
| `--backend`           | string | sqlite  | Storage backend (only `sqlite` implemented)                       |
| `--backend-url`       | string | ""      | Backend URL (reserved for future backends)                        |
| `--session-id`        | string | ""      | Session ID for event tracking                                     |
| `--agent-id`          | string | user    | Agent ID for event tracking                                       |
| `--offline`           | bool   | false   | Disable all network (skip sync, upgrade check, plugin downloads)  |
| `--profile`           | string | ""      | Named profile (credentials/config)                                |
| `--instance`          | string | ""      | Backend instance discriminator                                    |

`--format json|yaml` is the canonical machine-readable selector. The previous
top-level `--json` flag was removed in this release; see
[CHANGELOG](../../CHANGELOG.md).

## Commands

### ORGANIZE

#### `wsm workspace`

Workspace operations. Aliases: `ws`, `w`.

```
wsm workspace [command] [--flags]
```

Subcommands: `add`, `list`, `show`, `archive`, `unarchive`, `delete`, `config`.

##### `wsm workspace add <name>`

Create a workspace. Aliases: `create`, `new`, `a`, `+`.

```
wsm workspace add <name> [--description <text>] [--tag <tag>...]
```

| Flag                    | Default | Description           |
|-------------------------|---------|-----------------------|
| `-d`, `--description`   | ""      | Workspace description |
| `-t`, `--tag`           | -       | Tag (repeatable)      |

Examples:

```
wsm workspace add my-project
wsm workspace add backend -d "API service" -t go -t critical
wsm workspace add api --format json
```

##### `wsm workspace list`

List workspaces. Aliases: `ls`, `all`, `l`.

```
wsm workspace list [--status active|archived] [--sort name|created|updated]
```

| Flag             | Default | Description                              |
|------------------|---------|------------------------------------------|
| `-s`, `--status` | ""      | Filter by status: `active`, `archived`   |
| `--sort`         | updated | Sort by: `name`, `created`, `updated`    |

Empty-state hint goes to stderr; data goes to stdout (so `| jq` works).

Examples:

```
wsm workspace list
wsm workspace list --status active --format json
wsm workspace list --sort name --cols ID,NAME
```

##### `wsm workspace show <id-or-name>`

Show workspace details. Aliases: `get`, `inspect`, `info`, `i`.

```
wsm workspace show <id-or-name> [--events] [--devices]
```

| Flag           | Default | Description                            |
|----------------|---------|----------------------------------------|
| `--events`     | false   | Show all events instead of last 5      |
| `--events-all` | false   | Alias for `--events` (back-compat)     |
| `--devices`    | false   | Show attached devices                  |

Examples:

```
wsm workspace show my-project
wsm workspace show my-project --events --devices
wsm workspace show 01J7... --format json
```

##### `wsm workspace archive <workspace>` / `unarchive <workspace>`

Mark a workspace as archived (read-only) or restore an archived workspace to
active. Aliases: `arch` / `unarch`.

```
wsm workspace archive <workspace>
wsm workspace unarchive <workspace>
```

##### `wsm workspace delete <workspace>`

Permanently delete a workspace and its history. Aliases: `rm`, `del`.

Renamed from `remove` in this release; see [CHANGELOG](../../CHANGELOG.md).

```
wsm workspace delete <workspace> [--force] [--dry-run]
```

| Flag            | Default | Description                                 |
|-----------------|---------|---------------------------------------------|
| `-f`, `--force` | false   | Skip confirmation prompt                    |
| `--dry-run`     | false   | Preview removal without deleting data       |

Non-TTY runs require `--force`. On macOS, `WSM_DELETE_CONFIRM=y` skips the
prompt non-interactively.

Examples:

```
wsm workspace delete my-project --dry-run
wsm workspace delete my-project --force
```

##### `wsm workspace config`

Workspace configuration. Aliases: `cfg`.

```
wsm workspace config show
```

Currently exposes a single `show` subcommand; mirrors `wsm config show` for the
workspace context.

#### `wsm space`

OWSF Space management.

```
wsm space [command] [--flags]
```

Subcommands: `add`, `list`, `show`, `delete`, `sync`.

##### `wsm space add <workspace> <id>`

Associate a compute/storage space with a workspace. Write-through to rux
`projects.yaml`.

```
wsm space add <workspace> <id> [--type ...] [--path ...] [--cap ...] \
  [--trust ...] [--startup-cmd ...] [--auto]
```

| Flag            | Default     | Description                                                     |
|-----------------|-------------|-----------------------------------------------------------------|
| `--type`        | filesystem  | Space type: `filesystem`, `container`, `vm`, `api`, `database`, `machine`, `remote` |
| `--path`        | ""          | Resource path (filesystem path, URL, etc.)                      |
| `--cap`         | -           | Space capability OASF URI (repeatable)                          |
| `--trust`       | local       | Trust level: `local`, `verified`, `external`, `untrusted`       |
| `--startup-cmd` | ""          | Default command rux runs when connecting (empty = `$SHELL`)     |
| `--auto`        | false       | Auto-detect space attributes from `--path`                      |

If the rux `projects.yaml` write fails, wsm warns to stderr and the wsm DB
write still succeeds; recover with `wsm space sync`.

##### `wsm space list <workspace>`

List spaces associated with a workspace.

##### `wsm space show <space-id>`

Show space details (type, trust, boundary, parent, capabilities, observables).

##### `wsm space delete <workspace> <space-id>`

Dissociate a space from a workspace.

Renamed from `remove` in this release; see [CHANGELOG](../../CHANGELOG.md).

| Flag            | Default | Description                            |
|-----------------|---------|----------------------------------------|
| `-f`, `--force` | false   | Force removal (cascade child spaces)   |

##### `wsm space sync`

Rewrite rux `projects.yaml` from the wsm DB. Recovery path after partial-write
failures, fresh checkouts where `projects.yaml` is missing, or drift. Manual
entries written by hand or future `rux project add` are preserved by the kit
projects.Write contract.

#### `wsm device`

Device registry operations. Aliases: `devices`, `dev`, `d`.

```
wsm device [command] [--flags]
```

Subcommands: `list`, `show`.

##### `wsm device list`

List all registered devices. Aliases: `ls`, `all`, `l`.

##### `wsm device show <id-or-name>`

Show device details and recent sessions. Aliases: `get`, `inspect`, `info`, `s`.

### CAPTURE

#### `wsm event`

Event operations. Aliases: `events`, `ev`, `e`.

```
wsm event [command] [--flags]
```

Subcommands: `add`, `list`, `show`.

##### `wsm event add <workspace> <type>`

Append an event to a workspace. Aliases: `create`, `a`, `+`.

```
wsm event add <workspace> <type> [--data <json>] [typed flags...]
```

The previous `--format` (payload format) flag was renamed to
`--content-format` to free `--format` for the global output selector.

| Flag               | Default | Description                                                 |
|--------------------|---------|-------------------------------------------------------------|
| `--data`           | ""      | Raw JSON event payload (overrides typed flags)              |
| `--role`           | user    | `interaction.message`: role (`user`, `assistant`, `system`) |
| `--content`        | ""      | `interaction.message`: content text                         |
| `--content-format` | ""      | `interaction.message`: optional content format              |
| `--tool-name`      | ""      | `interaction.tool_*`: tool name                             |
| `--input`          | ""      | `interaction.tool_*`: tool input as JSON                    |
| `--output`         | ""      | `interaction.tool_*`: tool output as JSON                   |
| `--status`         | ""      | `interaction.tool_*`: status value                          |
| `--duration-ms`    | 0       | `interaction.tool_*`: duration in milliseconds              |
| `--path`           | ""      | `mutation.file_patch` / `mutation.artifact`: path           |
| `--operation`      | modify  | `mutation.file_patch`: operation (`create`, `modify`, `delete`) |
| `--diff`           | ""      | `mutation.file_patch`: optional diff                        |
| `--device-id`      | ""      | `device.*`: device identifier                               |
| `--device-name`    | ""      | `device.*`: device name                                     |

User-appendable types: `interaction.message`, `interaction.tool_call`,
`interaction.tool_done`, `mutation.file_patch`, `mutation.artifact`,
`device.attached`, `device.detached`. Other system lifecycle types are
reserved.

Examples:

```
wsm event add my-project interaction.message --content "kicked off review"
wsm event add my-project interaction.tool_call --tool-name git --input '{"args":["status"]}'
wsm event add my-project mutation.file_patch --path src/main.go --operation modify
```

##### `wsm event list <workspace>`

List workspace events with filtering. Aliases: `ls`, `all`, `l`.

```
wsm event list <workspace> [--type t1,t2] [--after-seq N] [--before-seq N] [--limit N]
```

| Flag              | Default | Description                                  |
|-------------------|---------|----------------------------------------------|
| `-t`, `--type`    | -       | Filter by event types (comma-separated)      |
| `--after-seq`     | 0       | Only events after this sequence number       |
| `--before-seq`    | 0       | Only events before this sequence number      |
| `-n`, `--limit`   | 0       | Maximum number of events to list             |

##### `wsm event show <workspace> <seq-or-id>`

Show full event details including payload. Aliases: `get`, `inspect`, `info`, `s`.

#### `wsm snapshot`

Snapshot operations. Aliases: `snap`, `s`.

```
wsm snapshot [command] [--flags]
```

Subcommands: `generate`, `list`, `show`, `update`, `delete`, `context`.

##### `wsm snapshot generate [workspace]`

Generate and persist a workspace snapshot. Aliases: `gen`, `g`. With no
argument, picks the most recently updated workspace.

| Flag             | Default | Description                |
|------------------|---------|----------------------------|
| `-s`, `--session`| ""      | Filter by session ID       |
| `-n`, `--name`   | ""      | Snapshot name/alias        |
| `--seed`         | false   | Mark as seed snapshot      |

##### `wsm snapshot list`

List snapshots with filtering. Aliases: `ls`, `all`, `l`.

| Flag                | Default | Description                                       |
|---------------------|---------|---------------------------------------------------|
| `-w`, `--workspace` | -       | Filter by workspace ID or name (repeatable)       |
| `-s`, `--session`   | -       | Filter by session ID (repeatable)                 |
| `--after`           | ""      | Snapshots after this time (RFC3339)               |
| `--before`          | ""      | Snapshots before this time (RFC3339)              |
| `--sort`            | desc    | Sort direction: `asc`, `desc`                     |
| `-n`, `--limit`     | 0       | Maximum number of snapshots                       |

##### `wsm snapshot show <workspace>`

Show latest snapshot for a workspace. Aliases: `get`, `s`.

##### `wsm snapshot update <snapshot-id-or-name>`

Update snapshot metadata. Aliases: `rename`.

| Flag             | Default | Description           |
|------------------|---------|-----------------------|
| `-n`, `--name`   | ""      | Snapshot name/alias   |
| `--seed`         | false   | Mark as seed snapshot |

##### `wsm snapshot delete <snapshot-id-or-name>`

Delete a snapshot. Aliases: `rm`, `del`.

Renamed from `remove` in this release; see [CHANGELOG](../../CHANGELOG.md).

| Flag            | Default | Description           |
|-----------------|---------|-----------------------|
| `-f`, `--force` | false   | Skip confirmation     |

Non-TTY runs require `--force`.

##### `wsm snapshot context <workspace>`

Output a markdown summary context for an LLM (writes to stdout).

| Flag       | Default | Description                          |
|------------|---------|--------------------------------------|
| `--events` | 5       | Number of recent events to include   |

### INTERACT

#### `wsm init`

Initialize a local config directory (`.wsm/` or `.hop/wsm/`) in the current
directory with a minimal `config.yaml` and a symlink from the other path.

By default creates `.wsm/` and symlinks `.hop/wsm/ -> .wsm/`. With `--hop`,
creates `.hop/wsm/` and symlinks `.wsm/ -> .hop/wsm/`.

| Flag            | Default | Description                                       |
|-----------------|---------|---------------------------------------------------|
| `--hop`         | false   | Use `.hop/wsm/` as primary (hop-managed projects) |
| `-f`, `--force` | false   | Overwrite existing config directories             |

Examples:

```
wsm init
wsm init --hop
wsm init --force
```

#### `wsm tui`

Launch the terminal UI (Charmbracelet bubbletea v2). Aliases: `ui`. Hidden by
default; invoked explicitly.

```
wsm tui
```

### MANAGEMENT

Hidden in default `wsm --help`; visible with `--help-all` or
`--help-management`.

#### `wsm config show`

Show resolved configuration including the source of each value.

```
wsm config show [--format json|yaml]
```

Output (text mode) shows `backend`, `data_dir`, `config_file` (with
`(not found)` suffix when absent), and `source` (one of `defaults`,
`config_file`, `env`, `flags`).

#### `wsm config path`

Print the highest-precedence existing wsm config file (single path).
Exits 1 with `no config file found in resolution chain` to stderr when
no rung exists.

```
wsm config path [--from <dir>] [--format text|json|yaml]
```

#### `wsm config paths`

Print every rung of the wsm config resolution chain, highest-precedence
first. Each entry includes path, source label, scope, and whether the
file exists.

```
wsm config paths [--from <dir>] [--format text|json|yaml]
```

Both subcommands are wired via `kit/cli/config.RegisterPathSubcommands`
and share the resolver order documented in [configuration.md](configuration.md).

#### `wsm alias`

Manage user-curated command aliases backed by a YAML store at
`$XDG_CONFIG_HOME/wsm/aliases.yaml`. Cobra-level command shortcuts
(e.g. `ws`, `ls`, `snap`) keep working — `alias` is additive and
user-controlled.

```
wsm alias                       # list (default)
wsm alias list                  # explicit list
wsm alias add <name> <target>   # add or update
wsm alias remove <name>         # remove (alias: rm)
```

#### `wsm upgrade`

Check for and install updates.

```
wsm upgrade [--auto] [--quiet]
```

| Flag            | Default | Description                                |
|-----------------|---------|--------------------------------------------|
| `--auto`        | false   | Install without prompting                  |
| `-q`, `--quiet` | false   | Suppress output when already up to date    |

Subcommand:

##### `wsm upgrade preamble`

Print (or install) the upgrade preamble fragment for skill files.

| Flag         | Default | Description                                    |
|--------------|---------|------------------------------------------------|
| `--auto`     | false   | Emit auto-upgrade (`SnoozeNever`) variant      |
| `--never`    | false   | Emit check-only (`SnoozeAlways`) variant       |
| `--install`  | false   | Write preamble to `~/.config/wsm/skills/`      |

Powered by `hop.top/kit/go/core/upgrade` (replaces the previous
`hop.top/upgrade` integration; see [CHANGELOG](../../CHANGELOG.md)).

#### `wsm version`

Print version information.

```
wsm version [--short] [--format json|yaml]
```

| Flag      | Default | Description                  |
|-----------|---------|------------------------------|
| `--short` | false   | Output only the version number |

JSON/YAML output includes `version`, `commit`, `goVersion`, `os`, `arch`.

#### `wsm completion`

Generate shell completion scripts.

```
wsm completion [bash|zsh|fish|powershell]
```

Examples:

```
wsm completion bash > /etc/bash_completion.d/wsm
wsm completion zsh  > "${fpath[1]}/_wsm"
wsm completion fish > ~/.config/fish/completions/wsm.fish
```

Workspace-name completion is registered automatically for `workspace show`,
`workspace archive`, `workspace unarchive`, `workspace delete`, `snapshot
generate`, `snapshot show`, `snapshot context`, `snapshot list --workspace`,
`event list`, `event show`, and `event add` (first arg).

## Output formats

`--format` selects the renderer powered by `kit/console/output`:

| Value   | Behavior                                                            |
|---------|---------------------------------------------------------------------|
| `table` | Default for TTY stdout. Pretty columns; `--cols` selects fields.    |
| `text`  | Compact text rendering; minimal styling.                            |
| `json`  | Machine-readable JSON. Use for scripting / piping into `jq`.        |
| `yaml`  | Machine-readable YAML.                                              |
| `csv`   | Comma-separated values; one row per record.                         |

`--cols`/`--columns` restrict columns (table/csv); `--template` applies a Go
`text/template` to results (mutually exclusive with `--cols`). Use
`--format-help` to list available formats and per-format options;
`--format <key> --format-help` shows options for a single format.

Per CLI conventions §6.3, ack messages, prompts, empty-state hints, and
warnings go to stderr; data goes to stdout. This preserves clean piping such
as `wsm workspace list --format json | jq`.

## Exit codes

Per CLI conventions §8.1:

| Code | Meaning                                |
|------|----------------------------------------|
| 0    | Success                                |
| 1    | Generic error                          |
| 2    | Usage error (bad flags/args)           |
| 3    | Not found (workspace/space/snapshot)   |
| 4    | Conflict (already exists, in use)      |
| 5    | Unauthorized / forbidden (capability)  |
| 64   | Backend / persistence failure (wsm)    |

Codes 64+ are tool-specific. wsm error helpers
(`workspace.NewErrorUsage`, `NewErrorNotFound`, `NewErrorBackend`,
`NewErrorForbidden`) map to these codes via `kit/cli` error rendering.
