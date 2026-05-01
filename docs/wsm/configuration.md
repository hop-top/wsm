# wsm — Configuration

Last updated: 2026-04-30
Author: $USER

`wsm` reads configuration from layered sources via `kit/go/core/config.Load`.
This page documents every key, env var, flag, and the precedence rules.

See also: [api-cli.md](api-cli.md), [../architecture.md](../architecture.md).

## Precedence (lowest → highest)

1. Built-in defaults (`backend=sqlite`, XDG-derived dirs)
2. System: `/etc/wsm/config.yaml`
3. User: `$XDG_CONFIG_HOME/wsm/config.yaml`
4. Project: `./.wsm.yaml` (cwd; not walked up)
5. `--config <path>` flag override (replaces the user-scope path)
6. Env vars: `WSM_<KEY>`
7. CLI flags

Each layer overrides only the keys it defines; missing keys fall through. The
resolved `Source` field reflects the highest layer that contributed
(`defaults`, `config_file`, `env`, or `flags`).

`--config` set to a non-existent path is a hard error (unlike layered files,
which are silently skipped when missing).

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

Two additional CLI-only globals are not file-backed:

| Flag           | Default | Description                       |
|----------------|---------|-----------------------------------|
| `--session-id` | ""      | Session ID for event tracking     |
| `--agent-id`   | user    | Agent ID for event tracking       |

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

Additional env var read by `wsm workspace delete` on macOS only:

| Env var                 | Effect                                                |
|-------------------------|-------------------------------------------------------|
| `WSM_DELETE_CONFIRM=y`  | Skip the interactive confirmation prompt for delete   |

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

## Verbose & log output

`-V`/`--verbose` controls the `kit/console/log` slog handler:

| Count    | slog level |
|----------|------------|
| (unset)  | info       |
| `-V`     | debug      |
| `-VV`    | trace      |

Logs always go to stderr regardless of `--format`.
