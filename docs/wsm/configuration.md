# wsm — Configuration

Last updated: 2026-05-02
Author: $USER

`wsm` reads configuration from layered sources via `kit/go/core/config.Load`.
This page documents every key, env var, flag, and the precedence rules.

See also: [api-cli.md](api-cli.md), [../architecture.md](../architecture.md).

## Precedence (lowest → highest)

1. Built-in defaults (`backend=sqlite`, XDG-derived dirs)
2. System: `/etc/wsm/config.yaml`
3. User: `$XDG_CONFIG_HOME/wsm/config.yaml`
4. Project: first existing of (cwd; not walked up):
   - `./.wsm.yaml` (legacy single-file form)
   - `./.wsm/config.yaml` (default `wsm init`)
   - `./.hop/wsm/config.yaml` (`wsm init --hop`)
5. `--config <path>` flag override (replaces the user-scope path)
6. Env vars: `WSM_<KEY>`
7. CLI flags

Each layer overrides only the keys it defines; missing keys fall through. The
resolved `Source` field reflects the highest layer that contributed
(`defaults`, `config_file`, `env`, or `flags`).

`--config` set to a non-existent path is a hard error (unlike layered files,
which are silently skipped when missing).

`wsm config path` and `wsm config paths` reflect this exact resolver order.

## XDG paths (kit/go/core/xdg)

| Purpose | Default                                                              | Override                       |
|---------|----------------------------------------------------------------------|--------------------------------|
| Config  | `$XDG_CONFIG_HOME/wsm` (Linux: `~/.config/wsm`; macOS: `~/Library/Application Support/wsm`) | `--config` flag           |
| Data    | `$XDG_DATA_HOME/wsm`                                                 | `WSM_DATA_DIR`, `--data-dir`   |
| Cache   | `$XDG_CACHE_HOME/wsm`                                                | `WSM_CACHE_DIR`                |
| State   | `$XDG_STATE_HOME/wsm`                                                | `WSM_STATE_DIR`                |

Data, cache, and state directories are auto-created on first run.

## Keys

| Key         | YAML          | Env var           | Flag             | Default        | Description                                       |
|-------------|---------------|-------------------|------------------|----------------|---------------------------------------------------|
| Backend     | `backend`     | `WSM_BACKEND`     | `--backend`      | `sqlite`       | Storage backend (only `sqlite` implemented)       |
| DataDir     | `data_dir`    | `WSM_DATA_DIR`    | `--data-dir`     | XDG data dir   | Data directory (sqlite db, machine-id)            |
| CacheDir    | `cache_dir`   | `WSM_CACHE_DIR`   | (none)           | XDG cache dir  | Cache directory                                   |
| StateDir    | `state_dir`   | `WSM_STATE_DIR`   | (none)           | XDG state dir  | State directory                                   |
| BackendURL  | `backend_url` | `WSM_BACKEND_URL` | `--backend-url`  | ""             | Backend URL (reserved for future backends)        |

CLI-only globals not file-backed (CLI conv §5 canonicals + wsm extras):

| Flag           | Default | Description                                                       |
|----------------|---------|-------------------------------------------------------------------|
| `--session-id` | ""      | Session ID for event tracking                                     |
| `--agent-id`   | user    | Agent ID for event tracking                                       |
| `--offline`    | false   | Disable all network (skip sync, upgrade check, plugin downloads)  |
| `--profile`    | ""      | Named credential / config profile                                 |
| `--instance`   | ""      | Backend instance discriminator                                    |

## Sample config

System / user / project files all use the same shape:

```yaml
backend: sqlite
data_dir: /custom/data/path
cache_dir: /custom/cache/path
state_dir: /custom/state/path
backend_url: ""
```

A minimal project override:

```yaml
data_dir: ./.wsm/data
```

## Local config directories

`wsm init` scaffolds a project-local config directory:

| Mode              | Primary dir   | Symlink                |
|-------------------|---------------|------------------------|
| Default           | `.wsm/`       | `.hop/wsm/ -> .wsm/`   |
| `--hop`           | `.hop/wsm/`   | `.wsm/ -> .hop/wsm/`   |

Both layouts contain a `config.yaml`. Both forms cannot coexist as
independent (non-symlinked) directories at the same project root; `wsm` errors
out when it detects that conflict.

`pkg/config.FindAnyLocalConfig` walks up from the current directory looking
for `.wsm/config.yaml`, falling back to `.hop/wsm/config.yaml`.

## Env vars

All env vars are read after layered files merge and before CLI flags. They
are absolute overrides (any non-empty value wins over a file value) and they
mark the resolved `Source` as `env`.

| Env var           | Maps to     |
|-------------------|-------------|
| `WSM_BACKEND`     | `backend`   |
| `WSM_DATA_DIR`    | `data_dir`  |
| `WSM_CACHE_DIR`   | `cache_dir` |
| `WSM_STATE_DIR`   | `state_dir` |
| `WSM_BACKEND_URL` | `backend_url` |

The convention is uppercase `WSM_` prefix with dots replaced by underscores
(matching the `kit/go/core/config` env hook contract).

Additional env vars read by specific commands:

| Env var                 | Effect                                                                |
|-------------------------|-----------------------------------------------------------------------|
| `WSM_DELETE_CONFIRM=y`  | `wsm workspace delete` (macOS only): skip confirmation prompt         |
| `WSM_TUI_SCRIPT`        | `wsm tui`: drive bubbletea via a script file (testing/automation)     |

## Inspect resolved config

```
wsm config show
wsm config show --format json
wsm config show --format yaml
```

Text output prints `backend`, `data_dir`, `config_file` (with
`(not found)` suffix when absent), and `source`. JSON/YAML emit the full
`Config` struct including `cache_dir`, `state_dir`, and `backend_url`.

## Legacy migration

On first run, wsm migrates a legacy `~/.wsm/wsm.db` to the XDG data dir
automatically (`pkg/config.MigrateFromLegacy`). The migration is idempotent
and preserves the legacy file when the target already exists (so a manual
backup is never overwritten).

## User aliases

Independent of `config.yaml`, `wsm` keeps a YAML-backed alias store at
`$XDG_CONFIG_HOME/wsm/aliases.yaml` (managed by `kit/console/alias`).

This store is the only alias mechanism in wsm — there are no hardcoded
cobra-level shortcuts (`ws`, `ls`, `snap`, etc. were removed). Every
alias is user-authored and lives in this file.

Mutate via `wsm alias add <name> <target>` / `wsm alias remove <name>`;
inspect with `wsm alias list`. The file is a flat `name: target` map:

```yaml
wl: workspace list
ws-show: workspace show
```

## Verbose & log output

`-V`/`--verbose` controls the `kit/console/log` slog handler:

| Count    | slog level |
|----------|------------|
| (unset)  | info       |
| `-V`     | debug      |
| `-VV`    | trace      |

Logs always go to stderr regardless of `--format`.
