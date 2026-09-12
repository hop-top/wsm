# wsm — CLI Reference

Last updated: 2026-05-02
Author: $USER

Generated reference for every wsm subcommand. See also:

- [configuration.md](configuration.md) — config keys, env vars, layered loading
- [policies.md](policies.md) — runtime policy engine, default rules, custom authoring
- [../architecture.md](../architecture.md) — system overview

## Synopsis

```
wsm [command] [--flags]
```

Bare `wsm` (no args) prints a quick-start hint to stderr (stdout stays empty so
`wsm | jq` keeps working).

## Help groups

`wsm --help` shows the three primary groups; management commands are hidden by
default. Use `--help-all` to see everything, or one of the group-scoped flags.

| Group         | Commands                                                   |
|---------------|------------------------------------------------------------|
| ORGANIZE      | `workspace`, `space`, `device`                             |
| CAPTURE       | `event`, `snapshot`                                        |
| INTERACT      | `init`, `irc`, `tui` (hidden)                              |
| MANAGEMENT    | `config`, `alias`, `upgrade`, `version`, `completion`      |

Group filters: `--help-organize`, `--help-capture`, `--help-interact`,
`--help-management`, `--help-knowledge`, `--help-curate`, `--help-all`.

### Subcommand groups (KNOWLEDGE / CURATE / CAPTURE)

Inside `wsm workspace`, `wsm snapshot`, and `wsm event`, verbs are bucketed
into KNOWLEDGE (read), CURATE (mutate), and CAPTURE (event recording) help
groups per CLI conv §4:

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
| `--help-knowledge`    | bool   | false   | Show only KNOWLEDGE subcommands of a parent                       |
| `--help-curate`       | bool   | false   | Show only CURATE subcommands of a parent                          |
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
| `--dry-run`           | bool   | false   | Preview side effects without applying them (command must support) |
| `--confirm`           | string | ""      | Confirmation policy for destructive commands (`auto`, `yes`, `no`, `prompt`); empty means prompt on a TTY, `no` otherwise |
| `--max-ops`           | int    | 0       | Cap mutating operations per invocation (`0` = unlimited)          |
| `--policy`            | string | ""      | Named delegation policy, loaded from `$XDG_CONFIG_HOME/wsm/policies/<name>.yaml` |
| `--api-version`       | string | ""      | Request a CLI schema version (`MAJOR.MINOR`) for compatibility-mode filtering |
| `--progress-format`   | string | human   | Progress output format (`human`, `json`); defaults to `json` when `--format=json` unless set explicitly |

`--format json|yaml` is the canonical machine-readable selector. The previous
top-level `--json` flag was removed in this release; see
[CHANGELOG](../../CHANGELOG.md).

The last six rows above are registered by `kit/cli`, not by wsm, and are
hidden from plain `wsm --help`. They appear in `wsm --help-all` and in the
GLOBAL FLAGS section of each subcommand's help.

`--idempotency-key <key>` is kit-provided but is **not** a global: kit
installs it per-leaf, on every leaf tagged idempotency `conditional` with a
`write-local`, `write-shared` or `destructive` side effect. Replaying a key
against the same tool replays the recorded output. Run
`wsm capabilities --format json` for the authoritative per-leaf flag list.

## Commands

### ORGANIZE

#### `wsm workspace`

Workspace operations.

```
wsm workspace [command] [--flags]
```

Subcommands: `add`, `list`, `show`, `archive`, `unarchive`, `delete`, `config`.

##### `wsm workspace add <name>`

Create a workspace.

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

List workspaces.

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

Show workspace details.

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
active.

```
wsm workspace archive <workspace> [--note <text>]
wsm workspace unarchive <workspace> [--note <text>]
```

| Flag           | Default | Description                                                          |
|----------------|---------|----------------------------------------------------------------------|
| `-n`, `--note` | ""      | Reason note (recorded for audit; required by default policy on archive) |

The default `archive-requires-note` policy rejects `archive` without a
non-empty `--note` (exit 4). `unarchive` accepts `--note` but no default
policy requires it. See [policies.md](policies.md).

Examples:

```
wsm workspace archive my-project --note "rotated to api-v2"
wsm workspace unarchive my-project -n "restoring for hotfix"
```

##### `wsm workspace delete <workspace>`

Permanently delete a workspace and its history.

Renamed from `remove` in this release; see [CHANGELOG](../../CHANGELOG.md).

```
wsm workspace delete <workspace> [--force] [--dry-run] [--note <text>]
```

| Flag            | Default | Description                                                       |
|-----------------|---------|-------------------------------------------------------------------|
| `-f`, `--force` | false   | Skip confirmation prompt                                          |
| `--dry-run`     | false   | Preview removal without deleting data                             |
| `-n`, `--note`  | ""      | Reason note (recorded for audit; required by default policy)      |

Non-TTY runs require `--force`. On macOS, `WSM_DELETE_CONFIRM=y` skips the
prompt non-interactively.

The default `delete-requires-note` policy rejects `delete` without a
non-empty `--note` (exit 4). See [policies.md](policies.md).

Examples:

```
wsm workspace delete my-project --dry-run
wsm workspace delete my-project --force --note "duplicate of api-v2"
```

##### `wsm workspace config`

Workspace configuration.

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
| `--auto`        | false       | Detect space attributes from `--path` and disclose them (reported, not applied) |

If the rux `projects.yaml` write fails, wsm warns to stderr and the wsm DB
write still succeeds; recover with `wsm space sync`.

##### `wsm space list <workspace>`

List spaces associated with a workspace.

##### `wsm space show <space-id>`

Show space details (type, trust, boundary, parent, capabilities, observables).

##### `wsm space delete <workspace> <space-id>`

Dissociate a space from a workspace.

Renamed from `remove` in this release; see [CHANGELOG](../../CHANGELOG.md).

| Flag            | Default | Description                                                            |
|-----------------|---------|------------------------------------------------------------------------|
| `-f`, `--force` | false   | Force removal (cascade child spaces)                                   |
| `-n`, `--note`  | ""      | Reason note (recorded for audit; not required by default policies)     |

`--note` is accepted and plumbed into ctx so user-authored policies can
read it; the bundled defaults do not require it for `space delete`. See
[policies.md](policies.md).

##### `wsm space sync`

Rewrite rux `projects.yaml` from the wsm DB. Recovery path after partial-write
failures, fresh checkouts where `projects.yaml` is missing, or drift. Manual
entries written by hand or future `rux project add` are preserved by the kit
projects.Write contract.

#### `wsm device`

Device registry operations.

```
wsm device [command] [--flags]
```

Subcommands: `list`, `show`.

##### `wsm device list`

List all registered devices.

##### `wsm device show <id-or-name>`

Show device details and recent sessions.

### CAPTURE

#### `wsm event`

Event operations.

```
wsm event [command] [--flags]
```

Subcommands: `add`, `list`, `show`.

##### `wsm event add <workspace> <type>`

Append an event to a workspace.

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

List workspace events with filtering.

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

Show full event details including payload.

#### `wsm snapshot`

Snapshot operations.

```
wsm snapshot [command] [--flags]
```

Subcommands: `generate`, `list`, `show`, `update`, `delete`, `context`.

##### `wsm snapshot generate [workspace]`

Generate and persist a workspace snapshot. With no argument, picks the most
recently updated workspace.

| Flag             | Default | Description                |
|------------------|---------|----------------------------|
| `-s`, `--session`| ""      | Filter by session ID       |
| `-n`, `--name`   | ""      | Snapshot name/alias        |
| `--seed`         | false   | Mark as seed snapshot      |

##### `wsm snapshot list`

List snapshots with filtering.

| Flag                | Default | Description                                       |
|---------------------|---------|---------------------------------------------------|
| `-w`, `--workspace` | -       | Filter by workspace ID or name (repeatable)       |
| `-s`, `--session`   | -       | Filter by session ID (repeatable)                 |
| `--after`           | ""      | Snapshots after this time (RFC3339)               |
| `--before`          | ""      | Snapshots before this time (RFC3339)              |
| `--sort`            | desc    | Sort direction: `asc`, `desc`                     |
| `-n`, `--limit`     | 0       | Maximum number of snapshots                       |

##### `wsm snapshot show <workspace>`

Show latest snapshot for a workspace.

##### `wsm snapshot update <snapshot-id-or-name>`

Update snapshot metadata.

| Flag             | Default | Description           |
|------------------|---------|-----------------------|
| `-n`, `--name`   | ""      | Snapshot name/alias   |
| `--seed`         | false   | Mark as seed snapshot |

##### `wsm snapshot delete <snapshot-id-or-name>`

Delete a snapshot.

Renamed from `remove` in this release; see [CHANGELOG](../../CHANGELOG.md).

| Flag            | Default | Description                                                            |
|-----------------|---------|------------------------------------------------------------------------|
| `-f`, `--force` | false   | Skip confirmation                                                      |
| `-n`, `--note`  | ""      | Reason note (recorded for audit; not required by default policies)     |

Non-TTY runs require `--force`.

`--note` is accepted and plumbed into ctx so user-authored policies can
read it; the bundled defaults do not require it for `snapshot delete`.
See [policies.md](policies.md).

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

#### `wsm irc`

Run and inspect a workspace's IRC presence. wsm either manages its own ergo
server for a workspace (managed mode) or connects to one it does not own
(external mode). Either way a per-workspace daemon runs the bot, keeps
channels and roles in line with the workspace's members, and captures channel
traffic into the event log.

```
wsm irc [command] [--flags]
```

Subcommands: `enable`, `disable`, `start`, `stop`, `restart`, `status`,
`sync`, `account`, `creds`, `url`, `channel`, `delegate`, `backfill`,
`daemon` (hidden).

Every verb below first requires `irc.enabled: true` in the resolved config;
a disabled config exits 2. Every verb except `enable` also requires the
workspace to have been enabled already — a missing profile exits 7 with
`run \`wsm irc enable <ws>\` first`.

Every verb that changes state, and the one that hands out a secret, checks a
capability (exit 6 when it is withheld):

| Capability    | Checked by                                                                 |
|---------------|----------------------------------------------------------------------------|
| `irc.enable`  | `enable` (both the managed and `--shared-with` paths), `account create\|rotate\|suspend`, `channel add\|rm\|adopt\|release\|purge`, `delegate add\|rm` |
| `irc.disable` | `disable`                                                                  |
| `irc.purge`   | `disable --purge`, `channel rm --purge`, `channel purge` (in addition to `irc.enable`) |
| `irc.start`   | `start`, `restart` (with `irc.stop`), and the daemon `enable` spawns        |
| `irc.stop`    | `stop`, `restart` (with `irc.start`), `disable`'s teardown                  |
| `irc.sync`    | `sync`, `backfill`                                                         |
| `irc.creds`   | `creds`                                                                    |

The checks live in `pkg/irc` rather than in the command layer, so the daemon
and the auto-provisioner that drives IRC off workspace lifecycle events are
gated by the same code the CLI is.

`--force` overrides the shared-host guard, never a capability: a caller
without `irc.stop` cannot take a server down from under its guests by adding
the flag, and is refused with 6 rather than the guard's 4.

`status`, `url`, `channel list`, `delegate list` and the hidden `daemon` check
nothing. The first four report state that anyone able to read the workspace
root can already read from its profile and config, so a gate there would be
theatre; `daemon` is spawned by `start`, which is gated.

`--dry-run`, `--confirm` and `--idempotency-key` are kit-provided rather
than wsm-authored; every mutating irc verb honors them. The destructive
paths (`disable --purge`, `channel rm --purge`, `channel purge`) refuse
under `--confirm=no`, or off a TTY, with exit 5.

No irc command registers a `--json` flag. Structured output comes from the
global `--format json|yaml`, and a command whose stdout is not a TTY emits
JSON automatically even when no format was named.

`--quiet` suppresses the result of every irc command except `url`, which
still prints the URL to stdout because the URL is the whole answer.

##### `wsm irc enable <workspace>`

Provision a workspace's IRC presence and start its daemon.

```
wsm irc enable <workspace> [--external] [--remote] [--shared-with <ws>] \
  [--port N] [--dir <root>] [--bot-password-stdin]
```

| Flag                   | Default | Description                                              |
|------------------------|---------|----------------------------------------------------------|
| `--external`           | false   | Connect to a server wsm does not own                     |
| `--remote`             | false   | Managed: enable the remote TLS/WebSocket listener        |
| `--shared-with`        | ""      | Attach to another workspace's server as a guest          |
| `--port`               | 0       | Managed: base port of the triple (0 scans)               |
| `--dir`                | ""      | Workspace root holding `.wsm/` (default: cwd)            |
| `--bot-password-stdin` | false   | External: read the bot password from stdin               |

Managed mode allocates a consecutive port triple, generates the bot secrets
and server API token, renders ergo's config, creates its database, and starts
the supervising daemon. External mode verifies the configured server is
reachable and starts the bot against it; the bot password comes from stdin
with `--bot-password-stdin` or from the aps secret named by
`irc.bot.secret_ref`. Re-running against a live daemon reports it rather than
starting a second one.

A root without `.wsm/` or `.hop/wsm/` exits 2, as does an invalid `irc` config
block. `--shared-with` naming the workspace itself exits 2; a host that is
already enabled exits 4, a host that is not managed or not running exits 7,
and a bot-account name the host reserves exits 4.

Examples:

```
wsm irc enable my-project
wsm irc enable my-project --remote --port 6700
wsm irc enable api --external --bot-password-stdin < bot.pass
wsm irc enable guest-ws --shared-with my-project
```

##### `wsm irc disable <workspace>`

Stop a workspace's IRC daemon, optionally removing what it owns.

```
wsm irc disable <workspace> [--purge] [--force]
```

| Flag        | Default | Description                                                  |
|-------------|---------|--------------------------------------------------------------|
| `--purge`   | false   | Also remove the workspace's IRC data and state directories   |
| `--force`   | false   | Disable even while guest workspaces are hosted here          |

Without `--purge` the data and state directories survive, so `start` brings
the workspace back with the same ports, accounts and history. `--purge`
destroys the server database, generated certificates and every secret wsm
holds for the workspace; it is gated by `--confirm` (`--force` stands in for
`--confirm=yes`) and refuses with exit 5 under `--confirm=no` or off a TTY.

A workspace hosting guests is refused with exit 4; `--force` overrides,
leaving guests to fail on their next reconnect.

##### `wsm irc start <workspace>`

Start the daemon for an already-enabled workspace.

In managed mode the recorded port triple is re-checked before spawning: a port
taken since `enable` is reported (exit 7) rather than silently reallocated.
Starting a workspace whose daemon is already up reports it and starts nothing.

Requires `irc.start`. The check sits at the one place a daemon is spawned, so
it covers `restart` and `enable`'s initial start as well.

##### `wsm irc stop <workspace>`

Stop the daemon, leaving everything it owns in place.

| Flag      | Default | Description                                       |
|-----------|---------|---------------------------------------------------|
| `--force` | false   | Stop even while guest workspaces are hosted here  |

Stopping an already-stopped workspace succeeds and says so. A workspace
hosting guests is refused with exit 4 unless `--force`.

Requires `irc.stop`, checked before the guest refusal: `--force` overrides the
guard, not the capability, and a withheld `irc.stop` is exit 6 either way.

##### `wsm irc restart <workspace>`

Stop the daemon if running, then start it again.

| Flag      | Default | Description                                          |
|-----------|---------|------------------------------------------------------|
| `--force` | false   | Restart even while guest workspaces are hosted here  |

Managed mode re-checks the port triple between the halves, after the old
daemon released it, so a port taken meanwhile is reported (exit 7) rather than
inherited. Restarting a stopped workspace simply starts it. Guests refuse with
exit 4 unless `--force`.

Requires both `irc.stop` and `irc.start`, asked for before the guest refusal.
A caller holding only one of them is refused with exit 6 naming the half it
lacks — `irc.stop` first, since that is the half that drops the server.

##### `wsm irc status <workspace>`

Report what the workspace's IRC presence is doing.

The daemon supplies state, pids, negotiated capabilities and the last
reconcile; this command adds the workspace name, configuration warnings, and —
when an ergo HTTP API is reachable — live channel occupancy. Discovery results
(adopted, peer and unmanaged channels) come from the daemon's last status
document and are reported even when stopped.

A workspace whose daemon is not running reports state `stopped` and exits 0:
"not running" is an answer, not a failure.

##### `wsm irc sync <workspace>`

Ask the running daemon to reconcile immediately.

Reconciliation creates missing accounts, registers missing channels and brings
roles in line with the workspace's members. The daemon does this on start and
whenever `members.yaml` changes; `sync` makes it happen now. Because the
request is a signal, a stopped daemon is an error (exit 1) rather than a
silent no-op.

Requires `irc.sync`: a reconcile rewrites accounts, roles and registrations on
the server, so it is a mutation asked of the daemon rather than a read of it.

##### `wsm irc account`

Manage member accounts on a workspace's IRC server.

```
wsm irc account [create|rotate|suspend] <workspace> [member] [--raw <name>]
```

Subcommands: `create`, `rotate`, `suspend`. Each takes a workspace and either
a member argument or `--raw <name>`; naming neither, or both, exits 2.

| Flag    | Default | Description                                       |
|---------|---------|---------------------------------------------------|
| `--raw` | ""      | Operate on this account name instead of a member's |

Accounts are never deleted, only suspended: event attribution is keyed by
account name, so deletion would orphan that member's message history.

`create` generates the passphrase, records it in the workspace secrets file
and applies it to the server in one run. An account that already exists keeps
its credential. `rotate` records the new passphrase only after the server
accepts it; existing sessions stay connected. `suspend` disables an account
without freeing the name.

`--raw` provisions an arbitrary account — how a host operator grants a
cross-machine guest access. That password is printed once and is **not**
stored, because the account belongs to the guest rather than this workspace.

A provider that does not support the operation exits 7, naming both the
provider and the setting to fix (`create`/`rotate` need the ergo HTTP API via
`irc.server.ergo_api`; `suspend` needs the bot to hold oper). A server that
rejects a well-formed request exits 1.

Examples:

```
wsm irc account create my-project alice
wsm irc account rotate my-project alice
wsm irc account create my-project --raw guest-bot
wsm irc account suspend my-project bob
```

##### `wsm irc creds <workspace> <member>`

Show the IRC credentials wsm holds for a member.

| Flag    | Default | Description                                      |
|---------|---------|--------------------------------------------------|
| `--env` | false   | Render as `WSM_IRC_CLIENT_*` shell assignments   |

Requires `irc.creds`, checked before the account is resolved. It is the only
read-shaped verb that is gated: it does not report state the workspace already
discloses, it renders a stored passphrase, and `--env` emits it ready to be
sourced into a shell.

Managed mode holds the passphrase and prints it; a member with none on file
exits 7 naming the `account rotate` that fixes it. External mode resolves the
account through the daemon's chain (workspace profile, then aps, then
`members.yaml`) and prints a password only when wsm actually holds one,
otherwise reporting the aps `secret_ref` as a hint rather than a value.

`--env` emits `WSM_IRC_CLIENT_SERVER`, `_PORT`, `_TLS`, `_ACCOUNT` and — only
when wsm holds one — `_PASSWORD`. The `CLIENT` infix is deliberate: these
configure an IRC client, and must never reconfigure wsm itself the way the
`WSM_IRC_*` config names would.

##### `wsm irc url <workspace>`

Print the URL a client uses to reach this workspace's IRC server.

| Flag    | Default | Description                                  |
|---------|---------|----------------------------------------------|
| `--web` | false   | Print the WebSocket URL for a browser client |

Managed mode answers with the loopback plaintext listener (`irc://`); `--web`
answers with the remote WebSocket listener (`wss://`) and requires the
workspace to have been enabled with `--remote` and `irc.remote.host` set —
otherwise exit 2. External mode answers `ircs://` when TLS is on and `irc://`
when it is off; `--web` is refused there with exit 2, and an empty
`irc.server.host` exits 7.

The URL goes to stdout even under `--quiet`, since it is the whole answer.

##### `wsm irc channel`

Manage a workspace's IRC channels.

Subcommands: `add`, `rm`, `list`, `adopt`, `release`, `purge`.

`add` and `rm` edit the workspace-local `irc.channels` override and signal the
daemon to reconcile; the daemon is the only thing that ever joins or parts a
channel. Other keys and comments in that config file are preserved
byte-for-byte. A channel name that is empty, or lacks a `#`/`&` prefix, or
contains a space, comma or BEL, exits 2.

###### `wsm irc channel add <workspace> <#channel>`

| Flag            | Default | Description                                             |
|-----------------|---------|---------------------------------------------------------|
| `--members`     | ""      | Membership rule: `all`, `principals`, `list`            |
| `--member-list` | -       | Profile ids when `--members list` (comma-separated)     |
| `--owner`       | ""      | Profile id given `+o` for an owner-managed channel      |
| `--modes`       | ""      | Channel modes applied on ensure, e.g. `+ntR`            |
| `--topic`       | ""      | Channel topic                                           |
| `--guest`       | -       | Guest allow-list entry `<profile>[:voice\|op]` (repeatable) |
| `--borrowed`    | ""      | Borrow a hosted channel: `<host-ws>:<#channel>`         |
| `--force`       | false   | Add even inside a creator namespace or discovered set   |

A name inside a channel creator's namespace, or one the daemon already adopted
through discovery, is refused with exit 4: those channels belong to a member's
own bot, and declaring one here would make wsm seize it on the next reconcile.
`--force` performs the seizure deliberately.

###### `wsm irc channel rm <workspace> <#channel>`

| Flag      | Default | Description                                            |
|-----------|---------|--------------------------------------------------------|
| `--purge` | false   | Also drop the channel registration on the server       |
| `--force` | false   | Proceed without the destructive confirmation           |

The template is dropped from the override and the daemon reconciles, clearing
the roles wsm authored and, where supported, making the channel invite-only. A
channel that is not declared exits 3. `--purge` additionally drops the server
registration and is gated by `--confirm` (`--force` stands in for
`--confirm=yes`; refusal is exit 5).

Here a provider that cannot purge is **not** an error: the template removal —
the part wsm owns — has already succeeded, so the run reports
`purged: false` with `reason: "unsupported"` and exits 0. This is the
opposite of standalone `channel purge`, where the same condition exits 7.

###### `wsm irc channel list <workspace>`

Show the channels this workspace maintains and what became of them. The list
merges configured templates with the daemon's last status, so a channel that
is declared but not yet registered appears with `registered=false` rather than
being omitted. Adopted, peer and unmanaged channels are listed with their
source.

###### `wsm irc channel adopt <workspace> <#channel>`

| Flag      | Default | Description                        |
|-----------|---------|------------------------------------|
| `--owner` | ""      | Account credited with the channel  |

Records a channel the daemon should join and capture, writing to `irc.adopted`
and signalling the daemon — whose discovery pass performs the join. Adopting a
previously released channel drops it from `irc.released`. The daemon must be
running (exit 1 otherwise): adoption that only reached a config file would look
identical to adoption that took effect.

###### `wsm irc channel release <workspace> <#channel>`

Writes the name to `irc.released`, drops any matching `irc.adopted` entry, and
signals the daemon to part the channel. Releasing never touches the channel
itself — wsm never owned it, so only wsm's participation ends. The daemon must
be running (exit 1 otherwise).

###### `wsm irc channel purge <workspace> <#channel>`

| Flag      | Default | Description                                  |
|-----------|---------|----------------------------------------------|
| `--force` | false   | Proceed without the destructive confirmation |

Drops a channel's registration entirely and empties it — a server operation,
not a configuration edit. On ergo this is `CS PURGE`, a two-step confirmation
the provider completes on wsm's behalf. There is no unpurge: the verb is
tagged destructive, requires both `irc.enable` and `irc.purge`, and is gated
by `--confirm` (`--force` stands in for `--confirm=yes`; refusal is exit 5). A
provider without the capability exits 7 rather than appearing to succeed; other
server failures exit 1.

The registration drop is the entire point of this leaf, so an unsupported
provider is fatal here — unlike `channel rm --purge`, where the same
condition is reported as `reason: "unsupported"` alongside a successful
template removal.

##### `wsm irc delegate`

Let another workspace's bot hand out roles on this one's channels.

Subcommands: `add`, `rm`, `list`. Delegation is a managed-mode arrangement: it
rests on server-side privilege ceilings, which only hold on a server wsm
controls.

###### `wsm irc delegate add <host-workspace> <guest>`

| Flag         | Default | Description                                                       |
|--------------|---------|-------------------------------------------------------------------|
| `--channel`  | ""      | Channel to grant on; every declared channel when omitted           |
| `--max-role` | voice   | Ceiling the delegate may hand out: `voice`, `op`                   |
| `--bot`      | ""      | Guest bot account on this server, for a guest not sharing locally  |

`--max-role voice` grants the guest bot halfop (a non-founder halfop may set a
persistent `+v` but is refused `+o`); `--max-role op` grants chanop. The cap is
enforced by IRC privilege rules, not by wsm — a delegate at `--max-role op` is
a channel operator and can op others.

The guest must already share the host's server, or `--bot` must name an account
the host provisioned with `wsm irc account create <host> --raw <name>`;
otherwise exit 4. A workspace that declares no channels, or a `--channel` that
is not declared, exits 3.

###### `wsm irc delegate rm <host-workspace> <guest>`

| Flag        | Default | Description                                               |
|-------------|---------|-----------------------------------------------------------|
| `--channel` | ""      | Channel to revoke on; every declared channel when omitted  |

Drops the entry from the override. The host's next reconcile revokes the bot's
privilege and sweeps the roles it can prove that bot handed out; a role wsm
cannot attribute to the departing bot is reported as unverified and left in
place rather than stripped on suspicion.

###### `wsm irc delegate list <host-workspace>`

Reports every delegation this workspace authored: the configured ceiling, the
role the daemon actually authored, and the count of roles in the channel that
wsm neither authored nor desires. A non-zero foreign count is not a fault — it
is what a delegate bot handing out roles looks like from the host's side.

##### `wsm irc backfill <workspace>`

Replay channel history into the event log.

| Flag        | Default | Description                            |
|-------------|---------|----------------------------------------|
| `--channel` | -       | Replay only this channel (repeatable)  |

This never opens a second IRC session beside the daemon. With a daemon running
the request is filed and signalled, and the daemon executes it, reporting
through `status.json` (`mode: daemon`); with the daemon stopped this command
takes the same exclusive lock and runs the backfill itself (`mode: offline`).
Either way exactly one process writes cursors, which is what keeps a replay
from recording the same messages twice.

Without `--channel` every channel with a cursor is replayed. A channel never
captured has no position to resume from, so live capture starts it rather than
a backfill.

Requires `irc.sync`, the same capability `sync` checks: a replay writes
messages into the workspace's event log, which is the shared state a reconcile
also touches.

##### `wsm irc daemon <workspace>`

Run the workspace's IRC daemon in the foreground. Hidden: `wsm irc start`
spawns it detached, and running it by hand is a debugging affordance rather
than an operator-facing verb.

Managed mode supervises an ergo child; both modes run the bot, reconcile on
start and on `SIGHUP`, capture channel traffic into the event log, and
maintain `status.json`. `SIGUSR1` runs a backfill; `SIGTERM` and `SIGINT` quit
the bot, stop the child, and exit. One daemon may hold a workspace at a time,
enforced by an exclusive lock held for the daemon's lifetime.

#### `wsm tui`

Launch the terminal UI (Charmbracelet bubbletea v2). Hidden by default;
invoked explicitly.

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
`$XDG_CONFIG_HOME/wsm/aliases.yaml`. The kit `alias.Store` is the only
alias mechanism — wsm has no hardcoded cobra-level shortcuts.

```
wsm alias                       # list (default)
wsm alias list                  # explicit list
wsm alias add <name> <target>   # add or update
wsm alias remove <name>         # remove
```

Aliases are resolved at parse time before cobra sees argv; targets can
be any wsm command path (e.g. `wsm alias add wl workspace list`).

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

| Code | Meaning                                                           |
|------|-------------------------------------------------------------------|
| 0    | Success                                                           |
| 1    | Generic error                                                     |
| 2    | Usage error (bad flags/args)                                      |
| 3    | Not found (workspace/space/snapshot)                              |
| 4    | Conflict (already exists, in use) **or policy denied** — see [policies.md](policies.md) |
| 5    | Backend / persistence failure; also kit's `--confirm` refusal    |
| 6    | Forbidden (capability gate denied the operation)                  |
| 7    | Prerequisite not satisfied (missing binary, busy port, unreachable server) |

wsm error helpers (`workspace.NewErrorUsage`, `NewErrorNotFound`,
`NewErrorBackend`, `NewErrorForbidden`, `NewErrorPrerequisite`) map to
these codes via `kit/cli` error rendering. Exit `7` messages state the
corrective action.

Policy denials produced by the runtime policy engine are routed
through `*output.Error` with `CodeConflict` (exit 4); message is
prefixed `policy "<name>" denied:`. See [policies.md](policies.md) for
the full vocabulary and authoring guide.

The current binary may return exit `1` for some paths that should
return `3` or `4` (tracked in T-1304); over time those regressions
will collapse onto the canonical table above.

## Published events (kit bus topics)

When a `bus.Bus` is wired into the workspace `Manager`, lifecycle
operations publish to the following topics. Topic names follow the kit
4-segment convention (`<tool>.<area>.<entity>.<verb-past>`):

| Topic                              | Trigger                              |
|------------------------------------|--------------------------------------|
| `wsm.runtime.workspace.created`    | `wsm workspace add`                  |
| `wsm.runtime.workspace.archived`   | `wsm workspace archive`              |
| `wsm.runtime.workspace.unarchived` | `wsm workspace unarchive`            |
| `wsm.runtime.workspace.deleted`    | `wsm workspace delete`               |
| `wsm.runtime.snapshot.created`     | `wsm snapshot generate`              |
| `wsm.runtime.snapshot.updated`     | `wsm snapshot update`                |
| `wsm.runtime.snapshot.deleted`     | `wsm snapshot delete`                |

Subscribers wanting all wsm runtime topics use the AMQP-style pattern
`wsm.runtime.#`; workspace-only is `wsm.runtime.workspace.#`. These
topics are emitted only when a bus is configured; CLI behavior is
unchanged when no bus is present.

Pre-event topics from `kit/runtime/domain.Service` (used by the policy
engine, not for end-user subscription):

| Topic                                     | Phase |
|-------------------------------------------|-------|
| `kit.runtime.entity.pre_persisted`        | After validation, before repo write |

Wsm publishes `pre_persisted` for every Workspace/Snapshot CRUD
operation routed through `pkg/workspace.Manager`. Policies subscribe
via `policy.Wire(bus, engine)`. See [policies.md](policies.md).
