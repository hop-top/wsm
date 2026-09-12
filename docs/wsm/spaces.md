# wsm — Spaces and `rux connect`

A space is a typed OWSF boundary bound to a workspace: a filesystem path,
container, VM, API endpoint, database, machine, or remote resource, scoped
by capabilities and a trust level. `wsm space add` is the canonical write
path for the rux project registry at `$XDG_CONFIG_HOME/rux/projects.yaml`:
every space added to wsm becomes resolvable by `rux connect <id>`.

```sh
wsm workspace add my-project
wsm space add my-project api --path ~/src/api --startup-cmd zsh
rux connect api          # attaches or starts the api session at ~/src/api
```

Each entry written to `projects.yaml` carries `path`, `startup_cmd`, and
`source: wsm` under `projects:` (`schema: 2`).

## Flags on `wsm space add`

| Flag | Default | Effect |
|---|---|---|
| `--type` | `filesystem` | `filesystem`, `container`, `vm`, `api`, `database`, `machine`, `remote` |
| `--path` | | Resource path or URL the space points at |
| `--startup-cmd` | | Command rux runs on connect (empty = `$SHELL`) |
| `--trust` | `local` | `local`, `verified`, `external`, `untrusted` |
| `--cap` | | Space capability as an OASF URI (repeatable) |
| `--auto` | `false` | Run the built-in detectors (Makefile, package.json, docker-compose, devcontainer, `.env.example`) against `--path` and disclose what they infer. Reported only, never applied: the space is saved exactly as specified |

### What `--auto` reports

`--auto` discloses the inference without applying it. Structured output
(`--format json|yaml`) gains an `_inferred` block naming, per field, whether
the value was determined from the boundary scheme, determined by a detector
that matched a file, or defaulted because nothing matched — plus a `matches`
list of every detector that fired and the `kind` it assigned. The table view
is unchanged; the same disclosure goes to stderr.

Detector confidence scores are not reported. Every detector `--auto` runs is
a file-existence check that hardcodes a confidence of `1.0` whenever it
matches, so the value was identical on every line it ever appeared on and
distinguished nothing. The `kind` each detector assigns (`build`, `runtime`)
is reported instead, since that does vary between detectors.

`--path` replaced the earlier `--boundary` flag without a
backwards-compatible alias: the CLI is pre-1.0 and no callers of the old
flag were found. The on-disk `boundary` field on the space model is
unchanged (it is the BOUNDARY column of `wsm space list`); only the flag was
renamed.

## Recovery: `wsm space sync`

The wsm database write and the `projects.yaml` write are not transactional.
If `wsm space add` succeeds against the database but the `projects.yaml`
update fails (disk full, permissions, projects writer not present), the
command warns on stderr and exits 0. Rebuild `projects.yaml` from the
database:

```text
$ wsm space sync
synced 1 spaces to projects.yaml
```

Sync iterates every root space in the database and rewrites its entry. It
is idempotent, preserves existing `startup_cmd` values, and only touches
`source: wsm` rows: `source: manual` entries (added by hand, or by a future
`rux project add`) survive a rewrite.

## Deleting a space

`wsm space delete <workspace> <space-id>` removes the binding and leaves the
underlying resource untouched. Confirmation follows the global `--confirm`
flag (prompt on a TTY, denied on non-TTY unless `--confirm=yes`); `--force`
cascade-removes child spaces and, for backward compatibility, also implies
`--confirm=yes`. `--note` records a reason for audit.

## See also

- [api-cli.md](api-cli.md#wsm-space): every `wsm space` subcommand
- [workspace-layout.md](workspace-layout.md): the `org-package` detector
